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
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	miragev1alpha1 "github.com/sauravrana646/mirage/api/v1alpha1"
)

func TestArgoApplicationNameUnique(t *testing.T) {
	a := &miragev1alpha1.PreviewEnvironment{
		ObjectMeta: metav1.ObjectMeta{Name: "pr-42", Namespace: "team-a"},
	}
	b := &miragev1alpha1.PreviewEnvironment{
		ObjectMeta: metav1.ObjectMeta{Name: "pr-42", Namespace: "team-b"},
	}
	na := argoApplicationName(a)
	nb := argoApplicationName(b)
	if na == nb {
		t.Fatalf("expected unique names, both %q", na)
	}
	if na == "pr-42" || nb == "pr-42" {
		t.Fatalf("names must not be bare pe.Name: %q %q", na, nb)
	}
	if len(na) > 63 || len(nb) > 63 {
		t.Fatalf("names exceed DNS limit: %d %d", len(na), len(nb))
	}
	if argoApplicationName(a) != na {
		t.Fatal("name not deterministic")
	}
}

func TestArgoDeleteCandidatesPreferStatus(t *testing.T) {
	pe := &miragev1alpha1.PreviewEnvironment{
		ObjectMeta: metav1.ObjectMeta{Name: "pr-1", Namespace: "ns"},
		Status:     miragev1alpha1.PreviewEnvironmentStatus{ArgoApplication: "argocd/custom-app"},
	}
	cands := argoDeleteCandidates(pe)
	if len(cands) < 1 || cands[0].Namespace != "argocd" || cands[0].Name != "custom-app" {
		t.Fatalf("status ref should be first: %+v", cands)
	}
}

func TestArgoOwnedBy(t *testing.T) {
	pe := &miragev1alpha1.PreviewEnvironment{
		ObjectMeta: metav1.ObjectMeta{UID: "uid-1"},
	}
	app := &unstructured.Unstructured{}
	if argoOwnedBy(pe, app) {
		t.Fatal("nil labels must not be owned")
	}
	app.SetLabels(map[string]string{
		miragev1alpha1.LabelManagedBy: "helm",
		miragev1alpha1.LabelOwnerUID:  "uid-1",
	})
	if argoOwnedBy(pe, app) {
		t.Fatal("non-mirage managed-by must not be owned")
	}
	app.SetLabels(map[string]string{
		miragev1alpha1.LabelManagedBy: miragev1alpha1.ManagedByValue,
		miragev1alpha1.LabelOwnerUID:  "uid-other",
	})
	if argoOwnedBy(pe, app) {
		t.Fatal("foreign owner must not be owned")
	}
	app.SetLabels(map[string]string{
		miragev1alpha1.LabelManagedBy: miragev1alpha1.ManagedByValue,
		miragev1alpha1.LabelOwnerUID:  "uid-1",
	})
	if !argoOwnedBy(pe, app) {
		t.Fatal("matching ownership should be owned")
	}
}

