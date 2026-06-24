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
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
	"github.com/insidegreen/rpc-operator-claude/internal/streams"
)

func makeReadyClusterPod(ctx context.Context, cluster string, ordinal int) {
	name := fmt.Sprintf("%s-%d", cluster, ordinal)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
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
		makeReadyClusterPod(ctx, "c1", 0)

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

	It("counts projectRef pipelines toward instance load by their placement", func() {
		const cluster = "loadproj-cluster"
		// A project-member pipeline carries spec.projectRef (not spec.clusterRef)
		// but is placed onto the project's managed cluster via status. Load
		// balancing must count it so the next pipeline avoids the same instance.
		member := &rpcv1alpha1.Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "loadproj-member-0", Namespace: namespace},
			Spec: rpcv1alpha1.PipelineSpec{
				ProjectRef: &rpcv1alpha1.ProjectRef{Name: "loadproj"},
				Input:      rpcv1alpha1.ComponentSpec{Type: "generate"},
				Output:     rpcv1alpha1.ComponentSpec{Type: "drop"},
			},
		}
		Expect(k8sClient.Create(ctx, member)).To(Succeed())
		member.Status.AssignedCluster = cluster
		member.Status.AssignedInstance = cluster + "-0"
		Expect(k8sClient.Status().Update(ctx, member)).To(Succeed())

		load, err := reconciler.loadByOrdinal(ctx, cluster, namespace, "someother")
		Expect(err).NotTo(HaveOccurred())
		Expect(load[0]).To(Equal(1))
	})

	It("deploys a cluster pipeline with SecretRefs, substituting the secret value", func() {
		cluster := &rpcv1alpha1.PipelineCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "cs1", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineClusterSpec{Replicas: 1},
		}
		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
		makeReadyClusterPod(ctx, "cs1", 0)

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "db-creds", Namespace: namespace},
			Data:       map[string][]byte{"password": []byte("topsecret")},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())

		pipe := &rpcv1alpha1.Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "ps1", Namespace: namespace},
			Spec: rpcv1alpha1.PipelineSpec{
				ClusterRef: "cs1",
				SecretRefs: []rpcv1alpha1.SecretRef{{EnvVar: "DB_PASS", SecretName: "db-creds", Key: "password"}},
				RawYAML:    "input:\n  generate:\n    mapping: 'root.pass = \"${DB_PASS}\"'\noutput:\n  drop: {}\n",
			},
		}
		Expect(k8sClient.Create(ctx, pipe)).To(Succeed())

		nn := assign("ps1")
		got := &rpcv1alpha1.Pipeline{}
		Expect(k8sClient.Get(ctx, nn, got)).To(Succeed())
		Expect(got.Status.Phase).To(Equal(rpcv1alpha1.PhaseRunning))

		url := "http://cs1-0.cs1." + namespace + ".svc:4195"
		Expect(fake.Has(url, "ps1")).To(BeTrue())
		body := fake.StreamBody(url, "ps1")
		Expect(body).To(ContainSubstring("${DB_PASS:topsecret}"))
		Expect(body).NotTo(ContainSubstring("topsecret\n")) // value only inside ${...}, never raw
	})

	It("re-deploys with the new secret value on the next reconcile (rotation support)", func() {
		cluster := &rpcv1alpha1.PipelineCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "cs2", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineClusterSpec{Replicas: 1},
		}
		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
		makeReadyClusterPod(ctx, "cs2", 0)

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "rot-creds", Namespace: namespace},
			Data:       map[string][]byte{"password": []byte("v1")},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())

		pipe := &rpcv1alpha1.Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "ps2", Namespace: namespace},
			Spec: rpcv1alpha1.PipelineSpec{
				ClusterRef: "cs2",
				SecretRefs: []rpcv1alpha1.SecretRef{{EnvVar: "DB_PASS", SecretName: "rot-creds", Key: "password"}},
				RawYAML:    "input:\n  generate:\n    mapping: 'root.pass = \"${DB_PASS}\"'\noutput:\n  drop: {}\n",
			},
		}
		Expect(k8sClient.Create(ctx, pipe)).To(Succeed())

		nn := assign("ps2")
		url := "http://cs2-0.cs2." + namespace + ".svc:4195"
		Expect(fake.StreamBody(url, "ps2")).To(ContainSubstring("${DB_PASS:v1}"))

		// Simulate secret rotation
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "rot-creds", Namespace: namespace}, secret)).To(Succeed())
		secret.Data["password"] = []byte("v2")
		Expect(k8sClient.Update(ctx, secret)).To(Succeed())

		// Next reconcile re-substitutes with the new value
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		Expect(fake.StreamBody(url, "ps2")).To(ContainSubstring("${DB_PASS:v2}"))
	})

	It("marks Failed with SecretNotFound when referenced secret does not exist", func() {
		cluster := &rpcv1alpha1.PipelineCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "cs3", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineClusterSpec{Replicas: 1},
		}
		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
		makeReadyClusterPod(ctx, "cs3", 0)

		pipe := &rpcv1alpha1.Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "ps3", Namespace: namespace},
			Spec: rpcv1alpha1.PipelineSpec{
				ClusterRef: "cs3",
				SecretRefs: []rpcv1alpha1.SecretRef{{EnvVar: "X", SecretName: "no-such-secret", Key: "k"}},
				Input:      rpcv1alpha1.ComponentSpec{Type: "generate"},
				Output:     rpcv1alpha1.ComponentSpec{Type: "drop"},
			},
		}
		Expect(k8sClient.Create(ctx, pipe)).To(Succeed())

		nn := assign("ps3")
		got := &rpcv1alpha1.Pipeline{}
		Expect(k8sClient.Get(ctx, nn, got)).To(Succeed())
		Expect(got.Status.Phase).To(Equal(rpcv1alpha1.PhaseFailed))
		cond := apimeta.FindStatusCondition(got.Status.Conditions, "Ready")
		Expect(cond).NotTo(BeNil())
		Expect(cond.Reason).To(Equal("SecretNotFound"))
		Expect(fake.Has("http://cs3-0.cs3."+namespace+".svc:4195", "ps3")).To(BeFalse())
	})

	It("marks Failed with StreamConfigInvalid when the streams API rejects the config", func() {
		cluster := &rpcv1alpha1.PipelineCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "cr1", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineClusterSpec{Replicas: 1},
		}
		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
		makeReadyClusterPod(ctx, "cr1", 0)

		// Simulate the Redpanda Connect streams API rejecting an invalid config
		// with a 400 lint error (the EnsureStream 4xx path).
		fake.EnsureErr = &streams.ConfigRejectedError{
			StreamID: "pr1", Status: 400,
			Body: `{"lint_errors":["(1,1) field inputs not recognised"]}`,
		}

		pipe := &rpcv1alpha1.Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pr1", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineSpec{ClusterRef: "cr1", Input: rpcv1alpha1.ComponentSpec{Type: "generate"}, Output: rpcv1alpha1.ComponentSpec{Type: "drop"}},
		}
		Expect(k8sClient.Create(ctx, pipe)).To(Succeed())

		// assign() asserts Reconcile returns no error — a rejected config must be
		// recorded in status, not bubbled up as a requeue-forever error.
		nn := assign("pr1")
		got := &rpcv1alpha1.Pipeline{}
		Expect(k8sClient.Get(ctx, nn, got)).To(Succeed())
		Expect(got.Status.Phase).To(Equal(rpcv1alpha1.PhaseFailed))
		cond := apimeta.FindStatusCondition(got.Status.Conditions, "Ready")
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal("StreamConfigInvalid"))
		Expect(cond.Message).To(ContainSubstring("field inputs not recognised"))
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
		makeReadyClusterPod(ctx, "c5", 0)
		makeReadyClusterPod(ctx, "c5", 1)

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
		for range 3 {
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
		makeReadyClusterPod(ctx, "c6", 0)
		makeReadyClusterPod(ctx, "c6", 1)

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
		makeReadyClusterPod(ctx, "c10", 0)

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
		makeReadyClusterPod(ctx, "c11", 0)

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

	It("does not re-deploy the stream on a resync when config is unchanged, but re-deploys when the stream is dropped", func() {
		cluster := &rpcv1alpha1.PipelineCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "c20", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineClusterSpec{Replicas: 1},
		}
		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
		makeReadyClusterPod(ctx, "c20", 0)

		pipe := &rpcv1alpha1.Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "p20", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineSpec{ClusterRef: "c20", Input: rpcv1alpha1.ComponentSpec{Type: "generate"}, Output: rpcv1alpha1.ComponentSpec{Type: "drop"}},
		}
		Expect(k8sClient.Create(ctx, pipe)).To(Succeed())

		nn := assign("p20")
		url := "http://c20-0.c20." + namespace + ".svc:4195"
		Expect(fake.Has(url, "p20")).To(BeTrue())

		// The deployed config hash is recorded in status.
		Expect(k8sClient.Get(ctx, nn, pipe)).To(Succeed())
		Expect(pipe.Status.StreamConfigHash).NotTo(BeEmpty())

		// A periodic resync with no config change must NOT re-deploy the stream
		// (this is the fix: avoids the ~resyncInterval tear-down/recreate churn).
		before := fake.EnsureCount
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		Expect(fake.EnsureCount).To(Equal(before), "unchanged-config resync must not re-deploy the stream")

		// If the stream is dropped (pod restart), the next resync must re-deploy it.
		fake.DropPod(url)
		_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		Expect(fake.EnsureCount).To(BeNumerically(">", before), "dropped stream must be re-deployed (self-heal)")
		Expect(fake.Has(url, "p20")).To(BeTrue())
	})

	It("reschedules onto a remaining instance and sets the Rescheduling reason on scale-down", func() {
		cluster := &rpcv1alpha1.PipelineCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "c12", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineClusterSpec{Replicas: 2},
		}
		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
		makeReadyClusterPod(ctx, "c12", 0)
		makeReadyClusterPod(ctx, "c12", 1)

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
			makeReadyClusterPod(ctx, c, 0)
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
		cond := apimeta.FindStatusCondition(got.Status.Conditions, "Ready")
		Expect(cond).NotTo(BeNil())
		Expect(cond.Reason).To(Equal("Assigned"))
	})

	It("releases the stream and clears placement when a clustered pipeline is stopped", func() {
		cluster := &rpcv1alpha1.PipelineCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "c15", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineClusterSpec{Replicas: 1},
		}
		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
		makeReadyClusterPod(ctx, "c15", 0)

		pipe := &rpcv1alpha1.Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "p15", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineSpec{ClusterRef: "c15", Input: rpcv1alpha1.ComponentSpec{Type: "generate"}, Output: rpcv1alpha1.ComponentSpec{Type: "drop"}},
		}
		Expect(k8sClient.Create(ctx, pipe)).To(Succeed())

		nn := assign("p15")
		url := "http://c15-0.c15." + namespace + ".svc:4195"
		Expect(fake.Has(url, "p15")).To(BeTrue())

		got := &rpcv1alpha1.Pipeline{}
		Expect(k8sClient.Get(ctx, nn, got)).To(Succeed())
		got.Spec.Stopped = true
		Expect(k8sClient.Update(ctx, got)).To(Succeed())

		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		Expect(fake.Has(url, "p15")).To(BeFalse()) // stream released
		Expect(k8sClient.Get(ctx, nn, got)).To(Succeed())
		Expect(got.Status.Phase).To(Equal(rpcv1alpha1.PhaseStopped))
		Expect(got.Status.AssignedCluster).To(BeEmpty())
		Expect(got.Status.AssignedInstance).To(BeEmpty())
		Expect(got.Status.StreamID).To(BeEmpty())
	})

	It("tears the stream down and recreates the pod when clusterRef is cleared (fallback)", func() {
		cluster := &rpcv1alpha1.PipelineCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "c14", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineClusterSpec{Replicas: 1},
		}
		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
		makeReadyClusterPod(ctx, "c14", 0)

		pipe := &rpcv1alpha1.Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "p14", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineSpec{ClusterRef: "c14", Input: rpcv1alpha1.ComponentSpec{Type: "generate"}, Output: rpcv1alpha1.ComponentSpec{Type: "drop"}},
		}
		Expect(k8sClient.Create(ctx, pipe)).To(Succeed())

		nn := assign("p14")
		url := "http://c14-0.c14." + namespace + ".svc:4195"
		Expect(fake.Has(url, "p14")).To(BeTrue())

		got := &rpcv1alpha1.Pipeline{}
		Expect(k8sClient.Get(ctx, nn, got)).To(Succeed())
		got.Spec.ClusterRef = ""
		Expect(k8sClient.Update(ctx, got)).To(Succeed())

		// First reconcile: tear down the stream + clear placement, then requeue.
		res, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		Expect(res.IsZero()).To(BeFalse())
		Expect(fake.Has(url, "p14")).To(BeFalse())
		Expect(k8sClient.Get(ctx, nn, got)).To(Succeed())
		Expect(got.Status.AssignedInstance).To(BeEmpty())
		Expect(got.Status.AssignedCluster).To(BeEmpty())

		// Second reconcile: normal pod path recreates the Pod.
		_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		pod := &corev1.Pod{}
		Expect(k8sClient.Get(ctx, nn, pod)).To(Succeed())
	})

	It("sets StreamActive=True/Running for an active placed stream", func() {
		cluster := &rpcv1alpha1.PipelineCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "ca1", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineClusterSpec{Replicas: 1},
		}
		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
		makeReadyClusterPod(ctx, "ca1", 0)

		pipe := &rpcv1alpha1.Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "p-active", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineSpec{ClusterRef: "ca1", Input: rpcv1alpha1.ComponentSpec{Type: "generate"}, Output: rpcv1alpha1.ComponentSpec{Type: "drop"}},
		}
		Expect(k8sClient.Create(ctx, pipe)).To(Succeed())

		nn := assign("p-active")
		var p rpcv1alpha1.Pipeline
		Expect(k8sClient.Get(ctx, nn, &p)).To(Succeed())

		sa := apimeta.FindStatusCondition(p.Status.Conditions, "StreamActive")
		Expect(sa).NotTo(BeNil())
		Expect(sa.Status).To(Equal(metav1.ConditionTrue))
		Expect(sa.Reason).To(Equal("Running"))
		Expect(p.Status.Phase).To(Equal(rpcv1alpha1.PhaseRunning))
		ready := apimeta.FindStatusCondition(p.Status.Conditions, "Ready")
		Expect(ready.Status).To(Equal(metav1.ConditionTrue))
	})

	It("sets StreamActive=False/StreamNotActive without touching Ready/Phase", func() {
		cluster := &rpcv1alpha1.PipelineCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "ca2", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineClusterSpec{Replicas: 1},
		}
		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
		makeReadyClusterPod(ctx, "ca2", 0)

		pipe := &rpcv1alpha1.Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "p-inactive", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineSpec{ClusterRef: "ca2", Input: rpcv1alpha1.ComponentSpec{Type: "generate"}, Output: rpcv1alpha1.ComponentSpec{Type: "drop"}},
		}
		Expect(k8sClient.Create(ctx, pipe)).To(Succeed())

		fake.SetStreamActive("p-inactive", false)
		nn := assign("p-inactive")
		var p rpcv1alpha1.Pipeline
		Expect(k8sClient.Get(ctx, nn, &p)).To(Succeed())

		sa := apimeta.FindStatusCondition(p.Status.Conditions, "StreamActive")
		Expect(sa.Status).To(Equal(metav1.ConditionFalse))
		Expect(sa.Reason).To(Equal("StreamNotActive"))
		Expect(p.Status.Phase).To(Equal(rpcv1alpha1.PhaseRunning))
		Expect(apimeta.FindStatusCondition(p.Status.Conditions, "Ready").Status).To(Equal(metav1.ConditionTrue))
	})

	It("sets StreamActive=Unknown/StatusUnavailable on a read error and keeps the placement", func() {
		cluster := &rpcv1alpha1.PipelineCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "ca3", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineClusterSpec{Replicas: 1},
		}
		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
		makeReadyClusterPod(ctx, "ca3", 0)

		pipe := &rpcv1alpha1.Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "p-unknown", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineSpec{ClusterRef: "ca3", Input: rpcv1alpha1.ComponentSpec{Type: "generate"}, Output: rpcv1alpha1.ComponentSpec{Type: "drop"}},
		}
		Expect(k8sClient.Create(ctx, pipe)).To(Succeed())

		fake.GetErr = fmt.Errorf("boom")
		nn := assign("p-unknown")
		var p rpcv1alpha1.Pipeline
		Expect(k8sClient.Get(ctx, nn, &p)).To(Succeed())

		sa := apimeta.FindStatusCondition(p.Status.Conditions, "StreamActive")
		Expect(sa.Status).To(Equal(metav1.ConditionUnknown))
		Expect(sa.Reason).To(Equal("StatusUnavailable"))
		Expect(p.Status.Phase).To(Equal(rpcv1alpha1.PhaseRunning))
		Expect(p.Status.AssignedInstance).NotTo(BeEmpty())
	})

	It("sets StreamActive=False/StreamMissing when the stream has vanished", func() {
		cluster := &rpcv1alpha1.PipelineCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "ca4", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineClusterSpec{Replicas: 1},
		}
		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
		makeReadyClusterPod(ctx, "ca4", 0)

		pipe := &rpcv1alpha1.Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "p-missing", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineSpec{ClusterRef: "ca4", Input: rpcv1alpha1.ComponentSpec{Type: "generate"}, Output: rpcv1alpha1.ComponentSpec{Type: "drop"}},
		}
		Expect(k8sClient.Create(ctx, pipe)).To(Succeed())

		fake.GetErr = streams.ErrStreamNotFound
		nn := assign("p-missing")
		var p rpcv1alpha1.Pipeline
		Expect(k8sClient.Get(ctx, nn, &p)).To(Succeed())

		sa := apimeta.FindStatusCondition(p.Status.Conditions, "StreamActive")
		Expect(sa.Status).To(Equal(metav1.ConditionFalse))
		Expect(sa.Reason).To(Equal("StreamMissing"))
	})

	It("removes StreamActive when placement fails (markClusterFailed)", func() {
		cluster := &rpcv1alpha1.PipelineCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "ca5", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineClusterSpec{Replicas: 1},
		}
		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
		makeReadyClusterPod(ctx, "ca5", 0)

		pipe := &rpcv1alpha1.Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "p-then-fail", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineSpec{ClusterRef: "ca5", Input: rpcv1alpha1.ComponentSpec{Type: "generate"}, Output: rpcv1alpha1.ComponentSpec{Type: "drop"}},
		}
		Expect(k8sClient.Create(ctx, pipe)).To(Succeed())

		nn := assign("p-then-fail")
		var p rpcv1alpha1.Pipeline
		Expect(k8sClient.Get(ctx, nn, &p)).To(Succeed())
		Expect(apimeta.FindStatusCondition(p.Status.Conditions, "StreamActive")).NotTo(BeNil())

		// The reconciler skips re-deploying an unchanged, present stream, so force a
		// real deploy attempt by dropping the stream first; that re-deploy is then
		// rejected, driving markClusterFailed.
		url := "http://ca5-0.ca5." + namespace + ".svc:4195"
		fake.DropPod(url)
		fake.EnsureErr = &streams.ConfigRejectedError{StreamID: "p-then-fail", Status: 400, Body: "lint"}
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, nn, &p)).To(Succeed())
		Expect(p.Status.Phase).To(Equal(rpcv1alpha1.PhaseFailed))
		Expect(apimeta.FindStatusCondition(p.Status.Conditions, "StreamActive")).To(BeNil())
	})

	It("removes StreamActive when the pipeline is stopped (handleStopped)", func() {
		cluster := &rpcv1alpha1.PipelineCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "cstop", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineClusterSpec{Replicas: 1},
		}
		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
		makeReadyClusterPod(ctx, "cstop", 0)

		pipe := &rpcv1alpha1.Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "p-then-stop", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineSpec{ClusterRef: "cstop", Input: rpcv1alpha1.ComponentSpec{Type: "generate"}, Output: rpcv1alpha1.ComponentSpec{Type: "drop"}},
		}
		Expect(k8sClient.Create(ctx, pipe)).To(Succeed())

		nn := assign("p-then-stop")
		var p rpcv1alpha1.Pipeline
		Expect(k8sClient.Get(ctx, nn, &p)).To(Succeed())
		Expect(apimeta.FindStatusCondition(p.Status.Conditions, "StreamActive")).NotTo(BeNil())

		p.Spec.Stopped = true
		Expect(k8sClient.Update(ctx, &p)).To(Succeed())
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, nn, &p)).To(Succeed())
		Expect(p.Status.Phase).To(Equal(rpcv1alpha1.PhaseStopped))
		Expect(apimeta.FindStatusCondition(p.Status.Conditions, "StreamActive")).To(BeNil())
	})

	It("removes StreamActive when clusterRef is cleared (handleClusterFallback)", func() {
		cluster := &rpcv1alpha1.PipelineCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "cfb", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineClusterSpec{Replicas: 1},
		}
		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
		makeReadyClusterPod(ctx, "cfb", 0)

		pipe := &rpcv1alpha1.Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "p-then-fallback", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineSpec{ClusterRef: "cfb", Input: rpcv1alpha1.ComponentSpec{Type: "generate"}, Output: rpcv1alpha1.ComponentSpec{Type: "drop"}},
		}
		Expect(k8sClient.Create(ctx, pipe)).To(Succeed())

		nn := assign("p-then-fallback")
		var p rpcv1alpha1.Pipeline
		Expect(k8sClient.Get(ctx, nn, &p)).To(Succeed())
		Expect(apimeta.FindStatusCondition(p.Status.Conditions, "StreamActive")).NotTo(BeNil())

		p.Spec.ClusterRef = "" // string field, not a pointer — empty clears the ref
		Expect(k8sClient.Update(ctx, &p)).To(Succeed())
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, nn, &p)).To(Succeed())
		Expect(apimeta.FindStatusCondition(p.Status.Conditions, "StreamActive")).To(BeNil())
	})

	It("deletes the assigned stream when the CR is deleted (issue #1)", func() {
		cluster := &rpcv1alpha1.PipelineCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "cdel", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineClusterSpec{Replicas: 1},
		}
		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
		makeReadyClusterPod(ctx, "cdel", 0)

		pipe := &rpcv1alpha1.Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "p-del", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineSpec{ClusterRef: "cdel", Input: rpcv1alpha1.ComponentSpec{Type: "generate"}, Output: rpcv1alpha1.ComponentSpec{Type: "drop"}},
		}
		Expect(k8sClient.Create(ctx, pipe)).To(Succeed())

		nn := assign("p-del")
		url := clusterPodURL("cdel", namespace, 0)
		Expect(fake.Has(url, "p-del")).To(BeTrue())

		// Delete the CR: the finalizer keeps the object until the reconciler
		// runs the cleanup path.
		var p rpcv1alpha1.Pipeline
		Expect(k8sClient.Get(ctx, nn, &p)).To(Succeed())
		Expect(k8sClient.Delete(ctx, &p)).To(Succeed())

		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		// Stream evicted from the instance pod, and the finalizer released so
		// the CR is gone.
		Expect(fake.Has(url, "p-del")).To(BeFalse())
		Expect(apierrors.IsNotFound(k8sClient.Get(ctx, nn, &p))).To(BeTrue())
	})

	It("deletes the CR without error when no stream is placed (pod-mode)", func() {
		// No clusterRef: the pipeline never holds a stream placement, so the
		// cleanup path must be a no-op and still release the finalizer.
		pipe := &rpcv1alpha1.Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "p-pod-del", Namespace: namespace},
			Spec:       rpcv1alpha1.PipelineSpec{Input: rpcv1alpha1.ComponentSpec{Type: "generate"}, Output: rpcv1alpha1.ComponentSpec{Type: "drop"}},
		}
		Expect(k8sClient.Create(ctx, pipe)).To(Succeed())

		nn := types.NamespacedName{Name: "p-pod-del", Namespace: namespace}
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn}) // adds finalizer
		Expect(err).NotTo(HaveOccurred())

		var p rpcv1alpha1.Pipeline
		Expect(k8sClient.Get(ctx, nn, &p)).To(Succeed())
		Expect(k8sClient.Delete(ctx, &p)).To(Succeed())

		_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		Expect(apierrors.IsNotFound(k8sClient.Get(ctx, nn, &p))).To(BeTrue())
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
		_ = k8sClient.DeleteAllOf(ctx, &corev1.Secret{}, client.InNamespace(namespace))
		clusters := &rpcv1alpha1.PipelineClusterList{}
		Expect(k8sClient.List(ctx, clusters, client.InNamespace(namespace))).To(Succeed())
		for i := range clusters.Items {
			_ = k8sClient.Delete(ctx, &clusters.Items[i])
		}
	})
})

// TestEnsureStreamPresent_SelfHealsWhenStreamDropped models a config-update PUT
// that returns 2xx but does not actually load the stream on the instance
// (DropNextEnsure). ensureStreamPresent must detect the missing stream via the
// status endpoint and recreate it, instead of leaving the pipeline reporting
// Running/StreamActive while the instance runs nothing.
func TestEnsureStreamPresent_SelfHealsWhenStreamDropped(t *testing.T) {
	f := streams.NewFakeClient()
	f.DropNextEnsure = true
	r := &PipelineReconciler{Streams: f}
	url, id := "http://scim-sync-0.scim-sync.dev.svc:4195", "sync-tenant-user"
	if err := r.ensureStreamPresent(context.Background(), url, id, "config: {}"); err != nil {
		t.Fatalf("ensureStreamPresent returned error: %v", err)
	}
	if !f.Has(url, id) {
		t.Error("stream missing after self-heal: a dropped config-update was not recreated")
	}
}
