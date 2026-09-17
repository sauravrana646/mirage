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

// Package resolve applies PreviewTemplate defaults and derives effective services.
// Pure functions shared by the controller and cost estimator (no kube client I/O).
package resolve

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation"

	miragev1alpha1 "github.com/sauravrana646/mirage/api/v1alpha1"
)

// Defaults matching the controller when fields are unset.
const (
	DefaultReplicas      int32 = 1
	DefaultContainerPort int32 = 8080
)

// EffectiveService is one workload after template/defaults merging.
type EffectiveService struct {
	Name            string
	Image           string
	Port            int32
	Replicas        int32
	Env             []corev1.EnvVar
	EnvFrom         []corev1.EnvFromSource
	Resources       corev1.ResourceRequirements
	Command         []string
	Args            []string
	ImagePullPolicy corev1.PullPolicy
	ReadinessProbe  *miragev1alpha1.ProbeSpec
	LivenessProbe   *miragev1alpha1.ProbeSpec
	Ingress         *miragev1alpha1.IngressSpec
	// Legacy single-service mode uses the PreviewEnvironment name as the Deployment name.
	WorkloadName string
}

// ApplyTemplate merges PreviewTemplate defaults into pe (in place).
// PreviewEnvironment fields win when already set.
func ApplyTemplate(pe *miragev1alpha1.PreviewEnvironment, tpl *miragev1alpha1.PreviewTemplate) {
	if pe == nil || tpl == nil {
		return
	}
	s := tpl.Spec
	if pe.Spec.NetworkPolicy == "" && s.NetworkPolicy != "" {
		pe.Spec.NetworkPolicy = s.NetworkPolicy
	}
	if pe.Spec.TTLSeconds == nil && s.TTLSeconds != nil {
		pe.Spec.TTLSeconds = s.TTLSeconds
	}
	if pe.Spec.Replicas == nil && s.Replicas != nil {
		pe.Spec.Replicas = s.Replicas
	}
	if pe.Spec.ContainerPort == 0 && s.ContainerPort != 0 {
		pe.Spec.ContainerPort = s.ContainerPort
	}
	if pe.Spec.Backend == "" && s.Backend != "" {
		pe.Spec.Backend = s.Backend
	}
	if !pe.Spec.RequireDigest && s.RequireDigest {
		pe.Spec.RequireDigest = true
	}
	if pe.Spec.ReadinessProbe == nil && s.ReadinessProbe != nil {
		pe.Spec.ReadinessProbe = s.ReadinessProbe.DeepCopy()
	}
	if pe.Spec.LivenessProbe == nil && s.LivenessProbe != nil {
		pe.Spec.LivenessProbe = s.LivenessProbe.DeepCopy()
	}
	if IsResourcesEmpty(pe.Spec.Resources) && s.Resources != nil {
		pe.Spec.Resources = ResourceFromPreset(s.Resources)
	}
	if pe.Spec.Ingress == nil && s.Ingress != nil && s.Ingress.Enabled {
		pe.Spec.Ingress = IngressFromTemplate(pe.Name, s.Ingress)
	} else if pe.Spec.Ingress != nil && s.Ingress != nil {
		MergeIngressDefaults(pe.Spec.Ingress, pe.Name, s.Ingress)
	}
}

// IsResourcesEmpty reports whether both requests and limits are unset.
func IsResourcesEmpty(r corev1.ResourceRequirements) bool {
	return len(r.Limits) == 0 && len(r.Requests) == 0
}

// ResourceFromPreset maps a template resource preset (or custom) to ResourceRequirements.
func ResourceFromPreset(p *miragev1alpha1.ResourcePresetSpec) corev1.ResourceRequirements {
	if p == nil {
		return corev1.ResourceRequirements{}
	}
	if p.Custom != nil {
		return *p.Custom.DeepCopy()
	}
	switch p.Preset {
	case miragev1alpha1.ResourcePresetMedium:
		return corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("200m"), corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
		}
	case miragev1alpha1.ResourcePresetLarge:
		return corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("1"), corev1.ResourceMemory: resource.MustParse("1Gi"),
			},
		}
	default: // small or unset
		return corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("50m"), corev1.ResourceMemory: resource.MustParse("64Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("200m"), corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
		}
	}
}

// IngressFromTemplate builds an IngressSpec from template defaults and preview name.
func IngressFromTemplate(previewName string, t *miragev1alpha1.IngressTemplateSpec) *miragev1alpha1.IngressSpec {
	ing := &miragev1alpha1.IngressSpec{
		Enabled:          t.Enabled,
		Path:             t.Path,
		IngressClassName: t.IngressClassName,
		Annotations:      CopyStringMap(t.Annotations),
	}
	if t.Domain != "" {
		ing.Host = previewName + "." + t.Domain
	}
	if t.TLS != nil {
		ing.TLS = t.TLS.DeepCopy()
	}
	return ing
}

