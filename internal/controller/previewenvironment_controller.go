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
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	miragev1alpha1 "github.com/sauravrana646/mirage/api/v1alpha1"
)

const (
	defaultReplicas      int32 = 1
	defaultContainerPort int32 = 8080
	appLabel                   = "app.kubernetes.io/name"
	componentLabel             = "app.kubernetes.io/component"
	requeueFast                = 15 * time.Second
	namespaceDeleteWait        = 5 * time.Second
)

// PreviewEnvironmentReconciler reconciles a PreviewEnvironment object.
type PreviewEnvironmentReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=mirage.dev,resources=previewenvironments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mirage.dev,resources=previewenvironments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=mirage.dev,resources=previewenvironments/finalizers,verbs=update
// +kubebuilder:rbac:groups=mirage.dev,resources=previewtemplates,verbs=get;list;watch
// +kubebuilder:rbac:groups=mirage.dev,resources=previewtemplates/status,verbs=get
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=resourcequotas,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=limitranges,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=create;delete;get;patch;update
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=argoproj.io,resources=applications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *PreviewEnvironmentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	pe := &miragev1alpha1.PreviewEnvironment{}
	if err := r.Get(ctx, req.NamespacedName, pe); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !controllerutil.ContainsFinalizer(pe, miragev1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(pe, miragev1alpha1.FinalizerName)
		if err := r.Update(ctx, pe); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	if !pe.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, pe)
	}

	if msg, reason := validateSpec(pe); msg != "" {
		r.record(pe, corev1.EventTypeWarning, reason, msg)
		return ctrl.Result{}, r.patchStatus(ctx, pe, statusPatch{
			Phase: miragev1alpha1.PhaseFailed, Ready: metav1.ConditionFalse, Reason: reason, Message: msg,
		})
	}

	resolved, err := r.resolvePreview(ctx, pe)
	if err != nil {
		reason := miragev1alpha1.ReasonInvalidSpec
		if strings.Contains(err.Error(), miragev1alpha1.ReasonTemplateNotFound) {
			reason = miragev1alpha1.ReasonTemplateNotFound
		}
		r.record(pe, corev1.EventTypeWarning, reason, err.Error())
		return ctrl.Result{}, r.patchStatus(ctx, pe, statusPatch{
			Phase: miragev1alpha1.PhaseFailed, Ready: metav1.ConditionFalse, Reason: reason, Message: err.Error(),
		})
	}

	if msg := validateRequireDigest(resolved); msg != "" {
		r.record(pe, corev1.EventTypeWarning, miragev1alpha1.ReasonImageInvalid, msg)
		// Existing workloads may still be running (e.g. template requireDigest flipped on).
		// Scale application Deployments to 0 for reversibility; leave dependencies alone.
		scaled, err := r.scalePreviewWorkloadsToZero(ctx, pe)
		if err != nil {
			return ctrl.Result{}, err
		}
		failMsg := msg
		if scaled > 0 {
			failMsg = fmt.Sprintf("%s; scaled %d application workload(s) to 0 replicas", msg, scaled)
		}
		return ctrl.Result{}, r.patchStatus(ctx, pe, statusPatch{
			Phase: miragev1alpha1.PhaseFailed, Ready: metav1.ConditionFalse,
			Reason: miragev1alpha1.ReasonImageInvalid, Message: failMsg,
			Template: resolved.TemplateName,
		})
	}

	if msg, reason := validateEffectiveSpec(resolved.PE); msg != "" {
		r.record(pe, corev1.EventTypeWarning, reason, msg)
		return ctrl.Result{}, r.patchStatus(ctx, pe, statusPatch{
			Phase: miragev1alpha1.PhaseFailed, Ready: metav1.ConditionFalse,
			Reason: reason, Message: msg, Template: resolved.TemplateName,
		})
	}

	// TTL applies even while suspended: compute expiry and expire before pausing children.
	expiresAt := computeExpiresAt(resolved.PE, time.Now())
	if expired, err := r.handleExpiry(ctx, pe, expiresAt); expired {
		return ctrl.Result{}, err
	}

	if pe.Spec.Suspend {
		r.record(pe, corev1.EventTypeNormal, miragev1alpha1.ReasonPaused, "Preview suspended")
		previewsPaused.Inc()
		requeue := ctrl.Result{RequeueAfter: requeueFast}
		if expiresAt != nil {
			requeue = ctrl.Result{RequeueAfter: time.Until(expiresAt.Time)}
			if requeue.RequeueAfter < time.Second {
				requeue.RequeueAfter = time.Second
			}
		}
		return requeue, r.patchStatus(ctx, pe, statusPatch{
			Phase: miragev1alpha1.PhasePaused, Ready: metav1.ConditionFalse,
			Reason:   miragev1alpha1.ReasonPaused,
			Message:  "spec.suspend=true; child reconciliation paused; TTL still applies",
			Template: resolved.TemplateName, ExpiresAt: expiresAt,
		})
	}

	logger.V(1).Info("reconciling preview", "targetNamespace", pe.Spec.TargetNamespace, "backend", resolved.PE.Spec.Backend)
	return r.reconcileActive(ctx, resolved, expiresAt)
}

