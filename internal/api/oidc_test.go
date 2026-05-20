// Package api_test, OIDC suite. Builds a mock IdP via httptest.Server and
// exercises the F20b PKCE flow end-to-end: login redirect, callback, refresh,
// logout, and the negative paths (state/nonce/expiry).
package api_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/insidegreen/rpc-operator-claude/internal/api"
	"github.com/insidegreen/rpc-operator-claude/internal/api/catalog"
)

const (
	testClientID    = "rpc-operator-test"
	testKID         = "test-kid-1"
	oidcCookieName  = "rpc-oidc-state"
	oidcCookiePath  = "/api/v1/auth/"
	uiRedirectHTTPS = "https://rpc.example.com/"
)

// mockIdP is an in-memory OpenID Connect provider built on top of httptest.
// It implements just enough of the protocol to drive go-oidc through the
// PKCE auth-code flow: discovery, JWKS, token endpoint.
type mockIdP struct {
	srv      *httptest.Server
	key      *rsa.PrivateKey
	clientID string

	mu                sync.Mutex
	codes             map[string]mockIdPCode // code -> issued context
	rotateRefresh     bool                   // when true, return a new refresh_token on each /token call
	refreshTokenCount int                    // monotonically-increasing suffix for rotation
	rejectRefresh     bool                   // when true, /token grant_type=refresh_token returns 401
	emitWrongNonce    bool                   // when true, id_token carries a different nonce than the request
	emitNoIDToken     bool                   // when true, /token response omits id_token
}

type mockIdPCode struct {
	codeChallenge string
	nonce         string
}

func newMockIdP(t *testing.T) *mockIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	idp := &mockIdP{
		key:      key,
		clientID: testClientID,
		codes:    map[string]mockIdPCode{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", idp.handleDiscovery)
	mux.HandleFunc("/jwks", idp.handleJWKS)
	mux.HandleFunc("/auth", idp.handleAuth)
	mux.HandleFunc("/token", idp.handleToken)
	idp.srv = httptest.NewServer(mux)
	t.Cleanup(idp.srv.Close)
	return idp
}

func (m *mockIdP) issuer() string { return m.srv.URL }

func (m *mockIdP) handleDiscovery(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"issuer":                                m.issuer(),
		"authorization_endpoint":                m.issuer() + "/auth",
		"token_endpoint":                        m.issuer() + "/token",
		"jwks_uri":                              m.issuer() + "/jwks",
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"response_types_supported":              []string{"code"},
		"subject_types_supported":               []string{"public"},
	})
}

func (m *mockIdP) handleJWKS(w http.ResponseWriter, r *http.Request) {
	jwk := jose.JSONWebKey{Key: &m.key.PublicKey, KeyID: testKID, Use: "sig", Algorithm: "RS256"}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{jwk}})
}

// handleAuth stands in for the user-interactive part of the flow. Tests bypass
// it and POST straight to /token (or invoke /callback with a known code).
func (m *mockIdP) handleAuth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}

// issueCode records an auth-code with its expected code_challenge and nonce.
// The test layer drives this directly because we do not need a UI roundtrip.
func (m *mockIdP) issueCode(code, codeChallenge, nonce string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.codes[code] = mockIdPCode{codeChallenge: codeChallenge, nonce: nonce}
}

func (m *mockIdP) handleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	switch r.Form.Get("grant_type") {
	case "authorization_code":
		m.handleAuthCodeGrant(w, r)
	case "refresh_token":
		m.handleRefreshGrant(w, r)
	default:
		http.Error(w, "unsupported grant", 400)
	}
}

func (m *mockIdP) handleAuthCodeGrant(w http.ResponseWriter, r *http.Request) {
	code := r.Form.Get("code")
	verifier := r.Form.Get("code_verifier")
	m.mu.Lock()
	entry, ok := m.codes[code]
	if ok {
		delete(m.codes, code) // codes are one-shot
	}
	m.mu.Unlock()
	if !ok {
		http.Error(w, "unknown code", 400)
		return
	}
	if got := pkceS256(verifier); got != entry.codeChallenge {
		http.Error(w, "code_verifier mismatch", 400)
		return
	}
	nonce := entry.nonce
	if m.emitWrongNonce {
		nonce = "wrong-nonce"
	}
	m.writeTokens(w, nonce)
}

func (m *mockIdP) handleRefreshGrant(w http.ResponseWriter, _ *http.Request) {
	if m.rejectRefresh {
		http.Error(w, "invalid_grant", http.StatusUnauthorized)
		return
	}
	// Refresh-grant flow does not bind to the original login nonce; emit
	// an id_token without one — go-oidc tolerates absence.
	m.writeTokens(w, "")
}

