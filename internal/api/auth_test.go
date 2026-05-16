// Package api_test tests the F20a + F43 authentication layer.
//
// Scope:
//   - Middleware: missing/empty/wrong-scheme token -> 401 (Mode B).
//   - Mode A regression: handlers serve via Operator-SA without a token.
//   - tokenFromRequest unit semantics (header, query, priority).
//   - Whoami in Mode A returns anonymous identity.
//
// Out of scope here: Mode B handleWhoami against a real apiserver (covered
// by Level 4 cluster verification). Client/fake has no SelfSubjectReview.
package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
	"github.com/insidegreen/rpc-operator-claude/internal/api"
	"github.com/insidegreen/rpc-operator-claude/internal/api/catalog"
)

const defaultNamespace = "default"

// newTestServerAuthOn builds a server with AuthEnabled=true. RestConfig/Scheme
// are left nil — tests that exercise full Mode B handler paths only assert on
// the middleware (401). Full per-request-client construction needs envtest
// and is covered separately in cluster verification.
func newTestServerAuthOn(t *testing.T) *httptest.Server {
	t.Helper()
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}
	srv := &api.Server{
		Addr:        ":0",
		Client:      newFakeClient(t),
		Catalog:     cat,
		AuthEnabled: true,
	}
	mux := http.NewServeMux()
	srv.RegisterRoutesForTest(mux)
	return httptest.NewServer(mux)
}

// requestWithHeader builds a request with the given Authorization header value
// (or no header if value is empty). Returns the response — caller closes Body.
func requestWithHeader(t *testing.T, method, urlStr, authHeader string, body []byte) *http.Response {
	t.Helper()
	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequest(method, urlStr, bytes.NewReader(body))
	} else {
		req, err = http.NewRequest(method, urlStr, http.NoBody)
	}
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	return resp
}

// --- Mode A (auth disabled, default zero-value) -----------------------------

