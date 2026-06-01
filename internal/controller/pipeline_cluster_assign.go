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
	"time"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
	"github.com/insidegreen/rpc-operator-claude/internal/render"
)

// resyncInterval is how often an assigned (or cluster-not-ready) pipeline is
// re-reconciled so its stream is re-asserted (self-heal) and a pending pipeline
// retries once its cluster becomes ready. F47 Phase 2b.
const resyncInterval = 2 * time.Minute

// clusterPodURL builds the streams-API base URL for one cluster instance via the
// headless service DNS (<pod>.<svc>.<ns>.svc:httpPort). svc name == cluster name.
func clusterPodURL(clusterName, namespace string, ordinal int32) string {
	return fmt.Sprintf("http://%s-%d.%s.%s.svc:%d", clusterName, ordinal, clusterName, namespace, httpPort)
}

// readyClusterOrdinals lists the cluster's pods and returns the ordinals of those
// that are Ready. Not derived from readyReplicas (ready pods need not be contiguous).
func (r *PipelineReconciler) readyClusterOrdinals(ctx context.Context, clusterName, namespace string) ([]int32, error) {
	var pods corev1.PodList
	if err := r.List(ctx, &pods,
		client.InNamespace(namespace),
		client.MatchingLabels{clusterLabelKey: clusterName},
	); err != nil {
		return nil, err
	}
	var ready []int32
	for i := range pods.Items {
		p := &pods.Items[i]
		o, ok := ordinalFromPodName(p.Name, clusterName)
		if !ok {
			continue
		}
		if isPodReady(p) {
			ready = append(ready, o)
		}
	}
	return ready, nil
}

// isPodReady reports whether the pod has a PodReady condition set to True.
func isPodReady(p *corev1.Pod) bool {
	for _, c := range p.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

// loadByOrdinal counts pipelines already placed on each instance of a cluster,
// by listing pipelines that reference it and reading their status.assignedInstance.
func (r *PipelineReconciler) loadByOrdinal(ctx context.Context, clusterName, namespace, excludePipeline string) (map[int32]int, error) {
	var pipes rpcv1alpha1.PipelineList
	if err := r.List(ctx, &pipes, client.InNamespace(namespace)); err != nil {
		return nil, err
	}
	load := map[int32]int{}
	for i := range pipes.Items {
		p := &pipes.Items[i]
		if p.Name == excludePipeline || p.Spec.ClusterRef != clusterName || p.Status.AssignedInstance == "" {
			continue
		}
		if o, ok := ordinalFromPodName(p.Status.AssignedInstance, clusterName); ok {
			load[o]++
		}
	}
	return load, nil
}

// handleClusterAssigned deploys a pipeline as a stream into the named cluster.
// Phase 2a: validation + teardown of pod-mode leftovers + schedule + deploy + placement.
// clusterName is the target PipelineCluster (spec.clusterRef for F47, or the
// project's managed cluster for F50.2). When ioPlan is non-nil, the rendered
// stream config is rewritten per the project's routes before secret
// substitution and PUT /streams. The Pipeline CR is never mutated.
func (r *PipelineReconciler) handleClusterAssigned(
	ctx context.Context, pipe *rpcv1alpha1.Pipeline, clusterName string, ioPlan *render.ProjectIOPlan,
) (ctrl.Result, error) {
	var cluster rpcv1alpha1.PipelineCluster
	if err := r.Get(ctx, client.ObjectKey{Name: clusterName, Namespace: pipe.Namespace}, &cluster); err != nil {
		if apierrors.IsNotFound(err) {
			return r.markClusterFailed(ctx, pipe, "ClusterNotFound",
				fmt.Sprintf("PipelineCluster %q not found", clusterName))
		}
		return ctrl.Result{}, err
	}

	if err := r.deletePodModeChildren(ctx, pipe); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.migrateFromOldCluster(ctx, pipe, cluster.Name); err != nil {
		return ctrl.Result{}, err
	}

	ready, err := r.readyClusterOrdinals(ctx, cluster.Name, pipe.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}
	load, err := r.loadByOrdinal(ctx, cluster.Name, pipe.Namespace, pipe.Name)
	if err != nil {
		return ctrl.Result{}, err
	}
	currentInstance := ""
	if pipe.Status.AssignedCluster == cluster.Name {
		currentInstance = pipe.Status.AssignedInstance
	}
	ordinal, ok := pickInstance(currentInstance, cluster.Name, ready, load)
	if !ok {
		return r.markClusterPending(ctx, pipe, "ClusterNotReady",
			fmt.Sprintf("cluster %q has no ready instances", cluster.Name))
	}

	body, err := render.RenderStreamConfig(&pipe.Spec)
	if err != nil {
		return r.markClusterFailed(ctx, pipe, "RenderError", err.Error())
	}
	if ioPlan != nil {
		body, err = render.ApplyProjectIO(body, *ioPlan)
		if err != nil {
			return r.markClusterFailed(ctx, pipe, "RewriteError", err.Error())
		}
	}
	if len(pipe.Spec.SecretRefs) > 0 {
		values, err := fetchSecretValues(ctx, r.Client, pipe.Namespace, pipe.Spec.SecretRefs)
		if err != nil {
			return r.markClusterFailed(ctx, pipe, "SecretNotFound", err.Error())
		}
		body = substituteSecrets(body, pipe.Spec.SecretRefs, values)
	}
	podURL := clusterPodURL(cluster.Name, pipe.Namespace, ordinal)
	if err := r.Streams.EnsureStream(ctx, podURL, pipe.Name, body); err != nil {
		return ctrl.Result{}, fmt.Errorf("ensure stream: %w", err)
	}

	instance := fmt.Sprintf("%s-%d", cluster.Name, ordinal)
	reason, msg := "Assigned", fmt.Sprintf("stream running on %s", instance)
	if pipe.Status.AssignedCluster == cluster.Name && pipe.Status.AssignedInstance != "" && pipe.Status.AssignedInstance != instance {
		reason = "Rescheduling"
		msg = fmt.Sprintf("rescheduled from %s to %s", pipe.Status.AssignedInstance, instance)
	}
	cond := metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: reason, Message: msg}
	return r.writeClusterStatus(ctx, pipe, rpcv1alpha1.PhaseRunning, cluster.Name, instance, pipe.Name, cond, resyncInterval)
}

