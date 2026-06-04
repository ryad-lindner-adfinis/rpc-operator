package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
)

// projectObj builds a Ready PipelineProject named "orders" in namespace "default".
func projectObj() *rpcv1alpha1.PipelineProject {
	return &rpcv1alpha1.PipelineProject{
		ObjectMeta: metav1.ObjectMeta{Name: orderProject, Namespace: defaultNamespace},
		Spec: rpcv1alpha1.PipelineProjectSpec{
			Description: "routed orders",
			Routes: []rpcv1alpha1.ProjectRoute{
				{Name: "ingest", From: "order-ingest", To: []rpcv1alpha1.ProjectRouteTarget{{Pipeline: "warehouse"}}},
			},
		},
		Status: rpcv1alpha1.PipelineProjectStatus{
			Phase:   rpcv1alpha1.ProjectPhaseReady,
			Cluster: rpcv1alpha1.ProjectChildStatus{Name: "orders-cluster", Ready: 1, Total: 1},
			NATS:    rpcv1alpha1.ProjectChildStatus{Name: "orders-nats", Ready: 1, Total: 1},
		},
	}
}

func TestHandlerListNamespacedProjects(t *testing.T) {
	ts := newTestServer(t, projectObj())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelineprojects")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body struct {
		Items []rpcv1alpha1.PipelineProject `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Items) != 1 || body.Items[0].Name != orderProject {
		t.Fatalf("expected [orders], got %+v", body.Items)
	}
}

func TestHandlerGetProject(t *testing.T) {
	ts := newTestServer(t, projectObj())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelineprojects/orders")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var got rpcv1alpha1.PipelineProject
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Name != orderProject || got.Status.Cluster.Name != "orders-cluster" {
		t.Fatalf("unexpected project: %+v", got)
	}
}

func TestHandlerGetProjectNotFound(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelineprojects/missing")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestHandlerCreateProject(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	payload := `{"apiVersion":"rpc.operator.io/v1alpha1","kind":"PipelineProject",` +
		`"metadata":{"name":"neo","namespace":"default"},` +
		`"spec":{"description":"new project"}}`
	resp, err := http.Post(ts.URL+"/api/v1/namespaces/default/pipelineprojects",
		"application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var got rpcv1alpha1.PipelineProject
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Name != "neo" || got.Namespace != defaultNamespace {
		t.Fatalf("unexpected created project: %+v", got)
	}
}

func TestHandlerUpdateProjectReplacesSpec(t *testing.T) {
	ts := newTestServer(t, projectObj())
	defer ts.Close()

	// Replace routes with an empty table (pure grouping).
	payload := `{"apiVersion":"rpc.operator.io/v1alpha1","kind":"PipelineProject",` +
		`"metadata":{"name":"orders","namespace":"default"},` +
		`"spec":{"description":"degrouped"}}`
	req, _ := http.NewRequest(http.MethodPut,
		ts.URL+"/api/v1/namespaces/default/pipelineprojects/orders",
		strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var got rpcv1alpha1.PipelineProject
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Spec.Routes) != 0 || got.Spec.Description != "degrouped" {
		t.Fatalf("spec not replaced: %+v", got.Spec)
	}
}

func TestHandlerDeleteProject(t *testing.T) {
	ts := newTestServer(t, projectObj())
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodDelete,
		ts.URL+"/api/v1/namespaces/default/pipelineprojects/orders", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// --- validate-on-commit tests ---

// namedProjectObj builds a PipelineProject with arbitrary ns/name and routes.
func namedProjectObj(ns, name string, routes []rpcv1alpha1.ProjectRoute) *rpcv1alpha1.PipelineProject {
	return &rpcv1alpha1.PipelineProject{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name},
		Spec:       rpcv1alpha1.PipelineProjectSpec{Routes: routes},
	}
}

// rawPipelineObj builds a raw-YAML Pipeline attached to a project. The rawYAML
// controls HasInput/HasOutput as ValidateProject sees them.
func rawPipelineObj(name, project, rawYAML string) *rpcv1alpha1.Pipeline {
	return &rpcv1alpha1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Namespace: defaultNamespace, Name: name},
		Spec: rpcv1alpha1.PipelineSpec{
			ProjectRef: &rpcv1alpha1.ProjectRef{Name: project},
			RawYAML:    rawYAML,
		},
	}
}

func putProjectBody(t *testing.T, ns, name string, routes []rpcv1alpha1.ProjectRoute) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"apiVersion": "rpc.operator.io/v1alpha1",
		"kind":       "PipelineProject",
		"metadata":   map[string]any{"name": name, "namespace": ns},
		"spec":       rpcv1alpha1.PipelineProjectSpec{Routes: routes},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// A sink pipeline that still declares its own input conflicts with the
// operator-managed input → update must be rejected 422 and NOT persisted.
func TestUpdateProject_InvalidGraphRejectedAndNotPersisted(t *testing.T) {
	ns := defaultNamespace
	source := rawPipelineObj("ingest", "orders2", "input:\n  generate: {}\npipeline:\n  processors: []\n")
	sink := rawPipelineObj("warehouse", "orders2", "input:\n  generate: {}\npipeline:\n  processors: []\noutput:\n  stdout: {}\n")
	proj := namedProjectObj(ns, "orders2", nil)
	ts := newTestServer(t, proj, source, sink)
	defer ts.Close()

	routes := []rpcv1alpha1.ProjectRoute{{
		Name: "fan", From: "ingest", To: []rpcv1alpha1.ProjectRouteTarget{{Pipeline: "warehouse"}},
	}}
	req, _ := http.NewRequest(http.MethodPut,
		ts.URL+"/api/v1/namespaces/default/pipelineprojects/orders2",
		bytes.NewReader(putProjectBody(t, ns, "orders2", routes)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("PUT: want 422, got %d", resp.StatusCode)
	}
	var body struct {
		Errors []struct{ Message string } `json:"errors"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if len(body.Errors) == 0 {
		t.Fatalf("want validation errors, got none")
	}

	// CR must be unchanged (routes still empty).
	getResp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelineprojects/orders2")
	if err != nil {
		t.Fatalf("GET after PUT: %v", err)
	}
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("GET: want 200, got %d", getResp.StatusCode)
	}
	defer func() { _ = getResp.Body.Close() }()
	var got rpcv1alpha1.PipelineProject
	_ = json.NewDecoder(getResp.Body).Decode(&got)
	if len(got.Spec.Routes) != 0 {
		t.Errorf("routes persisted despite invalid graph: %+v", got.Spec.Routes)
	}
}

