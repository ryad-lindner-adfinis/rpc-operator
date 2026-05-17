// Package api_test tests the API handlers using controller-runtime/client/fake.
// Justification: envtest is appropriate for the controller (reconciliation semantics,
// watch fan-out), but adds ~3s spin-up per suite for tests that only verify HTTP
// status codes and JSON marshaling. client/fake returns objects synchronously, which
// is exactly what handler tests need. Do NOT retrofit client/fake into controller tests.
package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
	"github.com/insidegreen/rpc-operator-claude/internal/api"
	"github.com/insidegreen/rpc-operator-claude/internal/api/catalog"
)

func newFakeClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	sch := runtime.NewScheme()
	if err := rpcv1alpha1.AddToScheme(sch); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(sch); err != nil {
		t.Fatal(err)
	}
	return fake.NewClientBuilder().WithScheme(sch).WithObjects(objs...).Build()
}

func newTestServer(t *testing.T, objs ...client.Object) *httptest.Server {
	t.Helper()
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}
	srv := &api.Server{
		Addr:    ":0",
		Client:  newFakeClient(t, objs...),
		Catalog: cat,
	}
	mux := http.NewServeMux()
	srv.RegisterRoutesForTest(mux)
	return httptest.NewServer(mux)
}

// validPipelineBody returns a minimal valid pipeline JSON body.
func validPipelineBody(name, ns string) []byte {
	p := map[string]any{
		"apiVersion": "rpc.operator.io/v1alpha1",
		"kind":       "Pipeline",
		"metadata":   map[string]any{"name": name, "namespace": ns},
		"spec": map[string]any{
			"input": map[string]any{
				"type":   "generate",
				"config": map[string]any{"mapping": `root = "hi"`, "interval": "1s", "count": 3},
			},
			"processors": []any{
				map[string]any{"type": "mapping", "label": "normalize", "config": `root = content().uppercase()`},
			},
			"output": map[string]any{"type": "stdout", "config": map[string]any{}},
		},
	}
	b, _ := json.Marshal(p)
	return b
}

func TestHandlerPipelines_Create(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Post(
		ts.URL+"/api/v1/namespaces/default/pipelines",
		"application/json",
		bytes.NewReader(validPipelineBody("test-pipe", "default")),
	)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}
	var result rpcv1alpha1.Pipeline
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Name != "test-pipe" {
		t.Errorf("expected name test-pipe, got %q", result.Name)
	}
}

func TestHandlerPipelines_Get(t *testing.T) {
	existing := &rpcv1alpha1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "my-pipe", Namespace: "default"},
		Spec: rpcv1alpha1.PipelineSpec{
			Input:  rpcv1alpha1.ComponentSpec{Type: "generate"},
			Output: rpcv1alpha1.ComponentSpec{Type: "stdout"},
		},
	}
	ts := newTestServer(t, existing)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelines/my-pipe")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestHandlerPipelines_GetNotFound(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelines/no-such")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestHandlerPipelines_List(t *testing.T) {
	p1 := &rpcv1alpha1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
		Spec: rpcv1alpha1.PipelineSpec{
			Input:  rpcv1alpha1.ComponentSpec{Type: "generate"},
			Output: rpcv1alpha1.ComponentSpec{Type: "stdout"},
		},
	}
	ts := newTestServer(t, p1)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelines")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	var result map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := result["items"]; !ok {
		t.Error("response missing 'items' key")
	}
}

func TestHandlerPipelines_Delete(t *testing.T) {
	existing := &rpcv1alpha1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "del-me", Namespace: "default"},
		Spec: rpcv1alpha1.PipelineSpec{
			Input:  rpcv1alpha1.ComponentSpec{Type: "generate"},
			Output: rpcv1alpha1.ComponentSpec{Type: "stdout"},
		},
	}
	ts := newTestServer(t, existing)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodDelete,
		ts.URL+"/api/v1/namespaces/default/pipelines/del-me", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestHandlerPipelines_Update_ReturnsUpdated(t *testing.T) {
	existing := &rpcv1alpha1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "upd-me", Namespace: "default"},
		Spec: rpcv1alpha1.PipelineSpec{
			Input:  rpcv1alpha1.ComponentSpec{Type: "generate"},
			Output: rpcv1alpha1.ComponentSpec{Type: "stdout"},
		},
	}
	ts := newTestServer(t, existing)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPut,
		ts.URL+"/api/v1/namespaces/default/pipelines/upd-me",
		bytes.NewReader(validPipelineBody("upd-me", "default")),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	var result map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := result["warnings"]; ok {
		t.Error("response must not contain 'warnings' key after pod-spec mutability ships")
	}
	if result["metadata"] == nil {
		t.Error("response missing 'metadata'")
	}
}

