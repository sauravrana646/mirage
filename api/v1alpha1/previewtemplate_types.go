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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ResourcePresetName is a named CPU/memory size class for preview workloads.
// +kubebuilder:validation:Enum=small;medium;large
type ResourcePresetName string

const (
	ResourcePresetSmall  ResourcePresetName = "small"
	ResourcePresetMedium ResourcePresetName = "medium"
	ResourcePresetLarge  ResourcePresetName = "large"
)

// SecurityProfileName selects a Mirage security baseline for preview namespaces.
// +kubebuilder:validation:Enum=restricted;baseline
type SecurityProfileName string

const (
	SecurityProfileRestricted SecurityProfileName = "restricted"
	SecurityProfileBaseline   SecurityProfileName = "baseline"
)

// TemplateRef references a PreviewTemplate.
type TemplateRef struct {
	// Name of the PreviewTemplate.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// Namespace of the PreviewTemplate. Defaults to the PreviewEnvironment namespace.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// ResourcePresetSpec selects a size class or custom resource requirements.
type ResourcePresetSpec struct {
	// Preset is a named size: small, medium, or large.
	// +optional
	Preset ResourcePresetName `json:"preset,omitempty"`
	// Custom overrides the preset when set.
	// +optional
	Custom *corev1.ResourceRequirements `json:"custom,omitempty"`
}

// SecurityProfileSpec configures PSA and related defaults for preview namespaces.
type SecurityProfileSpec struct {
	// Profile is restricted (default) or baseline.
	// +optional
	// +kubebuilder:default=restricted
	Profile SecurityProfileName `json:"profile,omitempty"`
	// RuntimeClassName optionally pins preview pods to a RuntimeClass (e.g. gvisor).
	// +optional
	RuntimeClassName string `json:"runtimeClassName,omitempty"`
}

// IngressTemplateSpec provides default ingress settings applied when a preview enables ingress.
type IngressTemplateSpec struct {
	// Enabled defaults ingress on for previews that do not override it.
	// +optional
	Enabled bool `json:"enabled,omitempty"`
	// Domain is appended as `<preview-name>.<domain>` when host is unset.
	// +optional
	Domain string `json:"domain,omitempty"`
	// Path default for ingress.
	// +optional
	Path string `json:"path,omitempty"`
	// IngressClassName default.
	// +optional
	IngressClassName *string `json:"ingressClassName,omitempty"`
	// Annotations merged onto preview ingresses.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
	// TLS defaults.
	// +optional
	TLS *TLSSpec `json:"tls,omitempty"`
}

// PreviewTemplateSpec defines platform defaults for PreviewEnvironments.
type PreviewTemplateSpec struct {
	// Resources size class for workloads that omit explicit resources.
	// +optional
	Resources *ResourcePresetSpec `json:"resources,omitempty"`

	// NetworkPolicy default mode.
	// +optional
	// +kubebuilder:default=baseline
	NetworkPolicy NetworkPolicyMode `json:"networkPolicy,omitempty"`

	// Ingress defaults (host derived from domain when enabled).
	// +optional
	Ingress *IngressTemplateSpec `json:"ingress,omitempty"`

	// TTLSeconds default lifetime when PreviewEnvironment omits ttlSeconds.
	// +optional
	// +kubebuilder:validation:Minimum=0
	TTLSeconds *int64 `json:"ttlSeconds,omitempty"`

	// Security profile for preview namespaces.
	// +optional
	Security *SecurityProfileSpec `json:"security,omitempty"`

	// Replicas default.
	// +optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=10
	Replicas *int32 `json:"replicas,omitempty"`

	// ContainerPort default for single-service previews.
	// +optional
	// +kubebuilder:default=8080
	ContainerPort int32 `json:"containerPort,omitempty"`

	// Backend default: direct or argocd.
	// +optional
	// +kubebuilder:default=direct
	// +kubebuilder:validation:Enum=direct;argocd
	Backend string `json:"backend,omitempty"`

	// RequireDigest default.
	// +optional
	RequireDigest bool `json:"requireDigest,omitempty"`

	// ReadinessProbe default.
	// +optional
	ReadinessProbe *ProbeSpec `json:"readinessProbe,omitempty"`

	// LivenessProbe default.
	// +optional
	LivenessProbe *ProbeSpec `json:"livenessProbe,omitempty"`
}

// PreviewTemplateStatus is reserved for observed template state.
type PreviewTemplateStatus struct {
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=pt;previewtemplate
// +kubebuilder:printcolumn:name="Preset",type=string,JSONPath=`.spec.resources.preset`
// +kubebuilder:printcolumn:name="Network",type=string,JSONPath=`.spec.networkPolicy`
// +kubebuilder:printcolumn:name="TTL",type=integer,JSONPath=`.spec.ttlSeconds`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// PreviewTemplate holds platform defaults for PreviewEnvironments.
type PreviewTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PreviewTemplateSpec   `json:"spec,omitempty"`
	Status PreviewTemplateStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PreviewTemplateList contains a list of PreviewTemplate.
type PreviewTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PreviewTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PreviewTemplate{}, &PreviewTemplateList{})
}
