package v1alpha1

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	miragev1alpha1 "github.com/sauravrana646/mirage/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("PreviewEnvironment Webhook", func() {
	It("rejects empty image", func() {
		pe := &miragev1alpha1.PreviewEnvironment{
			ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "default"},
			Spec:       miragev1alpha1.PreviewEnvironmentSpec{TargetNamespace: "preview-x"},
		}
		Expect(validatePreviewEnvironment(pe, nil)).To(HaveOccurred())
	})

	It("requires digest when RequireDigest set", func() {
		pe := &miragev1alpha1.PreviewEnvironment{
			ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "default"},
			Spec: miragev1alpha1.PreviewEnvironmentSpec{
				Image: "ghcr.io/org/app:latest", TargetNamespace: "preview-x", RequireDigest: true,
			},
		}
		Expect(validatePreviewEnvironment(pe, nil)).To(HaveOccurred())
		pe.Spec.Image = "ghcr.io/org/app@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		Expect(validatePreviewEnvironment(pe, nil)).NotTo(HaveOccurred())
	})

	It("rejects targetNamespace mutation", func() {
		old := &miragev1alpha1.PreviewEnvironment{
			ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "default"},
			Spec:       miragev1alpha1.PreviewEnvironmentSpec{Image: "nginx:1", TargetNamespace: "preview-a"},
		}
		neu := old.DeepCopy()
		neu.Spec.TargetNamespace = "preview-b"
		Expect(validatePreviewEnvironment(neu, old)).To(HaveOccurred())
	})

	It("requires argoCD fields when backend is argocd", func() {
		pe := &miragev1alpha1.PreviewEnvironment{
			ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "default"},
			Spec: miragev1alpha1.PreviewEnvironmentSpec{
				Image: "nginx:1", TargetNamespace: "preview-x", Backend: miragev1alpha1.BackendArgoCD,
			},
		}
		Expect(validatePreviewEnvironment(pe, nil)).To(HaveOccurred())
		pe.Spec.ArgoCD = &miragev1alpha1.ArgoCDSpec{RepoURL: "https://github.com/org/app", Path: "deploy"}
		Expect(validatePreviewEnvironment(pe, nil)).NotTo(HaveOccurred())
	})

	It("enforces MIRAGE_MAX_TTL_SECONDS", func() {
		Expect(os.Setenv("MIRAGE_MAX_TTL_SECONDS", "3600")).To(Succeed())
		DeferCleanup(func() { _ = os.Unsetenv("MIRAGE_MAX_TTL_SECONDS") })
		ttl := int64(7200)
		pe := &miragev1alpha1.PreviewEnvironment{
			ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "default"},
			Spec: miragev1alpha1.PreviewEnvironmentSpec{
				Image: "nginx:1", TargetNamespace: "preview-x", TTLSeconds: &ttl,
			},
		}
		Expect(validatePreviewEnvironment(pe, nil)).To(HaveOccurred())
		ttl = 1800
		Expect(validatePreviewEnvironment(pe, nil)).NotTo(HaveOccurred())
	})

	It("rejects argo destination outside targetNamespace", func() {
		pe := &miragev1alpha1.PreviewEnvironment{
			ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "default"},
			Spec: miragev1alpha1.PreviewEnvironmentSpec{
				Image: "nginx:1", TargetNamespace: "preview-x", Backend: miragev1alpha1.BackendArgoCD,
				ArgoCD: &miragev1alpha1.ArgoCDSpec{
					RepoURL: "https://github.com/org/app", Path: "deploy", DestinationNamespace: "kube-system",
				},
			},
		}
		Expect(validatePreviewEnvironment(pe, nil)).To(HaveOccurred())
		pe.Spec.ArgoCD.DestinationNamespace = "preview-x"
		Expect(validatePreviewEnvironment(pe, nil)).NotTo(HaveOccurred())
	})

	It("rejects dangerous ingress annotation snippets", func() {
		pe := &miragev1alpha1.PreviewEnvironment{
			ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "default"},
			Spec: miragev1alpha1.PreviewEnvironmentSpec{
				Image: "nginx:1", TargetNamespace: "preview-x",
				Ingress: &miragev1alpha1.IngressSpec{
					Enabled: true, Host: "pr.example.com",
					Annotations: map[string]string{
						"nginx.ingress.kubernetes.io/configuration-snippet": "more_set_headers \"X-Evil: 1\";",
					},
				},
			},
		}
		Expect(validatePreviewEnvironment(pe, nil)).To(HaveOccurred())
	})
})
