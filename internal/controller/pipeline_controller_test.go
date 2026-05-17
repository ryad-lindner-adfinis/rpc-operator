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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
)

// helloWorldSpec returns a valid PipelineSpec mirroring the sample CR.
func helloWorldSpec() rpcv1alpha1.PipelineSpec {
	return rpcv1alpha1.PipelineSpec{
		Input: rpcv1alpha1.ComponentSpec{
			Type: "generate",
			Config: runtime.RawExtension{Raw: []byte(
				`{"mapping":"root = \"hello world\"","interval":"1s","count":5}`,
			)},
		},
		Processors: []rpcv1alpha1.ComponentSpec{{
			Type: "mapping",
			Config: runtime.RawExtension{Raw: []byte(
				`{"mapping":"root = content().uppercase()"}`,
			)},
		}},
		Output: rpcv1alpha1.ComponentSpec{
			Type:   "stdout",
			Config: runtime.RawExtension{Raw: []byte(`{}`)},
		},
	}
}

var _ = Describe("Pipeline Controller", func() {
	const (
		resourceName = "test-pipeline"
		namespace    = "default"
	)

	var (
		ctx                  = context.Background()
		nn                   = types.NamespacedName{Name: resourceName, Namespace: namespace}
		controllerReconciler *PipelineReconciler
	)

	BeforeEach(func() {
		controllerReconciler = &PipelineReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}

		By("creating the Pipeline CR")
		pipe := &rpcv1alpha1.Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
			Spec:       helloWorldSpec(),
		}
		Expect(k8sClient.Create(ctx, pipe)).To(Succeed())
	})

	AfterEach(func() {
		By("removing leftover Pipeline + child resources")
		pipe := &rpcv1alpha1.Pipeline{}
		if err := k8sClient.Get(ctx, nn, pipe); err == nil {
			// Force-clear finalizer in case the test left one behind.
			pipe.Finalizers = nil
			_ = k8sClient.Update(ctx, pipe)
			_ = k8sClient.Delete(ctx, pipe)
		}
		_ = k8sClient.Delete(ctx, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
		})
		_ = k8sClient.Delete(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName + "-config", Namespace: namespace},
		})
	})

	It("creates a ConfigMap and Pod with owner references", func() {
		// First reconcile adds the finalizer, then requeues.
		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		// Second reconcile creates the children.
		_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		By("having a ConfigMap with the rendered pipeline.yaml")
		cm := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name: resourceName + "-config", Namespace: namespace,
		}, cm)).To(Succeed())
		Expect(cm.Data).To(HaveKey("pipeline.yaml"))
		Expect(cm.Data["pipeline.yaml"]).To(ContainSubstring("generate:"))
		Expect(cm.Data["pipeline.yaml"]).To(ContainSubstring("uppercase"))
		Expect(cm.OwnerReferences).To(HaveLen(1))
		Expect(*cm.OwnerReferences[0].Controller).To(BeTrue())
		Expect(cm.OwnerReferences[0].Kind).To(Equal("Pipeline"))

		By("having a Pod referencing the ConfigMap")
		pod := &corev1.Pod{}
		Expect(k8sClient.Get(ctx, nn, pod)).To(Succeed())
		Expect(pod.Spec.Containers).To(HaveLen(1))
		Expect(pod.Spec.Containers[0].Image).To(Equal(defaultImage))
		Expect(pod.OwnerReferences).To(HaveLen(1))
		Expect(*pod.OwnerReferences[0].Controller).To(BeTrue())
		Expect(pod.Labels).To(HaveKeyWithValue("rpc.operator.io/pipeline", resourceName))

		By("setting status.podName on the Pipeline")
		pipe := &rpcv1alpha1.Pipeline{}
		Expect(k8sClient.Get(ctx, nn, pipe)).To(Succeed())
		Expect(pipe.Status.PodName).To(Equal(resourceName))
		Expect(pipe.Status.ObservedGeneration).To(Equal(pipe.Generation))
	})

	It("removes the finalizer on delete and the CR disappears", func() {
		// Reconcile to add finalizer + create children.
		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		// Confirm finalizer present.
		pipe := &rpcv1alpha1.Pipeline{}
		Expect(k8sClient.Get(ctx, nn, pipe)).To(Succeed())
		Expect(pipe.Finalizers).To(ContainElement(finalizerName))

		// Delete the CR — API server keeps it around (with DeletionTimestamp) until
		// the finalizer is removed.
		Expect(k8sClient.Delete(ctx, pipe)).To(Succeed())

		// Reconcile observes the deletion timestamp and removes the finalizer.
		_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		// CR should now be gone.
		Eventually(func() bool {
			err := k8sClient.Get(ctx, nn, &rpcv1alpha1.Pipeline{})
			return apierrors.IsNotFound(err)
		}).Should(BeTrue())
	})

	It("re-rolls the pod when the Pipeline spec changes", func() {
		// Reconcile 1: add finalizer.
		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		// Reconcile 2: create ConfigMap + Pod.
		_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		By("having the spec-hash annotation on the initial pod")
		pod := &corev1.Pod{}
		Expect(k8sClient.Get(ctx, nn, pod)).To(Succeed())
		originalHash := pod.Annotations[specHashAnnotation]
		Expect(originalHash).NotTo(BeEmpty())

		By("updating the Pipeline spec")
		pipe := &rpcv1alpha1.Pipeline{}
		Expect(k8sClient.Get(ctx, nn, pipe)).To(Succeed())
		pipe.Spec.Input.Config = runtime.RawExtension{Raw: []byte(
			`{"mapping":"root = \"updated\"","interval":"2s","count":1}`,
		)}
		Expect(k8sClient.Update(ctx, pipe)).To(Succeed())

		By("reconcile detects hash mismatch and deletes the pod")
		result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{Requeue: true}))

		By("pod is gone after the stale-pod delete")
		Eventually(func() bool {
			return apierrors.IsNotFound(k8sClient.Get(ctx, nn, &corev1.Pod{}))
		}).Should(BeTrue())

		By("next reconcile creates a new pod with an updated hash annotation")
		_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		newPod := &corev1.Pod{}
		Expect(k8sClient.Get(ctx, nn, newPod)).To(Succeed())
		Expect(newPod.Annotations[specHashAnnotation]).NotTo(BeEmpty())
		Expect(newPod.Annotations[specHashAnnotation]).NotTo(Equal(originalHash))
	})

	It("deletes the pod and marks Stopped when spec.stopped flips to true", func() {
		// Reconcile 1+2: finalizer + create pod.
		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		By("pod exists after initial reconcile")
		Expect(k8sClient.Get(ctx, nn, &corev1.Pod{})).To(Succeed())

		By("flipping spec.stopped=true")
		pipe := &rpcv1alpha1.Pipeline{}
		Expect(k8sClient.Get(ctx, nn, pipe)).To(Succeed())
		pipe.Spec.Stopped = true
		Expect(k8sClient.Update(ctx, pipe)).To(Succeed())

		By("reconcile takes the stopped path and deletes the pod")
		_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() bool {
			return apierrors.IsNotFound(k8sClient.Get(ctx, nn, &corev1.Pod{}))
		}).Should(BeTrue())

		By("status reflects Stopped phase with reason StoppedByUser")
		Expect(k8sClient.Get(ctx, nn, pipe)).To(Succeed())
		Expect(pipe.Status.Phase).To(Equal(rpcv1alpha1.PhaseStopped))
		Expect(pipe.Status.PodName).To(BeEmpty())
		readyCond := findCondition(pipe.Status.Conditions, "Ready")
		Expect(readyCond).NotTo(BeNil())
		Expect(readyCond.Reason).To(Equal("StoppedByUser"))
		Expect(string(readyCond.Status)).To(Equal("False"))
	})

	It("recreates the pod when spec.stopped flips back to false", func() {
		// Reconcile 1+2: finalizer + create pod.
		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		By("stopping the pipeline")
		pipe := &rpcv1alpha1.Pipeline{}
		Expect(k8sClient.Get(ctx, nn, pipe)).To(Succeed())
		pipe.Spec.Stopped = true
		Expect(k8sClient.Update(ctx, pipe)).To(Succeed())
		_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		Eventually(func() bool {
			return apierrors.IsNotFound(k8sClient.Get(ctx, nn, &corev1.Pod{}))
		}).Should(BeTrue())

		By("flipping spec.stopped back to false")
		Expect(k8sClient.Get(ctx, nn, pipe)).To(Succeed())
		pipe.Spec.Stopped = false
		Expect(k8sClient.Update(ctx, pipe)).To(Succeed())

		By("reconcile recreates the pod")
		_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Get(ctx, nn, &corev1.Pod{})
		}).Should(Succeed())

		By("status phase leaves Stopped")
		Expect(k8sClient.Get(ctx, nn, pipe)).To(Succeed())
		Expect(pipe.Status.Phase).NotTo(Equal(rpcv1alpha1.PhaseStopped))
	})
})

// findCondition returns the named condition or nil if absent.
func findCondition(conds []metav1.Condition, name string) *metav1.Condition {
	for i := range conds {
		if conds[i].Type == name {
			return &conds[i]
		}
	}
	return nil
}
