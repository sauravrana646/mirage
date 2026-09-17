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

	// SCM providers for SourceSpec.Provider / mirage-notify.
	SCMProviderGitHub    = "github"
	SCMProviderGitLab    = "gitlab"
	SCMProviderBitbucket = "bitbucket"

	// BackendDirect applies Deployment/Service/Ingress directly.
	BackendDirect = "direct"
	// BackendArgoCD creates an Argo CD Application instead of direct apply.
	BackendArgoCD = "argocd"
	// DefaultArgoNamespace is where Application CRs live when unspecified.
	DefaultArgoNamespace = "argocd"

	ConditionReady             = "Ready"
	ConditionProgressing       = "Progressing"
	ConditionExpired           = "Expired"
	ConditionNamespaceReady    = "NamespaceReady"
	ConditionWorkloadReady     = "WorkloadReady"
	ConditionNetworkReady      = "NetworkReady"
	ConditionRouteReady        = "RouteReady"
	ConditionDependenciesReady = "DependenciesReady"

	PhasePending      = "Pending"
	PhaseProvisioning = "Provisioning"
	PhaseReady        = "Ready"
	PhaseFailed       = "Failed"
	PhaseExpiring     = "Expiring"
	PhasePaused       = "Paused"

	ReasonWorkloadReady       = "WorkloadReady"
	ReasonNamespaceReady      = "NamespaceReady"
	ReasonNamespaceConflict   = "NamespaceConflict"
	ReasonImageInvalid        = "ImageInvalid"
	ReasonInvalidSpec         = "InvalidSpec"
	ReasonRolloutFailed       = "RolloutFailed"
	ReasonReconciling         = "Reconciling"
	ReasonExpiring            = "Expiring"
	ReasonDeleting            = "Deleting"
	ReasonPaused              = "Paused"
	ReasonArgoSyncing         = "ArgoSyncing"
	ReasonArgoHealthy         = "ArgoHealthy"
	ReasonTemplateNotFound    = "TemplateNotFound"
	ReasonRouteReady          = "RouteReady"
	ReasonRoutePending        = "RoutePending"
	ReasonNetworkReady        = "NetworkReady"
	ReasonDependenciesReady   = "DependenciesReady"
	ReasonDependenciesPending = "DependenciesPending"

	LabelService    = "mirage.dev/service"
	LabelDependency = "mirage.dev/dependency"
)

// PreviewDependenciesSpec configures ephemeral in-namespace dependencies
// (Deployments/Services) for preview workloads. No Helm operators.
type PreviewDependenciesSpec struct {
	// +optional
	Postgres *PostgresDependencySpec `json:"postgres,omitempty"`
	// +optional
	Redis *RedisDependencySpec `json:"redis,omitempty"`
	// +optional
	Kafka *KafkaDependencySpec `json:"kafka,omitempty"`
}

// PostgresDependencySpec enables an ephemeral Postgres for the preview namespace.
type PostgresDependencySpec struct {
	// +optional
	Enabled bool `json:"enabled,omitempty"`
	// Version is the image tag major/minor (e.g. "16"). Defaults to "16".
	// +optional
	Version string `json:"version,omitempty"`
	// Storage size (e.g. "1Gi"). Reserved for PVC; MVP uses emptyDir only.
	// Non-empty values are rejected by admission until PVC support lands.
	// +optional
	Storage string `json:"storage,omitempty"`
}

// RedisDependencySpec enables an ephemeral Redis for the preview namespace.
type RedisDependencySpec struct {
	// +optional
	Enabled bool `json:"enabled,omitempty"`
	// Version is the image tag (e.g. "7.2"). Defaults to "7.2".
	// +optional
	Version string `json:"version,omitempty"`
}

// KafkaDependencySpec enables an ephemeral Kafka (KRaft, single-node) for the preview namespace.
type KafkaDependencySpec struct {
	// +optional
	Enabled bool `json:"enabled,omitempty"`
	// Version is the image tag (e.g. "3.7"). Defaults to "3.7".
	// +optional
	Version string `json:"version,omitempty"`
}

