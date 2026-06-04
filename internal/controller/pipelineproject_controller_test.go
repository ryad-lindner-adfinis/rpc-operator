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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
	"github.com/insidegreen/rpc-operator-claude/internal/nats"
	"github.com/insidegreen/rpc-operator-claude/internal/projectroute"
)

var _ = Describe("PipelineProject Controller", func() {
	const (
		projectName = "orders"
		namespace   = "default"
	)

	var (
		ctx     = context.Background()
		nn      = types.NamespacedName{Name: projectName, Namespace: namespace}
		r       *PipelineProjectReconciler
		project *rpcv1alpha1.PipelineProject
	)

	BeforeEach(func() {
		r = &PipelineProjectReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}

		project = &rpcv1alpha1.PipelineProject{
			ObjectMeta: metav1.ObjectMeta{Name: projectName, Namespace: namespace},
			Spec: rpcv1alpha1.PipelineProjectSpec{
				Cluster: &rpcv1alpha1.ProjectClusterSpec{
					Instances: ptr.To[int32](2),
				},
				NATS: &rpcv1alpha1.ProjectNATSSpec{
					Replicas: ptr.To[int32](1),
					Storage:  new(resource.MustParse("1Gi")),
				},
			},
		}
		Expect(k8sClient.Create(ctx, project)).To(Succeed())
	})

	AfterEach(func() {
		pp := &rpcv1alpha1.PipelineProject{}
		if err := k8sClient.Get(ctx, nn, pp); err == nil {
			// Drop finalizer so the object is not stuck in terminating state.
			if controllerutil.ContainsFinalizer(pp, projectFinalizer) {
				controllerutil.RemoveFinalizer(pp, projectFinalizer)
				_ = k8sClient.Update(ctx, pp)
			}
			_ = k8sClient.Delete(ctx, pp)
		}
		_ = k8sClient.Delete(ctx, &rpcv1alpha1.PipelineCluster{
			ObjectMeta: metav1.ObjectMeta{Name: projectName + "-cluster", Namespace: namespace},
		})
		_ = k8sClient.Delete(ctx, &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: projectName + "-nats", Namespace: namespace},
		})
		_ = k8sClient.Delete(ctx, &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: projectName + "-nats", Namespace: namespace},
		})
		_ = k8sClient.Delete(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: projectName + "-nats", Namespace: namespace},
		})
	})

	It("creates child PipelineCluster, NATS ConfigMap, Service, and StatefulSet with owner refs", func() {
		// First reconcile adds the finalizer and requeues; second does the work.
		res, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		Expect(res.Requeue).To(BeTrue()) //nolint:staticcheck // SA1019: asserts the reconciler's Requeue:true path (same deprecation as production)
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		By("creating a child PipelineCluster owned by the project")
		pc := &rpcv1alpha1.PipelineCluster{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name: projectName + "-cluster", Namespace: namespace,
		}, pc)).To(Succeed())
		Expect(pc.Spec.Replicas).To(Equal(int32(2)))
		Expect(pc.Spec.JSONLogging).To(BeTrue())
		Expect(pc.OwnerReferences).To(HaveLen(1))
		Expect(pc.OwnerReferences[0].Kind).To(Equal("PipelineProject"))

		By("creating a NATS ConfigMap with the rendered server config")
		cm := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name: projectName + "-nats", Namespace: namespace,
		}, cm)).To(Succeed())
		Expect(cm.Data).To(HaveKey("nats-server.conf"))
		Expect(cm.Data["nats-server.conf"]).To(ContainSubstring("jetstream"))
		Expect(cm.OwnerReferences).To(HaveLen(1))

		By("creating a headless NATS Service")
		svc := &corev1.Service{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name: projectName + "-nats", Namespace: namespace,
		}, svc)).To(Succeed())
		Expect(svc.Spec.ClusterIP).To(Equal("None"))
		Expect(svc.OwnerReferences).To(HaveLen(1))

		By("creating a NATS StatefulSet with the requested storage")
		ss := &appsv1.StatefulSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name: projectName + "-nats", Namespace: namespace,
		}, ss)).To(Succeed())
		Expect(*ss.Spec.Replicas).To(Equal(int32(1)))
		Expect(ss.Spec.VolumeClaimTemplates).To(HaveLen(1))
		Expect(ss.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests).To(HaveKey(corev1.ResourceStorage))
		got := ss.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests[corev1.ResourceStorage]
		Expect(got.String()).To(Equal("1Gi"))
		Expect(ss.OwnerReferences).To(HaveLen(1))
	})

	It("is idempotent on repeated reconcile", func() {
		// Establish the finalizer first.
		res, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		Expect(res.Requeue).To(BeTrue()) //nolint:staticcheck // SA1019: asserts the reconciler's Requeue:true path (same deprecation as production)
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		ss := &appsv1.StatefulSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name: projectName + "-nats", Namespace: namespace,
		}, ss)).To(Succeed())
		firstRV := ss.ResourceVersion

		// Third reconcile must not churn the StatefulSet.
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		ss2 := &appsv1.StatefulSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name: projectName + "-nats", Namespace: namespace,
		}, ss2)).To(Succeed())
		Expect(ss2.ResourceVersion).To(Equal(firstRV),
			"StatefulSet must not be updated on no-op reconcile")
	})

	It("reports Phase=Provisioning when children are not yet ready", func() {
		// First reconcile adds the finalizer and requeues; second does the work.
		res, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		Expect(res.Requeue).To(BeTrue()) //nolint:staticcheck // SA1019: asserts the reconciler's Requeue:true path (same deprecation as production)
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		pp := &rpcv1alpha1.PipelineProject{}
		Expect(k8sClient.Get(ctx, nn, pp)).To(Succeed())

		// envtest has no kubelet, so the NATS StatefulSet never reports ready replicas.
		Expect(pp.Status.Phase).To(Equal(rpcv1alpha1.ProjectPhaseProvisioning))
		Expect(pp.Status.Cluster.Total).To(Equal(int32(2)))
		Expect(pp.Status.NATS.Total).To(Equal(int32(1)))
		Expect(pp.Status.ObservedGeneration).To(Equal(pp.Generation))

		cond := apimeta.FindStatusCondition(pp.Status.Conditions, "Ready")
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal("Provisioning"))
	})

	It("provisions a stream per route and reports RoutesValid", func() {
		fake := nats.NewFakeManager()
		rr := &PipelineProjectReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Streams: fake}

		for _, n := range []string{"ingest", "sink"} {
			Expect(k8sClient.Create(ctx, &rpcv1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{Name: n, Namespace: namespace},
				Spec:       rpcv1alpha1.PipelineSpec{ProjectRef: &rpcv1alpha1.ProjectRef{Name: projectName}},
			})).To(Succeed())
		}
		DeferCleanup(func() {
			for _, n := range []string{"ingest", "sink"} {
				_ = k8sClient.Delete(ctx, &rpcv1alpha1.Pipeline{
					ObjectMeta: metav1.ObjectMeta{Name: n, Namespace: namespace},
				})
			}
		})

		pp := &rpcv1alpha1.PipelineProject{}
		Expect(k8sClient.Get(ctx, nn, pp)).To(Succeed())
		pp.Spec.Routes = []rpcv1alpha1.ProjectRoute{
			{Name: "fan-out", From: "ingest", To: []rpcv1alpha1.ProjectRouteTarget{{Pipeline: "sink"}}},
		}
		Expect(k8sClient.Update(ctx, pp)).To(Succeed())

		_, _ = rr.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		_, err := rr.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		url := projectroute.NATSURL(projectName, namespace)
		Expect(fake.Has(url, projectroute.StreamName(projectName, "fan-out"))).To(BeTrue())

		Expect(k8sClient.Get(ctx, nn, pp)).To(Succeed())
		Expect(apimeta.IsStatusConditionTrue(pp.Status.Conditions, "RoutesValid")).To(BeTrue())
		Expect(pp.Status.Routes).To(HaveLen(1))
	})

	It("marks the project Degraded and skips provisioning on an invalid route graph", func() {
		fake := nats.NewFakeManager()
		rr := &PipelineProjectReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Streams: fake}

		// Route references a pipeline that does not exist in the project.
		pp := &rpcv1alpha1.PipelineProject{}
		Expect(k8sClient.Get(ctx, nn, pp)).To(Succeed())
		pp.Spec.Routes = []rpcv1alpha1.ProjectRoute{
			{Name: "bad", From: "ghost", To: []rpcv1alpha1.ProjectRouteTarget{{Pipeline: "void"}}},
		}
		Expect(k8sClient.Update(ctx, pp)).To(Succeed())

		_, _ = rr.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		_, err := rr.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, nn, pp)).To(Succeed())
		Expect(pp.Status.Phase).To(Equal(rpcv1alpha1.ProjectPhaseDegraded))
		cond := apimeta.FindStatusCondition(pp.Status.Conditions, "RoutesValid")
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Message).To(Equal("route 'bad' from='ghost': pipeline not found in project"))

		// No stream should have been provisioned for an invalid graph.
		url := projectroute.NATSURL(projectName, namespace)
		Expect(fake.Has(url, projectroute.StreamName(projectName, "bad"))).To(BeFalse())
	})
})

