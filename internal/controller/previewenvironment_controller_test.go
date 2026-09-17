/*
Copyright 2026 Saurav Rana.

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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	miragev1alpha1 "github.com/sauravrana646/mirage/api/v1alpha1"
)

var _ = Describe("PreviewEnvironment Controller", func() {
	const (
		resourceName = "test-preview"
		timeout      = time.Second * 10
		interval     = time.Millisecond * 250
	)

	ctx := context.Background()

	Context("when creating a PreviewEnvironment", func() {
		var (
			targetNS           string
			typeNamespacedName types.NamespacedName
			reconciler         *PreviewEnvironmentReconciler
		)

		BeforeEach(func() {
			targetNS = "preview-test-" + randomSuffix()
			typeNamespacedName = types.NamespacedName{Name: resourceName, Namespace: "default"}
			reconciler = &PreviewEnvironmentReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			By("creating the PreviewEnvironment")
			pe := &miragev1alpha1.PreviewEnvironment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: miragev1alpha1.PreviewEnvironmentSpec{
					Image:           "nginxinc/nginx-unprivileged:1.27-alpine",
					TargetNamespace: targetNS,
					ContainerPort:   8080,
					Replicas:        int32Ptr(1),
				},
			}
			Expect(k8sClient.Create(ctx, pe)).To(Succeed())
		})

		AfterEach(func() {
			pe := &miragev1alpha1.PreviewEnvironment{}
			err := k8sClient.Get(ctx, typeNamespacedName, pe)
			if apierrors.IsNotFound(err) {
				forceRemoveNamespace(ctx, targetNS)
				return
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(ctx, pe)).To(Succeed())

			// Drive finalizer cleanup. envtest has no namespace controller, so strip
			// namespace finalizers so Delete can reach NotFound for reconcileDelete.
			Eventually(func(g Gomega) {
				forceRemoveNamespace(ctx, targetNS)
				_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
				g.Expect(err).NotTo(HaveOccurred())
				err = k8sClient.Get(ctx, typeNamespacedName, &miragev1alpha1.PreviewEnvironment{})
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})

		It("creates namespace, deployment, service and reaches Ready after deploy available", func() {
			By("adding finalizer")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("ensuring children")
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			ns := &corev1.Namespace{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: targetNS}, ns)).To(Succeed())
			Expect(ns.Labels[miragev1alpha1.LabelManagedBy]).To(Equal(miragev1alpha1.ManagedByValue))

			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: resourceName, Namespace: targetNS}, deploy)).To(Succeed())
			Expect(deploy.Spec.Template.Spec.Containers[0].Image).To(Equal("nginxinc/nginx-unprivileged:1.27-alpine"))

			svc := &corev1.Service{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: resourceName, Namespace: targetNS}, svc)).To(Succeed())

			quota := &corev1.ResourceQuota{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "mirage-quota", Namespace: targetNS}, quota)).To(Succeed())
			lr := &corev1.LimitRange{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "mirage-defaults", Namespace: targetNS}, lr)).To(Succeed())
			np := &networkingv1.NetworkPolicy{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "mirage-baseline", Namespace: targetNS}, np)).To(Succeed())
			Expect(ns.Labels["pod-security.kubernetes.io/enforce"]).To(Equal("restricted"))
			Expect(deploy.Spec.Template.Spec.AutomountServiceAccountToken).NotTo(BeNil())
			Expect(*deploy.Spec.Template.Spec.AutomountServiceAccountToken).To(BeFalse())

			By("simulating Deployment available")
			deploy.Status.Replicas = 1
			deploy.Status.ReadyReplicas = 1
			deploy.Status.UpdatedReplicas = 1
			deploy.Status.AvailableReplicas = 1
			deploy.Status.ObservedGeneration = deploy.Generation
			Expect(k8sClient.Status().Update(ctx, deploy)).To(Succeed())

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			pe := &miragev1alpha1.PreviewEnvironment{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, pe)).To(Succeed())
			Expect(pe.Status.Phase).To(Equal(miragev1alpha1.PhaseReady))
			Expect(pe.Status.Conditions).NotTo(BeEmpty())
		})

		It("does not mark Ready until all desired replicas are available", func() {
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: resourceName, Namespace: targetNS}, deploy)).To(Succeed())
			replicas := int32(2)
			deploy.Spec.Replicas = &replicas
			Expect(k8sClient.Update(ctx, deploy)).To(Succeed())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: resourceName, Namespace: targetNS}, deploy)).To(Succeed())
			deploy.Status.Replicas = 2
			deploy.Status.ReadyReplicas = 1
			deploy.Status.UpdatedReplicas = 1
			deploy.Status.AvailableReplicas = 1
			deploy.Status.ObservedGeneration = deploy.Generation
			Expect(k8sClient.Status().Update(ctx, deploy)).To(Succeed())

			pe := &miragev1alpha1.PreviewEnvironment{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, pe)).To(Succeed())
			pe.Spec.Replicas = &replicas
			Expect(k8sClient.Update(ctx, pe)).To(Succeed())

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Get(ctx, typeNamespacedName, pe)).To(Succeed())
			Expect(pe.Status.Phase).NotTo(Equal(miragev1alpha1.PhaseReady))
		})

		It("rejects targetNamespace mutation", func() {
			pe := &miragev1alpha1.PreviewEnvironment{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, pe)).To(Succeed())
			pe.Spec.TargetNamespace = "preview-mutated-" + randomSuffix()
			err := k8sClient.Update(ctx, pe)
			Expect(err).To(HaveOccurred())
		})

		It("reports NamespaceConflict when target namespace is foreign", func() {
			conflictNS := "preview-conflict-" + randomSuffix()
			Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: conflictNS}})).To(Succeed())

			name := types.NamespacedName{Name: "conflict-preview", Namespace: "default"}
			pe := &miragev1alpha1.PreviewEnvironment{
				ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
				Spec: miragev1alpha1.PreviewEnvironmentSpec{
					Image:           "nginxinc/nginx-unprivileged:1.27-alpine",
					TargetNamespace: conflictNS,
					ContainerPort:   8080,
					Replicas:        int32Ptr(0),
				},
			}
			Expect(k8sClient.Create(ctx, pe)).To(Succeed())

			rec := &PreviewEnvironmentReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, _ = rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
			_, err := rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, name, pe)).To(Succeed())
			Expect(pe.Status.Phase).To(Equal(miragev1alpha1.PhaseFailed))
			Expect(pe.Status.Conditions[0].Reason).To(Equal(miragev1alpha1.ReasonNamespaceConflict))

			Expect(k8sClient.Delete(ctx, pe)).To(Succeed())
			Eventually(func(g Gomega) {
				_, err := rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
				g.Expect(err).NotTo(HaveOccurred())
				err = k8sClient.Get(ctx, name, &miragev1alpha1.PreviewEnvironment{})
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}, timeout, interval).Should(Succeed())

			// Foreign namespace must still exist (controller must not delete it)
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: conflictNS}, &corev1.Namespace{})).To(Succeed())
			_ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: conflictNS}})
		})
	})

	Context("when image is missing", func() {
		It("is rejected by CRD validation", func() {
			pe := &miragev1alpha1.PreviewEnvironment{
				ObjectMeta: metav1.ObjectMeta{Name: "bad-image", Namespace: "default"},
				Spec: miragev1alpha1.PreviewEnvironmentSpec{
					TargetNamespace: "preview-bad-" + randomSuffix(),
				},
			}
			err := k8sClient.Create(ctx, pe)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("TTL expiry", func() {
		It("deletes the CR after expiry", func() {
			name := types.NamespacedName{Name: "ttl-preview", Namespace: "default"}
			targetNS := "preview-ttl-" + randomSuffix()
			ttl := int64(1)
			pe := &miragev1alpha1.PreviewEnvironment{
				ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
				Spec: miragev1alpha1.PreviewEnvironmentSpec{
					Image:           "nginxinc/nginx-unprivileged:1.27-alpine",
					TargetNamespace: targetNS,
					TTLSeconds:      &ttl,
					ContainerPort:   8080,
				},
			}
			Expect(k8sClient.Create(ctx, pe)).To(Succeed())

			reconciler := &PreviewEnvironmentReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: name})
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: name})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, name, pe)).To(Succeed())
			Expect(pe.Status.ExpiresAt).NotTo(BeNil())

			// Force expiry in the past
			past := metav1.NewTime(time.Now().Add(-time.Minute))
			pe.Status.ExpiresAt = &past
			Expect(k8sClient.Status().Update(ctx, pe)).To(Succeed())

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: name})
			Expect(err).NotTo(HaveOccurred())

			// Finalizer cleanup (strip ns finalizers for envtest)
			Eventually(func(g Gomega) {
				forceRemoveNamespace(ctx, targetNS)
				_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: name})
				g.Expect(err).NotTo(HaveOccurred())
				err = k8sClient.Get(ctx, name, &miragev1alpha1.PreviewEnvironment{})
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})
	})
})

func int32Ptr(v int32) *int32 { return &v }

func randomSuffix() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// forceRemoveNamespace clears finalizers so envtest can finish Namespace deletion.
func forceRemoveNamespace(ctx context.Context, name string) {
	ns := &corev1.Namespace{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, ns); err != nil {
		return
	}
	if len(ns.Finalizers) > 0 {
		ns.Finalizers = nil
		_ = k8sClient.Update(ctx, ns)
	}
	_ = k8sClient.Delete(ctx, ns)
}