// MergeIngressDefaults fills empty ingress fields from the template.
func MergeIngressDefaults(ing *miragev1alpha1.IngressSpec, previewName string, t *miragev1alpha1.IngressTemplateSpec) {
	if ing.Host == "" && t.Domain != "" {
		ing.Host = previewName + "." + t.Domain
	}
	if ing.Path == "" && t.Path != "" {
		ing.Path = t.Path
	}
	if ing.IngressClassName == nil && t.IngressClassName != nil {
		ing.IngressClassName = t.IngressClassName
	}
	if len(ing.Annotations) == 0 && len(t.Annotations) > 0 {
		ing.Annotations = CopyStringMap(t.Annotations)
	} else if len(t.Annotations) > 0 {
		merged := CopyStringMap(t.Annotations)
		for k, v := range ing.Annotations {
			merged[k] = v
		}
		ing.Annotations = merged
	}
	if ing.TLS == nil && t.TLS != nil {
		ing.TLS = t.TLS.DeepCopy()
	}
}

// CopyStringMap returns a shallow copy of in (nil stays nil).
func CopyStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// EffectiveServices expands single- or multi-service previews with defaults applied.
// Replica counts of 0 are preserved (not coerced to 1).
func EffectiveServices(pe *miragev1alpha1.PreviewEnvironment) ([]EffectiveService, error) {
	if pe == nil {
		return nil, fmt.Errorf("preview is nil")
	}
	if len(pe.Spec.Services) == 0 {
		if pe.Spec.Image == "" {
			return nil, fmt.Errorf("spec.image is required when services is empty")
		}
		port := pe.Spec.ContainerPort
		if port == 0 {
			port = DefaultContainerPort
		}
		replicas := DefaultReplicas
		if pe.Spec.Replicas != nil {
			replicas = *pe.Spec.Replicas
		}
		return []EffectiveService{{
			Name:            pe.Name,
			WorkloadName:    pe.Name,
			Image:           pe.Spec.Image,
			Port:            port,
			Replicas:        replicas,
			Env:             pe.Spec.Env,
			EnvFrom:         pe.Spec.EnvFrom,
			Resources:       pe.Spec.Resources,
			Command:         pe.Spec.Command,
			Args:            pe.Spec.Args,
			ImagePullPolicy: pe.Spec.ImagePullPolicy,
			ReadinessProbe:  pe.Spec.ReadinessProbe,
			LivenessProbe:   pe.Spec.LivenessProbe,
			Ingress:         pe.Spec.Ingress,
		}}, nil
	}

	seen := map[string]struct{}{}
	out := make([]EffectiveService, 0, len(pe.Spec.Services))
	for i, svc := range pe.Spec.Services {
		if svc.Name == "" {
			return nil, fmt.Errorf("services[%d].name is required", i)
		}
		if errs := validation.IsDNS1123Label(svc.Name); len(errs) > 0 {
			return nil, fmt.Errorf("services[%d].name invalid: %v", i, errs)
		}
		if _, dup := seen[svc.Name]; dup {
			return nil, fmt.Errorf("duplicate service name %q", svc.Name)
		}
		seen[svc.Name] = struct{}{}
		if svc.Image == "" {
			return nil, fmt.Errorf("services[%d].image is required", i)
		}
		port := svc.Port
		if port == 0 {
			port = pe.Spec.ContainerPort
		}
		if port == 0 {
			port = DefaultContainerPort
		}
		replicas := DefaultReplicas
		if pe.Spec.Replicas != nil {
			replicas = *pe.Spec.Replicas
		}
		if svc.Replicas != nil {
			replicas = *svc.Replicas
		}
		resources := svc.Resources
		if IsResourcesEmpty(resources) {
			resources = pe.Spec.Resources
		}
		readiness := svc.ReadinessProbe
		if readiness == nil {
			readiness = pe.Spec.ReadinessProbe
		}
		liveness := svc.LivenessProbe
		if liveness == nil {
			liveness = pe.Spec.LivenessProbe
		}
		pull := svc.ImagePullPolicy
		if pull == "" {
			pull = pe.Spec.ImagePullPolicy
		}
		ing := svc.Ingress
		if ing == nil && i == 0 {
			ing = pe.Spec.Ingress
		}
		env := svc.Env
		if len(env) == 0 {
			env = pe.Spec.Env
		}
		envFrom := svc.EnvFrom
		if len(envFrom) == 0 {
			envFrom = pe.Spec.EnvFrom
		}
		out = append(out, EffectiveService{
			Name:            svc.Name,
			WorkloadName:    svc.Name,
			Image:           svc.Image,
			Port:            port,
			Replicas:        replicas,
			Env:             env,
			EnvFrom:         envFrom,
			Resources:       resources,
			Command:         svc.Command,
			Args:            svc.Args,
			ImagePullPolicy: pull,
			ReadinessProbe:  readiness,
			LivenessProbe:   liveness,
			Ingress:         ing,
		})
	}
	return out, nil
}
