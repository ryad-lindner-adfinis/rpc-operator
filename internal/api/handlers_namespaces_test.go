package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
	"github.com/insidegreen/rpc-operator-claude/internal/api"
	"github.com/insidegreen/rpc-operator-claude/internal/api/catalog"
)

// newTestServerWithAllowlist builds a Server with WatchNamespaces set. Empty
// allowlist would equal cluster-wide; tests that need that use newTestServer.
func newTestServerWithAllowlist(t *testing.T, allowlist []string, objs ...client.Object) *httptest.Server {
	t.Helper()
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}
	srv := &api.Server{
		Addr:            ":0",
		Client:          newFakeClient(t, objs...),
		Catalog:         cat,
		WatchNamespaces: allowlist,
	}
	mux := http.NewServeMux()
	srv.RegisterRoutesForTest(mux)
	return httptest.NewServer(mux)
}

func decodeNamespaces(t *testing.T, body []byte) []string {
	t.Helper()
	var v struct {
		Namespaces []string `json:"namespaces"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, body)
	}
	return v.Namespaces
}

func TestHandleListNamespaces_EmptyMeansClusterWide(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/v1/namespaces")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body := new(bytes.Buffer)
	_, _ = body.ReadFrom(resp.Body)
	got := decodeNamespaces(t, body.Bytes())
	if len(got) != 0 {
		t.Errorf("expected empty list (cluster-wide), got %v", got)
	}
}

func TestHandleListNamespaces_ReturnsAllowlist(t *testing.T) {
	allow := []string{"data-eng", "analytics"}
	ts := newTestServerWithAllowlist(t, allow)
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/v1/namespaces")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body := new(bytes.Buffer)
	_, _ = body.ReadFrom(resp.Body)
	got := decodeNamespaces(t, body.Bytes())
	if !reflect.DeepEqual(got, allow) {
		t.Errorf("expected %v, got %v", allow, got)
	}
}

func TestAllowlist_RejectsForbiddenNamespace(t *testing.T) {
	ts := newTestServerWithAllowlist(t, []string{"data-eng"})
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/v1/namespaces/kube-system/pipelines")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
	var body struct {
		Error   string `json:"error"`
		Details string `json:"details"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error != "namespace_not_allowed" {
		t.Errorf("expected error=namespace_not_allowed, got %q", body.Error)
	}
	if body.Details == "" {
		t.Errorf("expected non-empty details")
	}
}

func TestAllowlist_AllowsListedNamespace(t *testing.T) {
	existing := &rpcv1alpha1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "data-eng"},
		Spec: rpcv1alpha1.PipelineSpec{
			Input: rpcv1alpha1.ComponentSpec{
				Type:   "generate",
				Config: runtime.RawExtension{Raw: []byte(`{"mapping":"root = \"hi\"","interval":"1s","count":1}`)},
			},
			Output: rpcv1alpha1.ComponentSpec{Type: "stdout"},
		},
	}
	ts := newTestServerWithAllowlist(t, []string{"data-eng"}, existing)
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/v1/namespaces/data-eng/pipelines/p1")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestAllowlist_EmptyAllowlistAllowsAll(t *testing.T) {
	existing := &rpcv1alpha1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "kube-system"},
		Spec: rpcv1alpha1.PipelineSpec{
			Input: rpcv1alpha1.ComponentSpec{
				Type:   "generate",
				Config: runtime.RawExtension{Raw: []byte(`{"mapping":"root = \"hi\"","interval":"1s","count":1}`)},
			},
			Output: rpcv1alpha1.ComponentSpec{Type: "stdout"},
		},
	}
	ts := newTestServer(t, existing)
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/v1/namespaces/kube-system/pipelines/p1")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusForbidden {
		t.Errorf("empty allowlist must not 403; got %d", resp.StatusCode)
	}
}

func TestAllowlist_RejectsAllVerbs(t *testing.T) {
	ts := newTestServerWithAllowlist(t, []string{"data-eng"})
	defer ts.Close()

	cases := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v1/namespaces/kube-system/pipelines"},
		{"GET", "/api/v1/namespaces/kube-system/pipelines/x"},
		{"POST", "/api/v1/namespaces/kube-system/pipelines"},
		{"PUT", "/api/v1/namespaces/kube-system/pipelines/x"},
		{"DELETE", "/api/v1/namespaces/kube-system/pipelines/x"},
		{"GET", "/api/v1/namespaces/kube-system/pipelines/x/logs"},
		{"GET", "/api/v1/namespaces/kube-system/pipelines/x/metrics"},
	}
	for _, c := range cases {
		req, err := http.NewRequest(c.method, ts.URL+c.path, http.NoBody)
		if err != nil {
			t.Fatalf("%s %s: %v", c.method, c.path, err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", c.method, c.path, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s %s: expected 403, got %d", c.method, c.path, resp.StatusCode)
		}
	}
}

func TestAllowlist_DoesNotAffectGlobalRoutes(t *testing.T) {
	ts := newTestServerWithAllowlist(t, []string{"data-eng"})
	defer ts.Close()

	cases := []struct {
		method string
		path   string
		body   []byte
	}{
		{"GET", "/api/v1/catalog", nil},
		{"GET", "/api/v1/pipelines", nil},
		{"GET", "/api/v1/namespaces", nil},
	}
	for _, c := range cases {
		var bodyReader *bytes.Reader
		if c.body != nil {
			bodyReader = bytes.NewReader(c.body)
		}
		var req *http.Request
		var err error
		if bodyReader != nil {
			req, err = http.NewRequest(c.method, ts.URL+c.path, bodyReader)
		} else {
			req, err = http.NewRequest(c.method, ts.URL+c.path, http.NoBody)
		}
		if err != nil {
			t.Fatalf("%s %s: %v", c.method, c.path, err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", c.method, c.path, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusForbidden {
			t.Errorf("%s %s: must not 403 (global route), got 403", c.method, c.path)
		}
	}
}
