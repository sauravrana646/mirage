package v1alpha1

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	miragev1alpha1 "github.com/sauravrana646/mirage/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func requireDigestFlag(pe *miragev1alpha1.PreviewEnvironment) bool {
	return pe.Spec.RequireDigest || envTruthy("MIRAGE_REQUIRE_DIGEST")
}

var _ = Describe("PreviewEnvironment Webhook", func() {
	It("rejects empty image", func() {
		pe := &miragev1alpha1.PreviewEnvironment{
			ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "default"},
			Spec:       miragev1alpha1.PreviewEnvironmentSpec{TargetNamespace: "preview-x"},
		}
		Expect(validatePreviewEnvironment(pe, nil, requireDigestFlag(pe))).To(HaveOccurred())
	})

	It("requires digest when RequireDigest set", func() {
		pe := &miragev1alpha1.PreviewEnvironment{
			ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "default"},
			Spec: miragev1alpha1.PreviewEnvironmentSpec{
				Image: "ghcr.io/org/app:latest", TargetNamespace: "preview-x", RequireDigest: true,
			},
		}
		Expect(validatePreviewEnvironment(pe, nil, true)).To(HaveOccurred())
		pe.Spec.Image = "ghcr.io/org/app@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		Expect(validatePreviewEnvironment(pe, nil, true)).NotTo(HaveOccurred())
	})

	It("rejects targetNamespace mutation", func() {
		old := &miragev1alpha1.PreviewEnvironment{
			ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "default"},
			Spec:       miragev1alpha1.PreviewEnvironmentSpec{Image: "nginx:1", TargetNamespace: "preview-a"},
		}
		neu := old.DeepCopy()
		neu.Spec.TargetNamespace = "preview-b"
		Expect(validatePreviewEnvironment(neu, old, false)).To(HaveOccurred())
	})

	It("requires argoCD fields when backend is argocd", func() {
		pe := &miragev1alpha1.PreviewEnvironment{
			ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "default"},
			Spec: miragev1alpha1.PreviewEnvironmentSpec{
				Image: "nginx:1", TargetNamespace: "preview-x", Backend: miragev1alpha1.BackendArgoCD,
			},
		}
		Expect(validatePreviewEnvironment(pe, nil, false)).To(HaveOccurred())
		pe.Spec.ArgoCD = &miragev1alpha1.ArgoCDSpec{RepoURL: "https://github.com/org/app", Path: "deploy"}
		Expect(validatePreviewEnvironment(pe, nil, false)).NotTo(HaveOccurred())
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
		Expect(validatePreviewEnvironment(pe, nil, false)).To(HaveOccurred())
		ttl = 1800
		Expect(validatePreviewEnvironment(pe, nil, false)).NotTo(HaveOccurred())
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
		Expect(validatePreviewEnvironment(pe, nil, false)).To(HaveOccurred())
		pe.Spec.ArgoCD.DestinationNamespace = "preview-x"
		Expect(validatePreviewEnvironment(pe, nil, false)).NotTo(HaveOccurred())
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
		Expect(validatePreviewEnvironment(pe, nil, false)).To(HaveOccurred())
	})

	It("rejects forbidden annotations on service-level ingress", func() {
		pe := &miragev1alpha1.PreviewEnvironment{
			ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "default"},
			Spec: miragev1alpha1.PreviewEnvironmentSpec{
				TargetNamespace: "preview-x",
				Services: []miragev1alpha1.PreviewServiceSpec{{
					Name:  "api",
					Image: "nginx:1",
					Ingress: &miragev1alpha1.IngressSpec{
						Enabled: true, Host: "api.example.com",
						Annotations: map[string]string{
							"nginx.ingress.kubernetes.io/server-snippet": "return 200;",
						},
					},
				}},
			},
		}
		err := validatePreviewEnvironment(pe, nil, false)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("services[0].ingress.annotations"))
	})

	It("enforces MIRAGE_INGRESS_HOST_SUFFIX on service-level ingress", func() {
		Expect(os.Setenv("MIRAGE_INGRESS_HOST_SUFFIX", ".previews.example.com")).To(Succeed())
		DeferCleanup(func() { _ = os.Unsetenv("MIRAGE_INGRESS_HOST_SUFFIX") })
		pe := &miragev1alpha1.PreviewEnvironment{
			ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "default"},
			Spec: miragev1alpha1.PreviewEnvironmentSpec{
				TargetNamespace: "preview-x",
				Services: []miragev1alpha1.PreviewServiceSpec{{
					Name:  "web",
					Image: "nginx:1",
					Ingress: &miragev1alpha1.IngressSpec{
						Enabled: true, Host: "web.evil.com",
					},
				}},
			},
		}
		err := validatePreviewEnvironment(pe, nil, false)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("services[0].ingress.host must end with"))
		pe.Spec.Services[0].Ingress.Host = "web.previews.example.com"
		Expect(validatePreviewEnvironment(pe, nil, false)).NotTo(HaveOccurred())
	})

	It("rejects service names that collide with enabled dependencies", func() {
		pe := &miragev1alpha1.PreviewEnvironment{
			ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "default"},
			Spec: miragev1alpha1.PreviewEnvironmentSpec{
				TargetNamespace: "preview-x",
				Services: []miragev1alpha1.PreviewServiceSpec{
					{Name: "postgres", Image: "nginx:1"},
					{Name: "mirage-dep-redis", Image: "nginx:1"},
				},
				Dependencies: &miragev1alpha1.PreviewDependenciesSpec{
					Postgres: &miragev1alpha1.PostgresDependencySpec{Enabled: true},
					Redis:    &miragev1alpha1.RedisDependencySpec{Enabled: true},
				},
			},
		}
		err := validatePreviewEnvironment(pe, nil, false)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("conflicts with dependencies.postgres"))
		Expect(err.Error()).To(ContainSubstring("conflicts with dependencies.redis"))
	})

	It("rejects dependencies.postgres.storage until PVC support exists", func() {
		pe := &miragev1alpha1.PreviewEnvironment{
			ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "default"},
			Spec: miragev1alpha1.PreviewEnvironmentSpec{
				Image: "nginx:1", TargetNamespace: "preview-x",
				Dependencies: &miragev1alpha1.PreviewDependenciesSpec{
					Postgres: &miragev1alpha1.PostgresDependencySpec{
						Enabled: true, Storage: "1Gi",
					},
				},
			},
		}
		err := validatePreviewEnvironment(pe, nil, false)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("dependencies.postgres.storage is not supported yet"))
	})

	It("enforces MIRAGE_ALLOWED_REGISTRIES with path-boundary matching", func() {
		Expect(os.Setenv("MIRAGE_ALLOWED_REGISTRIES", "ghcr.io/acme")).To(Succeed())
		DeferCleanup(func() { _ = os.Unsetenv("MIRAGE_ALLOWED_REGISTRIES") })

		allowed := []string{
			"ghcr.io/acme/application",
			"ghcr.io/acme/team/application",
		}
		for _, img := range allowed {
			pe := &miragev1alpha1.PreviewEnvironment{
				ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "default"},
				Spec:       miragev1alpha1.PreviewEnvironmentSpec{Image: img, TargetNamespace: "preview-x"},
			}
			Expect(validatePreviewEnvironment(pe, nil, false)).NotTo(HaveOccurred(), img)
		}

		rejected := []string{
			"ghcr.io/acme-evil/application",
			"ghcr.io/acmeattacker/application",
		}
		for _, img := range rejected {
			pe := &miragev1alpha1.PreviewEnvironment{
				ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "default"},
				Spec:       miragev1alpha1.PreviewEnvironmentSpec{Image: img, TargetNamespace: "preview-x"},
			}
			err := validatePreviewEnvironment(pe, nil, false)
			Expect(err).To(HaveOccurred(), img)
			Expect(err.Error()).To(ContainSubstring("registry not in allowlist"), img)
		}
	})

	It("allows repository prefixes with tags and digests", func() {
		Expect(os.Setenv("MIRAGE_ALLOWED_REGISTRIES", "ghcr.io/acme/application")).To(Succeed())
		DeferCleanup(func() { _ = os.Unsetenv("MIRAGE_ALLOWED_REGISTRIES") })

		allowed := []string{
			"ghcr.io/acme/application:v1",
			"ghcr.io/acme/application@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}
		for _, img := range allowed {
			pe := &miragev1alpha1.PreviewEnvironment{
				ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "default"},
				Spec:       miragev1alpha1.PreviewEnvironmentSpec{Image: img, TargetNamespace: "preview-x"},
			}
			Expect(validatePreviewEnvironment(pe, nil, false)).NotTo(HaveOccurred(), img)
		}

		// Nested path under a full-repo allowlist entry is not a match for prefix+"/".
		Expect(os.Setenv("MIRAGE_ALLOWED_REGISTRIES", "ghcr.io/acme")).To(Succeed())
		pe := &miragev1alpha1.PreviewEnvironment{
			ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "default"},
			Spec: miragev1alpha1.PreviewEnvironmentSpec{
				Image: "ghcr.io/acme/team/application:v1", TargetNamespace: "preview-x",
			},
		}
		Expect(validatePreviewEnvironment(pe, nil, false)).NotTo(HaveOccurred())

		rejected := []string{
			"ghcr.io/acme-evil/application:v1",
			"ghcr.io/acmeattacker/application:v1",
		}
		for _, img := range rejected {
			pe := &miragev1alpha1.PreviewEnvironment{
				ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "default"},
				Spec:       miragev1alpha1.PreviewEnvironmentSpec{Image: img, TargetNamespace: "preview-x"},
			}
			Expect(validatePreviewEnvironment(pe, nil, false)).To(HaveOccurred(), img)
		}
	})

	It("rejects reserved keys in workloadLabels", func() {
		pe := &miragev1alpha1.PreviewEnvironment{
			ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "default"},
			Spec: miragev1alpha1.PreviewEnvironmentSpec{
				Image: "nginx:1", TargetNamespace: "preview-x",
				WorkloadLabels: map[string]string{
					"app.kubernetes.io/name": "hijack",
					"custom.io/ok":           "yes",
				},
			},
		}
		err := validatePreviewEnvironment(pe, nil, false)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("spec.workloadLabels key"))
		Expect(err.Error()).To(ContainSubstring("reserved"))

		pe.Spec.WorkloadLabels = map[string]string{"custom.io/ok": "yes"}
		Expect(validatePreviewEnvironment(pe, nil, false)).NotTo(HaveOccurred())
	})

	It("enforces template requireDigest via client lookup", func() {
		scheme := runtime.NewScheme()
		Expect(miragev1alpha1.AddToScheme(scheme)).To(Succeed())
		tpl := &miragev1alpha1.PreviewTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "secure", Namespace: "default"},
			Spec:       miragev1alpha1.PreviewTemplateSpec{RequireDigest: true},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tpl).Build()
		v := &PreviewEnvironmentCustomValidator{Client: c}
		pe := &miragev1alpha1.PreviewEnvironment{
			ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "default"},
			Spec: miragev1alpha1.PreviewEnvironmentSpec{
				Image: "ghcr.io/org/app:latest", TargetNamespace: "preview-x",
				TemplateRef: &miragev1alpha1.TemplateRef{Name: "secure"},
				Services: []miragev1alpha1.PreviewServiceSpec{
					{Name: "api", Image: "ghcr.io/org/api:v1"},
				},
			},
		}
		err := v.validate(context.Background(), pe, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("must use a sha256 digest"))
		pe.Spec.Image = "ghcr.io/org/app@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		pe.Spec.Services[0].Image = "ghcr.io/org/api@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		Expect(v.validate(context.Background(), pe, nil)).NotTo(HaveOccurred())
	})

	It("rejects template ingress with forbidden nginx snippet annotations", func() {
		scheme := runtime.NewScheme()
		Expect(miragev1alpha1.AddToScheme(scheme)).To(Succeed())
		tpl := &miragev1alpha1.PreviewTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "snip-tpl", Namespace: "default"},
			Spec: miragev1alpha1.PreviewTemplateSpec{
				Ingress: &miragev1alpha1.IngressTemplateSpec{
					Enabled: true,
					Domain:  "previews.example.com",
					Annotations: map[string]string{
						"nginx.ingress.kubernetes.io/configuration-snippet": "more_set_headers \"X-Evil: 1\";",
					},
				},
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tpl).Build()
		v := &PreviewEnvironmentCustomValidator{Client: c}
		pe := &miragev1alpha1.PreviewEnvironment{
			ObjectMeta: metav1.ObjectMeta{Name: "pr-1", Namespace: "default"},
			Spec: miragev1alpha1.PreviewEnvironmentSpec{
				Image: "nginx:1", TargetNamespace: "preview-pr-1",
				TemplateRef: &miragev1alpha1.TemplateRef{Name: "snip-tpl"},
			},
		}
		err := v.validate(context.Background(), pe, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("configuration-snippet"))
	})

	It("rejects template ingress domain outside MIRAGE_INGRESS_HOST_SUFFIX", func() {
		Expect(os.Setenv("MIRAGE_INGRESS_HOST_SUFFIX", ".previews.example.com")).To(Succeed())
		DeferCleanup(func() { _ = os.Unsetenv("MIRAGE_INGRESS_HOST_SUFFIX") })

		scheme := runtime.NewScheme()
		Expect(miragev1alpha1.AddToScheme(scheme)).To(Succeed())
		tpl := &miragev1alpha1.PreviewTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "bad-domain", Namespace: "default"},
			Spec: miragev1alpha1.PreviewTemplateSpec{
				Ingress: &miragev1alpha1.IngressTemplateSpec{
					Enabled: true,
					Domain:  "evil.example.com",
				},
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tpl).Build()
		v := &PreviewEnvironmentCustomValidator{Client: c}
		pe := &miragev1alpha1.PreviewEnvironment{
			ObjectMeta: metav1.ObjectMeta{Name: "pr-2", Namespace: "default"},
			Spec: miragev1alpha1.PreviewEnvironmentSpec{
				Image: "nginx:1", TargetNamespace: "preview-pr-2",
				TemplateRef: &miragev1alpha1.TemplateRef{Name: "bad-domain"},
			},
		}
		err := v.validate(context.Background(), pe, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("spec.ingress.host must end with"))
	})

	It("rejects template ttlSeconds exceeding MIRAGE_MAX_TTL_SECONDS when PE omits ttl", func() {
		Expect(os.Setenv("MIRAGE_MAX_TTL_SECONDS", "3600")).To(Succeed())
		DeferCleanup(func() { _ = os.Unsetenv("MIRAGE_MAX_TTL_SECONDS") })

		scheme := runtime.NewScheme()
		Expect(miragev1alpha1.AddToScheme(scheme)).To(Succeed())
		ttl := int64(7200)
		tpl := &miragev1alpha1.PreviewTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "long-ttl", Namespace: "default"},
			Spec:       miragev1alpha1.PreviewTemplateSpec{TTLSeconds: &ttl},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tpl).Build()
		v := &PreviewEnvironmentCustomValidator{Client: c}
		pe := &miragev1alpha1.PreviewEnvironment{
			ObjectMeta: metav1.ObjectMeta{Name: "pr-3", Namespace: "default"},
			Spec: miragev1alpha1.PreviewEnvironmentSpec{
				Image: "nginx:1", TargetNamespace: "preview-pr-3",
				TemplateRef: &miragev1alpha1.TemplateRef{Name: "long-ttl"},
			},
		}
		err := v.validate(context.Background(), pe, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("exceeds max allowed"))
	})

	It("rejects argocd backend with enabled dependencies", func() {
		pe := &miragev1alpha1.PreviewEnvironment{
			ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "default"},
			Spec: miragev1alpha1.PreviewEnvironmentSpec{
				Image: "nginx:1", TargetNamespace: "preview-x",
				Backend: miragev1alpha1.BackendArgoCD,
				ArgoCD:  &miragev1alpha1.ArgoCDSpec{RepoURL: "https://github.com/org/app", Path: "deploy"},
				Dependencies: &miragev1alpha1.PreviewDependenciesSpec{
					Redis: &miragev1alpha1.RedisDependencySpec{Enabled: true},
				},
			},
		}
		err := validatePreviewEnvironment(pe, nil, false)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("dependencies are not supported when backend=argocd"))
	})

	It("rejects template-defaulted argocd backend without argoCD fields", func() {
		scheme := runtime.NewScheme()
		Expect(miragev1alpha1.AddToScheme(scheme)).To(Succeed())
		tpl := &miragev1alpha1.PreviewTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "argo-tpl", Namespace: "default"},
			Spec:       miragev1alpha1.PreviewTemplateSpec{Backend: miragev1alpha1.BackendArgoCD},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tpl).Build()
		v := &PreviewEnvironmentCustomValidator{Client: c}
		pe := &miragev1alpha1.PreviewEnvironment{
			ObjectMeta: metav1.ObjectMeta{Name: "pr-argo", Namespace: "default"},
			Spec: miragev1alpha1.PreviewEnvironmentSpec{
				Image: "nginx:1", TargetNamespace: "preview-pr-argo",
				TemplateRef: &miragev1alpha1.TemplateRef{Name: "argo-tpl"},
			},
		}
		err := v.validate(context.Background(), pe, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("spec.argoCD.repoURL and path are required"))
	})
})
