package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
	"github.com/insidegreen/rpc-operator-claude/internal/api"
)

// clusterObj builds a PipelineCluster for seeding the fake client.
func clusterObj(name, ns string, replicas, ready int32, phase rpcv1alpha1.PipelineClusterPhase) *rpcv1alpha1.PipelineCluster {
	return &rpcv1alpha1.PipelineCluster{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       rpcv1alpha1.PipelineClusterSpec{Replicas: replicas},
		Status:     rpcv1alpha1.PipelineClusterStatus{Phase: phase, ReadyReplicas: ready},
	}
}

// clusterPod builds an instance pod labelled for the cluster, ready or not.
func clusterPod(name, ns, cluster string, ready bool) *corev1.Pod {
	cond := corev1.ConditionFalse
	if ready {
		cond = corev1.ConditionTrue
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: ns,
			Labels: map[string]string{"rpc.operator.io/cluster": cluster},
		},
		Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: cond}}},
	}
}

// clusteredPipeline builds a Pipeline assigned (Phase-2 placement) to an instance.
func clusteredPipeline(name, ns, cluster, instance string) *rpcv1alpha1.Pipeline {
	return &rpcv1alpha1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       rpcv1alpha1.PipelineSpec{ClusterRef: cluster},
		Status:     rpcv1alpha1.PipelineStatus{AssignedInstance: instance},
	}
}

func TestHandlerListNamespacedClusters(t *testing.T) {
	ts := newTestServer(t, clusterObj("etl", "default", 2, 2, rpcv1alpha1.ClusterPhaseReady))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelineclusters")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body struct {
		Items []rpcv1alpha1.PipelineCluster `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Items) != 1 || body.Items[0].Name != "etl" {
		t.Fatalf("expected [etl], got %+v", body.Items)
	}
}

func TestHandlerGetCluster_NotFound(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelineclusters/missing")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

// validClusterBody is a minimal valid PipelineCluster create/update body.
func validClusterBody(name, ns string, replicas int32) []byte {
	c := map[string]any{
		"apiVersion": "rpc.operator.io/v1alpha1",
		"kind":       "PipelineCluster",
		"metadata":   map[string]any{"name": name, "namespace": ns},
		"spec":       map[string]any{"replicas": replicas},
	}
	b, _ := json.Marshal(c)
	return b
}

func TestHandlerCreateCluster(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/v1/namespaces/default/pipelineclusters",
		"application/json", bytes.NewReader(validClusterBody("etl", "default", 2)))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
}

func TestHandlerCreateCluster_NamespaceMismatch(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/v1/namespaces/default/pipelineclusters",
		"application/json", bytes.NewReader(validClusterBody("etl", "other", 2)))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHandlerUpdateCluster_Scale(t *testing.T) {
	ts := newTestServer(t, clusterObj("etl", "default", 2, 2, rpcv1alpha1.ClusterPhaseReady))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPut,
		ts.URL+"/api/v1/namespaces/default/pipelineclusters/etl",
		bytes.NewReader(validClusterBody("etl", "default", 5)))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var got rpcv1alpha1.PipelineCluster
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Spec.Replicas != 5 {
		t.Fatalf("expected replicas=5 after scale, got %d", got.Spec.Replicas)
	}
}

func TestHandlerDeleteCluster(t *testing.T) {
	ts := newTestServer(t, clusterObj("etl", "default", 2, 2, rpcv1alpha1.ClusterPhaseReady))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodDelete,
		ts.URL+"/api/v1/namespaces/default/pipelineclusters/etl", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestHandlerClusterInstances(t *testing.T) {
	objs := []client.Object{
		clusterObj("etl", "default", 3, 2, rpcv1alpha1.ClusterPhaseReady),
		clusterPod("etl-0", "default", "etl", true),
		clusterPod("etl-1", "default", "etl", false),
		clusteredPipeline("p1", "default", "etl", "etl-0"),
		clusteredPipeline("p2", "default", "etl", "etl-0"),
		clusteredPipeline("p3", "default", "etl", "etl-1"),
		clusteredPipeline("p9", "default", "etl", "etl-5"), // stale
	}
	ts := newTestServer(t, objs...)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelineclusters/etl/instances")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var dist api.ClusterDistribution
	if err := json.NewDecoder(resp.Body).Decode(&dist); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(dist.Instances) != 3 {
		t.Fatalf("expected 3 instances, got %d", len(dist.Instances))
	}
	if dist.Instances[0].Name != "etl-0" || !dist.Instances[0].Ready ||
		len(dist.Instances[0].AssignedPipelines) != 2 {
		t.Errorf("slot0 wrong: %+v", dist.Instances[0])
	}
	if len(dist.StalePlacements) != 1 || dist.StalePlacements[0].Pipeline != "p9" {
		t.Errorf("stale wrong: %+v", dist.StalePlacements)
	}
}

func TestHandlerClusterInstances_NotFound(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelineclusters/missing/instances")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestHandlerClusterMetrics_SumsAcrossInstances(t *testing.T) {
	var captured string
	prom := mockPrometheusServerCapturing(t, &captured)
	defer prom.Close()

	cl := clusterObj("etl", "default", 2, 2, rpcv1alpha1.ClusterPhaseReady)
	ts := newTestServerWithPrometheus(t, prom.URL, cl)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelineclusters/etl/metrics?query=throughput")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(captured, `sum(rate(output_sent{`) {
		t.Errorf("query not a cluster sum: %q", captured)
	}
	if !strings.Contains(captured, `pod=~"etl-[0-9]+"`) {
		t.Errorf("query missing anchored pod regex: %q", captured)
	}
	if !strings.Contains(captured, `namespace="default"`) {
		t.Errorf("query missing namespace selector: %q", captured)
	}
}

func TestHandlerClusterMetrics_UnknownQuery(t *testing.T) {
	cl := clusterObj("etl", "default", 2, 2, rpcv1alpha1.ClusterPhaseReady)
	ts := newTestServerWithPrometheus(t, "http://unused", cl)
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelineclusters/etl/metrics?query=bogus")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHandlerClusterMetrics_PrometheusUnconfigured(t *testing.T) {
	cl := clusterObj("etl", "default", 2, 2, rpcv1alpha1.ClusterPhaseReady)
	ts := newTestServer(t, cl) // no PrometheusURL
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelineclusters/etl/metrics?query=throughput")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
}

func TestHandlerClusterMetrics_ClusterNotFound(t *testing.T) {
	ts := newTestServerWithPrometheus(t, "http://unused") // no cluster object
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelineclusters/missing/metrics?query=throughput")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}