var _ = Describe("PipelineProject Controller — PVC reclaim", func() {
	const (
		projectName = "billing"
		namespace   = "default"
	)

	var (
		ctx = context.Background()
		nn  = types.NamespacedName{Name: projectName, Namespace: namespace}
		r   *PipelineProjectReconciler
	)

	BeforeEach(func() {
		r = &PipelineProjectReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

		project := &rpcv1alpha1.PipelineProject{
			ObjectMeta: metav1.ObjectMeta{Name: projectName, Namespace: namespace},
			Spec: rpcv1alpha1.PipelineProjectSpec{
				NATS: &rpcv1alpha1.ProjectNATSSpec{
					Replicas:             ptr.To[int32](1),
					Storage:              new(resource.MustParse("1Gi")),
					StorageReclaimPolicy: "Delete",
				},
			},
		}
		Expect(k8sClient.Create(ctx, project)).To(Succeed())
	})

	AfterEach(func() {
		pp := &rpcv1alpha1.PipelineProject{}
		_ = k8sClient.Get(ctx, nn, pp)
		// Drop finalizer if the test left it on.
		if controllerutil.ContainsFinalizer(pp, "rpc.operator.io/pipelineproject") {
			controllerutil.RemoveFinalizer(pp, "rpc.operator.io/pipelineproject")
			_ = k8sClient.Update(ctx, pp)
		}
		_ = k8sClient.Delete(ctx, pp)
		_ = k8sClient.Delete(ctx, &rpcv1alpha1.PipelineCluster{
			ObjectMeta: metav1.ObjectMeta{Name: projectName + "-cluster", Namespace: namespace},
		})
		_ = k8sClient.Delete(ctx, &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: projectName + "-nats", Namespace: namespace},
		})
		_ = k8sClient.Delete(ctx, &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: projectName + "-nats", Namespace: namespace},
		})
		_ = k8sClient.Delete(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: projectName + "-nats", Namespace: namespace},
		})
	})

	It("deletes labelled PVCs on delete when reclaim policy is Delete", func() {
		// Reconcile twice (finalizer add + provisioning).
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		// envtest does not run the StatefulSet controller, so no real PVCs exist.
		// Manually create one labelled like the project's PVCs to verify cleanup.
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "data-" + projectName + "-nats-0",
				Namespace: namespace,
				Labels:    map[string]string{projectLabelKey: projectName, "rpc.operator.io/component": "nats"},
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
				},
			},
		}
		Expect(k8sClient.Create(ctx, pvc)).To(Succeed())

		// Trigger deletion.
		pp := &rpcv1alpha1.PipelineProject{}
		Expect(k8sClient.Get(ctx, nn, pp)).To(Succeed())
		Expect(k8sClient.Delete(ctx, pp)).To(Succeed())

		// Reconcile after delete to run the finalizer cleanup.
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		// envtest adds kubernetes.io/pvc-protection automatically, so the PVC
		// enters Terminating rather than disappearing immediately. Verify the
		// operator issued the delete (DeletionTimestamp is set).
		var remaining corev1.PersistentVolumeClaim
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name: "data-" + projectName + "-nats-0", Namespace: namespace,
		}, &remaining)
		if apierrors.IsNotFound(err) {
			// PVC is fully gone — also acceptable.
			return
		}
		Expect(err).NotTo(HaveOccurred())
		Expect(remaining.DeletionTimestamp).NotTo(BeNil(), "PVC should have been marked for deletion")
	})
})
