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

// BuildCacheResourceConfig renders the YAML body posted to a cluster instance's
// Resources API (POST /resources/cache/{label}). For the managed natsKV variant
// it builds a nats_kv cache block pointing at the project's KV bucket. For the
// custom variant it converts the raw config block to YAML verbatim. The label is
// carried in the URL, not the body.
func BuildCacheResourceConfig(cr rpcv1alpha1.ProjectCacheResource, natsURL, bucket string) (string, error) {
	if cr.NatsKV != nil {
		doc := map[string]any{"nats_kv": map[string]any{
			"urls":   []any{natsURL},
			"bucket": bucket,
		}}
		out, err := yaml.Marshal(doc)
		if err != nil {
			return "", err
		}
		return string(out), nil
	}

	if len(cr.Config.Raw) == 0 || string(cr.Config.Raw) == "null" {
		return "", fmt.Errorf("cache resource %q: neither natsKV nor config set", cr.Name)
	}
	var raw any
	if err := json.Unmarshal(cr.Config.Raw, &raw); err != nil {
		return "", fmt.Errorf("cache resource %q: config not valid JSON: %w", cr.Name, err)
	}
	out, err := yaml.Marshal(raw)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
