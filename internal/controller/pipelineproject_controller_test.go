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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
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
					Storage:  ptr.To(resource.MustParse("1Gi")),
				},
			},
		}
		Expect(k8sClient.Create(ctx, project)).To(Succeed())
	})

	AfterEach(func() {
		pp := &rpcv1alpha1.PipelineProject{}
		if err := k8sClient.Get(ctx, nn, pp); err == nil {
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
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
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
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		ss := &appsv1.StatefulSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name: projectName + "-nats", Namespace: namespace,
		}, ss)).To(Succeed())
		firstRV := ss.ResourceVersion

		// Second reconcile must not churn the StatefulSet.
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		ss2 := &appsv1.StatefulSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name: projectName + "-nats", Namespace: namespace,
		}, ss2)).To(Succeed())
		Expect(ss2.ResourceVersion).To(Equal(firstRV),
			"StatefulSet must not be updated on no-op reconcile")
	})
})
