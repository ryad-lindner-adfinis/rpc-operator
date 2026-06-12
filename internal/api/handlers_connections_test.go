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

// --- handleNamespaceConnections (batch) ---

// mockBatchInstantServer returns a Prometheus stub for batch queries.
// For input_connection_up: pipe-a=1, pipe-b=0 on instance etl-0.
// For output_connection_up: pipe-a=1, pipe-b=1.
func mockBatchInstantServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")
		var results []map[string]any
		if strings.Contains(query, "input_connection_up") {
			results = []map[string]any{
				{"metric": map[string]string{"pod": "etl-0", "stream": "pipe-a"}, "value": []any{1715000000, "1"}},
				{"metric": map[string]string{"pod": "etl-0", "stream": "pipe-b"}, "value": []any{1715000000, "0"}},
			}
		} else {
			results = []map[string]any{
				{"metric": map[string]string{"pod": "etl-0", "stream": "pipe-a"}, "value": []any{1715000000, "1"}},
				{"metric": map[string]string{"pod": "etl-0", "stream": "pipe-b"}, "value": []any{1715000000, "1"}},
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data":   map[string]any{"resultType": "vector", "result": results},
		})
	}))
}

func TestHandlerNamespaceConnections_NoPrometheus(t *testing.T) {
	pipe := validRunningPipeline("demo", "default", "demo-pod-abc")
	ts := newTestServerWithPrometheus(t, "", pipe)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelines/connections")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", resp.StatusCode)
	}
}

func TestHandlerNamespaceConnections_EmptyNamespace(t *testing.T) {
	// No pipelines → 200 {} without calling Prometheus (nothing listens on :19999).
	ts := newTestServerWithPrometheus(t, "http://127.0.0.1:19999")
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelines/connections")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body) != 0 {
		t.Errorf("expected empty map, got %v", body)
	}
}

func TestHandlerNamespaceConnections_StoppedPipelineOmitted(t *testing.T) {
	prom := mockInstantServer(t, "1")
	defer prom.Close()

	running := validRunningPipeline("alive", "default", "alive-pod")
	stopped := &rpcv1alpha1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "stopped", Namespace: "default"},
		Spec:       rpcv1alpha1.PipelineSpec{Input: rpcv1alpha1.ComponentSpec{Type: "generate"}, Output: rpcv1alpha1.ComponentSpec{Type: "stdout"}},
		Status:     rpcv1alpha1.PipelineStatus{Phase: rpcv1alpha1.PhaseStopped},
	}
	ts := newTestServerWithPrometheus(t, prom.URL, running, stopped)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelines/connections")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]api.ConnectionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := body["stopped"]; ok {
		t.Error("stopped pipeline must not appear in batch response")
	}
	if _, ok := body["alive"]; !ok {
		t.Error("running pipeline must appear in batch response")
	}
}

func TestHandlerNamespaceConnections_TwoStreams_SameInstance_DistinctStates(t *testing.T) {
	// pipe-a and pipe-b share instance etl-0. Mock returns different input values
	// per stream, proving per-stream isolation (not pod-aggregate).
	prom := mockBatchInstantServer(t)
	defer prom.Close()

	pipeA := &rpcv1alpha1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "pipe-a", Namespace: "default"},
		Spec:       rpcv1alpha1.PipelineSpec{Input: rpcv1alpha1.ComponentSpec{Type: "generate"}, Output: rpcv1alpha1.ComponentSpec{Type: "stdout"}},
		Status: rpcv1alpha1.PipelineStatus{
			Phase:            rpcv1alpha1.PhaseRunning,
			AssignedCluster:  "etl",
			AssignedInstance: "etl-0",
			StreamID:         "pipe-a",
		},
	}
	pipeB := &rpcv1alpha1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "pipe-b", Namespace: "default"},
		Spec:       rpcv1alpha1.PipelineSpec{Input: rpcv1alpha1.ComponentSpec{Type: "generate"}, Output: rpcv1alpha1.ComponentSpec{Type: "stdout"}},
		Status: rpcv1alpha1.PipelineStatus{
			Phase:            rpcv1alpha1.PhaseRunning,
			AssignedCluster:  "etl",
			AssignedInstance: "etl-0",
			StreamID:         "pipe-b",
		},
	}
	ts := newTestServerWithPrometheus(t, prom.URL, pipeA, pipeB)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelines/connections")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]api.ConnectionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["pipe-a"].Input != "up" {
		t.Errorf("pipe-a input: expected up, got %q", body["pipe-a"].Input)
	}
	if body["pipe-b"].Input != "down" {
		t.Errorf("pipe-b input: expected down, got %q (stream isolation failed)", body["pipe-b"].Input)
	}
	if body["pipe-a"].Output != "up" || body["pipe-b"].Output != "up" {
		t.Errorf("expected both outputs up, got %+v", body)
	}
}

func TestHandlerNamespaceConnections_OwnPodPipeline(t *testing.T) {
	// Own-pod pipeline: PodName set, no AssignedInstance.
	// Prometheus groups by (pod, stream); own-pod metrics lack a stream label,
	// so Prometheus returns stream="" when grouping. Handler matches on stream="".
	prom := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"resultType": "vector",
				"result": []map[string]any{
					{"metric": map[string]string{"pod": "own-pod-abc", "stream": ""}, "value": []any{1715000000, "1"}},
				},
			},
		})
	}))
	defer prom.Close()

	pipe := validRunningPipeline("own-pipe", "default", "own-pod-abc")
	ts := newTestServerWithPrometheus(t, prom.URL, pipe)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelines/connections")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]api.ConnectionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["own-pipe"].Input != "up" {
		t.Errorf("expected own-pipe input=up, got %+v", body["own-pipe"])
	}
}

func TestHandlerNamespaceConnections_BatchRouteNotShadowed(t *testing.T) {
	// GET .../pipelines/connections must hit batch handler (not {name} wildcard).
	// With no Prometheus, batch handler returns 503 — that's our signal it was reached.
	ts := newTestServerWithPrometheus(t, "")
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelines/connections")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "text/html") {
		t.Error("batch connections route not registered — SPA catch-all intercepted request")
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503 from batch handler (no Prometheus), got %d", resp.StatusCode)
	}
}