func (m *mockIdP) writeTokens(w http.ResponseWriter, nonce string) {
	idToken := m.signIDToken(nonce)
	refresh := "refresh-" + randTokenSuffix()
	if m.rotateRefresh {
		m.mu.Lock()
		m.refreshTokenCount++
		refresh = fmt.Sprintf("refresh-rotated-%d", m.refreshTokenCount)
		m.mu.Unlock()
	}
	body := map[string]any{
		"access_token":  "access-" + randTokenSuffix(),
		"token_type":    "Bearer",
		"expires_in":    3600,
		"refresh_token": refresh,
	}
	if !m.emitNoIDToken {
		body["id_token"] = idToken
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

func (m *mockIdP) signIDToken(nonce string) string {
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: m.key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", testKID),
	)
	if err != nil {
		panic(err)
	}
	claims := map[string]any{
		"iss":   m.issuer(),
		"sub":   "user-123",
		"aud":   m.clientID,
		"exp":   time.Now().Add(time.Hour).Unix(),
		"iat":   time.Now().Unix(),
		"email": "user@example.com",
		"name":  "Test User",
		"jti":   randTokenSuffix(), // unique per signing so refresh produces a different token
	}
	if nonce != "" {
		claims["nonce"] = nonce
	}
	tok, err := jwt.Signed(signer).Claims(claims).Serialize()
	if err != nil {
		panic(err)
	}
	return tok
}

// pkceS256 mirrors the production transformation so tests verify the same way
// the production code computes the challenge.
func pkceS256(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

func randTokenSuffix() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// newOIDCTestServer wires an api.Server with OIDC enabled against the given
// mock IdP. We construct the Server directly (skipping api.New) because tests
// pass nil for RestConfig — api.New needs a real config to build a Clientset.
func newOIDCTestServer(t *testing.T, idp *mockIdP) *httptest.Server {
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
		OIDC: &api.OIDCConfig{
			Issuer:        idp.issuer(),
			ClientID:      testClientID,
			Scopes:        []string{"openid", "profile", "email", "offline_access"},
			RedirectURL:   "https://rpc.example.com/api/v1/auth/callback",
			UIRedirectURL: "https://rpc.example.com/",
		},
	}
	api.PrepareOIDCStoreForTest(srv)
	mux := http.NewServeMux()
	srv.RegisterRoutesForTest(mux)
	return httptest.NewServer(mux)
}

// nonRedirectingClient is an http.Client that captures redirects instead of
// following them — we want to inspect the 302 Location and the Set-Cookie
// without chasing them into the mock IdP or back into the API.
func nonRedirectingClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	return &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// driveLogin walks the full /auth/login -> IdP -> /auth/callback flow against
// the mock IdP and returns the final redirect response (the 302 from /callback
// to UIRedirectURL with #id_token=... in the Location). The returned client
// holds the session cookie for follow-up /refresh or /logout calls.
//
// Note: HTTPS-only cookies set against an HTTP test server are NOT stored by
// Go's cookiejar. The test sidesteps that by manually echoing the cookie back
// on the callback request using the value parsed from the login Set-Cookie.
func driveLogin(t *testing.T, ts *httptest.Server, idp *mockIdP) (*http.Client, *http.Response) {
	t.Helper()
	c := nonRedirectingClient(t)

	// Step 1: /api/v1/auth/login. Expect 302 to IdP /auth.
	loginResp, err := c.Get(ts.URL + "/api/v1/auth/login")
	if err != nil {
		t.Fatalf("GET /auth/login: %v", err)
	}
	_ = loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusFound {
		t.Fatalf("login: expected 302, got %d", loginResp.StatusCode)
	}
	loc, err := url.Parse(loginResp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("parse login Location: %v", err)
	}
	state := loc.Query().Get("state")
	challenge := loc.Query().Get("code_challenge")
	nonce := loc.Query().Get("nonce")
	if state == "" || challenge == "" || nonce == "" {
		t.Fatalf("login: missing state/challenge/nonce in %q", loginResp.Header.Get("Location"))
	}

	// Extract the session cookie from the login response; cookiejar discards
	// Secure cookies on HTTP test servers, so we re-attach it manually.
	var sessCookie *http.Cookie
	for _, ck := range loginResp.Cookies() {
		if ck.Name == oidcCookieName {
			sessCookie = ck
		}
	}
	if sessCookie == nil {
		t.Fatalf("login did not set rpc-oidc-state cookie")
	}

	// Step 2: simulate the IdP issuing an auth code bound to the same
	// challenge + nonce, then drive /callback.
	code := "code-" + randTokenSuffix()
	idp.issueCode(code, challenge, nonce)

	cbURL := ts.URL + "/api/v1/auth/callback?code=" + url.QueryEscape(code) + "&state=" + url.QueryEscape(state)
	req, err := http.NewRequest(http.MethodGet, cbURL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.AddCookie(sessCookie)
	cbResp, err := c.Do(req)
	if err != nil {
		t.Fatalf("GET /auth/callback: %v", err)
	}

	// Persist the cookie back into the jar via URL scheme rewrite so follow-up
	// /refresh and /logout requests carry it.
	c.Jar.SetCookies(mustParseURL(t, ts.URL), []*http.Cookie{sessCookie})

	// Also pick up any refreshed cookie set by /callback.
	for _, ck := range cbResp.Cookies() {
		if ck.Name == oidcCookieName && ck.Value != "" {
			c.Jar.SetCookies(mustParseURL(t, ts.URL), []*http.Cookie{ck})
		}
	}

	return c, cbResp
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse %q: %v", raw, err)
	}
	return u
}

// extractIDToken parses the redirect Location and returns the id_token fragment.
func extractIDToken(t *testing.T, resp *http.Response) string {
	t.Helper()
	loc := resp.Header.Get("Location")
	_, tok, ok := strings.Cut(loc, "#id_token=")
	if !ok {
		t.Fatalf("callback Location missing #id_token= prefix: %q", loc)
	}
	if tok == "" {
		t.Fatalf("callback Location has empty id_token fragment: %q", loc)
	}
	return tok
}

// readBodyJSON decodes resp.Body into a generic map; closes the body.
func readBodyJSON(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return out
}

// drainBody discards and closes resp.Body so test loops do not leak conns.
func drainBody(resp *http.Response) {
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

// --- Tests ------------------------------------------------------------------

func TestOIDC_LoginRedirectsToIdPWithPKCEParams(t *testing.T) {
	idp := newMockIdP(t)
	ts := newOIDCTestServer(t, idp)
	defer ts.Close()

	c := nonRedirectingClient(t)
	resp, err := c.Get(ts.URL + "/api/v1/auth/login")
	if err != nil {
		t.Fatalf("GET /auth/login: %v", err)
	}
	defer drainBody(resp)

	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 302, got %d", resp.StatusCode)
	}
	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("parse Location: %v", err)
	}
	if !strings.HasPrefix(loc.String(), idp.issuer()+"/auth") {
		t.Fatalf("Location should point at IdP, got %q", loc.String())
	}
	if got := loc.Query().Get("client_id"); got != testClientID {
		t.Fatalf("client_id: got %q want %q", got, testClientID)
	}
	if got := loc.Query().Get("code_challenge_method"); got != "S256" {
		t.Fatalf("code_challenge_method: got %q want S256", got)
	}
	for _, p := range []string{"state", "code_challenge", "nonce"} {
		if loc.Query().Get(p) == "" {
			t.Errorf("missing %s in redirect", p)
		}
	}
	if got := loc.Query().Get("scope"); !strings.Contains(got, "openid") {
		t.Errorf("scope missing openid: %q", got)
	}

	var found bool
	for _, ck := range resp.Cookies() {
		if ck.Name == oidcCookieName {
			found = true
			if !ck.HttpOnly || ck.Path != "/api/v1/auth/" {
				t.Errorf("cookie attrs: HttpOnly=%v Path=%q", ck.HttpOnly, ck.Path)
			}
		}
	}
	if !found {
		t.Errorf("rpc-oidc-state cookie not set")
	}
}

