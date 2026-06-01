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

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
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
