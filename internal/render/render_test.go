/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package render_test

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
	"github.com/insidegreen/rpc-operator-claude/internal/render"
)

func TestRenderPipelineYAML_Generate(t *testing.T) {
	spec := &rpcv1alpha1.PipelineSpec{
		Input: rpcv1alpha1.ComponentSpec{
			Type: "generate",
			Config: runtime.RawExtension{Raw: []byte(
				`{"mapping":"root = \"hello\"","interval":"1s","count":5}`,
			)},
		},
		Processors: []rpcv1alpha1.ComponentSpec{{
			Type: "mapping",
			Config: runtime.RawExtension{Raw: []byte(
				`{"mapping":"root = content().uppercase()"}`,
			)},
		}},
		Output: rpcv1alpha1.ComponentSpec{
			Type:   "stdout",
			Config: runtime.RawExtension{Raw: []byte(`{}`)},
		},
	}

	got, err := render.RenderPipelineYAML(spec)
	if err != nil {
		t.Fatalf("RenderPipelineYAML: %v", err)
	}

	mustContain := []string{
		"input:",
		"generate:",
		"interval: 1s",
		"count: 5",
		"processors:",
		"mapping:",
		"output:",
		"stdout:",
		"http:",
		"address: 0.0.0.0:4195",
	}
	for _, want := range mustContain {
		if !strings.Contains(got, want) {
			t.Errorf("rendered YAML missing %q\n--- output ---\n%s", want, got)
		}
	}
}

func TestRenderPipelineYAML_EmptyConfig(t *testing.T) {
	spec := &rpcv1alpha1.PipelineSpec{
		Input:  rpcv1alpha1.ComponentSpec{Type: "stdin"},
		Output: rpcv1alpha1.ComponentSpec{Type: "stdout"},
	}

	got, err := render.RenderPipelineYAML(spec)
	if err != nil {
		t.Fatalf("RenderPipelineYAML: %v", err)
	}
	for _, want := range []string{"stdin: {}", "stdout: {}"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered YAML missing %q\n--- output ---\n%s", want, got)
		}
	}
}

func TestRenderPipelineYAML_NullConfig(t *testing.T) {
	spec := &rpcv1alpha1.PipelineSpec{
		Input:  rpcv1alpha1.ComponentSpec{Type: "stdin", Config: runtime.RawExtension{Raw: []byte("null")}},
		Output: rpcv1alpha1.ComponentSpec{Type: "stdout"},
	}
	got, err := render.RenderPipelineYAML(spec)
	if err != nil {
		t.Fatalf("RenderPipelineYAML: %v", err)
	}
	if !strings.Contains(got, "stdin: {}") {
		t.Errorf("null Config.Raw should render as empty object\n%s", got)
	}
}

func TestRenderPipelineYAML_BrokerOutput(t *testing.T) {
	spec := &rpcv1alpha1.PipelineSpec{
		Input: rpcv1alpha1.ComponentSpec{
			Type:   "generate",
			Config: runtime.RawExtension{Raw: []byte(`{"mapping":"root = \"hi\"","count":1}`)},
		},
		Output: rpcv1alpha1.ComponentSpec{
			Type: "broker",
			Config: runtime.RawExtension{Raw: []byte(`{
				"copies": 1,
				"outputs": [
					{"type": "stdout", "config": {}},
					{"type": "stdout", "config": {}}
				]
			}`)},
		},
	}
	got, err := render.RenderPipelineYAML(spec)
	if err != nil {
		t.Fatalf("RenderPipelineYAML: %v", err)
	}
	for _, want := range []string{"broker:", "outputs:", "- stdout: {}"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q\n%s", want, got)
		}
	}
	if strings.Contains(got, "type: stdout") {
		t.Errorf("ComponentSpec format leaked into RPC YAML\n%s", got)
	}
}

func TestRenderPipelineYAML_BranchProcessor(t *testing.T) {
	spec := &rpcv1alpha1.PipelineSpec{
		Input: rpcv1alpha1.ComponentSpec{
			Type:   "generate",
			Config: runtime.RawExtension{Raw: []byte(`{"mapping":"root = \"hi\"","count":1}`)},
		},
		Output: rpcv1alpha1.ComponentSpec{Type: "stdout"},
		Processors: []rpcv1alpha1.ComponentSpec{{
			Type: "branch",
			Config: runtime.RawExtension{Raw: []byte(`{
				"request_map": "root = this",
				"processors": [{"type": "mapping", "config": "root = content().uppercase()"}],
				"result_map": "root = this"
			}`)},
		}},
	}
	got, err := render.RenderPipelineYAML(spec)
	if err != nil {
		t.Fatalf("RenderPipelineYAML: %v", err)
	}
	for _, want := range []string{"branch:", "request_map:", "processors:", "- mapping:", "result_map:"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q\n%s", want, got)
		}
	}
	if strings.Contains(got, "type: mapping") {
		t.Errorf("ComponentSpec format leaked into RPC YAML\n%s", got)
	}
}

func TestRenderPipelineYAML_ForEachProcessor(t *testing.T) {
	spec := &rpcv1alpha1.PipelineSpec{
		Input: rpcv1alpha1.ComponentSpec{
			Type:   "generate",
			Config: runtime.RawExtension{Raw: []byte(`{"mapping":"root = \"hi\"","count":1}`)},
		},
		Output: rpcv1alpha1.ComponentSpec{Type: "stdout"},
		Processors: []rpcv1alpha1.ComponentSpec{{
			Type:   "for_each",
			Config: runtime.RawExtension{Raw: []byte(`[{"type":"mapping","config":"root = content().uppercase()"}]`)},
		}},
	}
	got, err := render.RenderPipelineYAML(spec)
	if err != nil {
		t.Fatalf("RenderPipelineYAML: %v", err)
	}
	for _, want := range []string{"for_each:", "- mapping:"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q\n%s", want, got)
		}
	}
	if strings.Contains(got, "type: mapping") {
		t.Errorf("ComponentSpec format leaked into RPC YAML\n%s", got)
	}
}

