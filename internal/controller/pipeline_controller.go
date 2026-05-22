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
	"crypto/sha256"
	"encoding/json"
	"fmt"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
	"github.com/insidegreen/rpc-operator-claude/internal/render"
	"github.com/insidegreen/rpc-operator-claude/internal/streams"
)

const (
	finalizerName      = "rpc.operator.io/finalizer"
	specHashAnnotation = "rpc.operator.io/spec-hash"
)

// PipelineReconciler reconciles a Pipeline object.
type PipelineReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	Streams streams.Client
}

// +kubebuilder:rbac:groups=rpc.operator.io,resources=pipelines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rpc.operator.io,resources=pipelines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rpc.operator.io,resources=pipelines/finalizers,verbs=update
// +kubebuilder:rbac:groups=rpc.operator.io,resources=pipelineclusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=core,resources=pods/log,verbs=get
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=podmonitors,verbs=get;list;watch;create;update;patch;delete

// Reconcile drives the Pipeline CR towards its desired state: a ConfigMap
// holding the rendered Redpanda Connect config, and a Pod running the connect
// image with that config mounted.
func (r *PipelineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var pipe rpcv1alpha1.Pipeline
	if err := r.Get(ctx, req.NamespacedName, &pipe); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Deletion path: finalizer cleanup, then exit.
	if !pipe.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&pipe, finalizerName) {
			// OwnerReferences GC the children; nothing external to clean up in v0.1.
			controllerutil.RemoveFinalizer(&pipe, finalizerName)
			if err := r.Update(ctx, &pipe); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer on first sight, then requeue for a fresh fetch.
	if controllerutil.AddFinalizer(&pipe, finalizerName) {
		if err := r.Update(ctx, &pipe); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// F45: stopped pipelines have no pod and stay at phase=Stopped until
	// spec.stopped flips back to false. Short-circuit before render to avoid
	// recreating the pod on every loop.
	if pipe.Spec.Stopped {
		return r.handleStopped(ctx, &pipe)
	}

	// F47: clusterRef pipelines run as a stream inside a PipelineCluster, not a pod.
	if pipe.Spec.ClusterRef != "" {
		return r.handleClusterAssigned(ctx, &pipe)
	}

	yamlStr, err := render.RenderPipelineYAML(&pipe.Spec)
	if err != nil {
		log.Error(err, "render failed")
		return r.markFailed(ctx, &pipe, "RenderError", err.Error())
	}

	secretRefsJSON, _ := json.Marshal(pipe.Spec.SecretRefs)
	newHash := fmt.Sprintf("%x", sha256.Sum256(
		[]byte(yamlStr+"\x00"+pipe.Spec.Image+"\x00"+string(secretRefsJSON)),
	))

	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Name:      pipe.Name + "-config",
		Namespace: pipe.Namespace,
	}}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		cm.Data = map[string]string{configFileName: yamlStr}
		return controllerutil.SetControllerReference(&pipe, cm, r.Scheme)
	}); err != nil {
		return ctrl.Result{}, fmt.Errorf("apply configmap: %w", err)
	}

	// Delete the pod if its spec-hash annotation doesn't match the current render.
	// The next reconcile will recreate it with the updated spec.
	existingPod := &corev1.Pod{}
	if err := r.Get(ctx, client.ObjectKey{Name: pipe.Name, Namespace: pipe.Namespace}, existingPod); err == nil {
		if existingPod.Annotations[specHashAnnotation] != newHash {
			if delErr := r.Delete(ctx, existingPod); client.IgnoreNotFound(delErr) != nil {
				return ctrl.Result{}, fmt.Errorf("delete stale pod: %w", delErr)
			}
			return ctrl.Result{Requeue: true}, nil
		}
	} else if client.IgnoreNotFound(err) != nil {
		return ctrl.Result{}, fmt.Errorf("get pod: %w", err)
	}

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:      pipe.Name,
		Namespace: pipe.Namespace,
	}}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, pod, func() error {
		if pod.CreationTimestamp.IsZero() {
			pod.Spec = buildPodSpec(cm.Name, pipe.Spec.Image, secretRefsToEnvVars(pipe.Spec.SecretRefs))
			pod.Labels = map[string]string{
				"rpc.operator.io/pipeline": pipe.Name,
			}
			pod.Annotations = map[string]string{
				specHashAnnotation: newHash,
			}
		}
		return controllerutil.SetControllerReference(&pipe, pod, r.Scheme)
	}); err != nil {
		return ctrl.Result{}, fmt.Errorf("apply pod: %w", err)
	}

	// PodMonitor: auto-create per-pipeline scrape config.
	// Graceful: if the monitoring CRD is not installed, log and continue.
	pm := &monitoringv1.PodMonitor{ObjectMeta: metav1.ObjectMeta{
		Name:      pipe.Name,
		Namespace: pipe.Namespace,
	}}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, pm, func() error {
		pm.Spec = monitoringv1.PodMonitorSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"rpc.operator.io/pipeline": pipe.Name,
				},
			},
			PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{{
				Port:     ptr.To("http"),
				Path:     "/metrics",
				Interval: monitoringv1.Duration("15s"),
			}},
		}
		return controllerutil.SetControllerReference(&pipe, pm, r.Scheme)
	}); err != nil {
		if apimeta.IsNoMatchError(err) || runtime.IsNotRegisteredError(err) {
			log.V(1).Info("PodMonitor CRD not installed; skipping auto-scrape setup")
		} else {
			return ctrl.Result{}, fmt.Errorf("apply podmonitor: %w", err)
		}
	}

	desired := derivePhase(pod)
	desiredCond := deriveCondition(pod, desired)

	existingCond := apimeta.FindStatusCondition(pipe.Status.Conditions, "Ready")
	condChanged := existingCond == nil ||
		existingCond.Status != desiredCond.Status ||
		existingCond.Reason != desiredCond.Reason ||
		existingCond.Message != desiredCond.Message

	if condChanged || pipe.Status.Phase != desired ||
		pipe.Status.PodName != pod.Name ||
		pipe.Status.ObservedGeneration != pipe.Generation {
		pipe.Status.Phase = desired
		pipe.Status.PodName = pod.Name
		pipe.Status.ObservedGeneration = pipe.Generation
		apimeta.SetStatusCondition(&pipe.Status.Conditions, desiredCond)
		if err := r.Status().Update(ctx, &pipe); err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func derivePhase(pod *corev1.Pod) rpcv1alpha1.PipelinePhase {
	switch pod.Status.Phase {
	case corev1.PodRunning:
		return rpcv1alpha1.PhaseRunning
	case corev1.PodFailed:
		return rpcv1alpha1.PhaseFailed
	case corev1.PodSucceeded:
		return rpcv1alpha1.PhaseStopped
	default:
		return rpcv1alpha1.PhasePending
	}
}

// containerWaitReason returns the first non-empty Waiting.Reason from ContainerStatuses,
// or an empty string if all containers are running/terminated.
func containerWaitReason(pod *corev1.Pod) string {
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
			return cs.State.Waiting.Reason
		}
	}
	return ""
}

