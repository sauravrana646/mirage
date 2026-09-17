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
	FinalizerName       = "mirage.dev/finalizer"
	LabelOwnerUID       = "mirage.dev/owner-uid"
	LabelOwnerName      = "mirage.dev/owner-name"
	LabelOwnerNamespace = "mirage.dev/owner-namespace"
	LabelManagedBy      = "app.kubernetes.io/managed-by"
	ManagedByValue      = "mirage"

	AnnotationAISummary = "mirage.dev/ai-summary"
	AnnotationAIRisk    = "mirage.dev/ai-risk"

	ConditionReady       = "Ready"
	ConditionProgressing = "Progressing"
	ConditionExpired     = "Expired"

	PhasePending  = "Pending"
	PhaseReady    = "Ready"
	PhaseFailed   = "Failed"
	PhaseExpiring = "Expiring"
	PhasePaused   = "Paused"

	ReasonWorkloadReady     = "WorkloadReady"
	ReasonNamespaceConflict = "NamespaceConflict"
	ReasonImageInvalid      = "ImageInvalid"
	ReasonInvalidSpec       = "InvalidSpec"
	ReasonRolloutFailed     = "RolloutFailed"
	ReasonReconciling       = "Reconciling"
	ReasonExpiring          = "Expiring"
	ReasonDeleting          = "Deleting"
	ReasonPaused            = "Paused"
	ReasonArgoSyncing       = "ArgoSyncing"
	ReasonArgoHealthy       = "ArgoHealthy"
)

// SourceSpec identifies the git/PR origin of a preview.
type SourceSpec struct {
	// +optional
	Repo string `json:"repo,omitempty"`
	// +optional
	PullRequest int `json:"pullRequest,omitempty"`
	// +optional
	CommitSHA string `json:"commitSHA,omitempty"`
	// +optional
	Branch string `json:"branch,omitempty"`
}

// TLSSpec configures optional Ingress TLS.
type TLSSpec struct {
	// +optional
	Enabled bool `json:"enabled,omitempty"`
	// SecretName for the TLS certificate (cert-manager can create it).
	// +optional
	SecretName string `json:"secretName,omitempty"`
}

// IngressSpec configures optional Ingress for the preview.
type IngressSpec struct {
	// +optional
	Enabled bool `json:"enabled,omitempty"`
	// +optional
	Host string `json:"host,omitempty"`
	// +optional
	Path string `json:"path,omitempty"`
	// +optional
	IngressClassName *string `json:"ingressClassName,omitempty"`
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
	// +optional
	TLS *TLSSpec `json:"tls,omitempty"`
}

// ProbeSpec is a simplified HTTP probe.
type ProbeSpec struct {
	// +optional
	// +kubebuilder:default=/
	Path string `json:"path,omitempty"`
	// +optional
	// +kubebuilder:default=8080
	Port int32 `json:"port,omitempty"`
	// +optional
	// +kubebuilder:default=10
	InitialDelaySeconds int32 `json:"initialDelaySeconds,omitempty"`
	// +optional
	// +kubebuilder:default=10
	PeriodSeconds int32 `json:"periodSeconds,omitempty"`
}

// ArgoCDSpec configures the optional Argo CD Application backend.
type ArgoCDSpec struct {
	// Destination namespace for the Application (defaults to targetNamespace).
	// +optional
	DestinationNamespace string `json:"destinationNamespace,omitempty"`
	// Project is the Argo CD project (default "default").
	// +optional
	// +kubebuilder:default=default
	Project string `json:"project,omitempty"`
	// RepoURL for the Application source.
	// +optional
	RepoURL string `json:"repoURL,omitempty"`
	// Path within the repo.
	// +optional
	Path string `json:"path,omitempty"`
	// TargetRevision (branch/tag/SHA).
	// +optional
	TargetRevision string `json:"targetRevision,omitempty"`
	// ArgoNamespace where Application CRs live (default argocd).
	// +optional
	// +kubebuilder:default=argocd
	ArgoNamespace string `json:"argoNamespace,omitempty"`
}

// NetworkPolicyMode controls preview NetworkPolicy strictness.
// +kubebuilder:validation:Enum=baseline;permissive;disabled
type NetworkPolicyMode string

const (
	NetworkPolicyBaseline   NetworkPolicyMode = "baseline"
	NetworkPolicyPermissive NetworkPolicyMode = "permissive"
	NetworkPolicyDisabled   NetworkPolicyMode = "disabled"
)

// PreviewEnvironmentSpec defines the desired state of PreviewEnvironment.
type PreviewEnvironmentSpec struct {
	// +optional
	Source *SourceSpec `json:"source,omitempty"`

	// Image is the container image (digest strongly recommended).
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`

	// RequireDigest rejects tags without @sha256 when true (also enforced by webhook policy).
	// +optional
	RequireDigest bool `json:"requireDigest,omitempty"`

	// +optional
	// +kubebuilder:validation:Minimum=0
	TTLSeconds *int64 `json:"ttlSeconds,omitempty"`

	// TargetNamespace is immutable after create.
	// +kubebuilder:validation:MinLength=1
	TargetNamespace string `json:"targetNamespace"`

	// +optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=10
	Replicas *int32 `json:"replicas,omitempty"`

	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// +optional
	// +kubebuilder:default=8080
	ContainerPort int32 `json:"containerPort,omitempty"`

	// +optional
	// +kubebuilder:default=direct
	// +kubebuilder:validation:Enum=direct;argocd
	Backend string `json:"backend,omitempty"`

	// +optional
	Ingress *IngressSpec `json:"ingress,omitempty"`

	// Suspend stops reconcile of children without deleting them.
	// +optional
	Suspend bool `json:"suspend,omitempty"`

	// Labels merged onto owned workloads.
	// +optional
	WorkloadLabels map[string]string `json:"workloadLabels,omitempty"`

	// Annotations merged onto owned workloads.
	// +optional
	WorkloadAnnotations map[string]string `json:"workloadAnnotations,omitempty"`

	// +optional
	ReadinessProbe *ProbeSpec `json:"readinessProbe,omitempty"`

	// +optional
	LivenessProbe *ProbeSpec `json:"livenessProbe,omitempty"`

	// ImagePullSecrets for the preview pod.
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// +optional
	// +kubebuilder:default=baseline
	NetworkPolicy NetworkPolicyMode `json:"networkPolicy,omitempty"`

	// +optional
	ArgoCD *ArgoCDSpec `json:"argoCD,omitempty"`

	// PriorityClassName for preview pods.
	// +optional
	PriorityClassName string `json:"priorityClassName,omitempty"`
}

// PreviewEnvironmentStatus defines the observed state of PreviewEnvironment.
type PreviewEnvironmentStatus struct {
	// +optional
	Phase string `json:"phase,omitempty"`
	// +optional
	URL string `json:"url,omitempty"`
	// +optional
	ExpiresAt *metav1.Time `json:"expiresAt,omitempty"`
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// +optional
	Message string `json:"message,omitempty"`
	// +optional
	ReplicaStatus string `json:"replicaStatus,omitempty"`
	// +optional
	ArgoApplication string `json:"argoApplication,omitempty"`
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=pe;preview
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.status.url`
// +kubebuilder:printcolumn:name="Backend",type=string,JSONPath=`.spec.backend`
// +kubebuilder:printcolumn:name="Expires",type=string,JSONPath=`.status.expiresAt`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.spec.targetNamespace) || self.spec.targetNamespace == oldSelf.spec.targetNamespace",message="targetNamespace is immutable"

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