func TestRenderPipelineYAML_FallbackOutput(t *testing.T) {
	spec := &rpcv1alpha1.PipelineSpec{
		Input: rpcv1alpha1.ComponentSpec{
			Type:   "generate",
			Config: runtime.RawExtension{Raw: []byte(`{"mapping":"root = \"hi\"","count":1}`)},
		},
		Output: rpcv1alpha1.ComponentSpec{
			Type:   "fallback",
			Config: runtime.RawExtension{Raw: []byte(`[{"type":"stdout","config":{}},{"type":"stdout","config":{}}]`)},
		},
	}
	got, err := render.RenderPipelineYAML(spec)
	if err != nil {
		t.Fatalf("RenderPipelineYAML: %v", err)
	}
	for _, want := range []string{"fallback:", "- stdout: {}"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q\n%s", want, got)
		}
	}
	if strings.Contains(got, "type: stdout") {
		t.Errorf("ComponentSpec format leaked into RPC YAML\n%s", got)
	}
}

func TestRenderPipelineYAML_NestedComposite(t *testing.T) {
	// broker output containing a sequence input-like structure inside another broker
	// Tests 2-level nesting
	spec := &rpcv1alpha1.PipelineSpec{
		Input: rpcv1alpha1.ComponentSpec{
			Type:   "generate",
			Config: runtime.RawExtension{Raw: []byte(`{"mapping":"root = \"hi\"","count":1}`)},
		},
		Output: rpcv1alpha1.ComponentSpec{
			Type: "broker",
			Config: runtime.RawExtension{Raw: []byte(`{
				"outputs": [
					{"type": "stdout", "config": {}},
					{"type": "broker", "config": {"outputs": [{"type": "stdout", "config": {}}]}}
				]
			}`)},
		},
	}
	got, err := render.RenderPipelineYAML(spec)
	if err != nil {
		t.Fatalf("RenderPipelineYAML: %v", err)
	}
	if strings.Contains(got, "type: stdout") || strings.Contains(got, "type: broker") {
		t.Errorf("ComponentSpec format leaked into nested RPC YAML\n%s", got)
	}
	if !strings.Contains(got, "broker:") {
		t.Errorf("missing broker: in output\n%s", got)
	}
}

func TestRenderPipelineYAML_InvalidJSON(t *testing.T) {
	spec := &rpcv1alpha1.PipelineSpec{
		Input: rpcv1alpha1.ComponentSpec{
			Type:   "generate",
			Config: runtime.RawExtension{Raw: []byte(`{not valid json`)},
		},
		Output: rpcv1alpha1.ComponentSpec{Type: "stdout"},
	}
	_, err := render.RenderPipelineYAML(spec)
	if err == nil {
		t.Fatal("expected error for invalid JSON config, got nil")
	}
	if !strings.Contains(err.Error(), "config not valid JSON") {
		t.Errorf("expected error to mention 'config not valid JSON', got: %v", err)
	}
}

func TestRenderPipelineYAML_ProcessorLabel(t *testing.T) {
	spec := &rpcv1alpha1.PipelineSpec{
		Input: rpcv1alpha1.ComponentSpec{
			Type:   "generate",
			Config: runtime.RawExtension{Raw: []byte(`{"mapping":"root = \"hello\"","interval":"1s","count":1}`)},
		},
		Processors: []rpcv1alpha1.ComponentSpec{{
			Type:   "mapping",
			Label:  "normalize",
			Config: runtime.RawExtension{Raw: []byte(`"root = content().uppercase()"`)},
		}},
		Output: rpcv1alpha1.ComponentSpec{
			Type:   "stdout",
			Config: runtime.RawExtension{Raw: []byte(`{}`)},
		},
	}
	got, err := render.RenderPipelineYAML(spec)
	if err != nil {
		t.Fatalf("RenderPipelineYAML: %v", err)
	}
	for _, want := range []string{"label: normalize", "mapping:"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered YAML missing %q\n--- output ---\n%s", want, got)
		}
	}
}

func TestRenderStreamConfig_StripsHTTPBlock(t *testing.T) {
	spec := &rpcv1alpha1.PipelineSpec{
		Input:  rpcv1alpha1.ComponentSpec{Type: "generate", Config: runtime.RawExtension{Raw: []byte(`{"mapping":"root = \"x\"","count":1}`)}},
		Output: rpcv1alpha1.ComponentSpec{Type: "drop", Config: runtime.RawExtension{Raw: []byte(`{}`)}},
	}
	out, err := render.RenderStreamConfig(spec)
	if err != nil {
		t.Fatalf("RenderStreamConfig: %v", err)
	}
	if strings.Contains(out, "http:") {
		t.Errorf("stream config must not contain an http block, got:\n%s", out)
	}
	if !strings.Contains(out, "generate:") || !strings.Contains(out, "drop:") {
		t.Errorf("stream config must contain the pipeline components, got:\n%s", out)
	}

	disp, err := render.RenderPipelineYAMLForDisplay(spec)
	if err != nil {
		t.Fatalf("RenderPipelineYAMLForDisplay: %v", err)
	}
	if disp != out {
		t.Errorf("RenderPipelineYAMLForDisplay should equal RenderStreamConfig output")
	}
}
