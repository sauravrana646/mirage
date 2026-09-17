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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	miragev1alpha1 "github.com/sauravrana646/mirage/api/v1alpha1"
	"github.com/sauravrana646/mirage/internal/resolve"
)

var blockedIngressAnnotationKeys = []string{
	"nginx.ingress.kubernetes.io/configuration-snippet",
	"nginx.ingress.kubernetes.io/server-snippet",
	"nginx.ingress.kubernetes.io/stream-snippet",
	"nginx.ingress.kubernetes.io/auth-snippet",
	"nginx.ingress.kubernetes.io/modsecurity-snippet",
}

// Reserved dependency service names (short names used today and mirage-dep-* prefixes
// being adopted for dependency Deployments/Services).
var dependencyReservedNames = map[string]string{
	"postgres":            "postgres",
	"mirage-dep-postgres": "postgres",
	"redis":               "redis",
	"mirage-dep-redis":    "redis",
	"kafka":               "kafka",
	"mirage-dep-kafka":    "kafka",
}

func SetupPreviewEnvironmentWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&miragev1alpha1.PreviewEnvironment{}).
		WithValidator(&PreviewEnvironmentCustomValidator{Client: mgr.GetClient()}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-mirage-dev-v1alpha1-previewenvironment,mutating=false,failurePolicy=fail,sideEffects=None,groups=mirage.dev,resources=previewenvironments,verbs=create;update,versions=v1alpha1,name=vpreviewenvironment-v1alpha1.kb.io,admissionReviewVersions=v1

// PreviewEnvironmentCustomValidator validates PreviewEnvironment admission.
type PreviewEnvironmentCustomValidator struct {
	Client client.Client
}

var _ webhook.CustomValidator = &PreviewEnvironmentCustomValidator{}

func (v *PreviewEnvironmentCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	pe, ok := obj.(*miragev1alpha1.PreviewEnvironment)
	if !ok {
		return nil, fmt.Errorf("expected PreviewEnvironment, got %T", obj)
	}
	return nil, v.validate(ctx, pe, nil)
}

func (v *PreviewEnvironmentCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	pe, ok := newObj.(*miragev1alpha1.PreviewEnvironment)
	if !ok {
		return nil, fmt.Errorf("expected PreviewEnvironment, got %T", newObj)
	}
	oldPE, _ := oldObj.(*miragev1alpha1.PreviewEnvironment)
	return nil, v.validate(ctx, pe, oldPE)
}

func (v *PreviewEnvironmentCustomValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (v *PreviewEnvironmentCustomValidator) validate(ctx context.Context, pe, old *miragev1alpha1.PreviewEnvironment) error {
	effective, err := v.effectivePreview(ctx, pe)
	if err != nil {
		return err
	}
	requireDigest := effective.Spec.RequireDigest || envTruthy("MIRAGE_REQUIRE_DIGEST")
	return validatePreviewEnvironment(effective, old, requireDigest)
}

// effectivePreview returns a DeepCopy of pe with PreviewTemplate defaults applied.
// All admission checks run against this effective object so template-derived
// ingress, TTL, backend, and digest policy are enforced.
func (v *PreviewEnvironmentCustomValidator) effectivePreview(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment) (*miragev1alpha1.PreviewEnvironment, error) {
	out := pe.DeepCopy()
	if pe.Spec.TemplateRef == nil || pe.Spec.TemplateRef.Name == "" {
		return out, nil
	}
	if v.Client == nil {
		return nil, fmt.Errorf("invalid PreviewEnvironment: cannot resolve templateRef without a client")
	}
	ns := pe.Spec.TemplateRef.Namespace
	if ns == "" {
		ns = pe.Namespace
	}
	tpl := &miragev1alpha1.PreviewTemplate{}
	if err := v.Client.Get(ctx, types.NamespacedName{Name: pe.Spec.TemplateRef.Name, Namespace: ns}, tpl); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("invalid PreviewEnvironment: templateRef %s/%s not found", ns, pe.Spec.TemplateRef.Name)
		}
		return nil, fmt.Errorf("invalid PreviewEnvironment: failed to fetch templateRef %s/%s: %w", ns, pe.Spec.TemplateRef.Name, err)
	}
	resolve.ApplyTemplate(out, tpl)
	return out, nil
}

func validatePreviewEnvironment(pe, old *miragev1alpha1.PreviewEnvironment, requireDigest bool) error {
	var errs []string
	errs = append(errs, validateBasics(pe, old)...)
	errs = append(errs, validateWorkloadLabels(pe)...)
	errs = append(errs, validateImagePolicy(pe, requireDigest)...)
	errs = append(errs, validateIngress(pe)...)
	errs = append(errs, validateDependencies(pe)...)
	errs = append(errs, validateBackend(pe)...)
	errs = append(errs, validateTTLAndReplicas(pe)...)
	if len(errs) > 0 {
		return fmt.Errorf("invalid PreviewEnvironment: %s", strings.Join(errs, "; "))
	}
	return nil
}

