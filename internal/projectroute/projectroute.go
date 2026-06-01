/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package projectroute holds the pure topology logic for F50 PipelineProjects:
// the operator-managed NATS naming convention, per-pipeline role computation,
// route-graph validation, and the I/O rewrite plan handed to the render layer.
// It has no Kubernetes client and never mutates its inputs.
package projectroute

import (
	"fmt"
	"sort"
	"strings"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
	"github.com/redpanda-data/benthos/v4/public/bloblang"
)

// StreamName is the JetStream stream backing one route: rpc-<project>-<route>.
func StreamName(project, route string) string {
	return fmt.Sprintf("rpc-%s-%s", project, route)
}

// Subject is the NATS subject for one route: rpc.<project>.<route>.
func Subject(project, route string) string {
	return fmt.Sprintf("rpc.%s.%s", project, route)
}

// DurableName is the per-(route,target) JetStream durable consumer name:
// <project>-<route>-<pipeline>. Unique so each target acks independently.
func DurableName(project, route, pipeline string) string {
	return fmt.Sprintf("%s-%s-%s", project, route, pipeline)
}

// NATSURL is the in-cluster client URL of the project's NATS StatefulSet,
// reachable via its headless Service DNS from any namespace.
func NATSURL(project, namespace string) string {
	return fmt.Sprintf("nats://%s-nats.%s.svc:4222", project, namespace)
}

// IsProducer reports whether pipeline is the `from` of at least one route.
func IsProducer(routes []rpcv1alpha1.ProjectRoute, pipeline string) bool {
	for i := range routes {
		if routes[i].From == pipeline {
			return true
		}
	}
	return false
}

// IsConsumer reports whether pipeline is a `to[].pipeline` of at least one route.
func IsConsumer(routes []rpcv1alpha1.ProjectRoute, pipeline string) bool {
	for i := range routes {
		for j := range routes[i].To {
			if routes[i].To[j].Pipeline == pipeline {
				return true
			}
		}
	}
	return false
}

// Role classifies a pipeline within a project per the spec's shape table.
type Role string

const (
	RoleStandalone Role = "standalone" // in no route
	RoleSource     Role = "source"     // producer only
	RoleMiddle     Role = "middle"     // producer and consumer
	RoleSink       Role = "sink"       // consumer only
)

// RoleOf returns the pipeline's computed role from the route table.
func RoleOf(routes []rpcv1alpha1.ProjectRoute, pipeline string) Role {
	p, c := IsProducer(routes, pipeline), IsConsumer(routes, pipeline)
	switch {
	case p && c:
		return RoleMiddle
	case p:
		return RoleSource
	case c:
		return RoleSink
	default:
		return RoleStandalone
	}
}

// IncomingTarget is one inbound route for a consumer pipeline.
type IncomingTarget struct {
	Route   string
	Subject string
	Durable string
	When    string // Bloblang predicate; "" = always deliver
}

// IOPlan is the resolved rewrite instruction for a single pipeline. Empty
// OutgoingSubjects + empty Incoming means standalone (no rewrite).
type IOPlan struct {
	NATSURL          string
	OutgoingSubjects []string         // producer side, one per outgoing route
	Incoming         []IncomingTarget // consumer side, one per inbound route
}

// PlanFor builds the IOPlan for pipeline within project (project already
// resolved; namespace drives the NATS URL). Order follows route declaration
// order so renders are deterministic.
func PlanFor(project *rpcv1alpha1.PipelineProject, namespace, pipeline string) IOPlan {
	plan := IOPlan{NATSURL: NATSURL(project.Name, namespace)}
	for i := range project.Spec.Routes {
		r := &project.Spec.Routes[i]
		if r.From == pipeline {
			plan.OutgoingSubjects = append(plan.OutgoingSubjects, Subject(project.Name, r.Name))
		}
		for j := range r.To {
			if r.To[j].Pipeline == pipeline {
				plan.Incoming = append(plan.Incoming, IncomingTarget{
					Route:   r.Name,
					Subject: Subject(project.Name, r.Name),
					Durable: DurableName(project.Name, r.Name, pipeline),
					When:    r.To[j].When,
				})
			}
		}
	}
	return plan
}

// IsEmpty reports whether the plan requires no rewriting (standalone pipeline).
func (p IOPlan) IsEmpty() bool {
	return len(p.OutgoingSubjects) == 0 && len(p.Incoming) == 0
}

// ProjectError is a single validation failure with the spec's exact message.
type ProjectError struct {
	Route   string // route name, or "" for project-level errors
	Message string
}

func (e ProjectError) Error() string { return e.Message }

// PipelineView is the minimal pipeline shape ValidateProject needs: its name,
// the project it claims (spec.projectRef.name, "" if none), and whether it
// defines user input/output (for the I/O-conflict rule).
type PipelineView struct {
	Name        string
	ProjectName string
	HasInput    bool
	HasOutput   bool
}