var _ = Describe("Argo Application lifecycle", func() {
	ctx := context.Background()

	It("uses unique Application names and refuses ownership takeover", func() {
		rec := &PreviewEnvironmentReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		peA := &miragev1alpha1.PreviewEnvironment{
			ObjectMeta: metav1.ObjectMeta{Name: "pr-42", Namespace: "default", UID: "uid-a"},
			Spec: miragev1alpha1.PreviewEnvironmentSpec{
				Image: "nginx:1", TargetNamespace: "preview-a-" + randomSuffix(),
				Backend: miragev1alpha1.BackendArgoCD,
				ArgoCD: &miragev1alpha1.ArgoCDSpec{
					RepoURL: "https://github.com/org/app", Path: "deploy", ArgoNamespace: "default",
				},
			},
		}

		conflictName := argoApplicationName(peA)
		foreign := &unstructured.Unstructured{}
		foreign.SetGroupVersionKind(argoApplicationGVK)
		foreign.SetName(conflictName)
		foreign.SetNamespace("default")
		foreign.SetLabels(map[string]string{
			miragev1alpha1.LabelManagedBy:      miragev1alpha1.ManagedByValue,
			miragev1alpha1.LabelOwnerUID:       "uid-other",
			miragev1alpha1.LabelOwnerName:      "other",
			miragev1alpha1.LabelOwnerNamespace: "other-ns",
		})
		Expect(k8sClient.Create(ctx, foreign)).To(Succeed())
		defer func() { _ = k8sClient.Delete(ctx, foreign) }()

		_, err := rec.ensureArgoApplication(ctx, peA)
		Expect(err).To(HaveOccurred())
		_, ok := err.(*argoOwnershipConflictError)
		Expect(ok).To(BeTrue())
	})

	It("deletes template-backed Argo Application on PE delete even when Spec.Backend is empty", func() {
		rec := &PreviewEnvironmentReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		name := types.NamespacedName{Name: "tpl-argo-del", Namespace: "default"}
		targetNS := "preview-tpl-argo-" + randomSuffix()

		tpl := &miragev1alpha1.PreviewTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "argo-backend-del", Namespace: "default"},
			Spec:       miragev1alpha1.PreviewTemplateSpec{Backend: miragev1alpha1.BackendArgoCD},
		}
		Expect(k8sClient.Create(ctx, tpl)).To(Succeed())
		defer func() { _ = k8sClient.Delete(ctx, tpl) }()

		pe := &miragev1alpha1.PreviewEnvironment{
			ObjectMeta: metav1.ObjectMeta{Name: name.Name, Namespace: name.Namespace},
			Spec: miragev1alpha1.PreviewEnvironmentSpec{
				Image: "nginx:1", TargetNamespace: targetNS,
				TemplateRef: &miragev1alpha1.TemplateRef{Name: "argo-backend-del"},
				ArgoCD: &miragev1alpha1.ArgoCDSpec{
					RepoURL: "https://github.com/org/app", Path: "deploy", ArgoNamespace: "default",
				},
			},
		}
		Expect(k8sClient.Create(ctx, pe)).To(Succeed())

		_, _ = rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(k8sClient.Get(ctx, name, pe)).To(Succeed())
		Expect(pe.Spec.Backend).To(BeEmpty())

		resolved, err := rec.resolvePreview(ctx, pe)
		Expect(err).NotTo(HaveOccurred())
		Expect(resolved.PE.Spec.Backend).To(Equal(miragev1alpha1.BackendArgoCD))
		resolved.PE.UID = pe.UID

		app, err := rec.ensureArgoApplication(ctx, resolved.PE)
		Expect(err).NotTo(HaveOccurred())
		appNN := types.NamespacedName{Name: app.GetName(), Namespace: app.GetNamespace()}
		Expect(app.GetName()).NotTo(Equal(pe.Name))

		Expect(k8sClient.Get(ctx, name, pe)).To(Succeed())
		pe.Status.ArgoApplication = app.GetNamespace() + "/" + app.GetName()
		Expect(k8sClient.Status().Update(ctx, pe)).To(Succeed())

		Expect(k8sClient.Delete(ctx, pe)).To(Succeed())
		Eventually(func(g Gomega) {
			forceRemoveNamespace(ctx, targetNS)
			_, err := rec.Reconcile(ctx, reconcile.Request{NamespacedName: name})
			g.Expect(err).NotTo(HaveOccurred())
			err = k8sClient.Get(ctx, name, &miragev1alpha1.PreviewEnvironment{})
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
		}, "10s", "250ms").Should(Succeed())

		got := &unstructured.Unstructured{}
		got.SetGroupVersionKind(argoApplicationGVK)
		err = k8sClient.Get(ctx, appNN, got)
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "Argo Application should be deleted")
	})

	It("deleting one preview does not delete another Application", func() {
		rec := &PreviewEnvironmentReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

		peA := &miragev1alpha1.PreviewEnvironment{
			ObjectMeta: metav1.ObjectMeta{Name: "pr-42", Namespace: "team-a", UID: "uid-team-a"},
			Spec: miragev1alpha1.PreviewEnvironmentSpec{
				Image: "nginx:1", TargetNamespace: "preview-team-a",
				Backend: miragev1alpha1.BackendArgoCD,
				ArgoCD: &miragev1alpha1.ArgoCDSpec{
					RepoURL: "https://github.com/org/app", Path: "deploy", ArgoNamespace: "default",
				},
			},
		}
		peB := &miragev1alpha1.PreviewEnvironment{
			ObjectMeta: metav1.ObjectMeta{Name: "pr-42", Namespace: "team-b", UID: "uid-team-b"},
			Spec: miragev1alpha1.PreviewEnvironmentSpec{
				Image: "nginx:1", TargetNamespace: "preview-team-b",
				Backend: miragev1alpha1.BackendArgoCD,
				ArgoCD: &miragev1alpha1.ArgoCDSpec{
					RepoURL: "https://github.com/org/app", Path: "deploy", ArgoNamespace: "default",
				},
			},
		}
		Expect(argoApplicationName(peA)).NotTo(Equal(argoApplicationName(peB)))

		appA, err := rec.ensureArgoApplication(ctx, peA)
		Expect(err).NotTo(HaveOccurred())
		appB, err := rec.ensureArgoApplication(ctx, peB)
		Expect(err).NotTo(HaveOccurred())
		Expect(appA.GetName()).NotTo(Equal(appB.GetName()))

		peA.Status.ArgoApplication = appA.GetNamespace() + "/" + appA.GetName()
		Expect(rec.deleteArgoApplication(ctx, peA)).To(Succeed())

		gotB := &unstructured.Unstructured{}
		gotB.SetGroupVersionKind(argoApplicationGVK)
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: appB.GetName(), Namespace: appB.GetNamespace()}, gotB)).To(Succeed())

		gotA := &unstructured.Unstructured{}
		gotA.SetGroupVersionKind(argoApplicationGVK)
		err = k8sClient.Get(ctx, types.NamespacedName{Name: appA.GetName(), Namespace: appA.GetNamespace()}, gotA)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())

		Expect(rec.deleteArgoApplication(ctx, peB)).To(Succeed())
	})

	It("cleanup never deletes unlabeled, foreign, or non-Mirage Applications", func() {
		rec := &PreviewEnvironmentReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		pe := &miragev1alpha1.PreviewEnvironment{
			ObjectMeta: metav1.ObjectMeta{Name: "production", Namespace: "default", UID: "uid-pe"},
			Spec: miragev1alpha1.PreviewEnvironmentSpec{
				Image: "nginx:1", TargetNamespace: "preview-prod",
				ArgoCD: &miragev1alpha1.ArgoCDSpec{ArgoNamespace: "default"},
			},
			Status: miragev1alpha1.PreviewEnvironmentStatus{
				ArgoApplication: "default/tampered-ref",
			},
		}

		createApp := func(name string, labels map[string]string) *unstructured.Unstructured {
			app := &unstructured.Unstructured{}
			app.SetGroupVersionKind(argoApplicationGVK)
			app.SetName(name)
			app.SetNamespace("default")
			if labels != nil {
				app.SetLabels(labels)
			}
			Expect(k8sClient.Create(ctx, app)).To(Succeed())
			return app
		}
		getApp := func(name string) error {
			got := &unstructured.Unstructured{}
			got.SetGroupVersionKind(argoApplicationGVK)
			return k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, got)
		}

		unlabeled := createApp("production", nil)
		defer func() { _ = k8sClient.Delete(ctx, unlabeled) }()

		otherMgr := createApp("other-mgr", map[string]string{
			miragev1alpha1.LabelManagedBy: "helm",
			miragev1alpha1.LabelOwnerUID:  string(pe.UID),
		})
		defer func() { _ = k8sClient.Delete(ctx, otherMgr) }()

		foreign := createApp("foreign-mirage", map[string]string{
			miragev1alpha1.LabelManagedBy:      miragev1alpha1.ManagedByValue,
			miragev1alpha1.LabelOwnerUID:       "uid-other",
			miragev1alpha1.LabelOwnerName:      "other",
			miragev1alpha1.LabelOwnerNamespace: "other-ns",
		})
		defer func() { _ = k8sClient.Delete(ctx, foreign) }()

		tampered := createApp("tampered-ref", map[string]string{
			miragev1alpha1.LabelManagedBy:      miragev1alpha1.ManagedByValue,
			miragev1alpha1.LabelOwnerUID:       "uid-other",
			miragev1alpha1.LabelOwnerName:      "other",
			miragev1alpha1.LabelOwnerNamespace: "other-ns",
		})
		defer func() { _ = k8sClient.Delete(ctx, tampered) }()

		ownedUniqueName := argoApplicationName(pe)
		_ = createApp(ownedUniqueName, map[string]string{
			miragev1alpha1.LabelManagedBy:      miragev1alpha1.ManagedByValue,
			miragev1alpha1.LabelOwnerUID:       string(pe.UID),
			miragev1alpha1.LabelOwnerName:      pe.Name,
			miragev1alpha1.LabelOwnerNamespace: pe.Namespace,
		})

		Expect(rec.deleteArgoApplication(ctx, pe)).To(Succeed())

		Expect(getApp("production")).To(Succeed())
		Expect(getApp("other-mgr")).To(Succeed())
		Expect(getApp("foreign-mirage")).To(Succeed())
		Expect(getApp("tampered-ref")).To(Succeed())
		Expect(apierrors.IsNotFound(getApp(ownedUniqueName))).To(BeTrue())

		Expect(k8sClient.Delete(ctx, unlabeled)).To(Succeed())
		Eventually(func() bool {
			return apierrors.IsNotFound(getApp("production"))
		}, "5s", "100ms").Should(BeTrue())

		legacyOwned := createApp("production", map[string]string{
			miragev1alpha1.LabelManagedBy:      miragev1alpha1.ManagedByValue,
			miragev1alpha1.LabelOwnerUID:       string(pe.UID),
			miragev1alpha1.LabelOwnerName:      pe.Name,
			miragev1alpha1.LabelOwnerNamespace: pe.Namespace,
		})
		Expect(rec.deleteArgoApplication(ctx, pe)).To(Succeed())
		Expect(apierrors.IsNotFound(getApp(legacyOwned.GetName()))).To(BeTrue())
	})

	It("preserves legacy status-referenced Application and does not create a second one", func() {
		rec := &PreviewEnvironmentReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		pe := &miragev1alpha1.PreviewEnvironment{
			ObjectMeta: metav1.ObjectMeta{Name: "pr-42", Namespace: "default", UID: "uid-legacy"},
			Spec: miragev1alpha1.PreviewEnvironmentSpec{
				Image: "nginx:1", TargetNamespace: "preview-legacy-argo",
				Backend: miragev1alpha1.BackendArgoCD,
				ArgoCD: &miragev1alpha1.ArgoCDSpec{
					RepoURL: "https://github.com/org/app", Path: "deploy", ArgoNamespace: "default",
				},
			},
			Status: miragev1alpha1.PreviewEnvironmentStatus{
				ArgoApplication: "default/pr-42",
			},
		}

		legacy := &unstructured.Unstructured{}
		legacy.SetGroupVersionKind(argoApplicationGVK)
		legacy.SetName("pr-42")
		legacy.SetNamespace("default")
		legacy.SetLabels(map[string]string{
			miragev1alpha1.LabelManagedBy:      miragev1alpha1.ManagedByValue,
			miragev1alpha1.LabelOwnerUID:       string(pe.UID),
			miragev1alpha1.LabelOwnerName:      pe.Name,
			miragev1alpha1.LabelOwnerNamespace: pe.Namespace,
		})
		Expect(k8sClient.Create(ctx, legacy)).To(Succeed())
		defer func() { _ = k8sClient.Delete(ctx, legacy) }()

		app, err := rec.ensureArgoApplication(ctx, pe)
		Expect(err).NotTo(HaveOccurred())
		Expect(app.GetName()).To(Equal("pr-42"))
		Expect(app.GetNamespace()).To(Equal("default"))

		genName := argoApplicationName(pe)
		Expect(genName).NotTo(Equal("pr-42"))
		got := &unstructured.Unstructured{}
		got.SetGroupVersionKind(argoApplicationGVK)
		err = k8sClient.Get(ctx, types.NamespacedName{Name: genName, Namespace: "default"}, got)
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "must not create second Application")

		peForeign := pe.DeepCopy()
		peForeign.UID = "uid-other"
		peForeign.Status.ArgoApplication = "default/pr-42"
		_, err = rec.desiredArgoApplicationRef(ctx, peForeign)
		Expect(err).To(HaveOccurred())
		_, ok := err.(*argoOwnershipConflictError)
		Expect(ok).To(BeTrue())
	})
})