// validateRequireDigest enforces digest-pinned images when requireDigest is set
// on the PreviewEnvironment or inherited from its PreviewTemplate.
func validateRequireDigest(resolved *resolvedPreview) string {
	if resolved == nil || resolved.PE == nil || !resolved.PE.Spec.RequireDigest {
		return ""
	}
	for _, svc := range resolved.Services {
		if !strings.Contains(svc.Image, "@sha256:") {
			return fmt.Sprintf("service %q image must use a sha256 digest when requireDigest is set", svc.Name)
		}
	}
	return ""
}

// validateEffectiveSpec enforces rules on the PreviewEnvironment after template
// defaults are applied (e.g. template-defaulted backend=argocd).
func validateEffectiveSpec(pe *miragev1alpha1.PreviewEnvironment) (message, reason string) {
	if pe == nil {
		return "", ""
	}
	backend := pe.Spec.Backend
	if backend == "" {
		backend = miragev1alpha1.BackendDirect
	}
	if backend != miragev1alpha1.BackendArgoCD {
		return "", ""
	}
	if pe.Spec.ArgoCD == nil || pe.Spec.ArgoCD.RepoURL == "" || pe.Spec.ArgoCD.Path == "" {
		return "spec.argoCD.repoURL and path are required when backend=argocd", miragev1alpha1.ReasonInvalidSpec
	}
	dest := pe.Spec.ArgoCD.DestinationNamespace
	if dest == "" {
		dest = pe.Spec.TargetNamespace
	}
	if dest != pe.Spec.TargetNamespace {
		return "spec.argoCD.destinationNamespace must equal spec.targetNamespace", miragev1alpha1.ReasonInvalidSpec
	}
	if dependenciesEnabled(pe.Spec.Dependencies) {
		return "dependencies are not supported when backend=argocd", miragev1alpha1.ReasonInvalidSpec
	}
	return "", ""
}