func TestOIDC_CallbackSuccess_IDTokenInFragment(t *testing.T) {
	idp := newMockIdP(t)
	ts := newOIDCTestServer(t, idp)
	defer ts.Close()

	_, resp := driveLogin(t, ts, idp)
	defer drainBody(resp)

	if resp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 302, got %d: %s", resp.StatusCode, body)
	}
	loc := resp.Header.Get("Location")
	if !strings.HasPrefix(loc, "https://rpc.example.com/#id_token=") {
		t.Fatalf("Location: got %q, want UIRedirectURL with #id_token fragment", loc)
	}
	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Errorf("Cache-Control: got %q, want no-store", cc)
	}
}

func TestOIDC_Callback_StateMismatch_Returns400(t *testing.T) {
	idp := newMockIdP(t)
	ts := newOIDCTestServer(t, idp)
	defer ts.Close()

	c := nonRedirectingClient(t)
	loginResp, err := c.Get(ts.URL + "/api/v1/auth/login")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	drainBody(loginResp)

	var sessCookie *http.Cookie
	for _, ck := range loginResp.Cookies() {
		if ck.Name == oidcCookieName {
			sessCookie = ck
		}
	}
	if sessCookie == nil {
		t.Fatalf("missing session cookie")
	}

	req, _ := http.NewRequest(http.MethodGet,
		ts.URL+"/api/v1/auth/callback?code=irrelevant&state=tampered-state", nil)
	req.AddCookie(sessCookie)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("GET callback: %v", err)
	}
	defer drainBody(resp)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestOIDC_Callback_MissingCookie_Returns400(t *testing.T) {
	idp := newMockIdP(t)
	ts := newOIDCTestServer(t, idp)
	defer ts.Close()

	c := nonRedirectingClient(t)
	resp, err := c.Get(ts.URL + "/api/v1/auth/callback?code=x&state=y")
	if err != nil {
		t.Fatalf("GET callback: %v", err)
	}
	defer drainBody(resp)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestOIDC_Callback_NonceMismatch_Returns401(t *testing.T) {
	idp := newMockIdP(t)
	idp.emitWrongNonce = true
	ts := newOIDCTestServer(t, idp)
	defer ts.Close()

	_, resp := driveLogin(t, ts, idp)
	defer drainBody(resp)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 nonce_mismatch, got %d", resp.StatusCode)
	}
}

