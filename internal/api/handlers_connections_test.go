package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
	"github.com/insidegreen/rpc-operator-claude/internal/api"
)

// mockInstantServer returns a Prometheus httptest server that responds with a
// single-item vector for every instant query. value should be "1" or "0".
func mockInstantServer(t *testing.T, value string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"resultType": "vector",
				"result": []map[string]any{
					{
						"metric": map[string]string{},
						"value":  []any{1715000000, value},
					},
				},
			},
		})
	}))
}

// mockInstantServerEmpty returns a Prometheus httptest server that responds with
// an empty vector.
func mockInstantServerEmpty(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data":   map[string]any{"resultType": "vector", "result": []any{}},
		})
	}))
}

// mockInstantServerCapturing captures each query string and responds with value.
func mockInstantServerCapturing(t *testing.T, queries *[]string, value string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*queries = append(*queries, r.URL.Query().Get("query"))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"resultType": "vector",
				"result": []map[string]any{
					{"metric": map[string]string{}, "value": []any{1715000000, value}},
				},
			},
		})
	}))
}

// --- handleConnections (single) ---

func TestHandlerConnections_NoPod(t *testing.T) {
	pipe := &rpcv1alpha1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "idle", Namespace: "default"},
		Spec:       rpcv1alpha1.PipelineSpec{Input: rpcv1alpha1.ComponentSpec{Type: "generate"}, Output: rpcv1alpha1.ComponentSpec{Type: "stdout"}},
	}
	ts := newTestServerWithPrometheus(t, "http://localhost:9090", pipe)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelines/idle/connections")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("expected 409, got %d", resp.StatusCode)
	}
}

func TestHandlerConnections_NoPrometheus(t *testing.T) {
	pipe := validRunningPipeline("demo", "default", "demo-pod-abc")
	ts := newTestServerWithPrometheus(t, "", pipe)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelines/demo/connections")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", resp.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "prometheus_unavailable" {
		t.Errorf("expected error=prometheus_unavailable, got %q", body["error"])
	}
}

func TestHandlerConnections_Up(t *testing.T) {
	prom := mockInstantServer(t, "1")
	defer prom.Close()

	pipe := validRunningPipeline("demo", "default", "demo-pod-abc")
	ts := newTestServerWithPrometheus(t, prom.URL, pipe)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelines/demo/connections")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body api.ConnectionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Input != "up" || body.Output != "up" {
		t.Errorf("expected up/up, got %+v", body)
	}
}

func TestHandlerConnections_Down(t *testing.T) {
	prom := mockInstantServer(t, "0")
	defer prom.Close()

	pipe := validRunningPipeline("demo", "default", "demo-pod-abc")
	ts := newTestServerWithPrometheus(t, prom.URL, pipe)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelines/demo/connections")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var body api.ConnectionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Input != "down" || body.Output != "down" {
		t.Errorf("expected down/down, got %+v", body)
	}
}

func TestHandlerConnections_EmptyVector_Unknown(t *testing.T) {
	prom := mockInstantServerEmpty(t)
	defer prom.Close()

	pipe := validRunningPipeline("demo", "default", "demo-pod-abc")
	ts := newTestServerWithPrometheus(t, prom.URL, pipe)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelines/demo/connections")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	var body api.ConnectionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Input != "unknown" || body.Output != "unknown" {
		t.Errorf("expected unknown/unknown, got %+v", body)
	}
}

func TestHandlerConnections_PrometheusError_Unknown(t *testing.T) {
	prom := mockPrometheusServerError(t)
	defer prom.Close()

	pipe := validRunningPipeline("demo", "default", "demo-pod-abc")
	ts := newTestServerWithPrometheus(t, prom.URL, pipe)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelines/demo/connections")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 (graceful degradation), got %d", resp.StatusCode)
	}
	var body api.ConnectionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Input != "unknown" || body.Output != "unknown" {
		t.Errorf("expected unknown/unknown on Prometheus error, got %+v", body)
	}
}

func TestHandlerConnections_ClusterMode_QueryContainsStream(t *testing.T) {
	var queries []string
	prom := mockInstantServerCapturing(t, &queries, "1")
	defer prom.Close()

	pipe := validRunningClusterPipeline("demo", "default", "etl", "etl-0")
	ts := newTestServerWithPrometheus(t, prom.URL, pipe)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelines/demo/connections")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if len(queries) != 2 {
		t.Fatalf("expected 2 Prometheus queries, got %d: %v", len(queries), queries)
	}
	for _, q := range queries {
		if !strings.Contains(q, `pod="etl-0"`) {
			t.Errorf("query missing pod selector: %q", q)
		}
		if !strings.Contains(q, `stream="demo"`) {
			t.Errorf("query missing stream selector: %q", q)
		}
		if !strings.Contains(q, "min(") {
			t.Errorf("query missing min() aggregation: %q", q)
		}
	}
}

func TestHandlerConnections_OwnPodMode_QueryNoStream(t *testing.T) {
	var queries []string
	prom := mockInstantServerCapturing(t, &queries, "1")
	defer prom.Close()

	pipe := validRunningPipeline("demo", "default", "demo-pod-abc")
	ts := newTestServerWithPrometheus(t, prom.URL, pipe)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelines/demo/connections")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	for _, q := range queries {
		if strings.Contains(q, "stream=") {
			t.Errorf("own-pod query must not include stream selector: %q", q)
		}
		if !strings.Contains(q, `pod="demo-pod-abc"`) {
			t.Errorf("query missing pod selector: %q", q)
		}
	}
}

func TestHandlerConnections_RouteRegistered(t *testing.T) {
	ts := newTestServerWithPrometheus(t, "")
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelines/nope/connections")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "text/html") {
		t.Error("single connections route not registered — SPA catch-all intercepted request")
	}
}