func validateSpec(pe *miragev1alpha1.PreviewEnvironment) (message, reason string) {
	if pe.Spec.Image == "" && len(pe.Spec.Services) == 0 {
		return "spec.image or spec.services is required", miragev1alpha1.ReasonImageInvalid
	}
	if pe.Spec.TargetNamespace == "" {
		return "spec.targetNamespace is required", miragev1alpha1.ReasonInvalidSpec
	}
	if errs := validation.IsDNS1123Label(pe.Spec.TargetNamespace); len(errs) > 0 {
		return fmt.Sprintf("invalid targetNamespace: %v", errs), miragev1alpha1.ReasonInvalidSpec
	}
	if pe.Spec.Backend != "" && pe.Spec.Backend != miragev1alpha1.BackendDirect && pe.Spec.Backend != miragev1alpha1.BackendArgoCD {
		return fmt.Sprintf("backend %q is not supported", pe.Spec.Backend), miragev1alpha1.ReasonInvalidSpec
	}
	if pe.Spec.Dependencies != nil && pe.Spec.Dependencies.Postgres != nil &&
		strings.TrimSpace(pe.Spec.Dependencies.Postgres.Storage) != "" {
		return "dependencies.postgres.storage is not supported yet (emptyDir only)", miragev1alpha1.ReasonInvalidSpec
	}
	seen := map[string]struct{}{}
	for i, svc := range pe.Spec.Services {
		if svc.Name == "" {
			return fmt.Sprintf("services[%d].name is required", i), miragev1alpha1.ReasonInvalidSpec
		}
		if errs := validation.IsDNS1123Label(svc.Name); len(errs) > 0 {
			return fmt.Sprintf("services[%d].name invalid", i), miragev1alpha1.ReasonInvalidSpec
		}
		if _, dup := seen[svc.Name]; dup {
			return fmt.Sprintf("duplicate service name %q", svc.Name), miragev1alpha1.ReasonInvalidSpec
		}
		seen[svc.Name] = struct{}{}
		if svc.Image == "" {
			return fmt.Sprintf("services[%d].image is required", i), miragev1alpha1.ReasonImageInvalid
		}
		if msg := dependencyNameCollision(pe.Spec.Dependencies, svc.Name, i); msg != "" {
			return msg, miragev1alpha1.ReasonInvalidSpec
		}
	}
	return "", ""
}

// dependencyReservedServiceNames maps service names that collide with dependency
// Deployments/Services (short names and mirage-dep-* prefixes).
var dependencyReservedServiceNames = map[string]string{
	"postgres":            "postgres",
	"mirage-dep-postgres": "postgres",
	"redis":               "redis",
	"mirage-dep-redis":    "redis",
	"kafka":               "kafka",
	"mirage-dep-kafka":    "kafka",
}

func dependencyNameCollision(deps *miragev1alpha1.PreviewDependenciesSpec, svcName string, idx int) string {
	if deps == nil {
		return ""
	}
	kind, reserved := dependencyReservedServiceNames[svcName]
	if !reserved {
		return ""
	}
	enabled := false
	switch kind {
	case "postgres":
		enabled = deps.Postgres != nil && deps.Postgres.Enabled
	case "redis":
		enabled = deps.Redis != nil && deps.Redis.Enabled
	case "kafka":
		enabled = deps.Kafka != nil && deps.Kafka.Enabled
	}
	if !enabled {
		return ""
	}
	return fmt.Sprintf("services[%d].name %q conflicts with dependencies.%s", idx, svcName, kind)
}

func computeExpiresAt(pe *miragev1alpha1.PreviewEnvironment, now time.Time) *metav1.Time {
	expiresAt := pe.Status.ExpiresAt.DeepCopy()
	ttl := effectiveTTLSeconds(pe)
	if ttl > 0 && expiresAt == nil {
		t := metav1.NewTime(now.Add(time.Duration(ttl) * time.Second))
		expiresAt = &t
	}
	return expiresAt
}

// effectiveTTLSeconds returns spec.ttlSeconds, or MIRAGE_DEFAULT_TTL_SECONDS when unset.
func effectiveTTLSeconds(pe *miragev1alpha1.PreviewEnvironment) int64 {
	if pe.Spec.TTLSeconds != nil {
		return *pe.Spec.TTLSeconds
	}
	return defaultTTLFromEnv()
}

func (r *PreviewEnvironmentReconciler) handleExpiry(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment, expiresAt *metav1.Time) (bool, error) {
	if expiresAt == nil || expiresAt.After(time.Now()) {
		return false, nil
	}
	log.FromContext(ctx).Info("TTL expired; cleaning up preview", "expiresAt", expiresAt.Time)
	r.record(pe, corev1.EventTypeNormal, miragev1alpha1.ReasonExpiring, "TTL elapsed")
	_ = r.patchStatus(ctx, pe, statusPatch{
		Phase: miragev1alpha1.PhaseExpiring, Ready: metav1.ConditionFalse,
		Reason: miragev1alpha1.ReasonExpiring, Message: "TTL elapsed; deleting preview",
		URL: pe.Status.URL, ExpiresAt: expiresAt,
	})
	// Always attempt Argo cleanup: template-backed backends may leave pe.Spec.Backend empty.
	if err := r.deleteArgoApplication(ctx, pe); err != nil {
		return true, err
	}
	if err := r.cleanupTarget(ctx, pe); err != nil {
		return true, err
	}
	previewsExpired.Inc()
	if err := r.Delete(ctx, pe); err != nil && !apierrors.IsNotFound(err) {
		return true, err
	}
	return true, nil
}