// ValidateProject checks a project's route table against the live pipelines in
// its namespace. pipelines is keyed by pipeline name. Returns every violation
// (not just the first) so callers can surface a complete picture. Messages are
// verbatim from the spec's validation table.
func ValidateProject(project *rpcv1alpha1.PipelineProject, pipelines map[string]PipelineView) []ProjectError {
	var errs []ProjectError
	routes := project.Spec.Routes

	// Rule: route name uniqueness.
	seen := map[string]bool{}
	for i := range routes {
		n := routes[i].Name
		if seen[n] {
			errs = append(errs, ProjectError{Route: n, Message: fmt.Sprintf("route name %q is not unique", n)})
		}
		seen[n] = true
	}

	inProject := func(name string) bool {
		pv, ok := pipelines[name]
		return ok && pv.ProjectName == project.Name
	}

	for i := range routes {
		r := &routes[i]

		// Rule: from must reference a pipeline in this project.
		if r.From == "" || !inProject(r.From) {
			errs = append(errs, ProjectError{Route: r.Name,
				Message: fmt.Sprintf("route '%s' from='%s': pipeline not found in project", r.Name, r.From)})
		}
		// Rule: each to[i] must reference a pipeline in this project.
		for j := range r.To {
			t := &r.To[j]
			if t.Pipeline == "" || !inProject(t.Pipeline) {
				errs = append(errs, ProjectError{Route: r.Name,
					Message: fmt.Sprintf("route '%s' to[%d]='%s': pipeline not found in project", r.Name, j, t.Pipeline)})
			}
			// Rule: predicate must parse as Bloblang.
			if t.When != "" {
				if err := parsePredicate(t.When); err != nil {
					errs = append(errs, ProjectError{Route: r.Name,
						Message: fmt.Sprintf("route '%s' to[%d].when: %s", r.Name, j, err.Error())})
				}
			}
		}
	}

	// Rule: I/O conflict. A producer must not define user output; a consumer
	// must not define user input.
	for name, pv := range pipelines {
		if pv.ProjectName != project.Name {
			continue
		}
		if IsProducer(routes, name) && pv.HasOutput {
			errs = append(errs, ProjectError{
				Message: "output is managed by the project's routes; remove it"})
		}
		if IsConsumer(routes, name) && pv.HasInput {
			errs = append(errs, ProjectError{
				Message: "input is managed by the project's routes; remove it"})
		}
	}

	// Rule: no cycles.
	if cyc := findCycle(routes); len(cyc) > 0 {
		errs = append(errs, ProjectError{
			Message: fmt.Sprintf("route graph contains a cycle: %s", strings.Join(cyc, " → "))})
	}

	// Stable order for deterministic status/tests.
	sort.SliceStable(errs, func(a, b int) bool { return errs[a].Message < errs[b].Message })
	return errs
}

// parsePredicate wraps the predicate in the mapping the operator will generate
// and parses the whole thing, so syntax errors surface exactly as they would in
// the deployed pipeline.
func parsePredicate(when string) error {
	_, err := bloblang.Parse(fmt.Sprintf("root = if !(%s) { deleted() } else { this }", when))
	return err
}

// findCycle returns the node sequence of the first cycle found (e.g. ["A","B","A"]),
// or nil. Edges run from route.From to each route.To[].Pipeline. DFS with a
// recursion stack; deterministic via sorted node iteration.
func findCycle(routes []rpcv1alpha1.ProjectRoute) []string {
	adj := map[string][]string{}
	nodeSet := map[string]bool{}
	for i := range routes {
		f := routes[i].From
		nodeSet[f] = true
		for j := range routes[i].To {
			to := routes[i].To[j].Pipeline
			adj[f] = append(adj[f], to)
			nodeSet[to] = true
		}
	}
	nodes := make([]string, 0, len(nodeSet))
	for n := range nodeSet {
		nodes = append(nodes, n)
	}
	sort.Strings(nodes)
	for k := range adj {
		sort.Strings(adj[k])
	}

	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[string]int{}
	var path []string
	var dfs func(n string) []string
	dfs = func(n string) []string {
		color[n] = gray
		path = append(path, n)
		for _, m := range adj[n] {
			switch color[m] {
			case gray:
				for idx := range path {
					if path[idx] == m {
						return append(append([]string{}, path[idx:]...), m)
					}
				}
			case white:
				if c := dfs(m); c != nil {
					return c
				}
			}
		}
		path = path[:len(path)-1]
		color[n] = black
		return nil
	}
	for _, n := range nodes {
		if color[n] == white {
			if c := dfs(n); c != nil {
				return c
			}
		}
	}
	return nil
}

// RouteStatuses builds the status.routes slice for a project, marking each route
// with its subject + stream (controllers overwrite Phase with provisioning results).
func RouteStatuses(project *rpcv1alpha1.PipelineProject) []rpcv1alpha1.ProjectRouteStatus {
	out := make([]rpcv1alpha1.ProjectRouteStatus, 0, len(project.Spec.Routes))
	for i := range project.Spec.Routes {
		r := &project.Spec.Routes[i]
		out = append(out, rpcv1alpha1.ProjectRouteStatus{
			Name:    r.Name,
			Subject: Subject(project.Name, r.Name),
			Stream:  StreamName(project.Name, r.Name),
		})
	}
	return out
}
