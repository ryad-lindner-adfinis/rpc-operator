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

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
)

// PipelineClusterReconciler reconciles a PipelineCluster object.
type PipelineClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=rpc.operator.io,resources=pipelineclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rpc.operator.io,resources=pipelineclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rpc.operator.io,resources=pipelineclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete

// Reconcile drives a PipelineCluster towards its desired state: a ConfigMap
// (connect main config), a headless Service, and a StatefulSet of N streams-mode
// Redpanda Connect instances. Children are owned by the cluster and GC'd on delete.
func (r *PipelineClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var cluster rpcv1alpha1.PipelineCluster
	if err := r.Get(ctx, req.NamespacedName, &cluster); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	cmName := cluster.Name + "-config"
	svcName := cluster.Name

	// ConfigMap: the connect main config (http + logger).
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: cluster.Namespace}}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		cm.Data = map[string]string{clusterConfigFile: clusterConfigYAML(cluster.Spec.JSONLogging)}
		return controllerutil.SetControllerReference(&cluster, cm, r.Scheme)
	}); err != nil {
		return ctrl.Result{}, fmt.Errorf("apply configmap: %w", err)
	}

	// Headless Service.
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: svcName, Namespace: cluster.Namespace}}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		built := buildClusterService(cluster.Name, svcName)
		svc.Spec.ClusterIP = built.Spec.ClusterIP
		svc.Spec.Selector = built.Spec.Selector
		svc.Spec.Ports = built.Spec.Ports
		return controllerutil.SetControllerReference(&cluster, svc, r.Scheme)
	}); err != nil {
		return ctrl.Result{}, fmt.Errorf("apply service: %w", err)
	}

	// StatefulSet.
	ss := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: cluster.Name, Namespace: cluster.Namespace}}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, ss, func() error {
		built := buildClusterStatefulSet(
			cluster.Name, cluster.Spec.Image, cluster.Spec.Replicas,
			cluster.Spec.Resources, cmName, svcName,
		)
		// Selector is immutable after creation; only set it on first apply.
		if ss.CreationTimestamp.IsZero() {
			ss.Spec.Selector = built.Spec.Selector
		}
		ss.Spec.Replicas = built.Spec.Replicas
		ss.Spec.ServiceName = built.Spec.ServiceName
		ss.Spec.Template = built.Spec.Template
		return controllerutil.SetControllerReference(&cluster, ss, r.Scheme)
	}); err != nil {
		return ctrl.Result{}, fmt.Errorf("apply statefulset: %w", err)
	}

	// PodMonitor: scrape the cluster's instances so per-stream metrics are
	// queryable. Graceful: skip if the monitoring CRD is not installed.
	pm := &monitoringv1.PodMonitor{ObjectMeta: metav1.ObjectMeta{Name: cluster.Name, Namespace: cluster.Namespace}}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, pm, func() error {
		pm.Spec = buildClusterPodMonitor(cluster.Name, cluster.Namespace).Spec
		return controllerutil.SetControllerReference(&cluster, pm, r.Scheme)
	}); err != nil {
		if apimeta.IsNoMatchError(err) || runtime.IsNotRegisteredError(err) {
			log.V(1).Info("PodMonitor CRD not installed; skipping cluster auto-scrape setup")
		} else {
			return ctrl.Result{}, fmt.Errorf("apply podmonitor: %w", err)
		}
	}

	// Status: reflect ready replicas + phase + Ready condition.
	ready := ss.Status.ReadyReplicas
	desired := cluster.Spec.Replicas
	phase := rpcv1alpha1.ClusterPhasePending
	if ready >= desired && desired > 0 {
		phase = rpcv1alpha1.ClusterPhaseReady
	} else if ready > 0 {
		phase = rpcv1alpha1.ClusterPhaseDegraded
	}

	desiredCond := metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionFalse,
		Reason:  "ScalingUp",
		Message: fmt.Sprintf("%d/%d instances ready", ready, desired),
	}
	if phase == rpcv1alpha1.ClusterPhaseReady {
		desiredCond.Status = metav1.ConditionTrue
		desiredCond.Reason = "AllReady"
	}

	existingCond := apimeta.FindStatusCondition(cluster.Status.Conditions, "Ready")
	condChanged := existingCond == nil ||
		existingCond.Status != desiredCond.Status ||
		existingCond.Reason != desiredCond.Reason ||
		existingCond.Message != desiredCond.Message

	if condChanged || cluster.Status.Phase != phase ||
		cluster.Status.ReadyReplicas != ready ||
		cluster.Status.ObservedGeneration != cluster.Generation {
		cluster.Status.Phase = phase
		cluster.Status.ReadyReplicas = ready
		cluster.Status.ObservedGeneration = cluster.Generation
		apimeta.SetStatusCondition(&cluster.Status.Conditions, desiredCond)
		if err := r.Status().Update(ctx, &cluster); err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PipelineClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rpcv1alpha1.PipelineCluster{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&monitoringv1.PodMonitor{}).
		Named("pipelinecluster").
		Complete(r)
}
