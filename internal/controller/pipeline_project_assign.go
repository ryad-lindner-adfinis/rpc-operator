/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package controller

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
	"github.com/insidegreen/rpc-operator-claude/internal/projectroute"
	"github.com/insidegreen/rpc-operator-claude/internal/render"
)

// handleProjectAssigned places a projectRef pipeline onto the project's managed
// cluster (<project>-cluster) and applies the route-driven I/O rewrite. It
// resolves the project, builds the per-pipeline I/O plan, and delegates to the
// shared cluster-stream deploy path. The Pipeline CR is never mutated.
func (r *PipelineReconciler) handleProjectAssigned(ctx context.Context, pipe *rpcv1alpha1.Pipeline) (ctrl.Result, error) {
	var project rpcv1alpha1.PipelineProject
	if err := r.Get(ctx, client.ObjectKey{Name: pipe.Spec.ProjectRef.Name, Namespace: pipe.Namespace}, &project); err != nil {
		if apierrors.IsNotFound(err) {
			return r.markClusterFailed(ctx, pipe, "ProjectNotFound",
				fmt.Sprintf("PipelineProject %q not found", pipe.Spec.ProjectRef.Name))
		}
		return ctrl.Result{}, err
	}

	plan := projectroute.PlanFor(&project, pipe.Namespace, pipe.Name)
	clusterName := projectChildClusterName(project.Name)

	// Standalone-in-project (shape 1): no route wiring; deploy verbatim on the cluster.
	if plan.IsEmpty() {
		return r.handleClusterAssigned(ctx, pipe, clusterName, nil)
	}

	ioPlan := toRenderPlan(plan)
	return r.handleClusterAssigned(ctx, pipe, clusterName, &ioPlan)
}

// toRenderPlan bridges projectroute.IOPlan to render.ProjectIOPlan (render must
// not import projectroute).
func toRenderPlan(p projectroute.IOPlan) render.ProjectIOPlan {
	out := render.ProjectIOPlan{NATSURL: p.NATSURL, OutgoingSubjects: p.OutgoingSubjects}
	for _, in := range p.Incoming {
		out.Incoming = append(out.Incoming, render.IncomingRoute{
			Subject: in.Subject, Durable: in.Durable, When: in.When,
		})
	}
	return out
}