func TestUpdateProject_ValidGraphPersists(t *testing.T) {
	ns := defaultNamespace
	source := rawPipelineObj("ingest2", "orders3", "input:\n  generate: {}\npipeline:\n  processors: []\n")
	sink := rawPipelineObj("warehouse2", "orders3", "pipeline:\n  processors: []\noutput:\n  stdout: {}\n")
	proj := namedProjectObj(ns, "orders3", nil)
	ts := newTestServer(t, proj, source, sink)
	defer ts.Close()

	routes := []rpcv1alpha1.ProjectRoute{{
		Name: "fan", From: "ingest2", To: []rpcv1alpha1.ProjectRouteTarget{{Pipeline: "warehouse2"}},
	}}
	req, _ := http.NewRequest(http.MethodPut,
		ts.URL+"/api/v1/namespaces/default/pipelineprojects/orders3",
		bytes.NewReader(putProjectBody(t, ns, "orders3", routes)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT: want 200, got %d", resp.StatusCode)
	}
	getResp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelineprojects/orders3")
	if err != nil {
		t.Fatalf("GET after PUT: %v", err)
	}
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("GET: want 200, got %d", getResp.StatusCode)
	}
	defer func() { _ = getResp.Body.Close() }()
	var got rpcv1alpha1.PipelineProject
	_ = json.NewDecoder(getResp.Body).Decode(&got)
	if len(got.Spec.Routes) != 1 || got.Spec.Routes[0].Name != "fan" {
		t.Errorf("routes not persisted: %+v", got.Spec.Routes)
	}
}

func TestCreateProject_NoRoutesIsValid(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	body, _ := json.Marshal(map[string]any{
		"apiVersion": "rpc.operator.io/v1alpha1",
		"kind":       "PipelineProject",
		"metadata":   map[string]any{"name": "empty", "namespace": defaultNamespace},
		"spec":       rpcv1alpha1.PipelineProjectSpec{},
	})
	resp, err := http.Post(ts.URL+"/api/v1/namespaces/default/pipelineprojects",
		"application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST: want 201, got %d", resp.StatusCode)
	}
}

// The create path validates too: a sink that still declares its own input
// conflicts with the operator-managed input → 422 and the CR is not created.
func TestCreateProject_InvalidGraphRejected(t *testing.T) {
	ns := defaultNamespace
	source := rawPipelineObj("ingest3", "orders4", "input:\n  generate: {}\npipeline:\n  processors: []\n")
	sink := rawPipelineObj("warehouse3", "orders4", "input:\n  generate: {}\npipeline:\n  processors: []\noutput:\n  stdout: {}\n")
	ts := newTestServer(t, source, sink)
	defer ts.Close()

	routes := []rpcv1alpha1.ProjectRoute{{
		Name: "fan", From: "ingest3", To: []rpcv1alpha1.ProjectRouteTarget{{Pipeline: "warehouse3"}},
	}}
	resp, err := http.Post(ts.URL+"/api/v1/namespaces/default/pipelineprojects",
		"application/json", bytes.NewReader(putProjectBody(t, ns, "orders4", routes)))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("POST: want 422, got %d", resp.StatusCode)
	}
	var body struct {
		Errors []struct{ Message string } `json:"errors"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if len(body.Errors) == 0 {
		t.Fatalf("want validation errors, got none")
	}

	// CR must not have been created.
	getResp, err := http.Get(ts.URL + "/api/v1/namespaces/default/pipelineprojects/orders4")
	if err != nil {
		t.Fatalf("GET after rejected POST: %v", err)
	}
	defer func() { _ = getResp.Body.Close() }()
	if getResp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET: want 404 (not created), got %d", getResp.StatusCode)
	}
}