func (r *PreviewEnvironmentReconciler) reconcileActive(ctx context.Context, resolved *resolvedPreview, expiresAt *metav1.Time) (ctrl.Result, error) {
	pe := resolved.PE
	orig := &miragev1alpha1.PreviewEnvironment{}
	if err := r.Get(ctx, types.NamespacedName{Name: pe.Name, Namespace: pe.Namespace}, orig); err != nil {
		return ctrl.Result{}, err
	}

	psa := resolved.PSA
	if psa == "" {
		psa = string(miragev1alpha1.SecurityProfileRestricted)
	}
	if err := r.ensureNamespace(ctx, pe, psa); err != nil {
		reason := miragev1alpha1.ReasonRolloutFailed
		if isNamespaceConflict(err) {
			reason = miragev1alpha1.ReasonNamespaceConflict
		}
		r.record(orig, corev1.EventTypeWarning, reason, err.Error())
		_ = r.patchStatus(ctx, orig, statusPatch{
			Phase: miragev1alpha1.PhaseFailed, Ready: metav1.ConditionFalse, Reason: reason, Message: err.Error(),
			ExpiresAt: expiresAt, Template: resolved.TemplateName,
			NamespaceReady: metav1.ConditionFalse,
		})
		return ctrl.Result{RequeueAfter: requeueFast}, nil
	}

	backend := pe.Spec.Backend
	if backend == "" {
		backend = miragev1alpha1.BackendDirect
	}

	if backend == miragev1alpha1.BackendArgoCD {
		if err := r.cleanupDirectWorkloads(ctx, pe); err != nil {
			return ctrl.Result{}, err
		}
		return r.reconcileArgo(ctx, resolved, orig, expiresAt)
	}
	if err := r.deleteArgoApplication(ctx, pe); err != nil {
		return ctrl.Result{}, err
	}
	return r.reconcileDirect(ctx, resolved, orig, expiresAt)
}

