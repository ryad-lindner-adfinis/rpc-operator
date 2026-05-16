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

func TestRenderPipelineYAMLForDisplay_StripsHTTPBlock(t *testing.T) {
	spec := &rpcv1alpha1.PipelineSpec{
		Input: rpcv1alpha1.ComponentSpec{
			Type:   "generate",
			Config: runtime.RawExtension{Raw: []byte(`{"mapping":"root = \"hi\"","count":1}`)},
		},
		Output: rpcv1alpha1.ComponentSpec{Type: "stdout"},
	}

	got, err := render.RenderPipelineYAMLForDisplay(spec)
	if err != nil {
		t.Fatalf("RenderPipelineYAMLForDisplay: %v", err)
	}

	if strings.Contains(got, "http:") {
		t.Errorf("display YAML must not contain http block\n--- output ---\n%s", got)
	}
	if strings.Contains(got, "4195") {
		t.Errorf("display YAML must not contain probe port 4195\n--- output ---\n%s", got)
	}
	for _, want := range []string{"input:", "generate:", "output:", "stdout:"} {
		if !strings.Contains(got, want) {
			t.Errorf("display YAML missing %q\n--- output ---\n%s", want, got)
		}
	}
}

func TestRenderPipelineYAMLForDisplay_RawYAMLStripsHTTP(t *testing.T) {
	spec := &rpcv1alpha1.PipelineSpec{
		RawYAML: "input:\n  stdin: {}\noutput:\n  stdout: {}\n",
	}

	got, err := render.RenderPipelineYAMLForDisplay(spec)
	if err != nil {
		t.Fatalf("RenderPipelineYAMLForDisplay: %v", err)
	}

	if strings.Contains(got, "http:") {
		t.Errorf("display YAML must not contain http block\n--- output ---\n%s", got)
	}
	if !strings.Contains(got, "stdin:") || !strings.Contains(got, "stdout:") {
		t.Errorf("display YAML missing user content\n--- output ---\n%s", got)
	}
}

func TestRenderPipelineYAMLForDisplay_Idempotent(t *testing.T) {
	spec := &rpcv1alpha1.PipelineSpec{
		Input:  rpcv1alpha1.ComponentSpec{Type: "stdin"},
		Output: rpcv1alpha1.ComponentSpec{Type: "stdout"},
	}

	first, err := render.RenderPipelineYAMLForDisplay(spec)
	if err != nil {
		t.Fatalf("first render: %v", err)
	}
	second, err := render.RenderPipelineYAMLForDisplay(spec)
	if err != nil {
		t.Fatalf("second render: %v", err)
	}
	if first != second {
		t.Errorf("display render is not deterministic\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}
