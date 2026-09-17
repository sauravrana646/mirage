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
	"k8s.io/apimachinery/pkg/runtime"
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
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=resourcequotas,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=limitranges,verbs=get;list;watch;create;update;patch;delete
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

	if pe.Spec.Suspend {
		r.record(pe, corev1.EventTypeNormal, miragev1alpha1.ReasonPaused, "Preview suspended")
		previewsPaused.Inc()
		return ctrl.Result{}, r.patchStatus(ctx, pe, statusPatch{
			Phase: miragev1alpha1.PhasePaused, Ready: metav1.ConditionFalse,
			Reason: miragev1alpha1.ReasonPaused, Message: "spec.suspend=true",
		})
	}

	expiresAt := computeExpiresAt(pe, time.Now())
	if expired, err := r.handleExpiry(ctx, pe, expiresAt); expired {
		return ctrl.Result{}, err
	}

	logger.V(1).Info("reconciling preview", "targetNamespace", pe.Spec.TargetNamespace, "backend", pe.Spec.Backend)
	return r.reconcileActive(ctx, pe, expiresAt)
}

func validateSpec(pe *miragev1alpha1.PreviewEnvironment) (message, reason string) {
	if pe.Spec.Image == "" {
		return "spec.image is required", miragev1alpha1.ReasonImageInvalid
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
	return "", ""
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
	if pe.Spec.Backend == miragev1alpha1.BackendArgoCD {
		if err := r.deleteArgoApplication(ctx, pe); err != nil {
			return true, err
		}
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

func (r *PreviewEnvironmentReconciler) reconcileActive(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment, expiresAt *metav1.Time) (ctrl.Result, error) {
	if err := r.ensureNamespace(ctx, pe); err != nil {
		reason := miragev1alpha1.ReasonRolloutFailed
		if isNamespaceConflict(err) {
			reason = miragev1alpha1.ReasonNamespaceConflict
		}
		r.record(pe, corev1.EventTypeWarning, reason, err.Error())
		_ = r.patchStatus(ctx, pe, statusPatch{
			Phase: miragev1alpha1.PhaseFailed, Ready: metav1.ConditionFalse, Reason: reason, Message: err.Error(), ExpiresAt: expiresAt,
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
		return r.reconcileArgo(ctx, pe, expiresAt)
	}
	if err := r.deleteArgoApplication(ctx, pe); err != nil {
		return ctrl.Result{}, err
	}
	return r.reconcileDirect(ctx, pe, expiresAt)
}

func (r *PreviewEnvironmentReconciler) reconcileDirect(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment, expiresAt *metav1.Time) (ctrl.Result, error) {
	if err := r.ensureChildren(ctx, pe); err != nil {
		r.record(pe, corev1.EventTypeWarning, miragev1alpha1.ReasonRolloutFailed, err.Error())
		_ = r.patchStatus(ctx, pe, statusPatch{
			Phase: miragev1alpha1.PhaseFailed, Ready: metav1.ConditionFalse,
			Reason: miragev1alpha1.ReasonRolloutFailed, Message: err.Error(), ExpiresAt: expiresAt,
		})
		return ctrl.Result{}, err
	}

	deploy := &appsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{Name: pe.Name, Namespace: pe.Spec.TargetNamespace}, deploy); err != nil {
		return ctrl.Result{}, err
	}

	url := previewURL(pe)
	phase, cond, reason, msg, requeue := readinessResult(deploy, expiresAt)
	if phase != miragev1alpha1.PhaseReady {
		if pullMsg := r.imagePullMessage(ctx, pe); pullMsg != "" {
			msg = pullMsg
			reason = miragev1alpha1.ReasonRolloutFailed
			phase = miragev1alpha1.PhaseFailed
		}
	} else if !meta.IsStatusConditionTrue(pe.Status.Conditions, miragev1alpha1.ConditionReady) {
		r.record(pe, corev1.EventTypeNormal, miragev1alpha1.ReasonWorkloadReady, "Preview ready")
		previewsReady.Inc()
	}

	replicaStatus := fmt.Sprintf("%d/%d", deploy.Status.ReadyReplicas, ptrInt32(deploy.Spec.Replicas, 1))
	if err := r.patchStatus(ctx, pe, statusPatch{
		Phase: phase, Ready: cond, Reason: reason, Message: msg, URL: url, ExpiresAt: expiresAt, ReplicaStatus: replicaStatus,
	}); err != nil {
		return ctrl.Result{}, err
	}
	return requeue, nil
}

func (r *PreviewEnvironmentReconciler) reconcileDelete(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment) (ctrl.Result, error) {
	log.FromContext(ctx).Info("Deleting PreviewEnvironment children")
	r.record(pe, corev1.EventTypeNormal, miragev1alpha1.ReasonDeleting, "Cleaning up preview resources")
	if pe.Spec.Backend == miragev1alpha1.BackendArgoCD {
		if err := r.deleteArgoApplication(ctx, pe); err != nil {
			return ctrl.Result{}, err
		}
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

func readinessResult(deploy *appsv1.Deployment, expiresAt *metav1.Time) (phase string, cond metav1.ConditionStatus, reason, msg string, requeue ctrl.Result) {
	phase = miragev1alpha1.PhasePending
	cond = metav1.ConditionFalse
	reason = miragev1alpha1.ReasonReconciling
	msg = "Waiting for Deployment to become available"
	requeue = ctrl.Result{RequeueAfter: requeueFast}

	if deploymentReady(deploy) {
		phase = miragev1alpha1.PhaseReady
		cond = metav1.ConditionTrue
		reason = miragev1alpha1.ReasonWorkloadReady
		msg = "Deployment available"
		if expiresAt != nil {
			requeue = ctrl.Result{RequeueAfter: time.Until(expiresAt.Time)}
			if requeue.RequeueAfter < time.Second {
				requeue.RequeueAfter = time.Second
			}
		} else {
			requeue = ctrl.Result{}
		}
		return phase, cond, reason, msg, requeue
	}
	if deploymentProgressDeadline(deploy) {
		phase = miragev1alpha1.PhaseFailed
		reason = miragev1alpha1.ReasonRolloutFailed
		msg = "Deployment is not progressing; check ImagePullBackOff or crash loops"
	}
	return phase, cond, reason, msg, requeue
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
	Phase, Reason, Message, URL, ReplicaStatus, ArgoApp string
	Ready                                               metav1.ConditionStatus
	ExpiresAt                                           *metav1.Time
}

func (r *PreviewEnvironmentReconciler) patchStatus(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment, p statusPatch) error {
	latest := &miragev1alpha1.PreviewEnvironment{}
	if err := r.Get(ctx, types.NamespacedName{Name: pe.Name, Namespace: pe.Namespace}, latest); err != nil {
		return err
	}
	meta.SetStatusCondition(&latest.Status.Conditions, metav1.Condition{
		Type:               miragev1alpha1.ConditionReady,
		Status:             p.Ready,
		Reason:             p.Reason,
		Message:            p.Message,
		ObservedGeneration: latest.Generation,
	})
	progressing := metav1.ConditionFalse
	if p.Phase == miragev1alpha1.PhasePending || p.Phase == miragev1alpha1.PhaseExpiring {
		progressing = metav1.ConditionTrue
	}
	meta.SetStatusCondition(&latest.Status.Conditions, metav1.Condition{
		Type:               miragev1alpha1.ConditionProgressing,
		Status:             progressing,
		Reason:             p.Reason,
		Message:            p.Message,
		ObservedGeneration: latest.Generation,
	})
	if p.Phase == miragev1alpha1.PhaseExpiring {
		meta.SetStatusCondition(&latest.Status.Conditions, metav1.Condition{
			Type:               miragev1alpha1.ConditionExpired,
			Status:             metav1.ConditionTrue,
			Reason:             miragev1alpha1.ReasonExpiring,
			Message:            p.Message,
			ObservedGeneration: latest.Generation,
		})
	}
	latest.Status.Phase = p.Phase
	latest.Status.URL = p.URL
	latest.Status.Message = p.Message
	latest.Status.ReplicaStatus = p.ReplicaStatus
	if p.ArgoApp != "" {
		latest.Status.ArgoApplication = p.ArgoApp
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

func (r *PreviewEnvironmentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("previewenvironment-controller")
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
	return ctrl.NewControllerManagedBy(mgr).
		For(&miragev1alpha1.PreviewEnvironment{}).
		Watches(&appsv1.Deployment{}, mapOwned).
		Watches(&corev1.Service{}, mapOwned).
		Watches(&networkingv1.Ingress{}, mapOwned).
		Named("previewenvironment").
		Complete(r)
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
