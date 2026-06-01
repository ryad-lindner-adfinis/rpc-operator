/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package render

import (
	"fmt"

	"sigs.k8s.io/yaml"
)

// IncomingRoute mirrors projectroute.IncomingTarget without the import, so the
// render layer stays dependency-free. The controller maps one to the other.
type IncomingRoute struct {
	Subject string
	Durable string
	When    string // "" = no filter
}

// ProjectIOPlan is the resolved rewrite instruction for one pipeline.
type ProjectIOPlan struct {
	NATSURL          string
	OutgoingSubjects []string
	Incoming         []IncomingRoute
}

// ApplyProjectIO rewrites yamlText's input/output/processors per plan and
// returns the new YAML. Producer side replaces `output`; consumer side replaces
// `input` and prepends a predicate processor. An empty plan returns yamlText
// unchanged. The function never reads or needs the project's secrets — secret
// substitution runs afterwards on the rewritten text (F48 ordering preserved).
func ApplyProjectIO(yamlText string, plan ProjectIOPlan) (string, error) {
	if len(plan.OutgoingSubjects) == 0 && len(plan.Incoming) == 0 {
		return yamlText, nil
	}
	var raw any
	if err := yaml.Unmarshal([]byte(yamlText), &raw); err != nil {
		return "", fmt.Errorf("invalid YAML: %w", err)
	}
	doc, ok := raw.(map[string]any)
	if !ok || doc == nil {
		return "", fmt.Errorf("YAML must be a mapping")
	}

	if len(plan.OutgoingSubjects) > 0 {
		doc["output"] = buildOutput(plan.NATSURL, plan.OutgoingSubjects)
	}
	if len(plan.Incoming) > 0 {
		doc["input"] = buildInput(plan.NATSURL, plan.Incoming)
		if proc := buildPredicateProcessor(plan.Incoming); proc != nil {
			prependProcessor(doc, proc)
		}
	}

	out, err := yaml.Marshal(doc)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func natsOutput(url, subject string) map[string]any {
	return map[string]any{"nats_jetstream": map[string]any{
		"urls":    []any{url},
		"subject": subject,
	}}
}

// buildOutput returns a single nats_jetstream output, or a fan_out broker over
// all outgoing subjects when the pipeline is the `from` of several routes.
func buildOutput(url string, subjects []string) map[string]any {
	if len(subjects) == 1 {
		return natsOutput(url, subjects[0])
	}
	outs := make([]any, len(subjects))
	for i, s := range subjects {
		outs[i] = natsOutput(url, s)
	}
	return map[string]any{"broker": map[string]any{
		"pattern": "fan_out",
		"outputs": outs,
	}}
}

func natsInput(url string, in IncomingRoute) map[string]any {
	return map[string]any{"nats_jetstream": map[string]any{
		"urls":    []any{url},
		"subject": in.Subject,
		"durable": in.Durable,
		"queue":   "", // fan-out (independent delivery), not a work queue
	}}
}

// buildInput returns a single nats_jetstream input, or a broker of inputs when
// the pipeline is the target of several routes (fan-in).
func buildInput(url string, incoming []IncomingRoute) map[string]any {
	if len(incoming) == 1 {
		return natsInput(url, incoming[0])
	}
	ins := make([]any, len(incoming))
	for i := range incoming {
		ins[i] = natsInput(url, incoming[i])
	}
	return map[string]any{"broker": map[string]any{"inputs": ins}}
}

// buildPredicateProcessor returns the consumer-side filter, or nil when no
// inbound route has a `when:`. With a single filtered inbound route it emits a
// mapping; with multiple inbound routes (fan-in) and >=1 predicate it emits a
// switch keyed on @nats_subject so each upstream is filtered independently.
func buildPredicateProcessor(incoming []IncomingRoute) map[string]any {
	withWhen := 0
	for i := range incoming {
		if incoming[i].When != "" {
			withWhen++
		}
	}
	if withWhen == 0 {
		return nil
	}
	if len(incoming) == 1 {
		return map[string]any{"mapping": filterMapping(incoming[0].When)}
	}
	// Fan-in: one switch case per filtered subject. Unfiltered subjects match no
	// case and pass through unchanged (Benthos switch default behavior).
	cases := make([]any, 0, withWhen)
	for i := range incoming {
		if incoming[i].When == "" {
			continue
		}
		cases = append(cases, map[string]any{
			"check": fmt.Sprintf("@nats_subject == %q", incoming[i].Subject),
			"processors": []any{
				map[string]any{"mapping": filterMapping(incoming[i].When)},
			},
		})
	}
	return map[string]any{"switch": cases}
}

func filterMapping(when string) string {
	return fmt.Sprintf("root = if !(%s) { deleted() } else { this }", when)
}

// prependProcessor inserts proc at the head of doc.pipeline.processors,
// creating the pipeline/processors structure if absent.
func prependProcessor(doc map[string]any, proc map[string]any) {
	pipe, _ := doc["pipeline"].(map[string]any)
	if pipe == nil {
		pipe = map[string]any{}
		doc["pipeline"] = pipe
	}
	existing, _ := pipe["processors"].([]any)
	pipe["processors"] = append([]any{proc}, existing...)
}
