package render_test

import (
	"strings"
	"testing"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
	"github.com/insidegreen/rpc-operator-claude/internal/render"
)

func TestRenderPipelineYAML_RawYAML_InjectsHTTP(t *testing.T) {
	spec := &rpcv1alpha1.PipelineSpec{
		RawYAML: "input:\n  generate:\n    mapping: 'root = \"hi\"'\n    interval: 1s\noutput:\n  stdout: {}\n",
	}
	got, err := render.RenderPipelineYAML(spec)
	if err != nil {
		t.Fatalf("RenderPipelineYAML: %v", err)
	}
	for _, want := range []string{"generate:", "stdout:", "http:", "address: 0.0.0.0:4195"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in output:\n%s", want, got)
		}
	}
}

func TestRenderPipelineYAML_RawYAML_KeepsExistingHTTP(t *testing.T) {
	spec := &rpcv1alpha1.PipelineSpec{
		RawYAML: "input:\n  stdin: {}\noutput:\n  stdout: {}\nhttp:\n  enabled: false\n  address: 0.0.0.0:9999\n",
	}
	got, err := render.RenderPipelineYAML(spec)
	if err != nil {
		t.Fatalf("RenderPipelineYAML: %v", err)
	}
	if !strings.Contains(got, "9999") {
		t.Errorf("existing http block should be preserved, got:\n%s", got)
	}
	if strings.Contains(got, "4195") {
		t.Errorf("default http block should not be injected when http already present, got:\n%s", got)
	}
}

func TestRenderPipelineYAML_RawYAML_InvalidYAML(t *testing.T) {
	spec := &rpcv1alpha1.PipelineSpec{
		RawYAML: "{invalid: yaml: [",
	}
	_, err := render.RenderPipelineYAML(spec)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
	if !strings.Contains(err.Error(), "invalid YAML") {
		t.Errorf("expected 'invalid YAML' in error, got: %v", err)
	}
}

func TestRenderPipelineYAML_RawYAML_NonMapping(t *testing.T) {
	spec := &rpcv1alpha1.PipelineSpec{
		RawYAML: "- item1\n- item2\n",
	}
	_, err := render.RenderPipelineYAML(spec)
	if err == nil {
		t.Fatal("expected error for non-mapping YAML, got nil")
	}
	if !strings.Contains(err.Error(), "mapping") {
		t.Errorf("expected 'mapping' in error, got: %v", err)
	}
}

func TestRenderPipelineYAML_RawYAML_EmptyFallsThrough(t *testing.T) {
	// Empty RawYAML should fall through to the structured path.
	spec := &rpcv1alpha1.PipelineSpec{
		RawYAML: "",
		Input:   rpcv1alpha1.ComponentSpec{Type: "stdin"},
		Output:  rpcv1alpha1.ComponentSpec{Type: "stdout"},
	}
	got, err := render.RenderPipelineYAML(spec)
	if err != nil {
		t.Fatalf("RenderPipelineYAML: %v", err)
	}
	if !strings.Contains(got, "stdin:") {
		t.Errorf("expected structured render, got:\n%s", got)
	}
}