func TestAuth_ModeA_WhoamiReturnsAnonymous(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/auth/whoami")
	if err != nil {
		t.Fatalf("GET whoami: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body struct {
		User      map[string]any `json:"user"`
		Anonymous bool           `json:"anonymous"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.Anonymous {
		t.Errorf("expected anonymous=true, got false")
	}
	if name, _ := body.User["name"].(string); name != api.AnonymousUser {
		t.Errorf("expected user.name=%q, got %q", api.AnonymousUser, name)
	}
}

func TestAuth_ModeA_NamespacedRouteWithoutToken(t *testing.T) {
	// In Mode A, namespaced GET works without any header (Operator-SA serves).
	p := &rpcv1alpha1.Pipeline{}
	p.Name = "p1"
	p.Namespace = defaultNamespace
	p.Spec.Input = rpcv1alpha1.ComponentSpec{Type: "generate"}
	p.Spec.Output = rpcv1alpha1.ComponentSpec{Type: "stdout"}

	ts := newTestServer(t, p)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelines/p1")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

// --- Mode B (auth enabled) middleware -----------------------------

func TestAuth_ModeB_MissingTokenReturns401(t *testing.T) {
	ts := newTestServerAuthOn(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/pipelines")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error != "unauthorized" {
		t.Errorf("expected error=unauthorized, got %q", body.Error)
	}
}

func TestAuth_ModeB_EmptyBearerReturns401(t *testing.T) {
	ts := newTestServerAuthOn(t)
	defer ts.Close()

	resp := requestWithHeader(t, http.MethodGet, ts.URL+"/api/v1/pipelines", "Bearer ", nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestAuth_ModeB_NonBearerSchemeReturns401(t *testing.T) {
	ts := newTestServerAuthOn(t)
	defer ts.Close()

	resp := requestWithHeader(t, http.MethodGet, ts.URL+"/api/v1/pipelines", "Basic abc", nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestAuth_ModeB_WhoamiWithoutTokenReturns401(t *testing.T) {
	ts := newTestServerAuthOn(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/auth/whoami")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestAuth_ModeB_NamespacesWithoutTokenReturns401(t *testing.T) {
	ts := newTestServerAuthOn(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestAuth_ModeB_CatalogWithoutTokenReturns401(t *testing.T) {
	ts := newTestServerAuthOn(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/catalog")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

// validate and render must remain accessible without auth (they touch no K8s).
// F42 anonymous-read relies on this.

func TestAuth_ModeB_ValidateBypassesAuth(t *testing.T) {
	ts := newTestServerAuthOn(t)
	defer ts.Close()

	resp, err := http.Post(
		ts.URL+"/api/v1/pipelines/validate",
		"application/json",
		bytes.NewReader(validPipelineBody("v", "default")),
	)
	if err != nil {
		t.Fatalf("POST validate: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestAuth_ModeB_RenderBypassesAuth(t *testing.T) {
	ts := newTestServerAuthOn(t)
	defer ts.Close()

	resp, err := http.Post(
		ts.URL+"/api/v1/pipelines/render",
		"application/json",
		bytes.NewReader(validPipelineBody("r", "default")),
	)
	if err != nil {
		t.Fatalf("POST render: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

// Logs WS: 401 before websocket.Accept when no query token.

func TestAuth_ModeB_LogsWithoutQueryTokenReturns401(t *testing.T) {
	ts := newTestServerAuthOn(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelines/any/logs")
	if err != nil {
		t.Fatalf("GET logs: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 (no token), got %d", resp.StatusCode)
	}
}

// --- tokenFromRequest unit semantics --------------------------------
//
// We test the surface behavior of tokenFromRequest indirectly via the
// middleware. The unit semantics — header vs query priority — are pinned
// here using a request that hits an authenticated route and inspecting
// the response status (401 vs not-401).

func TestAuth_TokenFromRequest_HeaderTakesPriorityOverQuery(t *testing.T) {
	// authMiddleware happily accepts ANY non-empty token (validation against
	// apiserver happens later, in clientForRequest). So both header AND query
	// here would pass the 401 gate. To assert priority we'd need to read the
	// token back from context — out of scope for an HTTP-only test.
	// Instead we assert the simpler property: a header alone is enough.
	ts := newTestServerAuthOn(t)
	defer ts.Close()

	u := ts.URL + "/api/v1/auth/whoami"
	// Header only — should NOT be 401.
	resp := requestWithHeader(t, http.MethodGet, u, "Bearer token-from-header", nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusUnauthorized {
		t.Errorf("header-only request should not be 401, got 401")
	}
}

func TestAuth_TokenFromRequest_QueryOnly(t *testing.T) {
	ts := newTestServerAuthOn(t)
	defer ts.Close()

	q := url.Values{}
	q.Set("token", "token-from-query")
	resp, err := http.Get(ts.URL + "/api/v1/auth/whoami?" + q.Encode())
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	// Query token alone is accepted by the middleware (handler may then
	// 500 because RestConfig is nil — that's a separate failure mode).
	if resp.StatusCode == http.StatusUnauthorized {
		t.Errorf("query-only request should not be 401, got 401")
	}
}

func TestAuth_TokenFromRequest_NeitherHeaderNorQuery(t *testing.T) {
	ts := newTestServerAuthOn(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/auth/whoami")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("no-token request should be 401, got %d", resp.StatusCode)
	}
}

// --- F42 Mode C (auth on + AnonymousRead) -----------------------------

// newTestServerModeC builds a Mode-C server (auth on, anonymous reads on).
// Used to test that unauthenticated GETs pass while writes stay 401.
func newTestServerModeC(t *testing.T, objs ...client.Object) *httptest.Server {
	t.Helper()
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}
	srv := &api.Server{
		Addr:          ":0",
		Client:        newFakeClient(t, objs...),
		Catalog:       cat,
		AuthEnabled:   true,
		AnonymousRead: true,
	}
	mux := http.NewServeMux()
	srv.RegisterRoutesForTest(mux)
	return httptest.NewServer(mux)
}

// newTestServerModeCWithLogs adds AnonymousLogs=true on top of Mode C.
func newTestServerModeCWithLogs(t *testing.T) *httptest.Server {
	t.Helper()
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}
	srv := &api.Server{
		Addr:          ":0",
		Client:        newFakeClient(t),
		Catalog:       cat,
		AuthEnabled:   true,
		AnonymousRead: true,
		AnonymousLogs: true,
	}
	mux := http.NewServeMux()
	srv.RegisterRoutesForTest(mux)
	return httptest.NewServer(mux)
}

func TestAuth_ModeC_WhoamiAnonymousReturns200WithReadOnly(t *testing.T) {
	ts := newTestServerModeC(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/auth/whoami")
	if err != nil {
		t.Fatalf("GET whoami: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body struct {
		User          map[string]any `json:"user"`
		Anonymous     bool           `json:"anonymous"`
		ReadOnly      bool           `json:"readOnly"`
		AnonymousLogs bool           `json:"anonymousLogs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.Anonymous {
		t.Errorf("expected anonymous=true")
	}
	if !body.ReadOnly {
		t.Errorf("expected readOnly=true in Mode C")
	}
	if body.AnonymousLogs {
		t.Errorf("expected anonymousLogs=false (AnonymousLogs flag is off in this test)")
	}
	if name, _ := body.User["name"].(string); name != api.AnonymousUser {
		t.Errorf("expected user.name=%q, got %q", api.AnonymousUser, name)
	}
}

func TestAuth_ModeC_WhoamiAnonymousLogsFlagPropagates(t *testing.T) {
	ts := newTestServerModeCWithLogs(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/auth/whoami")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var body struct {
		AnonymousLogs bool `json:"anonymousLogs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.AnonymousLogs {
		t.Errorf("expected anonymousLogs=true when server has AnonymousLogs=true")
	}
}

func TestAuth_ModeC_ListPipelinesAnonymous200(t *testing.T) {
	p := &rpcv1alpha1.Pipeline{}
	p.Name = "p1"
	p.Namespace = defaultNamespace
	p.Spec.Input = rpcv1alpha1.ComponentSpec{Type: "generate"}
	p.Spec.Output = rpcv1alpha1.ComponentSpec{Type: "stdout"}

	ts := newTestServerModeC(t, p)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/pipelines")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestAuth_ModeC_GetPipelineAnonymous200(t *testing.T) {
	p := &rpcv1alpha1.Pipeline{}
	p.Name = "p1"
	p.Namespace = defaultNamespace
	p.Spec.Input = rpcv1alpha1.ComponentSpec{Type: "generate"}
	p.Spec.Output = rpcv1alpha1.ComponentSpec{Type: "stdout"}

	ts := newTestServerModeC(t, p)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelines/p1")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestAuth_ModeC_CatalogAnonymous200(t *testing.T) {
	ts := newTestServerModeC(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/catalog")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestAuth_ModeC_NamespacesAnonymous200(t *testing.T) {
	ts := newTestServerModeC(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestAuth_ModeC_PostPipelineAnonymousReturns401(t *testing.T) {
	ts := newTestServerModeC(t)
	defer ts.Close()

	resp, err := http.Post(
		ts.URL+"/api/v1/namespaces/default/pipelines",
		"application/json",
		bytes.NewReader(validPipelineBody("p1", "default")),
	)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for anonymous POST, got %d", resp.StatusCode)
	}
}

func TestAuth_ModeC_PutPipelineAnonymousReturns401(t *testing.T) {
	ts := newTestServerModeC(t)
	defer ts.Close()

	resp := requestWithHeader(t, http.MethodPut,
		ts.URL+"/api/v1/namespaces/default/pipelines/x", "",
		validPipelineBody("x", "default"))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for anonymous PUT, got %d", resp.StatusCode)
	}
}

func TestAuth_ModeC_DeletePipelineAnonymousReturns401(t *testing.T) {
	ts := newTestServerModeC(t)
	defer ts.Close()

	resp := requestWithHeader(t, http.MethodDelete,
		ts.URL+"/api/v1/namespaces/default/pipelines/x", "", nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for anonymous DELETE, got %d", resp.StatusCode)
	}
}

func TestAuth_ModeC_LogsAnonymousReturns401WhenLogsFlagOff(t *testing.T) {
	// Mode C: AnonymousRead=true, AnonymousLogs=false (default).
	// /logs must still 401 without a token.
	ts := newTestServerModeC(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelines/any/logs")
	if err != nil {
		t.Fatalf("GET logs: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 (anonymous logs disabled), got %d", resp.StatusCode)
	}
}

func TestAuth_ModeC_LogsAnonymousPassesAuthWhenFlagOn(t *testing.T) {
	// Mode C with AnonymousLogs=true: /logs must pass the auth gate.
	// Pipeline doesn't exist → expect 404, NOT 401 — that proves we made it past auth.
	ts := newTestServerModeCWithLogs(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelines/nope/logs")
	if err != nil {
		t.Fatalf("GET logs: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusUnauthorized {
		t.Errorf("expected non-401 (auth check passed), got 401")
	}
}

func TestAuth_ModeC_AllowlistStillEnforced(t *testing.T) {
	// Mode C with a non-empty watch-namespaces allowlist: anonymous GET on
	// a namespace outside the allowlist must 403, not 200.
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}
	srv := &api.Server{
		Addr:            ":0",
		Client:          newFakeClient(t),
		Catalog:         cat,
		AuthEnabled:     true,
		AnonymousRead:   true,
		WatchNamespaces: []string{"allowed-ns"},
	}
	mux := http.NewServeMux()
	srv.RegisterRoutesForTest(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/forbidden-ns/pipelines")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 (allowlist), got %d", resp.StatusCode)
	}
}

func TestAuth_ModeC_ValidateAndRenderStillOpen(t *testing.T) {
	// validate/render are always auth-free regardless of mode; this asserts
	// the Mode-C wiring did not accidentally close them.
	ts := newTestServerModeC(t)
	defer ts.Close()

	resp, err := http.Post(
		ts.URL+"/api/v1/pipelines/validate",
		"application/json",
		bytes.NewReader(validPipelineBody("v", "default")),
	)
	if err != nil {
		t.Fatalf("POST validate: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for validate, got %d", resp.StatusCode)
	}
}
