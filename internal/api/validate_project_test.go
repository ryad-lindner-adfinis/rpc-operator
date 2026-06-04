package api_test

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
	"github.com/insidegreen/rpc-operator-claude/internal/api"
)

// orderProject is the sample project name reused across the api_test fixtures.
const orderProject = "orders"

func TestValidatePipeline_ProjectClusterMutualExclusion(t *testing.T) {
	p := &rpcv1alpha1.Pipeline{Spec: rpcv1alpha1.PipelineSpec{
		ProjectRef: &rpcv1alpha1.ProjectRef{Name: orderProject},
		ClusterRef: "etl",
	}}
	errs := api.ValidatePipeline(p, mustLoadCatalog(t))
	if len(errs) != 1 || errs[0].Message != "use projectRef or clusterRef, not both" {
		t.Fatalf("expected mutual-exclusion error, got %v", errs)
	}
}

func TestValidatePipeline_ProjectRefAllowsEmptyOutput(t *testing.T) {
	// A source pipeline in a project: input set (valid config), output managed (empty).
	p := &rpcv1alpha1.Pipeline{Spec: rpcv1alpha1.PipelineSpec{
		ProjectRef: &rpcv1alpha1.ProjectRef{Name: orderProject},
		Input: rpcv1alpha1.ComponentSpec{
			Type:   "generate",
			Config: runtime.RawExtension{Raw: []byte(`{"mapping":"root = {}"}`)},
		},
	}}
	if errs := api.ValidatePipeline(p, mustLoadCatalog(t)); len(errs) != 0 {
		t.Fatalf("empty managed output should be allowed, got %v", errs)
	}
}

func TestValidateProject_DelegatesMessages(t *testing.T) {
	proj := &rpcv1alpha1.PipelineProject{Spec: rpcv1alpha1.PipelineProjectSpec{
		Routes: []rpcv1alpha1.ProjectRoute{
			{Name: "r", From: "ghost", To: []rpcv1alpha1.ProjectRouteTarget{{Pipeline: "void"}}},
		}}}
	proj.Name = orderProject
	errs := api.ValidateProject(proj, nil)
	if len(errs) == 0 || errs[0].Path != "spec.routes" {
		t.Fatalf("expected routes errors, got %v", errs)
	}
}
