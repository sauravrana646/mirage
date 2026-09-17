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

const (
	// FinalizerName is added to PreviewEnvironment CRs so the controller can
	// tear down targetNamespace before the CR is removed.
	FinalizerName = "mirage.dev/finalizer"

	// LabelOwnerUID identifies the PreviewEnvironment UID that owns a namespace.
	LabelOwnerUID = "mirage.dev/owner-uid"
	// LabelOwnerName is the CR name.
	LabelOwnerName = "mirage.dev/owner-name"
	// LabelOwnerNamespace is the CR namespace.
	LabelOwnerNamespace = "mirage.dev/owner-namespace"
	// LabelManagedBy marks resources created by Mirage.
	LabelManagedBy = "app.kubernetes.io/managed-by"
	// ManagedByValue is the managed-by label value.
	ManagedByValue = "mirage"

	// ConditionReady is the primary readiness condition.
	ConditionReady = "Ready"

	// Phase values.
	PhasePending  = "Pending"
	PhaseReady    = "Ready"
	PhaseFailed   = "Failed"
	PhaseExpiring = "Expiring"

	// Condition reasons.
	ReasonWorkloadReady     = "WorkloadReady"
	ReasonNamespaceConflict = "NamespaceConflict"
	ReasonImageInvalid      = "ImageInvalid"
	ReasonRolloutFailed     = "RolloutFailed"
	ReasonReconciling       = "Reconciling"
	ReasonExpiring          = "Expiring"
	ReasonDeleting          = "Deleting"
)

// SourceSpec identifies the git/PR origin of a preview (optional until Phase 4).
type SourceSpec struct {
	// Repo is the repository URL.
	// +optional
	Repo string `json:"repo,omitempty"`
	// PullRequest is the PR number.
	// +optional
	PullRequest int `json:"pullRequest,omitempty"`
	// CommitSHA is the git commit being previewed.
	// +optional
	CommitSHA string `json:"commitSHA,omitempty"`
}

// IngressSpec configures optional Ingress for the preview.
type IngressSpec struct {
	// Enabled turns Ingress creation on.
	// +optional
	Enabled bool `json:"enabled,omitempty"`
	// Host is the Ingress host (e.g. nip.io style on kind).
	// +optional
	Host string `json:"host,omitempty"`
	// IngressClassName selects the ingress controller class.
	// +optional
	IngressClassName *string `json:"ingressClassName,omitempty"`
}

// PreviewEnvironmentSpec defines the desired state of PreviewEnvironment.
type PreviewEnvironmentSpec struct {
	// Source optionally describes the PR / commit identity.
	// +optional
	Source *SourceSpec `json:"source,omitempty"`

	// Image is the container image to run (prefer digest).
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`

	// TTLSeconds is how long the preview lives after creation (status.expiresAt).
	// When elapsed, the controller cleans up and deletes the CR.
	// +optional
	// +kubebuilder:validation:Minimum=0
	TTLSeconds *int64 `json:"ttlSeconds,omitempty"`

	// TargetNamespace is where workloads are created (one namespace per preview).
	// +kubebuilder:validation:MinLength=1
	TargetNamespace string `json:"targetNamespace"`

	// Replicas for the preview Deployment.
	// +optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	Replicas *int32 `json:"replicas,omitempty"`

	// Env is injected into the preview container.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Resources for the preview container.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// ContainerPort is the port the container listens on.
	// +optional
	// +kubebuilder:default=8080
	ContainerPort int32 `json:"containerPort,omitempty"`

	// Backend selects the apply path. Only "direct" is implemented in MVP.
	// +optional
	// +kubebuilder:default=direct
	// +kubebuilder:validation:Enum=direct;argocd
	Backend string `json:"backend,omitempty"`

	// Ingress configures optional HTTP access.
	// +optional
	Ingress *IngressSpec `json:"ingress,omitempty"`
}

// PreviewEnvironmentStatus defines the observed state of PreviewEnvironment.
type PreviewEnvironmentStatus struct {
	// Phase is a high-level summary: Pending, Ready, Failed, Expiring.
	// +optional
	Phase string `json:"phase,omitempty"`

	// URL is the preview access URL when Ingress is ready.
	// +optional
	URL string `json:"url,omitempty"`

	// ExpiresAt is computed from ttlSeconds on first observe.
	// +optional
	ExpiresAt *metav1.Time `json:"expiresAt,omitempty"`

	// ObservedGeneration is the last reconciled generation.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions follow Kubernetes convention.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.status.url`
// +kubebuilder:printcolumn:name="Expires",type=string,JSONPath=`.status.expiresAt`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// PreviewEnvironment is the Schema for the previewenvironments API.
type PreviewEnvironment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PreviewEnvironmentSpec   `json:"spec,omitempty"`
	Status PreviewEnvironmentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PreviewEnvironmentList contains a list of PreviewEnvironment.
type PreviewEnvironmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PreviewEnvironment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PreviewEnvironment{}, &PreviewEnvironmentList{})
}