func (r *PreviewEnvironmentReconciler) reconcileDirect(ctx context.Context, resolved *resolvedPreview, orig *miragev1alpha1.PreviewEnvironment, expiresAt *metav1.Time) (ctrl.Result, error) {
	pe := resolved.PE
	if err := r.ensureChildren(ctx, resolved); err != nil {
		r.record(orig, corev1.EventTypeWarning, miragev1alpha1.ReasonRolloutFailed, err.Error())
		_ = r.patchStatus(ctx, orig, statusPatch{
			Phase: miragev1alpha1.PhaseFailed, Ready: metav1.ConditionFalse,
			Reason: miragev1alpha1.ReasonRolloutFailed, Message: err.Error(), ExpiresAt: expiresAt,
			Template: resolved.TemplateName, NamespaceReady: metav1.ConditionTrue,
			NetworkReady: metav1.ConditionTrue, WorkloadReady: metav1.ConditionFalse,
			DependenciesReady: metav1.ConditionFalse,
		})
		return ctrl.Result{}, err
	}

	depsReady, depsMsg, err := r.dependenciesReady(ctx, pe)
	if err != nil {
		return ctrl.Result{}, err
	}
	depsCond := metav1.ConditionTrue
	if !depsReady {
		depsCond = metav1.ConditionFalse
	}

	svcStatuses := make([]miragev1alpha1.ServiceStatus, 0, len(resolved.Services))
	allReady := true
	var firstURL string
	replicaParts := make([]string, 0, len(resolved.Services))
	var failMsg string

	for _, svc := range resolved.Services {
		deploy := &appsv1.Deployment{}
		if err := r.Get(ctx, types.NamespacedName{Name: svc.WorkloadName, Namespace: pe.Spec.TargetNamespace}, deploy); err != nil {
			return ctrl.Result{}, err
		}
		ready := deploymentReady(deploy)
		url := serviceURL(svc)
		if firstURL == "" && url != "" {
			firstURL = url
		}
		st := miragev1alpha1.ServiceStatus{
			Name:     svc.Name,
			Ready:    ready,
			URL:      url,
			Replicas: fmt.Sprintf("%d/%d", deploy.Status.ReadyReplicas, ptrInt32(deploy.Spec.Replicas, 1)),
		}
		if !ready {
			allReady = false
			if deploymentProgressDeadline(deploy) {
				st.Message = "ProgressDeadlineExceeded"
				failMsg = fmt.Sprintf("service %s: Deployment is not progressing", svc.Name)
			} else {
				st.Message = "Waiting for Deployment"
			}
		}
		svcStatuses = append(svcStatuses, st)
		replicaParts = append(replicaParts, svc.Name+"="+st.Replicas)
	}

	phase := miragev1alpha1.PhaseProvisioning
	cond := metav1.ConditionFalse
	reason := miragev1alpha1.ReasonReconciling
	msg := "Waiting for workloads to become available"
	requeue := ctrl.Result{RequeueAfter: requeueFast}
	workloadReady := metav1.ConditionFalse
	routeReady := metav1.ConditionFalse

	routeDesired := false
	routeOK := true
	for _, svc := range resolved.Services {
		if svc.Ingress != nil && svc.Ingress.Enabled {
			routeDesired = true
			if svc.Ingress.Host == "" {
				routeOK = false
			}
		}
	}
	if !routeDesired {
		routeReady = metav1.ConditionTrue
	} else if routeOK {
		routeReady = metav1.ConditionTrue
	}

	fullyReady := allReady && depsReady
	if fullyReady {
		phase = miragev1alpha1.PhaseReady
		cond = metav1.ConditionTrue
		reason = miragev1alpha1.ReasonWorkloadReady
		msg = "All preview services available"
		workloadReady = metav1.ConditionTrue
		if expiresAt != nil {
			requeue = ctrl.Result{RequeueAfter: time.Until(expiresAt.Time)}
			if requeue.RequeueAfter < time.Second {
				requeue.RequeueAfter = time.Second
			}
		} else {
			requeue = ctrl.Result{}
		}
		if !meta.IsStatusConditionTrue(orig.Status.Conditions, miragev1alpha1.ConditionReady) {
			r.record(orig, corev1.EventTypeNormal, miragev1alpha1.ReasonWorkloadReady, "Preview ready")
			previewsReady.Inc()
		}
	} else if allReady && !depsReady {
		workloadReady = metav1.ConditionTrue
		reason = miragev1alpha1.ReasonDependenciesPending
		msg = depsMsg
	} else if failMsg != "" {
		phase = miragev1alpha1.PhaseFailed
		reason = miragev1alpha1.ReasonRolloutFailed
		msg = failMsg
	} else if pullMsg := r.imagePullMessage(ctx, pe); pullMsg != "" {
		phase = miragev1alpha1.PhaseFailed
		reason = miragev1alpha1.ReasonRolloutFailed
		msg = pullMsg
	}

	if firstURL == "" {
		firstURL = previewURL(pe)
	}

	if err := r.patchStatus(ctx, orig, statusPatch{
		Phase: phase, Ready: cond, Reason: reason, Message: msg, URL: firstURL,
		ExpiresAt: expiresAt, ReplicaStatus: strings.Join(replicaParts, ","),
		Template: resolved.TemplateName, Services: svcStatuses,
		NamespaceReady: metav1.ConditionTrue, WorkloadReady: workloadReady,
		NetworkReady: metav1.ConditionTrue, RouteReady: routeReady,
		DependenciesReady: depsCond,
	}); err != nil {
		return ctrl.Result{}, err
	}
	observePreviewResources(pe, time.Now())
	return requeue, nil
}