// deleteAssignedStream deletes the pipeline's stream on its currently assigned
// cluster instance, if any. No-op when the pipeline holds no placement.
// Idempotent: DeleteStream treats a missing stream (404) as success. F47 Phase 2b.
func (r *PipelineReconciler) deleteAssignedStream(ctx context.Context, pipe *rpcv1alpha1.Pipeline) error {
	if pipe.Status.AssignedCluster == "" || pipe.Status.AssignedInstance == "" {
		return nil
	}
	ord, ok := ordinalFromPodName(pipe.Status.AssignedInstance, pipe.Status.AssignedCluster)
	if !ok {
		return nil
	}
	if err := r.Streams.DeleteStream(ctx, clusterPodURL(pipe.Status.AssignedCluster, pipe.Namespace, ord), pipe.Name); err != nil {
		return fmt.Errorf("delete assigned stream: %w", err)
	}
	return nil
}

// handleClusterFallback runs when spec.clusterRef has been cleared but the
// pipeline still holds a stream placement. It deletes the stream on its old
// instance, clears placement, and requeues so the next reconcile falls through
// to the normal single-pod path. F47 Phase 2b.
func (r *PipelineReconciler) handleClusterFallback(ctx context.Context, pipe *rpcv1alpha1.Pipeline) (ctrl.Result, error) {
	if err := r.deleteAssignedStream(ctx, pipe); err != nil {
		return ctrl.Result{}, err
	}
	pipe.Status.AssignedCluster = ""
	pipe.Status.AssignedInstance = ""
	pipe.Status.StreamID = ""
	if err := r.Status().Update(ctx, pipe); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}
	return ctrl.Result{Requeue: true}, nil
}

