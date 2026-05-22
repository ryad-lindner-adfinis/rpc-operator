/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package render

import (
	"encoding/json"
	"fmt"

	"sigs.k8s.io/yaml"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
)

// RenderPipelineYAML produces a Redpanda Connect config from a PipelineSpec.
// The rendered document also enables the HTTP server on :4195 for liveness and
// readiness probes.
func RenderPipelineYAML(spec *rpcv1alpha1.PipelineSpec) (string, error) {
	if spec.RawYAML != "" {
		return injectHTTPConfig(spec.RawYAML)
	}

	inputBlock, err := componentBlock(&spec.Input)
	if err != nil {
		return "", fmt.Errorf("input: %w", err)
	}
	outputBlock, err := componentBlock(&spec.Output)
	if err != nil {
		return "", fmt.Errorf("output: %w", err)
	}
	procBlocks := make([]map[string]any, 0, len(spec.Processors))
	for i := range spec.Processors {
		b, err := componentBlock(&spec.Processors[i])
		if err != nil {
			return "", fmt.Errorf("processors[%d]: %w", i, err)
		}
		procBlocks = append(procBlocks, b)
	}

	doc := map[string]any{
		"input":    inputBlock,
		"pipeline": map[string]any{"processors": procBlocks},
		"output":   outputBlock,
		"http": map[string]any{
			"enabled": true,
			"address": "0.0.0.0:4195",
		},
	}

	out, err := yaml.Marshal(doc)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// RenderStreamConfig produces the config body posted to a cluster instance's
// streams API (PUT /streams/{id}). It is the rendered pipeline minus the http
// server block, because the cluster pod already runs its own HTTP server on that port.
func RenderStreamConfig(spec *rpcv1alpha1.PipelineSpec) (string, error) {
	out, err := RenderPipelineYAML(spec)
	if err != nil {
		return "", err
	}
	return stripHTTPBlock(out)
}

// RenderPipelineYAMLForDisplay produces the user-facing YAML shown in the UI:
// the rendered config minus the operator-injected http server block. It is the
// same output as RenderStreamConfig (delegates to it) — the controller must keep
// using RenderPipelineYAML so the pod retains its liveness/readiness probes.
func RenderPipelineYAMLForDisplay(spec *rpcv1alpha1.PipelineSpec) (string, error) {
	return RenderStreamConfig(spec)
}

// stripHTTPBlock removes the top-level "http" key from a YAML document.
func stripHTTPBlock(yamlText string) (string, error) {
	var raw any
	if err := yaml.Unmarshal([]byte(yamlText), &raw); err != nil {
		return "", fmt.Errorf("invalid YAML: %w", err)
	}
	doc, ok := raw.(map[string]any)
	if !ok || doc == nil {
		return yamlText, nil
	}
	delete(doc, "http")
	out, err := yaml.Marshal(doc)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// injectHTTPConfig parses rawYAML, adds the http server block if absent, and
// re-serializes. This ensures liveness/readiness probes and Prometheus scraping work.
func injectHTTPConfig(rawYAML string) (string, error) {
	var raw any
	if err := yaml.Unmarshal([]byte(rawYAML), &raw); err != nil {
		return "", fmt.Errorf("invalid YAML: %w", err)
	}
	doc, ok := raw.(map[string]any)
	if !ok || doc == nil {
		return "", fmt.Errorf("YAML must be a mapping")
	}
	if _, exists := doc["http"]; !exists {
		doc["http"] = map[string]any{
			"enabled": true,
			"address": "0.0.0.0:4195",
		}
	}
	out, err := yaml.Marshal(doc)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func componentBlock(c *rpcv1alpha1.ComponentSpec) (map[string]any, error) {
	var cfg any = map[string]any{}
	if len(c.Config.Raw) > 0 && string(c.Config.Raw) != "null" {
		if err := json.Unmarshal(c.Config.Raw, &cfg); err != nil {
			return nil, fmt.Errorf("config not valid JSON: %w", err)
		}
	}
	// Recursively convert any embedded ComponentSpec objects/arrays to RPC-native format.
	cfg = renderCompositeFields(cfg)
	result := map[string]any{c.Type: cfg}
	if c.Label != "" {
		result["label"] = c.Label
	}
	return result, nil
}

// renderCompositeFields traverses a config value recursively.
// When it finds a ComponentSpec array ([]any where every element has exactly
// "type" string + optional "config") or a single ComponentSpec map, it converts
// them to RPC-native format {typeName: configValue}.
// This handles Pattern A (named field), Pattern B (direct array), and Pattern C
// (case-structures with nested output/processors fields) without catalog imports.
func renderCompositeFields(v any) any {
	switch val := v.(type) {
	case []any:
		if looksLikeComponentSpecArray(val) {
			return componentSpecArrayToRPC(val)
		}
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = renderCompositeFields(item)
		}
		return out
	case map[string]any:
		if looksLikeComponentSpec(val) {
			return componentSpecToRPCMap(val)
		}
		out := make(map[string]any, len(val))
		for k, fv := range val {
			out[k] = renderCompositeFields(fv)
		}
		return out
	default:
		return v
	}
}

// looksLikeComponentSpecArray returns true when val is a non-empty []any where
// every element passes looksLikeComponentSpec.
func looksLikeComponentSpecArray(val []any) bool {
	if len(val) == 0 {
		return false
	}
	for _, item := range val {
		m, ok := item.(map[string]any)
		if !ok || !looksLikeComponentSpec(m) {
			return false
		}
	}
	return true
}

// looksLikeComponentSpec returns true when m contains exactly the keys "type"
// (non-empty string) and optionally "config" and/or "label" — no other keys allowed.
// This strict signature prevents false-positives on normal config maps.
func looksLikeComponentSpec(m map[string]any) bool {
	t, ok := m["type"].(string)
	if !ok || t == "" {
		return false
	}
	for k := range m {
		if k != "type" && k != "config" && k != "label" {
			return false
		}
	}
	return true
}

func componentSpecArrayToRPC(arr []any) []any {
	out := make([]any, len(arr))
	for i, item := range arr {
		out[i] = componentSpecToRPCMap(item.(map[string]any))
	}
	return out
}

func componentSpecToRPCMap(m map[string]any) map[string]any {
	typeName := m["type"].(string)
	cfg := m["config"]
	if cfg == nil {
		cfg = map[string]any{}
	}
	return map[string]any{typeName: renderCompositeFields(cfg)}
}