func (r *PreviewEnvironmentReconciler) reconcileDelete(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment) (ctrl.Result, error) {
	log.FromContext(ctx).Info("Deleting PreviewEnvironment children")
	r.record(pe, corev1.EventTypeNormal, miragev1alpha1.ReasonDeleting, "Cleaning up preview resources")
	// Always attempt Argo cleanup (template may supply backend=argocd while pe.Spec.Backend is empty).
	if err := r.deleteArgoApplication(ctx, pe); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.cleanupTarget(ctx, pe); err != nil {
		return ctrl.Result{}, err
	}
	// Wait until the owned namespace is gone or deletion is in progress.
	// If delete has not been observed yet, requeue briefly; once Terminating
	// (or NotFound / not owned), release the finalizer so the CR can finish.
	ns := &corev1.Namespace{}
	err := r.Get(ctx, types.NamespacedName{Name: pe.Spec.TargetNamespace}, ns)
	if err == nil && ownsNamespace(pe, ns) && ns.DeletionTimestamp.IsZero() {
		if delErr := r.Delete(ctx, ns); delErr != nil && !apierrors.IsNotFound(delErr) {
			return ctrl.Result{}, delErr
		}
		return ctrl.Result{RequeueAfter: namespaceDeleteWait}, nil
	}
	if err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}
	controllerutil.RemoveFinalizer(pe, miragev1alpha1.FinalizerName)
	if err := r.Update(ctx, pe); err != nil {
		return ctrl.Result{}, err
	}
	clearPreviewResources(pe)
	previewsDeleted.Inc()
	return ctrl.Result{}, nil
}

func (r *PreviewEnvironmentReconciler) record(pe *miragev1alpha1.PreviewEnvironment, eventType, reason, msg string) {
	if r.Recorder != nil {
		r.Recorder.Event(pe, eventType, reason, msg)
	}
}

func ptrInt32(p *int32, def int32) int32 {
	if p == nil {
		return def
	}
	return *p
}

func previewURL(pe *miragev1alpha1.PreviewEnvironment) string {
	if pe.Spec.Ingress != nil && pe.Spec.Ingress.Enabled && pe.Spec.Ingress.Host != "" {
		scheme := "http"
		if pe.Spec.Ingress.TLS != nil && pe.Spec.Ingress.TLS.Enabled {
			scheme = "https"
		}
		path := pe.Spec.Ingress.Path
		if path == "" || path == "/" {
			return scheme + "://" + pe.Spec.Ingress.Host
		}
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		return scheme + "://" + pe.Spec.Ingress.Host + path
	}
	return ""
}

func deploymentReady(d *appsv1.Deployment) bool {
	desired := int32(1)
	if d.Spec.Replicas != nil {
		desired = *d.Spec.Replicas
	}
	if desired == 0 {
		return true
	}
	return d.Status.ReadyReplicas >= desired &&
		d.Status.UpdatedReplicas >= desired &&
		d.Status.AvailableReplicas >= desired &&
		d.Status.ObservedGeneration >= d.Generation
}

func deploymentProgressDeadline(d *appsv1.Deployment) bool {
	for _, c := range d.Status.Conditions {
		if c.Type == appsv1.DeploymentProgressing && c.Status == corev1.ConditionFalse && c.Reason == "ProgressDeadlineExceeded" {
			return true
		}
	}
	return false
}

type statusPatch struct {
	Phase, Reason, Message, URL, ReplicaStatus, ArgoApp, Template                     string
	Ready, NamespaceReady, WorkloadReady, NetworkReady, RouteReady, DependenciesReady metav1.ConditionStatus
	ExpiresAt                                                                         *metav1.Time
	Services                                                                          []miragev1alpha1.ServiceStatus
}

