/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
	"github.com/insidegreen/rpc-operator-claude/internal/nats"
)

// projectFinalizer guards Project deletion until the operator has applied the
// configured PVC reclaim policy. "Retain" leaves PVCs in place; "Delete"
// removes them before the finalizer is cleared.
const projectFinalizer = "rpc.operator.io/pipelineproject"

// PipelineProjectReconciler reconciles a PipelineProject object: it owns a
// child PipelineCluster CR and a NATS JetStream StatefulSet (with Service +
// ConfigMap) in the same namespace. Phase 1 provisions infrastructure only;
// routes are accepted but inert.
type PipelineProjectReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// NATSImage overrides the default NATS server image. Wired via the chart
	// (features.projects.nats.image+tag) and passed in from main.go.
	NATSImage string

	// Streams provisions one JetStream stream per valid route. Wired from main.go.
	Streams nats.StreamManager

	// Recorder emits Events (e.g. an InvalidRoutes warning on a bad route graph).
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=rpc.operator.io,resources=pipelineprojects,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rpc.operator.io,resources=pipelineprojects/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rpc.operator.io,resources=pipelineprojects/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=rpc.operator.io,resources=pipelines,verbs=get;list;watch

// Reconcile drives a PipelineProject towards its desired state.
//
//nolint:gocyclo // Reconcile orchestrates many sequential lifecycle steps; splitting it would obscure the linear flow.
func (r *PipelineProjectReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var project rpcv1alpha1.PipelineProject
	if err := r.Get(ctx, req.NamespacedName, &project); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Deletion path: run the configured reclaim policy, then drop the finalizer.
	if !project.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&project, projectFinalizer) {
			if err := r.cleanupForDelete(ctx, &project); err != nil {
				return ctrl.Result{}, fmt.Errorf("project cleanup: %w", err)
			}
			controllerutil.RemoveFinalizer(&project, projectFinalizer)
			if err := r.Update(ctx, &project); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Ensure the finalizer is present before provisioning so deletion always
	// goes through the cleanup branch above.
	if !controllerutil.ContainsFinalizer(&project, projectFinalizer) {
		controllerutil.AddFinalizer(&project, projectFinalizer)
		if err := r.Update(ctx, &project); err != nil {
			return ctrl.Result{}, err
		}
		// Requeue: the Update bumps resourceVersion; next reconcile owns provisioning.
		return ctrl.Result{Requeue: true}, nil
	}

	// Step 1: Child PipelineCluster CR.
	cluster := &rpcv1alpha1.PipelineCluster{ObjectMeta: metav1.ObjectMeta{
		Name: projectChildClusterName(project.Name), Namespace: project.Namespace,
	}}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, cluster, func() error {
		built := buildProjectCluster(&project)
		cluster.Labels = built.Labels
		cluster.Spec = built.Spec
		return controllerutil.SetControllerReference(&project, cluster, r.Scheme)
	}); err != nil {
		return ctrl.Result{}, fmt.Errorf("apply pipelinecluster: %w", err)
	}

	// Step 2: NATS ConfigMap.
	natsReplicas := projectNATSReplicas(&project)
	natsStorage := projectNATSStorage(&project)

	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Name: projectChildNATSName(project.Name), Namespace: project.Namespace,
	}}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		built := buildProjectNATSConfigMap(project.Name, natsReplicas)
		cm.Labels = built.Labels
		cm.Data = built.Data
		return controllerutil.SetControllerReference(&project, cm, r.Scheme)
	}); err != nil {
		return ctrl.Result{}, fmt.Errorf("apply nats configmap: %w", err)
	}

	// Step 3: NATS headless Service.
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Name: projectChildNATSName(project.Name), Namespace: project.Namespace,
	}}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		built := buildProjectNATSService(project.Name)
		svc.Labels = built.Labels
		svc.Spec.ClusterIP = built.Spec.ClusterIP
		svc.Spec.Selector = built.Spec.Selector
		svc.Spec.Ports = built.Spec.Ports
		return controllerutil.SetControllerReference(&project, svc, r.Scheme)
	}); err != nil {
		return ctrl.Result{}, fmt.Errorf("apply nats service: %w", err)
	}

	// Step 4: NATS StatefulSet.
	ss := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{
		Name: projectChildNATSName(project.Name), Namespace: project.Namespace,
	}}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, ss, func() error {
		built := buildProjectNATSStatefulSet(project.Name, r.NATSImage, natsReplicas, natsStorage)
		ss.Labels = built.Labels
		// Selector + ServiceName + VolumeClaimTemplates are immutable after creation.
		if ss.CreationTimestamp.IsZero() {
			ss.Spec.Selector = built.Spec.Selector
			ss.Spec.ServiceName = built.Spec.ServiceName
			ss.Spec.VolumeClaimTemplates = built.Spec.VolumeClaimTemplates
		}
		ss.Spec.Replicas = built.Spec.Replicas
		ss.Spec.Template = built.Spec.Template
		return controllerutil.SetControllerReference(&project, ss, r.Scheme)
	}); err != nil {
		return ctrl.Result{}, fmt.Errorf("apply nats statefulset: %w", err)
	}

	// Validate the route graph against live pipelines. An invalid graph marks
	// the project Degraded and skips stream provisioning (the operational
	// admission gate).
	routeErr, err := r.validateProjectRoutes(ctx, &project)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("validate routes: %w", err)
	}
	if routeErr != "" && r.Recorder != nil {
		r.Recorder.Event(&project, corev1.EventTypeWarning, "InvalidRoutes", routeErr)
	}

	var routeStatuses []rpcv1alpha1.ProjectRouteStatus
	if routeErr == "" {
		routeStatuses, err = r.reconcileRouteStreams(ctx, &project)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("reconcile route streams: %w", err)
		}
	}

	// Status: derive phase and child readiness from the children we just (re-)applied.
	clusterChild := rpcv1alpha1.ProjectChildStatus{
		Name:  cluster.Name,
		Ready: cluster.Status.ReadyReplicas,
		Total: cluster.Spec.Replicas,
	}
	natsChild := rpcv1alpha1.ProjectChildStatus{
		Name:  ss.Name,
		Ready: ss.Status.ReadyReplicas,
		Total: natsReplicas,
	}

	phase := deriveProjectPhase(clusterChild, natsChild)

	if routeErr != "" {
		phase = rpcv1alpha1.ProjectPhaseDegraded
	}

	routesCond := metav1.Condition{
		Type: "RoutesValid", Status: metav1.ConditionTrue, Reason: "Valid", Message: "all routes valid",
	}
	if routeErr != "" {
		routesCond.Status = metav1.ConditionFalse
		routesCond.Reason = "InvalidRoutes"
		routesCond.Message = routeErr
	}
	existingRoutesCond := apimeta.FindStatusCondition(project.Status.Conditions, "RoutesValid")
	routesCondChanged := existingRoutesCond == nil ||
		existingRoutesCond.Status != routesCond.Status ||
		existingRoutesCond.Message != routesCond.Message

	desiredCond := metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionFalse,
		Reason:  "Provisioning",
		Message: fmt.Sprintf("cluster %d/%d, nats %d/%d", clusterChild.Ready, clusterChild.Total, natsChild.Ready, natsChild.Total),
	}
	switch phase {
	case rpcv1alpha1.ProjectPhaseReady:
		desiredCond.Status = metav1.ConditionTrue
		desiredCond.Reason = "AllReady"
	case rpcv1alpha1.ProjectPhaseDegraded:
		desiredCond.Reason = "ChildDegraded"
	}

	existingCond := apimeta.FindStatusCondition(project.Status.Conditions, "Ready")
	condChanged := existingCond == nil ||
		existingCond.Status != desiredCond.Status ||
		existingCond.Reason != desiredCond.Reason ||
		existingCond.Message != desiredCond.Message

	if condChanged || routesCondChanged ||
		!reflect.DeepEqual(project.Status.Routes, routeStatuses) ||
		project.Status.Phase != phase ||
		project.Status.Cluster != clusterChild ||
		project.Status.NATS != natsChild ||
		project.Status.ObservedGeneration != project.Generation {
		project.Status.Phase = phase
		project.Status.Cluster = clusterChild
		project.Status.NATS = natsChild
		project.Status.ObservedGeneration = project.Generation
		project.Status.Routes = routeStatuses
		apimeta.SetStatusCondition(&project.Status.Conditions, desiredCond)
		apimeta.SetStatusCondition(&project.Status.Conditions, routesCond)
		if err := r.Status().Update(ctx, &project); err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// projectNATSReplicas returns the requested NATS replica count, defaulting to 1.
func projectNATSReplicas(p *rpcv1alpha1.PipelineProject) int32 {
	if p.Spec.NATS == nil || p.Spec.NATS.Replicas == nil {
		return 1
	}
	return *p.Spec.NATS.Replicas
}

// projectNATSStorage returns the requested NATS PVC size, defaulting to natsStorageDefault.
func projectNATSStorage(p *rpcv1alpha1.PipelineProject) resource.Quantity {
	if p.Spec.NATS == nil || p.Spec.NATS.Storage == nil {
		return natsStorageDefault
	}
	return *p.Spec.NATS.Storage
}

// deriveProjectPhase returns the project's phase from the two child states.
// Provisioning: at least one child not yet at its desired ready count.
// Ready:        both children fully ready.
// Note: Reconcile overrides the result to Degraded when the route graph is
// invalid (see the RoutesValid handling); this function only covers child
// readiness.
func deriveProjectPhase(cluster, natsStatus rpcv1alpha1.ProjectChildStatus) rpcv1alpha1.PipelineProjectPhase {
	if cluster.Total > 0 && cluster.Ready >= cluster.Total &&
		natsStatus.Total > 0 && natsStatus.Ready >= natsStatus.Total {
		return rpcv1alpha1.ProjectPhaseReady
	}
	return rpcv1alpha1.ProjectPhaseProvisioning
}

// cleanupForDelete applies the configured PVC reclaim policy. With "Retain"
// (default) the operator does nothing — Kubernetes leaves StatefulSet PVCs
// in place after StatefulSet deletion. With "Delete" the operator removes
// every PVC labelled with this project before clearing the finalizer.
func (r *PipelineProjectReconciler) cleanupForDelete(
	ctx context.Context, project *rpcv1alpha1.PipelineProject,
) error {
	policy := "Retain"
	if project.Spec.NATS != nil && project.Spec.NATS.StorageReclaimPolicy != "" {
		policy = project.Spec.NATS.StorageReclaimPolicy
	}
	if policy != "Delete" {
		return nil
	}

	var pvcs corev1.PersistentVolumeClaimList
	if err := r.List(ctx, &pvcs,
		client.InNamespace(project.Namespace),
		client.MatchingLabels{projectLabelKey: project.Name},
	); err != nil {
		return fmt.Errorf("list pvcs: %w", err)
	}
	for i := range pvcs.Items {
		if err := r.Delete(ctx, &pvcs.Items[i]); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete pvc %s: %w", pvcs.Items[i].Name, err)
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PipelineProjectReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// GetEventRecorder (new events API) returns a different recorder type; migrating
	// Recorder + all Event() call sites is a separate effort.
	r.Recorder = mgr.GetEventRecorderFor("pipelineproject") //nolint:staticcheck // SA1019: events-API migration deferred
	return ctrl.NewControllerManagedBy(mgr).
		For(&rpcv1alpha1.PipelineProject{}).
		Owns(&rpcv1alpha1.PipelineCluster{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		Owns(&appsv1.StatefulSet{}).
		Watches(&rpcv1alpha1.Pipeline{}, handler.EnqueueRequestsFromMapFunc(r.projectsForPipeline)).
		Named("pipelineproject").
		Complete(r)
}

// projectsForPipeline enqueues every PipelineProject in the changed pipeline's
// namespace. A pipeline change (including clearing or removing projectRef) can
// invalidate any project's route graph, and the mapper only sees the new object
// state — so a projectRef→nil transition would be invisible if we keyed off the
// ref. Listing projects in the namespace keeps route status from going stale.
func (r *PipelineProjectReconciler) projectsForPipeline(ctx context.Context, obj client.Object) []reconcile.Request {
	p, ok := obj.(*rpcv1alpha1.Pipeline)
	if !ok {
		return nil
	}
	var projects rpcv1alpha1.PipelineProjectList
	if err := r.List(ctx, &projects, client.InNamespace(p.Namespace)); err != nil {
		return nil
	}
	reqs := make([]reconcile.Request, 0, len(projects.Items))
	for i := range projects.Items {
		reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{
			Name: projects.Items[i].Name, Namespace: projects.Items[i].Namespace,
		}})
	}
	return reqs
}
