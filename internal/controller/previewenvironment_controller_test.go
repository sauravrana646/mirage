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
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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

	Context("suspend", func() {
		It("sets Paused phase without deleting children", func() {
			name := types.NamespacedName{Name: "suspend-preview", Namespace: "default"}
			targetNS := "preview-suspend-" + randomSuffix()
			pe := &miragev1alpha1.PreviewEnvironment{
				ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
				Spec: miragev1alpha1.PreviewEnvironmentSpec{
					Image:           "nginxinc/nginx-unprivileged:1.27-alpine",
					TargetNamespace: targetNS,
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
			pe.Spec.Suspend = true
			Expect(k8sClient.Update(ctx, pe)).To(Succeed())
			result, err := rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(requeueFast))
			Expect(k8sClient.Get(ctx, name, pe)).To(Succeed())
			Expect(pe.Status.Phase).To(Equal(miragev1alpha1.PhasePaused))
			Expect(pe.Status.Message).To(ContainSubstring("TTL still applies"))

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: targetNS}, &corev1.Namespace{})).To(Succeed())

			Expect(k8sClient.Delete(ctx, pe)).To(Succeed())
			Eventually(func(g Gomega) {
				forceRemoveNamespace(ctx, targetNS)
				_, err := rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
				g.Expect(err).NotTo(HaveOccurred())
				err = k8sClient.Get(ctx, name, &miragev1alpha1.PreviewEnvironment{})
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})

		It("still expires and requeues while suspended", func() {
			name := types.NamespacedName{Name: "suspend-ttl-preview", Namespace: "default"}
			targetNS := "preview-suspend-ttl-" + randomSuffix()
			ttl := int64(3600)
			pe := &miragev1alpha1.PreviewEnvironment{
				ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
				Spec: miragev1alpha1.PreviewEnvironmentSpec{
					Image:           "nginxinc/nginx-unprivileged:1.27-alpine",
					TargetNamespace: targetNS,
					TTLSeconds:      &ttl,
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
			Expect(pe.Status.ExpiresAt).NotTo(BeNil())
			pe.Spec.Suspend = true
			Expect(k8sClient.Update(ctx, pe)).To(Succeed())

			result, err := rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Get(ctx, name, pe)).To(Succeed())
			Expect(pe.Status.Phase).To(Equal(miragev1alpha1.PhasePaused))
			Expect(pe.Status.ExpiresAt).NotTo(BeNil())
			Expect(result.RequeueAfter).To(BeNumerically(">", time.Second))
			Expect(result.RequeueAfter).To(BeNumerically("<=", time.Until(pe.Status.ExpiresAt.Time)+time.Second))

			past := metav1.NewTime(time.Now().Add(-time.Minute))
			pe.Status.ExpiresAt = &past
			Expect(k8sClient.Status().Update(ctx, pe)).To(Succeed())

			_, err = rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func(g Gomega) {
				forceRemoveNamespace(ctx, targetNS)
				_, err := rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
				g.Expect(err).NotTo(HaveOccurred())
				err = k8sClient.Get(ctx, name, &miragev1alpha1.PreviewEnvironment{})
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("PSA label reconcile", func() {
		It("updates PSA labels on existing owned namespaces", func() {
			name := types.NamespacedName{Name: "psa-preview", Namespace: "default"}
			targetNS := "preview-psa-" + randomSuffix()
			pe := &miragev1alpha1.PreviewEnvironment{
				ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
				Spec: miragev1alpha1.PreviewEnvironmentSpec{
					Image:           "nginxinc/nginx-unprivileged:1.27-alpine",
					TargetNamespace: targetNS,
					ContainerPort:   8080,
					Replicas:        int32Ptr(0),
				},
			}
			Expect(k8sClient.Create(ctx, pe)).To(Succeed())
			rec := &PreviewEnvironmentReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, _ = rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
			_, err := rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
			Expect(err).NotTo(HaveOccurred())

			ns := &corev1.Namespace{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: targetNS}, ns)).To(Succeed())
			Expect(ns.Labels["pod-security.kubernetes.io/enforce"]).To(Equal("restricted"))
			unrelated := "team.example.com/owner"
			baselinePSA := string(miragev1alpha1.SecurityProfileBaseline)
			ns.Labels["pod-security.kubernetes.io/enforce"] = baselinePSA
			ns.Labels["pod-security.kubernetes.io/warn"] = baselinePSA
			ns.Labels["pod-security.kubernetes.io/audit"] = baselinePSA
			ns.Labels[unrelated] = "platform"
			Expect(k8sClient.Update(ctx, ns)).To(Succeed())

			Expect(k8sClient.Get(ctx, name, pe)).To(Succeed())
			Expect(rec.ensureNamespace(ctx, pe, "restricted")).To(Succeed())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: targetNS}, ns)).To(Succeed())
			Expect(ns.Labels["pod-security.kubernetes.io/enforce"]).To(Equal("restricted"))
			Expect(ns.Labels["pod-security.kubernetes.io/warn"]).To(Equal("restricted"))
			Expect(ns.Labels["pod-security.kubernetes.io/audit"]).To(Equal("restricted"))
			Expect(ns.Labels[unrelated]).To(Equal("platform"))

			Expect(k8sClient.Delete(ctx, pe)).To(Succeed())
			Eventually(func(g Gomega) {
				forceRemoveNamespace(ctx, targetNS)
				_, err := rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
				g.Expect(err).NotTo(HaveOccurred())
				err = k8sClient.Get(ctx, name, &miragev1alpha1.PreviewEnvironment{})
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("requireDigest", func() {
		It("fails with ImageInvalid when digest is missing", func() {
			name := types.NamespacedName{Name: "digest-preview", Namespace: "default"}
			targetNS := "preview-digest-" + randomSuffix()
			pe := &miragev1alpha1.PreviewEnvironment{
				ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
				Spec: miragev1alpha1.PreviewEnvironmentSpec{
					Image:           "nginxinc/nginx-unprivileged:1.27-alpine",
					TargetNamespace: targetNS,
					RequireDigest:   true,
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
			Expect(pe.Status.Message).To(ContainSubstring("sha256"))
			var ready *metav1.Condition
			for i := range pe.Status.Conditions {
				if pe.Status.Conditions[i].Type == miragev1alpha1.ConditionReady {
					ready = &pe.Status.Conditions[i]
					break
				}
			}
			Expect(ready).NotTo(BeNil())
			Expect(ready.Reason).To(Equal(miragev1alpha1.ReasonImageInvalid))

			// No workloads should have been created.
			err = k8sClient.Get(ctx, types.NamespacedName{Name: targetNS}, &corev1.Namespace{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue())

			Expect(k8sClient.Delete(ctx, pe)).To(Succeed())
			Eventually(func(g Gomega) {
				_, err := rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
				g.Expect(err).NotTo(HaveOccurred())
				err = k8sClient.Get(ctx, name, &miragev1alpha1.PreviewEnvironment{})
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})

		It("scales existing application Deployments to 0 when requireDigest fails", func() {
			name := types.NamespacedName{Name: "digest-scale-preview", Namespace: "default"}
			targetNS := "preview-digest-scale-" + randomSuffix()
			pe := &miragev1alpha1.PreviewEnvironment{
				ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
				Spec: miragev1alpha1.PreviewEnvironmentSpec{
					Image:           "nginxinc/nginx-unprivileged:1.27-alpine",
					TargetNamespace: targetNS,
					ContainerPort:   8080,
					Replicas:        int32Ptr(1),
					Dependencies: &miragev1alpha1.PreviewDependenciesSpec{
						Redis: &miragev1alpha1.RedisDependencySpec{Enabled: true},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pe)).To(Succeed())
			rec := &PreviewEnvironmentReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, _ = rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
			_, err := rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, name, pe)).To(Succeed())
			appDeploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name.Name, Namespace: targetNS}, appDeploy)).To(Succeed())
			Expect(appDeploy.Spec.Replicas).NotTo(BeNil())
			Expect(*appDeploy.Spec.Replicas).To(Equal(int32(1)))

			redisDeploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: depRedisName, Namespace: targetNS}, redisDeploy)).To(Succeed())
			Expect(redisDeploy.Spec.Replicas).NotTo(BeNil())
			redisReplicas := *redisDeploy.Spec.Replicas

			pe.Spec.RequireDigest = true
			Expect(k8sClient.Update(ctx, pe)).To(Succeed())
			_, err = rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, name, pe)).To(Succeed())
			Expect(pe.Status.Phase).To(Equal(miragev1alpha1.PhaseFailed))
			Expect(pe.Status.Message).To(ContainSubstring("sha256"))
			Expect(pe.Status.Message).To(ContainSubstring("scaled"))

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name.Name, Namespace: targetNS}, appDeploy)).To(Succeed())
			Expect(appDeploy.Spec.Replicas).NotTo(BeNil())
			Expect(*appDeploy.Spec.Replicas).To(Equal(int32(0)))

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: depRedisName, Namespace: targetNS}, redisDeploy)).To(Succeed())
			Expect(redisDeploy.Spec.Replicas).NotTo(BeNil())
			Expect(*redisDeploy.Spec.Replicas).To(Equal(redisReplicas))

			Expect(k8sClient.Delete(ctx, pe)).To(Succeed())
			Eventually(func(g Gomega) {
				forceRemoveNamespace(ctx, targetNS)
				_, err := rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
				g.Expect(err).NotTo(HaveOccurred())
				err = k8sClient.Get(ctx, name, &miragev1alpha1.PreviewEnvironment{})
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("PreviewTemplate watch mapping", func() {
		It("enqueues PreviewEnvironments that reference the template", func() {
			scheme := k8sClient.Scheme()
			tpl := &miragev1alpha1.PreviewTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "watch-tpl", Namespace: "default"},
				Spec:       miragev1alpha1.PreviewTemplateSpec{},
			}
			name := types.NamespacedName{Name: "watch-preview", Namespace: "default"}
			pe := &miragev1alpha1.PreviewEnvironment{
				ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
				Spec: miragev1alpha1.PreviewEnvironmentSpec{
					TemplateRef:     &miragev1alpha1.TemplateRef{Name: "watch-tpl"},
					Image:           "nginxinc/nginx-unprivileged:1.27-alpine",
					TargetNamespace: "preview-watch-" + randomSuffix(),
					ContainerPort:   8080,
					Replicas:        int32Ptr(0),
				},
			}
			other := &miragev1alpha1.PreviewEnvironment{
				ObjectMeta: metav1.ObjectMeta{Name: "watch-other", Namespace: "default"},
				Spec: miragev1alpha1.PreviewEnvironmentSpec{
					Image:           "nginxinc/nginx-unprivileged:1.27-alpine",
					TargetNamespace: "preview-watch-other-" + randomSuffix(),
					ContainerPort:   8080,
					Replicas:        int32Ptr(0),
				},
			}
			indexed := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tpl, pe, other).
				WithIndex(&miragev1alpha1.PreviewEnvironment{}, templateRefFieldIndex, indexPreviewEnvironmentByTemplateRef).
				Build()

			reqs := previewEnvironmentsForTemplate(ctx, indexed, tpl)
			Expect(reqs).To(ContainElement(reconcile.Request{NamespacedName: name}))
			Expect(reqs).NotTo(ContainElement(reconcile.Request{
				NamespacedName: types.NamespacedName{Name: other.Name, Namespace: other.Namespace},
			}))
		})
	})

	Context("ingress and probes", func() {
		It("creates Ingress with TLS and applies probes", func() {
			name := types.NamespacedName{Name: "ingress-preview", Namespace: "default"}
			targetNS := "preview-ing-" + randomSuffix()
			class := "nginx"
			pe := &miragev1alpha1.PreviewEnvironment{
				ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
				Spec: miragev1alpha1.PreviewEnvironmentSpec{
					Image:           "nginxinc/nginx-unprivileged:1.27-alpine",
					TargetNamespace: targetNS,
					ContainerPort:   8080,
					Replicas:        int32Ptr(0),
					ReadinessProbe:  &miragev1alpha1.ProbeSpec{Path: "/ready", Port: 8080},
					Command:         []string{"/bin/app"},
					Args:            []string{"--preview"},
					Ingress: &miragev1alpha1.IngressSpec{
						Enabled: true, Host: "pr.example.com", IngressClassName: &class,
						TLS: &miragev1alpha1.TLSSpec{Enabled: true, SecretName: "pr-tls"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pe)).To(Succeed())
			rec := &PreviewEnvironmentReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, _ = rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
			_, err := rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
			Expect(err).NotTo(HaveOccurred())

			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name.Name, Namespace: targetNS}, deploy)).To(Succeed())
			Expect(deploy.Spec.Template.Spec.Containers[0].ReadinessProbe).NotTo(BeNil())
			Expect(deploy.Spec.Template.Spec.Containers[0].Command).To(Equal([]string{"/bin/app"}))

			ing := &networkingv1.Ingress{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name.Name, Namespace: targetNS}, ing)).To(Succeed())
			Expect(ing.Spec.TLS).NotTo(BeEmpty())
			Expect(ing.Spec.TLS[0].SecretName).To(Equal("pr-tls"))

			Expect(k8sClient.Get(ctx, name, pe)).To(Succeed())
			Expect(pe.Status.URL).To(Equal("https://pr.example.com"))

			Expect(k8sClient.Delete(ctx, pe)).To(Succeed())
			Eventually(func(g Gomega) {
				forceRemoveNamespace(ctx, targetNS)
				_, err := rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
				g.Expect(err).NotTo(HaveOccurred())
				err = k8sClient.Get(ctx, name, &miragev1alpha1.PreviewEnvironment{})
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("baseline multi-service NetworkPolicy", func() {
		It("allows same-preview app ingress and egress peers", func() {
			name := types.NamespacedName{Name: "baseline-multi", Namespace: "default"}
			targetNS := "preview-baseline-multi-" + randomSuffix()
			pe := &miragev1alpha1.PreviewEnvironment{
				ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
				Spec: miragev1alpha1.PreviewEnvironmentSpec{
					TargetNamespace: targetNS,
					NetworkPolicy:   miragev1alpha1.NetworkPolicyBaseline,
					Services: []miragev1alpha1.PreviewServiceSpec{
						{Name: "api", Image: "nginxinc/nginx-unprivileged:1.27-alpine", Port: 8080},
						{Name: "web", Image: "nginxinc/nginx-unprivileged:1.27-alpine", Port: 8080},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pe)).To(Succeed())
			rec := &PreviewEnvironmentReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, _ = rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
			_, err := rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, name, pe)).To(Succeed())
			np := &networkingv1.NetworkPolicy{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: networkPolicyBaselineName, Namespace: targetNS}, np)).To(Succeed())

			samePreview := map[string]string{
				miragev1alpha1.LabelOwnerUID: string(pe.UID),
				componentLabel:               previewComponent,
			}
			Expect(np.Spec.Ingress).NotTo(BeEmpty())
			Expect(np.Spec.Ingress[0].From).To(ContainElement(networkingv1.NetworkPolicyPeer{
				PodSelector: &metav1.LabelSelector{MatchLabels: samePreview},
			}))

			var appEgress *networkingv1.NetworkPolicyEgressRule
			for i := range np.Spec.Egress {
				rule := &np.Spec.Egress[i]
				if len(rule.To) == 0 || rule.To[0].PodSelector == nil {
					continue
				}
				labels := rule.To[0].PodSelector.MatchLabels
				if labels[miragev1alpha1.LabelOwnerUID] == string(pe.UID) && labels[componentLabel] == previewComponent {
					appEgress = rule
					break
				}
			}
			Expect(appEgress).NotTo(BeNil(), "baseline egress must allow traffic to same-preview app pods")
			Expect(appEgress.Ports).NotTo(BeEmpty())
			Expect(appEgress.Ports[0].Port.IntVal).To(Equal(int32(8080)))

			Expect(k8sClient.Delete(ctx, pe)).To(Succeed())
			Eventually(func(g Gomega) {
				forceRemoveNamespace(ctx, targetNS)
				_, err := rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
				g.Expect(err).NotTo(HaveOccurred())
				err = k8sClient.Get(ctx, name, &miragev1alpha1.PreviewEnvironment{})
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("multi-service with template", func() {
		It("creates per-service Deployments and applies template defaults", func() {
			tpl := &miragev1alpha1.PreviewTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "std-web", Namespace: "default"},
				Spec: miragev1alpha1.PreviewTemplateSpec{
					NetworkPolicy: miragev1alpha1.NetworkPolicyPermissive,
					Resources:     &miragev1alpha1.ResourcePresetSpec{Preset: miragev1alpha1.ResourcePresetSmall},
				},
			}
			Expect(k8sClient.Create(ctx, tpl)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, tpl) }()

			name := types.NamespacedName{Name: "multi-preview", Namespace: "default"}
			targetNS := "preview-multi-" + randomSuffix()
			pe := &miragev1alpha1.PreviewEnvironment{
				ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
				Spec: miragev1alpha1.PreviewEnvironmentSpec{
					TemplateRef:     &miragev1alpha1.TemplateRef{Name: "std-web"},
					TargetNamespace: targetNS,
					Services: []miragev1alpha1.PreviewServiceSpec{
						{Name: "api", Image: "nginxinc/nginx-unprivileged:1.27-alpine", Port: 8080},
						{Name: "web", Image: "nginxinc/nginx-unprivileged:1.27-alpine", Port: 8080},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pe)).To(Succeed())
			rec := &PreviewEnvironmentReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, _ = rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
			_, err := rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
			Expect(err).NotTo(HaveOccurred())

			for _, svcName := range []string{"api", "web"} {
				deploy := &appsv1.Deployment{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: svcName, Namespace: targetNS}, deploy)).To(Succeed())
				Expect(deploy.Labels[miragev1alpha1.LabelService]).To(Equal(svcName))
				svc := &corev1.Service{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: svcName, Namespace: targetNS}, svc)).To(Succeed())
			}
			Expect(k8sClient.Get(ctx, name, pe)).To(Succeed())
			Expect(pe.Status.Template).To(Equal("std-web"))
			np := &networkingv1.NetworkPolicy{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "mirage-baseline", Namespace: targetNS}, np)).To(Succeed())
			// Template sets permissive → allow-all egress (no port filters).
			Expect(np.Spec.Egress).To(HaveLen(1))
			Expect(np.Spec.Egress[0].Ports).To(BeEmpty())

			Expect(k8sClient.Delete(ctx, pe)).To(Succeed())
			Eventually(func(g Gomega) {
				forceRemoveNamespace(ctx, targetNS)
				_, err := rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
				g.Expect(err).NotTo(HaveOccurred())
				err = k8sClient.Get(ctx, name, &miragev1alpha1.PreviewEnvironment{})
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("ephemeral dependencies", func() {
		It("creates redis dependency, injects REDIS_URL, and opens NetworkPolicy egress", func() {
			name := types.NamespacedName{Name: "deps-preview", Namespace: "default"}
			targetNS := "preview-deps-" + randomSuffix()
			pe := &miragev1alpha1.PreviewEnvironment{
				ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
				Spec: miragev1alpha1.PreviewEnvironmentSpec{
					Image:           "nginxinc/nginx-unprivileged:1.27-alpine",
					TargetNamespace: targetNS,
					ContainerPort:   8080,
					Replicas:        int32Ptr(0),
					Dependencies: &miragev1alpha1.PreviewDependenciesSpec{
						Redis: &miragev1alpha1.RedisDependencySpec{Enabled: true},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pe)).To(Succeed())
			rec := &PreviewEnvironmentReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, _ = rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
			_, err := rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
			Expect(err).NotTo(HaveOccurred())

			redisDeploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: depRedisName, Namespace: targetNS}, redisDeploy)).To(Succeed())
			Expect(redisDeploy.Labels[miragev1alpha1.LabelDependency]).To(Equal(depRedisKind))
			Expect(redisDeploy.Labels[componentLabel]).To(Equal(depComponent))

			redisSvc := &corev1.Service{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: depRedisName, Namespace: targetNS}, redisSvc)).To(Succeed())
			Expect(redisSvc.Spec.Ports[0].Port).To(Equal(int32(6379)))

			redisSecret := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: depRedisName, Namespace: targetNS}, redisSecret)).To(Succeed())
			Expect(redisSecret.Data[depSecretKeyPass]).NotTo(BeEmpty())
			Expect(redisSecret.Data[depSecretKeyRedisURL]).NotTo(BeEmpty())
			firstPass := string(redisSecret.Data[depSecretKeyPass])

			appDeploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name.Name, Namespace: targetNS}, appDeploy)).To(Succeed())
			Expect(appDeploy.Spec.Template.Spec.Containers[0].Env).To(ContainElement(corev1.EnvVar{
				Name: "REDIS_URL",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: depRedisName},
						Key:                  depSecretKeyRedisURL,
					},
				},
			}))

			np := &networkingv1.NetworkPolicy{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: networkPolicyBaselineName, Namespace: targetNS}, np)).To(Succeed())
			Expect(np.Spec.Ingress[0].From).To(HaveLen(2))
			Expect(np.Spec.Egress).To(HaveLen(3))
			Expect(np.Spec.Egress[2].To).NotTo(BeEmpty())
			Expect(np.Spec.Egress[2].To[0].PodSelector).NotTo(BeNil())
			Expect(np.Spec.Egress[2].To[0].PodSelector.MatchLabels[componentLabel]).To(Equal(depComponent))
			Expect(np.Spec.Egress[2].Ports[0].Port.IntVal).To(Equal(int32(6379)))

			depNP := &networkingv1.NetworkPolicy{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: networkPolicyDependenciesName, Namespace: targetNS}, depNP)).To(Succeed())
			Expect(depNP.Spec.PodSelector.MatchLabels[componentLabel]).To(Equal(depComponent))
			Expect(depNP.Spec.Ingress[0].From[0].PodSelector.MatchLabels[componentLabel]).To(Equal(previewComponent))

			By("reconciling again preserves redis secret password")
			_, err = rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: depRedisName, Namespace: targetNS}, redisSecret)).To(Succeed())
			Expect(string(redisSecret.Data[depSecretKeyPass])).To(Equal(firstPass))

			By("marking redis Deployment available sets DependenciesReady")
			redisDeploy.Status.Replicas = 1
			redisDeploy.Status.ReadyReplicas = 1
			redisDeploy.Status.UpdatedReplicas = 1
			redisDeploy.Status.AvailableReplicas = 1
			redisDeploy.Status.ObservedGeneration = redisDeploy.Generation
			Expect(k8sClient.Status().Update(ctx, redisDeploy)).To(Succeed())

			_, err = rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Get(ctx, name, pe)).To(Succeed())
			var depsCond *metav1.Condition
			for i := range pe.Status.Conditions {
				if pe.Status.Conditions[i].Type == miragev1alpha1.ConditionDependenciesReady {
					depsCond = &pe.Status.Conditions[i]
					break
				}
			}
			Expect(depsCond).NotTo(BeNil())
			Expect(depsCond.Status).To(Equal(metav1.ConditionTrue))

			Expect(k8sClient.Delete(ctx, pe)).To(Succeed())
			Eventually(func(g Gomega) {
				forceRemoveNamespace(ctx, targetNS)
				_, err := rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
				g.Expect(err).NotTo(HaveOccurred())
				err = k8sClient.Get(ctx, name, &miragev1alpha1.PreviewEnvironment{})
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})

		It("creates postgres secret and wires DATABASE_URL via secretKeyRef", func() {
			name := types.NamespacedName{Name: "deps-pg", Namespace: "default"}
			targetNS := "preview-pg-" + randomSuffix()
			pe := &miragev1alpha1.PreviewEnvironment{
				ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
				Spec: miragev1alpha1.PreviewEnvironmentSpec{
					Image:           "nginxinc/nginx-unprivileged:1.27-alpine",
					TargetNamespace: targetNS,
					ContainerPort:   8080,
					Replicas:        int32Ptr(0),
					Dependencies: &miragev1alpha1.PreviewDependenciesSpec{
						Postgres: &miragev1alpha1.PostgresDependencySpec{Enabled: true},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pe)).To(Succeed())
			rec := &PreviewEnvironmentReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, _ = rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
			_, err := rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
			Expect(err).NotTo(HaveOccurred())

			secret := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: depPostgresName, Namespace: targetNS}, secret)).To(Succeed())
			Expect(string(secret.Data[depSecretKeyUser])).To(Equal(depPostgresUser))
			Expect(secret.Data[depSecretKeyPass]).NotTo(BeEmpty())
			Expect(string(secret.Data[depSecretKeyPass])).NotTo(Equal("preview"))
			Expect(secret.Data[depSecretKeyDBURL]).NotTo(BeEmpty())
			Expect(string(secret.Data[depSecretKeyDBURL])).To(ContainSubstring("@" + depPostgresName + ":5432/"))

			pgDeploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: depPostgresName, Namespace: targetNS}, pgDeploy)).To(Succeed())
			var passEnv *corev1.EnvVar
			for i := range pgDeploy.Spec.Template.Spec.Containers[0].Env {
				if pgDeploy.Spec.Template.Spec.Containers[0].Env[i].Name == "POSTGRESQL_PASSWORD" {
					passEnv = &pgDeploy.Spec.Template.Spec.Containers[0].Env[i]
					break
				}
			}
			Expect(passEnv).NotTo(BeNil())
			Expect(passEnv.Value).To(BeEmpty())
			Expect(passEnv.ValueFrom.SecretKeyRef.Name).To(Equal(depPostgresName))

			appDeploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name.Name, Namespace: targetNS}, appDeploy)).To(Succeed())
			Expect(appDeploy.Spec.Template.Spec.Containers[0].Env).To(ContainElement(corev1.EnvVar{
				Name: "DATABASE_URL",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: depPostgresName},
						Key:                  depSecretKeyDBURL,
					},
				},
			}))

			Expect(k8sClient.Delete(ctx, pe)).To(Succeed())
			Eventually(func(g Gomega) {
				forceRemoveNamespace(ctx, targetNS)
				_, err := rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
				g.Expect(err).NotTo(HaveOccurred())
				err = k8sClient.Get(ctx, name, &miragev1alpha1.PreviewEnvironment{})
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("effective spec validation after template resolve", func() {
		It("fails InvalidSpec when template sets backend=argocd without argoCD on PE", func() {
			tpl := &miragev1alpha1.PreviewTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "argo-default", Namespace: "default"},
				Spec:       miragev1alpha1.PreviewTemplateSpec{Backend: miragev1alpha1.BackendArgoCD},
			}
			Expect(k8sClient.Create(ctx, tpl)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, tpl) }()

			name := types.NamespacedName{Name: "tpl-argo-missing", Namespace: "default"}
			targetNS := "preview-tpl-argo-" + randomSuffix()
			pe := &miragev1alpha1.PreviewEnvironment{
				ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
				Spec: miragev1alpha1.PreviewEnvironmentSpec{
					TemplateRef:     &miragev1alpha1.TemplateRef{Name: "argo-default"},
					Image:           "nginxinc/nginx-unprivileged:1.27-alpine",
					TargetNamespace: targetNS,
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
			Expect(pe.Status.Message).To(ContainSubstring("spec.argoCD.repoURL and path are required"))
			var ready *metav1.Condition
			for i := range pe.Status.Conditions {
				if pe.Status.Conditions[i].Type == miragev1alpha1.ConditionReady {
					ready = &pe.Status.Conditions[i]
					break
				}
			}
			Expect(ready).NotTo(BeNil())
			Expect(ready.Reason).To(Equal(miragev1alpha1.ReasonInvalidSpec))

			Expect(k8sClient.Delete(ctx, pe)).To(Succeed())
			Eventually(func(g Gomega) {
				forceRemoveNamespace(ctx, targetNS)
				_, err := rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
				g.Expect(err).NotTo(HaveOccurred())
				err = k8sClient.Get(ctx, name, &miragev1alpha1.PreviewEnvironment{})
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})

		It("fails InvalidSpec when effective backend is argocd with dependencies enabled", func() {
			name := types.NamespacedName{Name: "argo-deps", Namespace: "default"}
			targetNS := "preview-argo-deps-" + randomSuffix()
			pe := &miragev1alpha1.PreviewEnvironment{
				ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
				Spec: miragev1alpha1.PreviewEnvironmentSpec{
					Image:           "nginxinc/nginx-unprivileged:1.27-alpine",
					TargetNamespace: targetNS,
					ContainerPort:   8080,
					Replicas:        int32Ptr(0),
					Backend:         miragev1alpha1.BackendArgoCD,
					ArgoCD: &miragev1alpha1.ArgoCDSpec{
						RepoURL: "https://github.com/org/app", Path: "deploy",
					},
					Dependencies: &miragev1alpha1.PreviewDependenciesSpec{
						Redis: &miragev1alpha1.RedisDependencySpec{Enabled: true},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pe)).To(Succeed())
			rec := &PreviewEnvironmentReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			_, _ = rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
			_, err := rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, name, pe)).To(Succeed())
			Expect(pe.Status.Phase).To(Equal(miragev1alpha1.PhaseFailed))
			Expect(pe.Status.Message).To(ContainSubstring("dependencies are not supported when backend=argocd"))
			var ready *metav1.Condition
			for i := range pe.Status.Conditions {
				if pe.Status.Conditions[i].Type == miragev1alpha1.ConditionReady {
					ready = &pe.Status.Conditions[i]
					break
				}
			}
			Expect(ready).NotTo(BeNil())
			Expect(ready.Reason).To(Equal(miragev1alpha1.ReasonInvalidSpec))

			Expect(k8sClient.Delete(ctx, pe)).To(Succeed())
			Eventually(func(g Gomega) {
				_, err := rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
				g.Expect(err).NotTo(HaveOccurred())
				err = k8sClient.Get(ctx, name, &miragev1alpha1.PreviewEnvironment{})
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})
	})
})

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