func (r *PreviewEnvironmentReconciler) patchStatus(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment, p statusPatch) error {
	latest := &miragev1alpha1.PreviewEnvironment{}
	if err := r.Get(ctx, types.NamespacedName{Name: pe.Name, Namespace: pe.Namespace}, latest); err != nil {
		return err
	}
	setCond := func(condType string, status metav1.ConditionStatus, reason, msg string) {
		if status == "" {
			return
		}
		if reason == "" {
			reason = p.Reason
		}
		if msg == "" {
			msg = p.Message
		}
		meta.SetStatusCondition(&latest.Status.Conditions, metav1.Condition{
			Type:               condType,
			Status:             status,
			Reason:             reason,
			Message:            msg,
			ObservedGeneration: latest.Generation,
		})
	}
	setCond(miragev1alpha1.ConditionReady, p.Ready, p.Reason, p.Message)
	progressing := metav1.ConditionFalse
	if p.Phase == miragev1alpha1.PhasePending || p.Phase == miragev1alpha1.PhaseProvisioning || p.Phase == miragev1alpha1.PhaseExpiring {
		progressing = metav1.ConditionTrue
	}
	setCond(miragev1alpha1.ConditionProgressing, progressing, p.Reason, p.Message)
	setCond(miragev1alpha1.ConditionNamespaceReady, p.NamespaceReady, miragev1alpha1.ReasonNamespaceReady, p.Message)
	setCond(miragev1alpha1.ConditionWorkloadReady, p.WorkloadReady, p.Reason, p.Message)
	setCond(miragev1alpha1.ConditionNetworkReady, p.NetworkReady, miragev1alpha1.ReasonNetworkReady, p.Message)
	routeReason := miragev1alpha1.ReasonRouteReady
	if p.RouteReady == metav1.ConditionFalse {
		routeReason = miragev1alpha1.ReasonRoutePending
	}
	setCond(miragev1alpha1.ConditionRouteReady, p.RouteReady, routeReason, p.Message)
	depsReason := miragev1alpha1.ReasonDependenciesReady
	if p.DependenciesReady == metav1.ConditionFalse {
		depsReason = miragev1alpha1.ReasonDependenciesPending
	}
	setCond(miragev1alpha1.ConditionDependenciesReady, p.DependenciesReady, depsReason, p.Message)
	if p.Phase == miragev1alpha1.PhaseExpiring {
		setCond(miragev1alpha1.ConditionExpired, metav1.ConditionTrue, miragev1alpha1.ReasonExpiring, p.Message)
	}
	latest.Status.Phase = p.Phase
	latest.Status.URL = p.URL
	latest.Status.Message = p.Message
	latest.Status.ReplicaStatus = p.ReplicaStatus
	if p.ArgoApp != "" {
		latest.Status.ArgoApplication = p.ArgoApp
	}
	if p.Template != "" {
		latest.Status.Template = p.Template
	}
	if p.Services != nil {
		latest.Status.Services = p.Services
	}
	if p.ExpiresAt != nil {
		latest.Status.ExpiresAt = p.ExpiresAt
	}
	latest.Status.ObservedGeneration = latest.Generation
	if err := r.Status().Update(ctx, latest); err != nil {
		return err
	}
	pe.Status = latest.Status
	return nil
}

// templateRefFieldIndex is the field indexer key for PreviewEnvironment → template ref.
// Value format is "namespace/name" with empty template namespace normalized to the PE namespace.
const templateRefFieldIndex = "spec.templateRef"

func templateRefIndexKey(pe *miragev1alpha1.PreviewEnvironment) string {
	if pe == nil || pe.Spec.TemplateRef == nil || pe.Spec.TemplateRef.Name == "" {
		return ""
	}
	ns := pe.Spec.TemplateRef.Namespace
	if ns == "" {
		ns = pe.Namespace
	}
	return ns + "/" + pe.Spec.TemplateRef.Name
}

func indexPreviewEnvironmentByTemplateRef(obj client.Object) []string {
	pe, ok := obj.(*miragev1alpha1.PreviewEnvironment)
	if !ok {
		return nil
	}
	key := templateRefIndexKey(pe)
	if key == "" {
		return nil
	}
	return []string{key}
}