// migrateFromOldCluster deletes the pipeline's stream on a previously assigned
// cluster when spec.clusterRef now points at a different cluster. Idempotent:
// DeleteStream treats a missing stream (404) as success. F47 Phase 2b.
func (r *PipelineReconciler) migrateFromOldCluster(ctx context.Context, pipe *rpcv1alpha1.Pipeline, newCluster string) error {
	old := pipe.Status.AssignedCluster
	if old == "" || old == newCluster || pipe.Status.AssignedInstance == "" {
		return nil
	}
	ord, ok := ordinalFromPodName(pipe.Status.AssignedInstance, old)
	if !ok {
		return nil
	}
	if err := r.Streams.DeleteStream(ctx, clusterPodURL(old, pipe.Namespace, ord), pipe.Name); err != nil {
		return fmt.Errorf("delete stream on old cluster %s: %w", old, err)
	}
	return nil
}

// deletePodModeChildren removes the Pod, -config ConfigMap, and PodMonitor a
// pipeline may have had in single-pod mode. All deletes tolerate NotFound.
func (r *PipelineReconciler) deletePodModeChildren(ctx context.Context, pipe *rpcv1alpha1.Pipeline) error {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: pipe.Name, Namespace: pipe.Namespace}}
	if err := r.Delete(ctx, pod); client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("delete pod: %w", err)
	}
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: pipe.Name + "-config", Namespace: pipe.Namespace}}
	if err := r.Delete(ctx, cm); client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("delete configmap: %w", err)
	}
	pm := &monitoringv1.PodMonitor{ObjectMeta: metav1.ObjectMeta{Name: pipe.Name, Namespace: pipe.Namespace}}
	if err := r.Delete(ctx, pm); client.IgnoreNotFound(err) != nil && !apimeta.IsNoMatchError(err) && !runtime.IsNotRegisteredError(err) {
		return fmt.Errorf("delete podmonitor: %w", err)
	}
	return nil
}

func (r *PipelineReconciler) markClusterFailed(ctx context.Context, pipe *rpcv1alpha1.Pipeline, reason, msg string) (ctrl.Result, error) {
	cond := metav1.Condition{Type: "Ready", Status: metav1.ConditionFalse, Reason: reason, Message: msg}
	return r.writeClusterStatus(ctx, pipe, rpcv1alpha1.PhaseFailed, "", "", "", cond, 0)
}

func (r *PipelineReconciler) markClusterPending(ctx context.Context, pipe *rpcv1alpha1.Pipeline, reason, msg string) (ctrl.Result, error) {
	cond := metav1.Condition{Type: "Ready", Status: metav1.ConditionFalse, Reason: reason, Message: msg}
	return r.writeClusterStatus(ctx, pipe, rpcv1alpha1.PhasePending, "", "", "", cond, resyncInterval)
}

// writeClusterStatus updates placement + phase + Ready condition only when changed.
func (r *PipelineReconciler) writeClusterStatus(
	ctx context.Context, pipe *rpcv1alpha1.Pipeline,
	phase rpcv1alpha1.PipelinePhase, assignedCluster, assignedInstance, streamID string,
	cond metav1.Condition, requeueAfter time.Duration,
) (ctrl.Result, error) {
	existing := apimeta.FindStatusCondition(pipe.Status.Conditions, "Ready")
	condChanged := existing == nil || existing.Status != cond.Status || existing.Reason != cond.Reason || existing.Message != cond.Message
	if condChanged ||
		pipe.Status.Phase != phase ||
		pipe.Status.AssignedCluster != assignedCluster ||
		pipe.Status.AssignedInstance != assignedInstance ||
		pipe.Status.StreamID != streamID ||
		pipe.Status.PodName != "" ||
		pipe.Status.ObservedGeneration != pipe.Generation {
		pipe.Status.Phase = phase
		pipe.Status.AssignedCluster = assignedCluster
		pipe.Status.AssignedInstance = assignedInstance
		pipe.Status.StreamID = streamID
		pipe.Status.PodName = ""
		pipe.Status.ObservedGeneration = pipe.Generation
		apimeta.SetStatusCondition(&pipe.Status.Conditions, cond)
		if err := r.Status().Update(ctx, pipe); err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}