// reservedWorkloadLabelKeys must not appear in spec.workloadLabels.
var reservedWorkloadLabelKeys = []string{
	"app.kubernetes.io/name",
	"app.kubernetes.io/component",
	"app.kubernetes.io/managed-by",
	miragev1alpha1.LabelOwnerUID,
	miragev1alpha1.LabelOwnerName,
	miragev1alpha1.LabelOwnerNamespace,
	miragev1alpha1.LabelService,
	miragev1alpha1.LabelDependency,
}

func validateWorkloadLabels(pe *miragev1alpha1.PreviewEnvironment) []string {
	if len(pe.Spec.WorkloadLabels) == 0 {
		return nil
	}
	var errs []string
	for _, key := range reservedWorkloadLabelKeys {
		if _, ok := pe.Spec.WorkloadLabels[key]; ok {
			errs = append(errs, fmt.Sprintf("spec.workloadLabels key %q is reserved", key))
		}
	}
	return errs
}

func validateBasics(pe, old *miragev1alpha1.PreviewEnvironment) []string {
	var errs []string
	if pe.Spec.Image == "" && len(pe.Spec.Services) == 0 {
		errs = append(errs, "spec.image or spec.services is required")
	}
	if pe.Spec.TargetNamespace == "" {
		errs = append(errs, "spec.targetNamespace is required")
	} else if msgs := validation.IsDNS1123Label(pe.Spec.TargetNamespace); len(msgs) > 0 {
		errs = append(errs, fmt.Sprintf("spec.targetNamespace invalid: %v", msgs))
	}
	if old != nil && old.Spec.TargetNamespace != pe.Spec.TargetNamespace {
		errs = append(errs, "spec.targetNamespace is immutable")
	}
	seen := map[string]struct{}{}
	for i, svc := range pe.Spec.Services {
		if svc.Name == "" {
			errs = append(errs, fmt.Sprintf("services[%d].name is required", i))
			continue
		}
		if msgs := validation.IsDNS1123Label(svc.Name); len(msgs) > 0 {
			errs = append(errs, fmt.Sprintf("services[%d].name invalid: %v", i, msgs))
		}
		if _, dup := seen[svc.Name]; dup {
			errs = append(errs, fmt.Sprintf("duplicate service name %q", svc.Name))
		}
		seen[svc.Name] = struct{}{}
		if svc.Image == "" {
			errs = append(errs, fmt.Sprintf("services[%d].image is required", i))
		}
	}
	return errs
}

func validateImagePolicy(pe *miragev1alpha1.PreviewEnvironment, requireDigest bool) []string {
	var errs []string
	checkImage := func(img, field string) {
		if img == "" {
			return
		}
		if requireDigest && !strings.Contains(img, "@sha256:") {
			errs = append(errs, fmt.Sprintf("%s must use a sha256 digest when requireDigest is set", field))
		}
		allow := strings.TrimSpace(os.Getenv("MIRAGE_ALLOWED_REGISTRIES"))
		if allow == "" {
			return
		}
		ok := false
		for _, prefix := range strings.Split(allow, ",") {
			if imageMatchesRegistryPrefix(img, prefix) {
				ok = true
				break
			}
		}
		if !ok {
			errs = append(errs, fmt.Sprintf("%s registry not in allowlist (%s)", field, allow))
		}
	}
	checkImage(pe.Spec.Image, "spec.image")
	for i, svc := range pe.Spec.Services {
		checkImage(svc.Image, fmt.Sprintf("services[%d].image", i))
	}
	return errs
}

// imageMatchesRegistryPrefix reports whether image is allowed by an allowlist entry.
// Matching is path-boundary aware so "ghcr.io/acme" does not allow "ghcr.io/acme-evil/...".
func imageMatchesRegistryPrefix(image, prefix string) bool {
	prefix = strings.TrimRight(strings.TrimSpace(prefix), "/")
	if prefix == "" {
		return false
	}
	return image == prefix ||
		strings.HasPrefix(image, prefix+"/") ||
		strings.HasPrefix(image, prefix+":") ||
		strings.HasPrefix(image, prefix+"@")
}

func validateIngress(pe *miragev1alpha1.PreviewEnvironment) []string {
	var errs []string
	errs = append(errs, validateIngressSpec(pe.Spec.Ingress, "spec.ingress")...)
	for i, svc := range pe.Spec.Services {
		errs = append(errs, validateIngressSpec(svc.Ingress, fmt.Sprintf("services[%d].ingress", i))...)
	}
	return errs
}

