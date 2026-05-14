package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
	"github.com/insidegreen/rpc-operator-claude/internal/api"
	"github.com/insidegreen/rpc-operator-claude/internal/api/catalog"
)

func newTestServerWithPrometheus(t *testing.T, prometheusURL string, objs ...client.Object) *httptest.Server {
	t.Helper()
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}
	srv := &api.Server{
		Addr:          ":0",
		Client:        newFakeClient(t, objs...),
		Catalog:       cat,
		PrometheusURL: prometheusURL,
	}
	mux := http.NewServeMux()
	srv.RegisterRoutesForTest(mux)
	return httptest.NewServer(mux)
}

func validRunningPipeline(name, ns, podName string) *rpcv1alpha1.Pipeline {
	return &rpcv1alpha1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: rpcv1alpha1.PipelineSpec{
			Input:  rpcv1alpha1.ComponentSpec{Type: "generate"},
			Output: rpcv1alpha1.ComponentSpec{Type: "stdout"},
		},
		Status: rpcv1alpha1.PipelineStatus{
			Phase:   rpcv1alpha1.PhaseRunning,
			PodName: podName,
		},
	}
}

func mockPrometheusServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"resultType": "matrix",
				"result": []map[string]any{
					{"metric": map[string]string{}, "values": [][2]any{
						{1715000000, "1.5"},
						{1715000030, "2.0"},
					}},
				},
			},
		})
	}))
}

func mockPrometheusServerError(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
}

func mockPrometheusServerEmpty(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"resultType": "matrix",
				"result":     []any{},
			},
		})
	}))
}

func TestHandlerMetrics_PipelineNotFound(t *testing.T) {
	ts := newTestServerWithPrometheus(t, "http://localhost:9090")
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelines/no-such/metrics?query=throughput")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestHandlerMetrics_NoPod(t *testing.T) {
	pipe := &rpcv1alpha1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "idle", Namespace: "default"},
		Spec: rpcv1alpha1.PipelineSpec{
			Input:  rpcv1alpha1.ComponentSpec{Type: "generate"},
			Output: rpcv1alpha1.ComponentSpec{Type: "stdout"},
		},
	}
	ts := newTestServerWithPrometheus(t, "http://localhost:9090", pipe)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelines/idle/metrics?query=throughput")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("expected 409, got %d", resp.StatusCode)
	}
}

func TestHandlerMetrics_NoPrometheus(t *testing.T) {
	pipe := validRunningPipeline("demo", "default", "demo-pod-abc")
	ts := newTestServerWithPrometheus(t, "", pipe)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelines/demo/metrics?query=throughput")
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

func TestHandlerMetrics_UnknownQuery(t *testing.T) {
	pipe := validRunningPipeline("my-pipe", "default", "my-pipe-pod-abc")
	ts := newTestServerWithPrometheus(t, "http://localhost:9090", pipe)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelines/my-pipe/metrics?query=invalid")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHandlerMetrics_Success(t *testing.T) {
	prom := mockPrometheusServer(t)
	defer prom.Close()

	pipe := validRunningPipeline("demo", "default", "demo-pod-abc")
	ts := newTestServerWithPrometheus(t, prom.URL, pipe)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelines/demo/metrics?query=throughput")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body api.MetricsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Datapoints) != 2 {
		t.Errorf("expected 2 datapoints, got %d", len(body.Datapoints))
	}
	if body.Query != "throughput" {
		t.Errorf("expected query=throughput, got %q", body.Query)
	}
}

func TestHandlerMetrics_PrometheusError(t *testing.T) {
	prom := mockPrometheusServerError(t)
	defer prom.Close()

	pipe := validRunningPipeline("demo", "default", "demo-pod-abc")
	ts := newTestServerWithPrometheus(t, prom.URL, pipe)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelines/demo/metrics?query=throughput")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", resp.StatusCode)
	}
}

func TestHandlerMetrics_PrometheusDown(t *testing.T) {
	// Point to a port nothing is listening on.
	pipe := validRunningPipeline("demo", "ns-prod", "demo-pod-abc")
	ts := newTestServerWithPrometheus(t, "http://127.0.0.1:19999", pipe)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/ns-prod/pipelines/demo/metrics?query=throughput")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", resp.StatusCode)
	}
}

func TestHandlerMetrics_EmptyResult(t *testing.T) {
	prom := mockPrometheusServerEmpty(t)
	defer prom.Close()

	pipe := validRunningPipeline("demo", "default", "demo-pod-abc")
	ts := newTestServerWithPrometheus(t, prom.URL, pipe)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelines/demo/metrics?query=throughput")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	var body api.MetricsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Datapoints) != 0 {
		t.Errorf("expected 0 datapoints, got %d", len(body.Datapoints))
	}
}

func TestHandlerMetrics_Defaults(t *testing.T) {
	prom := mockPrometheusServer(t)
	defer prom.Close()

	pipe := validRunningPipeline("demo", "default", "demo-pod-abc")
	ts := newTestServerWithPrometheus(t, prom.URL, pipe)
	defer ts.Close()

	// No start/end/step — handler must apply defaults without panic.
	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelines/demo/metrics?query=throughput")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestHandlerMetrics_RouteRegistered(t *testing.T) {
	ts := newTestServerWithPrometheus(t, "")
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelines/nope/metrics?query=throughput")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "text/html") {
		t.Error("route not registered — SPA catch-all intercepted request")
	}
}