func (r *PreviewEnvironmentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("previewenvironment-controller")
	}
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &miragev1alpha1.PreviewEnvironment{},
		templateRefFieldIndex, indexPreviewEnvironmentByTemplateRef); err != nil {
		return err
	}
	mapOwned := handler.EnqueueRequestsFromMapFunc(func(_ context.Context, obj client.Object) []reconcile.Request {
		labels := obj.GetLabels()
		if labels[miragev1alpha1.LabelManagedBy] != miragev1alpha1.ManagedByValue {
			return nil
		}
		name := labels[miragev1alpha1.LabelOwnerName]
		ns := labels[miragev1alpha1.LabelOwnerNamespace]
		if name == "" || ns == "" {
			return nil
		}
		return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: name, Namespace: ns}}}
	})
	mapTemplate := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		return previewEnvironmentsForTemplate(ctx, r.Client, obj)
	})
	builder := ctrl.NewControllerManagedBy(mgr).
		For(&miragev1alpha1.PreviewEnvironment{}).
		Watches(&miragev1alpha1.PreviewTemplate{}, mapTemplate).
		Watches(&appsv1.Deployment{}, mapOwned).
		Watches(&corev1.Service{}, mapOwned).
		Watches(&networkingv1.Ingress{}, mapOwned)

	// Optionally watch Argo CD Applications when the CRD is available so Ready
	// previews observe Degraded transitions without waiting for the poll interval.
	if _, err := mgr.GetRESTMapper().RESTMapping(schema.GroupKind{
		Group: argoApplicationGVK.Group, Kind: argoApplicationGVK.Kind,
	}, argoApplicationGVK.Version); err == nil {
		argoApp := &unstructured.Unstructured{}
		argoApp.SetGroupVersionKind(argoApplicationGVK)
		builder = builder.Watches(argoApp, mapOwned)
	}

	return builder.Named("previewenvironment").Complete(r)
}

// previewEnvironmentsForTemplate enqueues every PreviewEnvironment that references
// the given PreviewTemplate (spec.templateRef.name + namespace defaulting to the PE namespace).
func previewEnvironmentsForTemplate(ctx context.Context, c client.Client, obj client.Object) []reconcile.Request {
	tpl, ok := obj.(*miragev1alpha1.PreviewTemplate)
	if !ok || tpl == nil {
		return nil
	}
	list := &miragev1alpha1.PreviewEnvironmentList{}
	if err := c.List(ctx, list, client.MatchingFields{
		templateRefFieldIndex: tpl.Namespace + "/" + tpl.Name,
	}); err != nil {
		return nil
	}
	reqs := make([]reconcile.Request, 0, len(list.Items))
	for i := range list.Items {
		pe := &list.Items[i]
		reqs = append(reqs, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: pe.Name, Namespace: pe.Namespace},
		})
	}
	return reqs
}

// scalePreviewWorkloadsToZero scales application Deployments (component=preview) to 0
// replicas so requireDigest failures stop running unpinned images without deleting
// dependency workloads. Prefer scale-to-zero for reversibility.
// Returns the number of Deployments that were scaled down.
func (r *PreviewEnvironmentReconciler) scalePreviewWorkloadsToZero(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment) (int, error) {
	if pe == nil || pe.Spec.TargetNamespace == "" {
		return 0, nil
	}
	var deploys appsv1.DeploymentList
	if err := r.List(ctx, &deploys, client.InNamespace(pe.Spec.TargetNamespace), client.MatchingLabels{
		miragev1alpha1.LabelManagedBy: miragev1alpha1.ManagedByValue,
		miragev1alpha1.LabelOwnerUID:  string(pe.UID),
		componentLabel:                previewComponent,
	}); err != nil {
		return 0, err
	}
	zero := int32(0)
	scaled := 0
	for i := range deploys.Items {
		d := &deploys.Items[i]
		if d.Spec.Replicas != nil && *d.Spec.Replicas == 0 {
			continue
		}
		patched := d.DeepCopy()
		patched.Spec.Replicas = &zero
		if err := r.Patch(ctx, patched, client.MergeFrom(d)); err != nil {
			return scaled, err
		}
		scaled++
	}
	return scaled, nil
}

func defaultTTLFromEnv() int64 {
	raw := strings.TrimSpace(os.Getenv("MIRAGE_DEFAULT_TTL_SECONDS"))
	if raw == "" {
		return 0
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v < 0 {
		return 0
	}
	return v
}