func validateIngressSpec(ing *miragev1alpha1.IngressSpec, fieldPath string) []string {
	if ing == nil || !ing.Enabled {
		return nil
	}
	var errs []string
	if ing.Host == "" {
		errs = append(errs, fmt.Sprintf("%s.host is required when ingress.enabled=true", fieldPath))
	}
	if suffix := strings.TrimSpace(os.Getenv("MIRAGE_INGRESS_HOST_SUFFIX")); suffix != "" && ing.Host != "" {
		if !strings.HasSuffix(ing.Host, suffix) {
			errs = append(errs, fmt.Sprintf("%s.host must end with %q", fieldPath, suffix))
		}
	}
	if ing.TLS != nil && ing.TLS.Enabled {
		if ing.TLS.SecretName == "" && ing.Host == "" {
			errs = append(errs, fmt.Sprintf("%s.tls requires host or secretName", fieldPath))
		}
	}
	for key := range ing.Annotations {
		lk := strings.ToLower(key)
		for _, bad := range blockedIngressAnnotationKeys {
			if lk == bad {
				errs = append(errs, fmt.Sprintf("%s.annotations key %q is not allowed", fieldPath, key))
			}
		}
	}
	return errs
}

func validateDependencies(pe *miragev1alpha1.PreviewEnvironment) []string {
	deps := pe.Spec.Dependencies
	if deps == nil {
		return nil
	}
	errs := make([]string, 0, 4)
	if deps.Postgres != nil && strings.TrimSpace(deps.Postgres.Storage) != "" {
		errs = append(errs, "dependencies.postgres.storage is not supported yet (emptyDir only)")
	}
	enabled := map[string]bool{}
	if deps.Postgres != nil && deps.Postgres.Enabled {
		enabled["postgres"] = true
	}
	if deps.Redis != nil && deps.Redis.Enabled {
		enabled["redis"] = true
	}
	if deps.Kafka != nil && deps.Kafka.Enabled {
		enabled["kafka"] = true
	}
	if len(enabled) == 0 {
		return errs
	}
	for i, svc := range pe.Spec.Services {
		depKind, reserved := dependencyReservedNames[svc.Name]
		if !reserved || !enabled[depKind] {
			continue
		}
		errs = append(errs, fmt.Sprintf("services[%d].name %q conflicts with dependencies.%s", i, svc.Name, depKind))
	}
	return errs
}

func validateBackend(pe *miragev1alpha1.PreviewEnvironment) []string {
	if pe.Spec.Backend != miragev1alpha1.BackendArgoCD {
		return nil
	}
	var errs []string
	if pe.Spec.ArgoCD == nil || pe.Spec.ArgoCD.RepoURL == "" || pe.Spec.ArgoCD.Path == "" {
		errs = append(errs, "spec.argoCD.repoURL and path are required when backend=argocd")
	} else {
		dest := pe.Spec.ArgoCD.DestinationNamespace
		if dest == "" {
			dest = pe.Spec.TargetNamespace
		}
		if dest != pe.Spec.TargetNamespace {
			errs = append(errs, "spec.argoCD.destinationNamespace must equal spec.targetNamespace")
		}
	}
	if dependenciesEnabled(pe.Spec.Dependencies) {
		errs = append(errs, "dependencies are not supported when backend=argocd")
	}
	return errs
}

func dependenciesEnabled(deps *miragev1alpha1.PreviewDependenciesSpec) bool {
	if deps == nil {
		return false
	}
	if deps.Postgres != nil && deps.Postgres.Enabled {
		return true
	}
	if deps.Redis != nil && deps.Redis.Enabled {
		return true
	}
	if deps.Kafka != nil && deps.Kafka.Enabled {
		return true
	}
	return false
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
		ttl := effectiveTTLSeconds(pe)
		if ttl > maxTTL {
			errs = append(errs, fmt.Sprintf("effective ttlSeconds %d exceeds max allowed %d", ttl, maxTTL))
		}
	}
	return errs
}

func effectiveTTLSeconds(pe *miragev1alpha1.PreviewEnvironment) int64 {
	if pe.Spec.TTLSeconds != nil {
		return *pe.Spec.TTLSeconds
	}
	return defaultTTLFromEnv()
}

func envTruthy(key string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	return v == "1" || v == "true" || v == "yes"
}

func maxTTLFromEnv() int64 {
	return parseIntEnv("MIRAGE_MAX_TTL_SECONDS")
}

func defaultTTLFromEnv() int64 {
	return parseIntEnv("MIRAGE_DEFAULT_TTL_SECONDS")
}

func parseIntEnv(key string) int64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v < 0 {
		return 0
	}
	return v
}