// SourceSpec identifies the git/PR/MR origin of a preview.
type SourceSpec struct {
	// Provider is the SCM system: github, gitlab, or bitbucket.
	// +optional
	// +kubebuilder:validation:Enum=github;gitlab;bitbucket
	Provider string `json:"provider,omitempty"`
	// +optional
	Repo string `json:"repo,omitempty"`
	// PullRequest is the GitHub PR / GitLab MR / Bitbucket PR number.
	// +optional
	PullRequest int `json:"pullRequest,omitempty"`
	// +optional
	CommitSHA string `json:"commitSHA,omitempty"`
	// +optional
	Branch string `json:"branch,omitempty"`
	// ProjectID is the GitLab project ID or path (group/project).
	// +optional
	ProjectID string `json:"projectID,omitempty"`
	// Workspace is the Bitbucket Cloud workspace slug.
	// +optional
	Workspace string `json:"workspace,omitempty"`
	// RepoSlug is the Bitbucket repository slug when different from the URL path.
	// +optional
	RepoSlug string `json:"repoSlug,omitempty"`
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

// PreviewServiceSpec defines one workload inside a multi-service preview.
type PreviewServiceSpec struct {
	// Name is the service identity (DNS-1123 label). Used for Deployment/Service names.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	Name string `json:"name"`

	// Image is the container image (digest strongly recommended).
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`

	// Port is the container port (defaults to template/preview containerPort or 8080).
	// +optional
	Port int32 `json:"port,omitempty"`

	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=10
	Replicas *int32 `json:"replicas,omitempty"`

	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// +optional
	EnvFrom []corev1.EnvFromSource `json:"envFrom,omitempty"`

	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// +optional
	Command []string `json:"command,omitempty"`

	// +optional
	Args []string `json:"args,omitempty"`

	// +optional
	// +kubebuilder:validation:Enum=Always;IfNotPresent;Never
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`

	// +optional
	ReadinessProbe *ProbeSpec `json:"readinessProbe,omitempty"`

	// +optional
	LivenessProbe *ProbeSpec `json:"livenessProbe,omitempty"`

	// Ingress exposes this service. When omitted, top-level ingress applies only to the primary service.
	// +optional
	Ingress *IngressSpec `json:"ingress,omitempty"`
}

// ServiceStatus reports observed state for one preview service.
type ServiceStatus struct {
	// Name of the service.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// +optional
	Ready bool `json:"ready,omitempty"`
	// +optional
	URL string `json:"url,omitempty"`
	// +optional
	Replicas string `json:"replicas,omitempty"`
	// +optional
	Message string `json:"message,omitempty"`
}

// PreviewEnvironmentSpec defines the desired state of PreviewEnvironment.
type PreviewEnvironmentSpec struct {
	// TemplateRef applies platform defaults from a PreviewTemplate.
	// Explicit PreviewEnvironment fields override the template.
	// +optional
	TemplateRef *TemplateRef `json:"templateRef,omitempty"`

	// +optional
	Source *SourceSpec `json:"source,omitempty"`

	// Image is the container image for single-service previews (digest strongly recommended).
	// Required when services is empty; ignored for named entries in services.
	// +optional
	Image string `json:"image,omitempty"`

	// Services defines multi-service preview workloads. When set, each entry gets its own
	// Deployment/Service (and optional Ingress). When empty, Image drives a single workload.
	// +optional
	// +listType=map
	// +listMapKey=name
	Services []PreviewServiceSpec `json:"services,omitempty"`

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
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=10
	Replicas *int32 `json:"replicas,omitempty"`

	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// +optional
	ContainerPort int32 `json:"containerPort,omitempty"`

	// +optional
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

	// ImagePullPolicy for the preview container (default IfNotPresent; Always for :latest tags if unset by kubelet).
	// +optional
	// +kubebuilder:validation:Enum=Always;IfNotPresent;Never
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`

	// Command overrides the container entrypoint.
	// +optional
	Command []string `json:"command,omitempty"`

	// Args overrides the container args.
	// +optional
	Args []string `json:"args,omitempty"`

	// EnvFrom sources for the preview container.
	// +optional
	EnvFrom []corev1.EnvFromSource `json:"envFrom,omitempty"`

	// ServiceAccountName for preview pods (must exist in targetNamespace).
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// NodeSelector for preview pods.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Tolerations for preview pods.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Affinity for preview pods.
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// TerminationGracePeriodSeconds for preview pods.
	// +optional
	// +kubebuilder:validation:Minimum=0
	TerminationGracePeriodSeconds *int64 `json:"terminationGracePeriodSeconds,omitempty"`

	// NetworkPolicy mode. When empty, uses PreviewTemplate or baseline.
	// +optional
	NetworkPolicy NetworkPolicyMode `json:"networkPolicy,omitempty"`

	// +optional
	ArgoCD *ArgoCDSpec `json:"argoCD,omitempty"`

	// PriorityClassName for preview pods.
	// +optional
	PriorityClassName string `json:"priorityClassName,omitempty"`

	// RuntimeClassName for preview pods (overrides template security.runtimeClassName).
	// +optional
	RuntimeClassName string `json:"runtimeClassName,omitempty"`

	// Dependencies provisions ephemeral postgres/redis/kafka in the target namespace
	// and injects DATABASE_URL / REDIS_URL / KAFKA_BROKERS into preview app containers.
	// +optional
	Dependencies *PreviewDependenciesSpec `json:"dependencies,omitempty"`
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
	// TemplateRef records the resolved template name when templateRef was used.
	// +optional
	Template string `json:"template,omitempty"`
	// Services reports per-service readiness for multi-service previews.
	// +optional
	// +listType=map
	// +listMapKey=name
	Services []ServiceStatus `json:"services,omitempty"`
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
// +kubebuilder:validation:XValidation:rule="(has(self.spec.image) && self.spec.image != \"\") || (has(self.spec.services) && size(self.spec.services) > 0)",message="spec.image or spec.services is required"

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