// deriveCondition computes the desired Ready condition based on the pod's current phase
// and container wait reason. LastTransitionTime is intentionally not set here —
// apimeta.SetStatusCondition handles it (only updates when Status changes).
func deriveCondition(pod *corev1.Pod, phase rpcv1alpha1.PipelinePhase) metav1.Condition {
	reason := containerWaitReason(pod)
	switch {
	case reason == "ImagePullBackOff" || reason == "ErrImagePull":
		return metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionFalse,
			Reason:  "ImagePullBackOff",
			Message: "Container image cannot be pulled: " + reason,
		}
	case reason == "CrashLoopBackOff":
		return metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionFalse,
			Reason:  "CrashLoopBackOff",
			Message: "Container is crash-looping",
		}
	case phase == rpcv1alpha1.PhaseRunning:
		return metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionTrue,
			Reason:  "Running",
			Message: "Pipeline pod is running",
		}
	case phase == rpcv1alpha1.PhaseStopped:
		return metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionFalse,
			Reason:  "Completed",
			Message: "Pipeline pod has completed",
		}
	case phase == rpcv1alpha1.PhaseFailed:
		return metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionFalse,
			Reason:  "PodFailed",
			Message: "Pipeline pod has failed",
		}
	default:
		return metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionUnknown,
			Reason:  "Pending",
			Message: "Pipeline pod is pending",
		}
	}
}

func (r *PipelineReconciler) markFailed(
	ctx context.Context,
	pipe *rpcv1alpha1.Pipeline,
	reason, msg string,
) (ctrl.Result, error) {
	pipe.Status.Phase = rpcv1alpha1.PhaseFailed
	apimeta.SetStatusCondition(&pipe.Status.Conditions, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: msg,
	})
	if err := r.Status().Update(ctx, pipe); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// handleStopped enforces the spec.stopped=true contract:
// the pipeline pod is deleted if present and the status is set to Stopped.
// ConfigMap and PodMonitor are intentionally left in place so that flipping
// spec.stopped back to false resumes cleanly.
func (r *PipelineReconciler) handleStopped(
	ctx context.Context,
	pipe *rpcv1alpha1.Pipeline,
) (ctrl.Result, error) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:      pipe.Name,
		Namespace: pipe.Namespace,
	}}
	if err := r.Delete(ctx, pod); client.IgnoreNotFound(err) != nil {
		return ctrl.Result{}, fmt.Errorf("delete pod on stop: %w", err)
	}

	desiredCond := metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionFalse,
		Reason:  "StoppedByUser",
		Message: "Pipeline stopped by user (spec.stopped=true)",
	}
	existingCond := apimeta.FindStatusCondition(pipe.Status.Conditions, "Ready")
	condChanged := existingCond == nil ||
		existingCond.Status != desiredCond.Status ||
		existingCond.Reason != desiredCond.Reason

	if condChanged ||
		pipe.Status.Phase != rpcv1alpha1.PhaseStopped ||
		pipe.Status.PodName != "" ||
		pipe.Status.ObservedGeneration != pipe.Generation {
		pipe.Status.Phase = rpcv1alpha1.PhaseStopped
		pipe.Status.PodName = ""
		pipe.Status.ObservedGeneration = pipe.Generation
		apimeta.SetStatusCondition(&pipe.Status.Conditions, desiredCond)
		if err := r.Status().Update(ctx, pipe); err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// secretRefsToEnvVars converts PipelineSpec.SecretRefs into Kubernetes EnvVar
// entries that the kubelet resolves from the referenced Secrets at container start.
func secretRefsToEnvVars(refs []rpcv1alpha1.SecretRef) []corev1.EnvVar {
	if len(refs) == 0 {
		return nil
	}
	vars := make([]corev1.EnvVar, len(refs))
	for i, r := range refs {
		vars[i] = corev1.EnvVar{
			Name: r.EnvVar,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: r.SecretName},
					Key:                  r.Key,
				},
			},
		}
	}
	return vars
}

// SetupWithManager sets up the controller with the Manager.
func (r *PipelineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rpcv1alpha1.Pipeline{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Pod{}).
		Owns(&monitoringv1.PodMonitor{}).
		Named("pipeline").
		Complete(r)
}