func TestHandlerPipelines_ValidationFailureReturns422(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	// object body for a scalar-body component (mapping processor)
	bad := map[string]any{
		"spec": map[string]any{
			"input": map[string]any{
				"type":   "generate",
				"config": map[string]any{"mapping": `root = "hi"`},
			},
			"processors": []any{
				map[string]any{
					"type":   "mapping",
					"config": map[string]any{"mapping": "root = content().uppercase()"},
				},
			},
			"output": map[string]any{"type": "stdout", "config": map[string]any{}},
		},
	}
	body, _ := json.Marshal(bad)
	resp, err := http.Post(
		ts.URL+"/api/v1/namespaces/default/pipelines",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", resp.StatusCode)
	}
	var result map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := result["errors"]; !ok {
		t.Error("422 response missing 'errors' key")
	}
}

func TestHandlerPipelines_NamespaceMismatchReturns400(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Post(
		ts.URL+"/api/v1/namespaces/default/pipelines",
		"application/json",
		bytes.NewReader(validPipelineBody("x", "other-ns")),
	)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHandlerPipelines_BadJSON(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Post(
		ts.URL+"/api/v1/namespaces/default/pipelines",
		"application/json",
		strings.NewReader("{not valid json"),
	)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHandlerPipelines_ListAll(t *testing.T) {
	p1 := &rpcv1alpha1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1"},
		Spec: rpcv1alpha1.PipelineSpec{
			Input:  rpcv1alpha1.ComponentSpec{Type: "generate"},
			Output: rpcv1alpha1.ComponentSpec{Type: "stdout"},
		},
	}
	p2 := &rpcv1alpha1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "p2", Namespace: "ns2"},
		Spec: rpcv1alpha1.PipelineSpec{
			Input:  rpcv1alpha1.ComponentSpec{Type: "generate"},
			Output: rpcv1alpha1.ComponentSpec{Type: "stdout"},
		},
	}
	ts := newTestServer(t, p1, p2)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/pipelines")
	if err != nil {
		t.Fatalf("GET /pipelines: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	var result map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := result["items"]; !ok {
		t.Error("response missing 'items' key")
	}
}

func TestHandlerPipelines_DeleteNotFound(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodDelete,
		ts.URL+"/api/v1/namespaces/default/pipelines/no-such", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestHandlerPipelines_UpdateNotFound(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPut,
		ts.URL+"/api/v1/namespaces/default/pipelines/no-such",
		bytes.NewReader(validPipelineBody("no-such", "default")),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestHandlerPipelines_UpdateBadJSON(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPut,
		ts.URL+"/api/v1/namespaces/default/pipelines/any",
		strings.NewReader("{bad json"),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHandlerPipelines_CreateAlreadyExists(t *testing.T) {
	existing := &rpcv1alpha1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "dup", Namespace: "default"},
		Spec: rpcv1alpha1.PipelineSpec{
			Input:  rpcv1alpha1.ComponentSpec{Type: "generate"},
			Output: rpcv1alpha1.ComponentSpec{Type: "stdout"},
		},
	}
	ts := newTestServer(t, existing)
	defer ts.Close()

	resp, err := http.Post(
		ts.URL+"/api/v1/namespaces/default/pipelines",
		"application/json",
		bytes.NewReader(validPipelineBody("dup", "default")),
	)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("expected 409, got %d", resp.StatusCode)
	}
}

func TestHandlerValidate_HappyPath(t *testing.T) {
	ts := newTestServer(t)
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
	var result map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(result["valid"]) != "true" {
		t.Errorf("expected valid=true, got %s", result["valid"])
	}
}

