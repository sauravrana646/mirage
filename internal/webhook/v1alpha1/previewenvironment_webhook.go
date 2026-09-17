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

package v1alpha1

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	miragev1alpha1 "github.com/sauravrana646/mirage/api/v1alpha1"
)

func SetupPreviewEnvironmentWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&miragev1alpha1.PreviewEnvironment{}).
		WithValidator(&PreviewEnvironmentCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-mirage-dev-v1alpha1-previewenvironment,mutating=false,failurePolicy=fail,sideEffects=None,groups=mirage.dev,resources=previewenvironments,verbs=create;update,versions=v1alpha1,name=vpreviewenvironment-v1alpha1.kb.io,admissionReviewVersions=v1

type PreviewEnvironmentCustomValidator struct{}

var _ webhook.CustomValidator = &PreviewEnvironmentCustomValidator{}

func (v *PreviewEnvironmentCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	pe, ok := obj.(*miragev1alpha1.PreviewEnvironment)
	if !ok {
		return nil, fmt.Errorf("expected PreviewEnvironment, got %T", obj)
	}
	return nil, validatePreviewEnvironment(pe, nil)
}

func (v *PreviewEnvironmentCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	pe, ok := newObj.(*miragev1alpha1.PreviewEnvironment)
	if !ok {
		return nil, fmt.Errorf("expected PreviewEnvironment, got %T", newObj)
	}
	oldPE, _ := oldObj.(*miragev1alpha1.PreviewEnvironment)
	return nil, validatePreviewEnvironment(pe, oldPE)
}

func (v *PreviewEnvironmentCustomValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func validatePreviewEnvironment(pe, old *miragev1alpha1.PreviewEnvironment) error {
	var errs []string
	errs = append(errs, validateBasics(pe, old)...)
	errs = append(errs, validateImagePolicy(pe)...)
	errs = append(errs, validateIngress(pe)...)
	errs = append(errs, validateBackend(pe)...)
	errs = append(errs, validateTTLAndReplicas(pe)...)
	if len(errs) > 0 {
		return fmt.Errorf("invalid PreviewEnvironment: %s", strings.Join(errs, "; "))
	}
	return nil
}

func validateBasics(pe, old *miragev1alpha1.PreviewEnvironment) []string {
	var errs []string
	if pe.Spec.Image == "" {
		errs = append(errs, "spec.image is required")
	}
	if pe.Spec.TargetNamespace == "" {
		errs = append(errs, "spec.targetNamespace is required")
	} else if msgs := validation.IsDNS1123Label(pe.Spec.TargetNamespace); len(msgs) > 0 {
		errs = append(errs, fmt.Sprintf("spec.targetNamespace invalid: %v", msgs))
	}
	if old != nil && old.Spec.TargetNamespace != pe.Spec.TargetNamespace {
		errs = append(errs, "spec.targetNamespace is immutable")
	}
	return errs
}

func validateImagePolicy(pe *miragev1alpha1.PreviewEnvironment) []string {
	var errs []string
	if pe.Spec.RequireDigest || envTruthy("MIRAGE_REQUIRE_DIGEST") {
		if !strings.Contains(pe.Spec.Image, "@sha256:") {
			errs = append(errs, "spec.image must use a sha256 digest when requireDigest is set")
		}
	}
	allow := strings.TrimSpace(os.Getenv("MIRAGE_ALLOWED_REGISTRIES"))
	if allow == "" || pe.Spec.Image == "" {
		return errs
	}
	ok := false
	for _, prefix := range strings.Split(allow, ",") {
		prefix = strings.TrimSpace(prefix)
		if prefix != "" && strings.HasPrefix(pe.Spec.Image, prefix) {
			ok = true
			break
		}
	}
	if !ok {
		errs = append(errs, fmt.Sprintf("spec.image registry not in allowlist (%s)", allow))
	}
	return errs
}

func validateIngress(pe *miragev1alpha1.PreviewEnvironment) []string {
	var errs []string
	if pe.Spec.Ingress == nil || !pe.Spec.Ingress.Enabled {
		return errs
	}
	if pe.Spec.Ingress.Host == "" {
		errs = append(errs, "spec.ingress.host is required when ingress.enabled=true")
	}
	if suffix := strings.TrimSpace(os.Getenv("MIRAGE_INGRESS_HOST_SUFFIX")); suffix != "" {
		if !strings.HasSuffix(pe.Spec.Ingress.Host, suffix) {
			errs = append(errs, fmt.Sprintf("spec.ingress.host must end with %q", suffix))
		}
	}
	if pe.Spec.Ingress.TLS != nil && pe.Spec.Ingress.TLS.Enabled {
		if pe.Spec.Ingress.TLS.SecretName == "" && pe.Spec.Ingress.Host == "" {
			errs = append(errs, "spec.ingress.tls requires host or secretName")
		}
	}
	return errs
}

func validateBackend(pe *miragev1alpha1.PreviewEnvironment) []string {
	if pe.Spec.Backend != miragev1alpha1.BackendArgoCD {
		return nil
	}
	if pe.Spec.ArgoCD == nil || pe.Spec.ArgoCD.RepoURL == "" || pe.Spec.ArgoCD.Path == "" {
		return []string{"spec.argoCD.repoURL and path are required when backend=argocd"}
	}
	return nil
}

func validateTTLAndReplicas(pe *miragev1alpha1.PreviewEnvironment) []string {
	var errs []string
	if pe.Spec.Replicas != nil && (*pe.Spec.Replicas < 0 || *pe.Spec.Replicas > 10) {
		errs = append(errs, "spec.replicas must be between 0 and 10")
	}
	if pe.Spec.TTLSeconds != nil && *pe.Spec.TTLSeconds < 0 {
		errs = append(errs, "spec.ttlSeconds must be >= 0")
	}
	if maxTTL := maxTTLFromEnv(); maxTTL > 0 {
		ttl := int64(0)
		if pe.Spec.TTLSeconds != nil {
			ttl = *pe.Spec.TTLSeconds
		}
		if ttl > maxTTL {
			errs = append(errs, fmt.Sprintf("spec.ttlSeconds %d exceeds max allowed %d", ttl, maxTTL))
		}
	}
	return errs
}

func envTruthy(key string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	return v == "1" || v == "true" || v == "yes"
}

func maxTTLFromEnv() int64 {
	raw := strings.TrimSpace(os.Getenv("MIRAGE_MAX_TTL_SECONDS"))
	if raw == "" {
		return 0
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v < 0 {
		return 0
	}
	return v
}