func TestOIDC_Callback_MissingIDToken_Returns502(t *testing.T) {
	idp := newMockIdP(t)
	idp.emitNoIDToken = true
	ts := newOIDCTestServer(t, idp)
	defer ts.Close()

	_, resp := driveLogin(t, ts, idp)
	defer drainBody(resp)
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected 502 no_id_token, got %d", resp.StatusCode)
	}
}

func TestOIDC_Refresh_ReturnsNewIDToken(t *testing.T) {
	idp := newMockIdP(t)
	ts := newOIDCTestServer(t, idp)
	defer ts.Close()

	c, cbResp := driveLogin(t, ts, idp)
	originalIDToken := extractIDToken(t, cbResp)
	drainBody(cbResp)

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/auth/refresh", http.NoBody)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("POST /auth/refresh: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	body := readBodyJSON(t, resp)
	newTok, _ := body["id_token"].(string)
	if newTok == "" {
		t.Fatalf("no id_token in refresh response: %v", body)
	}
	if newTok == originalIDToken {
		t.Errorf("refresh returned identical id_token — expected a fresh sign with new jti")
	}
}

func TestOIDC_Refresh_NoSession_Returns401(t *testing.T) {
	idp := newMockIdP(t)
	ts := newOIDCTestServer(t, idp)
	defer ts.Close()

	c := nonRedirectingClient(t)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/auth/refresh", http.NoBody)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("POST refresh: %v", err)
	}
	defer drainBody(resp)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestOIDC_Refresh_IdPRejects_ClearsSessionAnd401(t *testing.T) {
	idp := newMockIdP(t)
	ts := newOIDCTestServer(t, idp)
	defer ts.Close()

	c, cbResp := driveLogin(t, ts, idp)
	drainBody(cbResp)

	idp.rejectRefresh = true

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/auth/refresh", http.NoBody)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	defer drainBody(resp)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 refresh_failed, got %d", resp.StatusCode)
	}

	req2, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/auth/refresh", http.NoBody)
	resp2, _ := c.Do(req2)
	defer drainBody(resp2)
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected second refresh 401 (session cleared), got %d", resp2.StatusCode)
	}
}

func TestOIDC_Refresh_RotatesRefreshToken(t *testing.T) {
	idp := newMockIdP(t)
	idp.rotateRefresh = true
	ts := newOIDCTestServer(t, idp)
	defer ts.Close()

	c, cbResp := driveLogin(t, ts, idp)
	drainBody(cbResp)

	req1, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/auth/refresh", http.NoBody)
	r1, err := c.Do(req1)
	if err != nil {
		t.Fatalf("first refresh: %v", err)
	}
	drainBody(r1)
	if r1.StatusCode != http.StatusOK {
		t.Fatalf("first refresh: %d", r1.StatusCode)
	}

	req2, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/auth/refresh", http.NoBody)
	r2, err := c.Do(req2)
	if err != nil {
		t.Fatalf("second refresh: %v", err)
	}
	defer drainBody(r2)
	if r2.StatusCode != http.StatusOK {
		t.Fatalf("second refresh: %d (rotation did not persist new token)", r2.StatusCode)
	}
}

