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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
	"github.com/insidegreen/rpc-operator-claude/internal/streams"
)

func makeReadyClusterPod(ctx context.Context, cluster, namespace string, ordinal int) {
	name := fmt.Sprintf("%s-%d", cluster, ordinal)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{clusterLabelKey: cluster},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "connect", Image: "x"}}},
	}
	Expect(k8sClient.Create(ctx, pod)).To(Succeed())
	pod.Status.Phase = corev1.PodRunning
	pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}
	Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())
}

var _ = Describe("Pipeline clusterRef assignment", func() {
	const namespace = "default"
	var (
		ctx        = context.Background()
		fake       *streams.FakeClient
		reconciler *PipelineReconciler
	)

	BeforeEach(func() {
		fake = streams.NewFakeClient()
		reconciler = &PipelineReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Streams: fake}
	})

	assign := func(name string) types.NamespacedName {
		nn := types.NamespacedName{Name: name, Namespace: namespace}
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		return nn
	}

	It("deploys a stream onto a ready instance and records placement", func() {
		cluster := &rpcv1alpha1.PipelineCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineClusterSpec{Replicas: 1},
		}
		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
		makeReadyClusterPod(ctx, "c1", namespace, 0)

		pipe := &rpcv1alpha1.Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineSpec{ClusterRef: "c1", Input: rpcv1alpha1.ComponentSpec{Type: "generate"}, Output: rpcv1alpha1.ComponentSpec{Type: "drop"}},
		}
		Expect(k8sClient.Create(ctx, pipe)).To(Succeed())

		nn := assign("p1")
		Expect(fake.Has("http://c1-0.c1."+namespace+".svc:4195", "p1")).To(BeTrue())

		got := &rpcv1alpha1.Pipeline{}
		Expect(k8sClient.Get(ctx, nn, got)).To(Succeed())
		Expect(got.Status.AssignedCluster).To(Equal("c1"))
		Expect(got.Status.AssignedInstance).To(Equal("c1-0"))
		Expect(got.Status.StreamID).To(Equal("p1"))
		Expect(got.Status.Phase).To(Equal(rpcv1alpha1.PhaseRunning))

		pod := &corev1.Pod{}
		Expect(k8sClient.Get(ctx, nn, pod)).To(HaveOccurred())
	})

	It("rejects a pipeline with SecretRefs", func() {
		cluster := &rpcv1alpha1.PipelineCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "c2", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineClusterSpec{Replicas: 1},
		}
		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
		makeReadyClusterPod(ctx, "c2", namespace, 0)

		pipe := &rpcv1alpha1.Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "p2", Namespace: namespace},
			Spec: rpcv1alpha1.PipelineSpec{
				ClusterRef: "c2",
				SecretRefs: []rpcv1alpha1.SecretRef{{EnvVar: "X", SecretName: "s", Key: "k"}},
				Input:      rpcv1alpha1.ComponentSpec{Type: "generate"},
				Output:     rpcv1alpha1.ComponentSpec{Type: "drop"},
			},
		}
		Expect(k8sClient.Create(ctx, pipe)).To(Succeed())

		nn := assign("p2")
		got := &rpcv1alpha1.Pipeline{}
		Expect(k8sClient.Get(ctx, nn, got)).To(Succeed())
		Expect(got.Status.Phase).To(Equal(rpcv1alpha1.PhaseFailed))
		Expect(fake.Has("http://c2-0.c2."+namespace+".svc:4195", "p2")).To(BeFalse())
	})

	It("marks Pending when the cluster has no ready instances", func() {
		cluster := &rpcv1alpha1.PipelineCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "c3", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineClusterSpec{Replicas: 1},
		}
		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())

		pipe := &rpcv1alpha1.Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "p3", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineSpec{ClusterRef: "c3", Input: rpcv1alpha1.ComponentSpec{Type: "generate"}, Output: rpcv1alpha1.ComponentSpec{Type: "drop"}},
		}
		Expect(k8sClient.Create(ctx, pipe)).To(Succeed())

		nn := assign("p3")
		got := &rpcv1alpha1.Pipeline{}
		Expect(k8sClient.Get(ctx, nn, got)).To(Succeed())
		Expect(got.Status.Phase).To(Equal(rpcv1alpha1.PhasePending))
	})

	It("marks Failed when clusterRef names a missing cluster", func() {
		pipe := &rpcv1alpha1.Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "p4", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineSpec{ClusterRef: "nope", Input: rpcv1alpha1.ComponentSpec{Type: "generate"}, Output: rpcv1alpha1.ComponentSpec{Type: "drop"}},
		}
		Expect(k8sClient.Create(ctx, pipe)).To(Succeed())

		nn := assign("p4")
		got := &rpcv1alpha1.Pipeline{}
		Expect(k8sClient.Get(ctx, nn, got)).To(Succeed())
		Expect(got.Status.Phase).To(Equal(rpcv1alpha1.PhaseFailed))
	})

	It("keeps a placed pipeline on the same instance across reconciles (sticky)", func() {
		cluster := &rpcv1alpha1.PipelineCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "c5", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineClusterSpec{Replicas: 2},
		}
		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
		makeReadyClusterPod(ctx, "c5", namespace, 0)
		makeReadyClusterPod(ctx, "c5", namespace, 1)

		pipe := &rpcv1alpha1.Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "p5", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineSpec{ClusterRef: "c5", Input: rpcv1alpha1.ComponentSpec{Type: "generate"}, Output: rpcv1alpha1.ComponentSpec{Type: "drop"}},
		}
		Expect(k8sClient.Create(ctx, pipe)).To(Succeed())

		nn := assign("p5")
		got := &rpcv1alpha1.Pipeline{}
		Expect(k8sClient.Get(ctx, nn, got)).To(Succeed())
		firstInstance := got.Status.AssignedInstance
		Expect(firstInstance).NotTo(BeEmpty())

		// reconcile several more times; placement must not move
		for i := 0; i < 3; i++ {
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(k8sClient.Get(ctx, nn, got)).To(Succeed())
		Expect(got.Status.AssignedInstance).To(Equal(firstInstance))
		// exactly one placement: the stream exists only on its assigned instance
		Expect(fake.Has("http://"+firstInstance+".c5."+namespace+".svc:4195", "p5")).To(BeTrue())
		other := "c5-0"
		if firstInstance == "c5-0" {
			other = "c5-1"
		}
		Expect(fake.Has("http://"+other+".c5."+namespace+".svc:4195", "p5")).To(BeFalse())
	})

	It("spreads two pipelines across ready instances (least-loaded)", func() {
		cluster := &rpcv1alpha1.PipelineCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "c6", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineClusterSpec{Replicas: 2},
		}
		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
		makeReadyClusterPod(ctx, "c6", namespace, 0)
		makeReadyClusterPod(ctx, "c6", namespace, 1)

		for _, n := range []string{"p6", "p7"} {
			p := &rpcv1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{Name: n, Namespace: namespace},
				Spec:       rpcv1alpha1.PipelineSpec{ClusterRef: "c6", Input: rpcv1alpha1.ComponentSpec{Type: "generate"}, Output: rpcv1alpha1.ComponentSpec{Type: "drop"}},
			}
			Expect(k8sClient.Create(ctx, p)).To(Succeed())
			assign(n)
		}
		g6 := &rpcv1alpha1.Pipeline{}
		g7 := &rpcv1alpha1.Pipeline{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "p6", Namespace: namespace}, g6)).To(Succeed())
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "p7", Namespace: namespace}, g7)).To(Succeed())
		Expect(g6.Status.AssignedInstance).NotTo(Equal(g7.Status.AssignedInstance))
	})

	It("requeues an assigned pipeline after the resync interval", func() {
		cluster := &rpcv1alpha1.PipelineCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "c10", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineClusterSpec{Replicas: 1},
		}
		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
		makeReadyClusterPod(ctx, "c10", namespace, 0)

		pipe := &rpcv1alpha1.Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "p10", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineSpec{ClusterRef: "c10", Input: rpcv1alpha1.ComponentSpec{Type: "generate"}, Output: rpcv1alpha1.ComponentSpec{Type: "drop"}},
		}
		Expect(k8sClient.Create(ctx, pipe)).To(Succeed())

		nn := types.NamespacedName{Name: "p10", Namespace: namespace}
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn}) // adds finalizer
		Expect(err).NotTo(HaveOccurred())
		res, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn}) // assigns
		Expect(err).NotTo(HaveOccurred())
		Expect(res.RequeueAfter).To(Equal(resyncInterval))
	})

	It("re-asserts the stream after a pod restart (resync)", func() {
		cluster := &rpcv1alpha1.PipelineCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "c11", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineClusterSpec{Replicas: 1},
		}
		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
		makeReadyClusterPod(ctx, "c11", namespace, 0)

		pipe := &rpcv1alpha1.Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "p11", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineSpec{ClusterRef: "c11", Input: rpcv1alpha1.ComponentSpec{Type: "generate"}, Output: rpcv1alpha1.ComponentSpec{Type: "drop"}},
		}
		Expect(k8sClient.Create(ctx, pipe)).To(Succeed())

		url := "http://c11-0.c11." + namespace + ".svc:4195"
		assign("p11")
		Expect(fake.Has(url, "p11")).To(BeTrue())

		fake.DropPod(url) // simulate the cluster pod restarting and losing its streams
		Expect(fake.Has(url, "p11")).To(BeFalse())

		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: "p11", Namespace: namespace}})
		Expect(err).NotTo(HaveOccurred())
		Expect(fake.Has(url, "p11")).To(BeTrue()) // resync re-asserted the stream
	})

	It("reschedules onto a remaining instance and sets the Rescheduling reason on scale-down", func() {
		cluster := &rpcv1alpha1.PipelineCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "c12", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineClusterSpec{Replicas: 2},
		}
		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
		makeReadyClusterPod(ctx, "c12", namespace, 0)
		makeReadyClusterPod(ctx, "c12", namespace, 1)

		pipe := &rpcv1alpha1.Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "p12", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineSpec{ClusterRef: "c12", Input: rpcv1alpha1.ComponentSpec{Type: "generate"}, Output: rpcv1alpha1.ComponentSpec{Type: "drop"}},
		}
		Expect(k8sClient.Create(ctx, pipe)).To(Succeed())

		nn := assign("p12")
		got := &rpcv1alpha1.Pipeline{}
		Expect(k8sClient.Get(ctx, nn, got)).To(Succeed())
		Expect(got.Status.AssignedInstance).To(Equal("c12-0")) // ties -> lowest ordinal

		// Scale down: remove the assigned instance's pod.
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "c12-0", Namespace: namespace}}
		Expect(k8sClient.Delete(ctx, pod)).To(Succeed())

		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, nn, got)).To(Succeed())
		Expect(got.Status.AssignedInstance).To(Equal("c12-1"))
		cond := apimeta.FindStatusCondition(got.Status.Conditions, "Ready")
		Expect(cond).NotTo(BeNil())
		Expect(cond.Reason).To(Equal("Rescheduling"))
		Expect(fake.Has("http://c12-1.c12."+namespace+".svc:4195", "p12")).To(BeTrue())
	})

	It("deletes the stream on the old cluster when clusterRef changes (migration)", func() {
		for _, c := range []string{"c13a", "c13b"} {
			cl := &rpcv1alpha1.PipelineCluster{
				ObjectMeta: metav1.ObjectMeta{Name: c, Namespace: namespace},
				Spec:       rpcv1alpha1.PipelineClusterSpec{Replicas: 1},
			}
			Expect(k8sClient.Create(ctx, cl)).To(Succeed())
			makeReadyClusterPod(ctx, c, namespace, 0)
		}

		pipe := &rpcv1alpha1.Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "p13", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineSpec{ClusterRef: "c13a", Input: rpcv1alpha1.ComponentSpec{Type: "generate"}, Output: rpcv1alpha1.ComponentSpec{Type: "drop"}},
		}
		Expect(k8sClient.Create(ctx, pipe)).To(Succeed())

		nn := assign("p13")
		urlA := "http://c13a-0.c13a." + namespace + ".svc:4195"
		urlB := "http://c13b-0.c13b." + namespace + ".svc:4195"
		Expect(fake.Has(urlA, "p13")).To(BeTrue())

		got := &rpcv1alpha1.Pipeline{}
		Expect(k8sClient.Get(ctx, nn, got)).To(Succeed())
		got.Spec.ClusterRef = "c13b"
		Expect(k8sClient.Update(ctx, got)).To(Succeed())

		assign("p13")
		Expect(fake.Has(urlA, "p13")).To(BeFalse()) // old stream removed
		Expect(fake.Has(urlB, "p13")).To(BeTrue())  // new stream created

		Expect(k8sClient.Get(ctx, nn, got)).To(Succeed())
		Expect(got.Status.AssignedCluster).To(Equal("c13b"))
		Expect(got.Status.AssignedInstance).To(Equal("c13b-0"))
	})

	AfterEach(func() {
		pipes := &rpcv1alpha1.PipelineList{}
		Expect(k8sClient.List(ctx, pipes, client.InNamespace(namespace))).To(Succeed())
		for i := range pipes.Items {
			p := &pipes.Items[i]
			p.Finalizers = nil
			_ = k8sClient.Update(ctx, p)
			_ = k8sClient.Delete(ctx, p)
		}
		_ = k8sClient.DeleteAllOf(ctx, &corev1.Pod{}, client.InNamespace(namespace))
		clusters := &rpcv1alpha1.PipelineClusterList{}
		Expect(k8sClient.List(ctx, clusters, client.InNamespace(namespace))).To(Succeed())
		for i := range clusters.Items {
			_ = k8sClient.Delete(ctx, &clusters.Items[i])
		}
	})
})