func TestHandlerValidate_BadJSON(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Post(
		ts.URL+"/api/v1/pipelines/validate",
		"application/json",
		strings.NewReader("{bad json"),
	)
	if err != nil {
		t.Fatalf("POST validate: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHandlerValidate_InvalidPipeline(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	// unknown component type should produce valid=false
	bad := map[string]any{
		"spec": map[string]any{
			"input":  map[string]any{"type": "unknown-input"},
			"output": map[string]any{"type": "stdout"},
		},
	}
	body, _ := json.Marshal(bad)
	resp, err := http.Post(
		ts.URL+"/api/v1/pipelines/validate",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST validate: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 (validate always returns 200), got %d", resp.StatusCode)
	}
	var result map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(result["valid"]) != "false" {
		t.Errorf("expected valid=false for unknown component, got %s", result["valid"])
	}
}

func rawPipelineBody(name, ns, rawYAML string) []byte {
	p := map[string]any{
		"apiVersion": "rpc.operator.io/v1alpha1",
		"kind":       "Pipeline",
		"metadata":   map[string]any{"name": name, "namespace": ns},
		"spec":       map[string]any{"rawYAML": rawYAML},
	}
	b, _ := json.Marshal(p)
	return b
}

func TestHandlerPipelines_Create_RawYAML(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	body := rawPipelineBody("raw-pipe", "default",
		"input:\n  generate:\n    mapping: 'root = \"hi\"'\n    interval: 1s\noutput:\n  stdout: {}\n")
	resp, err := http.Post(
		ts.URL+"/api/v1/namespaces/default/pipelines",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}
	var result rpcv1alpha1.Pipeline
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Spec.RawYAML == "" {
		t.Error("expected spec.rawYAML to be set in stored CR")
	}
}

func TestHandlerPipelines_Create_RawYAML_InvalidYAML(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	body := rawPipelineBody("bad-raw", "default", "{invalid: yaml: [")
	resp, err := http.Post(
		ts.URL+"/api/v1/namespaces/default/pipelines",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for invalid rawYAML, got %d", resp.StatusCode)
	}
}

// F45: stop/run subresources.

func TestHandlerPipelines_Stop_SetsStopped(t *testing.T) {
	existing := &rpcv1alpha1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "stoppable", Namespace: "default"},
		Spec: rpcv1alpha1.PipelineSpec{
			Input:  rpcv1alpha1.ComponentSpec{Type: "generate"},
			Output: rpcv1alpha1.ComponentSpec{Type: "stdout"},
		},
	}
	ts := newTestServer(t, existing)
	defer ts.Close()

	resp, err := http.Post(
		ts.URL+"/api/v1/namespaces/default/pipelines/stoppable/stop",
		"application/json", nil,
	)
	if err != nil {
		t.Fatalf("POST /stop: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var got rpcv1alpha1.Pipeline
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Spec.Stopped {
		t.Errorf("expected spec.stopped=true after /stop, got false")
	}
}

func TestHandlerPipelines_Run_ClearsStopped(t *testing.T) {
	existing := &rpcv1alpha1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "runnable", Namespace: "default"},
		Spec: rpcv1alpha1.PipelineSpec{
			Input:   rpcv1alpha1.ComponentSpec{Type: "generate"},
			Output:  rpcv1alpha1.ComponentSpec{Type: "stdout"},
			Stopped: true,
		},
	}
	ts := newTestServer(t, existing)
	defer ts.Close()

	resp, err := http.Post(
		ts.URL+"/api/v1/namespaces/default/pipelines/runnable/run",
		"application/json", nil,
	)
	if err != nil {
		t.Fatalf("POST /run: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var got rpcv1alpha1.Pipeline
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Spec.Stopped {
		t.Errorf("expected spec.stopped=false after /run, got true")
	}
}

func TestHandlerPipelines_Stop_NotFound(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	resp, err := http.Post(
		ts.URL+"/api/v1/namespaces/default/pipelines/nope/stop",
		"application/json", nil,
	)
	if err != nil {
		t.Fatalf("POST /stop: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestHandlerPipelines_Stop_Idempotent(t *testing.T) {
	existing := &rpcv1alpha1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "already-stopped", Namespace: "default"},
		Spec: rpcv1alpha1.PipelineSpec{
			Input:   rpcv1alpha1.ComponentSpec{Type: "generate"},
			Output:  rpcv1alpha1.ComponentSpec{Type: "stdout"},
			Stopped: true,
		},
	}
	ts := newTestServer(t, existing)
	defer ts.Close()
	resp, err := http.Post(
		ts.URL+"/api/v1/namespaces/default/pipelines/already-stopped/stop",
		"application/json", nil,
	)
	if err != nil {
		t.Fatalf("POST /stop: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 (idempotent), got %d", resp.StatusCode)
	}
	var got rpcv1alpha1.Pipeline
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Spec.Stopped {
		t.Errorf("expected spec.stopped=true to remain true, got false")
	}
}