func TestOIDC_Logout_ClearsSessionAndCookie(t *testing.T) {
	idp := newMockIdP(t)
	ts := newOIDCTestServer(t, idp)
	defer ts.Close()

	c, cbResp := driveLogin(t, ts, idp)
	drainBody(cbResp)

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/auth/logout", http.NoBody)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("POST logout: %v", err)
	}
	defer drainBody(resp)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
	var cleared bool
	for _, ck := range resp.Cookies() {
		if ck.Name == oidcCookieName && ck.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Errorf("logout did not expire the session cookie: %v", resp.Cookies())
	}

	req2, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/auth/refresh", http.NoBody)
	resp2, _ := c.Do(req2)
	defer drainBody(resp2)
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Errorf("post-logout refresh: expected 401, got %d", resp2.StatusCode)
	}
}

func TestOIDC_Logout_NoCookie_Idempotent(t *testing.T) {
	idp := newMockIdP(t)
	ts := newOIDCTestServer(t, idp)
	defer ts.Close()

	c := nonRedirectingClient(t)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/auth/logout", http.NoBody)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("logout: %v", err)
	}
	defer drainBody(resp)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 even without cookie, got %d", resp.StatusCode)
	}
}

func TestOIDC_Whoami_ReportsOIDCEnabled(t *testing.T) {
	idp := newMockIdP(t)
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}
	// Use Mode A (auth disabled) so /whoami returns 200 without a token —
	// we just want to read the oidcEnabled flag, no token needed.
	srvModeA := &api.Server{
		Addr:    ":0",
		Client:  newFakeClient(t),
		Catalog: cat,
		OIDC:    &api.OIDCConfig{Issuer: idp.issuer(), ClientID: testClientID},
	}
	api.PrepareOIDCStoreForTest(srvModeA)
	mux := http.NewServeMux()
	srvModeA.RegisterRoutesForTest(mux)
	tsModeA := httptest.NewServer(mux)
	defer tsModeA.Close()

	resp, err := http.Get(tsModeA.URL + "/api/v1/auth/whoami")
	if err != nil {
		t.Fatalf("GET whoami: %v", err)
	}
	body := readBodyJSON(t, resp)
	if enabled, _ := body["oidcEnabled"].(bool); !enabled {
		t.Errorf("whoami.oidcEnabled = false, want true (OIDC configured)")
	}
}

func TestOIDC_Whoami_ReportsOIDCDisabledWhenNoConfig(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/auth/whoami")
	if err != nil {
		t.Fatalf("GET whoami: %v", err)
	}
	body := readBodyJSON(t, resp)
	if enabled, ok := body["oidcEnabled"].(bool); !ok || enabled {
		t.Errorf("whoami.oidcEnabled = %v, want false (no OIDC config)", body["oidcEnabled"])
	}
}

// The regression this guards: in Mode B strict the SSO button could never
// appear because oidcEnabled only rode on whoami, which 401s without a token.
// /auth/config must report oidcEnabled WITHOUT authentication.
func TestOIDC_AuthConfig_ReachableWithoutTokenInModeBStrict(t *testing.T) {
	idp := newMockIdP(t)
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}
	// Auth on, no anonymous-read (Mode B strict), OIDC configured.
	srv := &api.Server{
		Addr:        ":0",
		Client:      newFakeClient(t),
		Catalog:     cat,
		AuthEnabled: true,
		OIDC:        &api.OIDCConfig{Issuer: idp.issuer(), ClientID: testClientID},
	}
	api.PrepareOIDCStoreForTest(srv)
	mux := http.NewServeMux()
	srv.RegisterRoutesForTest(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/auth/config") // no Authorization header
	if err != nil {
		t.Fatalf("GET /auth/config: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 without token, got %d", resp.StatusCode)
	}
	body := readBodyJSON(t, resp)
	if enabled, _ := body["oidcEnabled"].(bool); !enabled {
		t.Errorf("config.oidcEnabled = false, want true (OIDC configured)")
	}
}

func TestOIDC_AuthConfig_ReportsDisabledWhenNoConfig(t *testing.T) {
	ts := newTestServerAuthOn(t) // auth on but no OIDC
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/auth/config")
	if err != nil {
		t.Fatalf("GET /auth/config: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body := readBodyJSON(t, resp)
	if enabled, ok := body["oidcEnabled"].(bool); !ok || enabled {
		t.Errorf("config.oidcEnabled = %v, want false (no OIDC config)", body["oidcEnabled"])
	}
}

func TestOIDC_Disabled_LoginRoute404(t *testing.T) {
	ts := newTestServerAuthOn(t) // auth on but no OIDC
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/auth/login")
	if err != nil {
		t.Fatalf("GET /auth/login: %v", err)
	}
	defer drainBody(resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 (route unregistered), got %d", resp.StatusCode)
	}
}
