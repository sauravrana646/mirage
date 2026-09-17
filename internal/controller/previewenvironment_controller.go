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
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	miragev1alpha1 "github.com/sauravrana646/mirage/api/v1alpha1"
)

const (
	defaultReplicas      int32 = 1
	defaultContainerPort int32 = 8080
	appLabel                   = "app.kubernetes.io/name"
	componentLabel             = "app.kubernetes.io/component"
	requeueFast                = 15 * time.Second
)

// PreviewEnvironmentReconciler reconciles a PreviewEnvironment object.
//
// Ownership model (see docs/decisions.md): the CR is Namespaced and cannot
// owner-reference a Namespace or workloads in another namespace. Cleanup is
// done via a finalizer that deletes the labeled targetNamespace (cascading
// children). Workloads are labeled with mirage.dev/owner-uid for conflict checks.
type PreviewEnvironmentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=mirage.dev,resources=previewenvironments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mirage.dev,resources=previewenvironments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=mirage.dev,resources=previewenvironments/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile moves cluster state toward the PreviewEnvironment spec.
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

	if pe.Spec.Image == "" {
		if err := r.patchStatus(ctx, pe, miragev1alpha1.PhaseFailed, metav1.ConditionFalse,
			miragev1alpha1.ReasonImageInvalid, "spec.image is required", "", nil); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if pe.Spec.TargetNamespace == "" {
		if err := r.patchStatus(ctx, pe, miragev1alpha1.PhaseFailed, metav1.ConditionFalse,
			miragev1alpha1.ReasonImageInvalid, "spec.targetNamespace is required", "", nil); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if pe.Spec.Backend != "" && pe.Spec.Backend != "direct" {
		if err := r.patchStatus(ctx, pe, miragev1alpha1.PhaseFailed, metav1.ConditionFalse,
			miragev1alpha1.ReasonRolloutFailed, fmt.Sprintf("backend %q is not implemented yet", pe.Spec.Backend), "", nil); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	now := time.Now()
	expiresAt := pe.Status.ExpiresAt.DeepCopy()
	if pe.Spec.TTLSeconds != nil && *pe.Spec.TTLSeconds > 0 && expiresAt == nil {
		t := metav1.NewTime(now.Add(time.Duration(*pe.Spec.TTLSeconds) * time.Second))
		expiresAt = &t
	}

	if expiresAt != nil && !expiresAt.After(now) {
		logger.Info("TTL expired; cleaning up preview", "expiresAt", expiresAt.Time)
		if err := r.patchStatus(ctx, pe, miragev1alpha1.PhaseExpiring, metav1.ConditionFalse,
			miragev1alpha1.ReasonExpiring, "TTL elapsed; deleting preview", pe.Status.URL, expiresAt); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.cleanupTarget(ctx, pe); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Delete(ctx, pe); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if err := r.ensureNamespace(ctx, pe); err != nil {
		reason := miragev1alpha1.ReasonRolloutFailed
		if isNamespaceConflict(err) {
			reason = miragev1alpha1.ReasonNamespaceConflict
		}
		_ = r.patchStatus(ctx, pe, miragev1alpha1.PhaseFailed, metav1.ConditionFalse, reason, err.Error(), "", expiresAt)
		return ctrl.Result{RequeueAfter: requeueFast}, nil
	}

	deploy, err := r.ensureDeployment(ctx, pe)
	if err != nil {
		_ = r.patchStatus(ctx, pe, miragev1alpha1.PhaseFailed, metav1.ConditionFalse,
			miragev1alpha1.ReasonRolloutFailed, err.Error(), "", expiresAt)
		return ctrl.Result{}, err
	}

	if err := r.ensureService(ctx, pe); err != nil {
		_ = r.patchStatus(ctx, pe, miragev1alpha1.PhaseFailed, metav1.ConditionFalse,
			miragev1alpha1.ReasonRolloutFailed, err.Error(), "", expiresAt)
		return ctrl.Result{}, err
	}

	url := ""
	if pe.Spec.Ingress != nil && pe.Spec.Ingress.Enabled {
		if err := r.ensureIngress(ctx, pe); err != nil {
			_ = r.patchStatus(ctx, pe, miragev1alpha1.PhaseFailed, metav1.ConditionFalse,
				miragev1alpha1.ReasonRolloutFailed, err.Error(), "", expiresAt)
			return ctrl.Result{}, err
		}
		if pe.Spec.Ingress.Host != "" {
			url = "http://" + pe.Spec.Ingress.Host
		}
	}

	ready := deploymentReady(deploy)
	phase := miragev1alpha1.PhasePending
	cond := metav1.ConditionFalse
	reason := miragev1alpha1.ReasonReconciling
	msg := "Waiting for Deployment to become available"
	requeue := ctrl.Result{RequeueAfter: requeueFast}

	if ready {
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
	} else if deploymentProgressDeadline(deploy) {
		phase = miragev1alpha1.PhaseFailed
		reason = miragev1alpha1.ReasonRolloutFailed
		msg = "Deployment is not progressing; check ImagePullBackOff or crash loops"
	}

	if err := r.patchStatus(ctx, pe, phase, cond, reason, msg, url, expiresAt); err != nil {
		return ctrl.Result{}, err
	}
	return requeue, nil
}

func (r *PreviewEnvironmentReconciler) reconcileDelete(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Deleting PreviewEnvironment children")
	if err := r.cleanupTarget(ctx, pe); err != nil {
		return ctrl.Result{}, err
	}
	controllerutil.RemoveFinalizer(pe, miragev1alpha1.FinalizerName)
	if err := r.Update(ctx, pe); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *PreviewEnvironmentReconciler) cleanupTarget(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment) error {
	if pe.Spec.TargetNamespace == "" {
		return nil
	}
	ns := &corev1.Namespace{}
	err := r.Get(ctx, types.NamespacedName{Name: pe.Spec.TargetNamespace}, ns)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !ownsNamespace(pe, ns) {
		return nil
	}
	if err := r.Delete(ctx, ns); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

type namespaceConflictError struct {
	name string
}

func (e *namespaceConflictError) Error() string {
	return fmt.Sprintf("namespace %q exists but is not owned by this PreviewEnvironment", e.name)
}

func isNamespaceConflict(err error) bool {
	_, ok := err.(*namespaceConflictError)
	return ok
}

func ownsNamespace(pe *miragev1alpha1.PreviewEnvironment, ns *corev1.Namespace) bool {
	if ns.Labels == nil {
		return false
	}
	return ns.Labels[miragev1alpha1.LabelOwnerUID] == string(pe.UID)
}

func (r *PreviewEnvironmentReconciler) ensureNamespace(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment) error {
	ns := &corev1.Namespace{}
	err := r.Get(ctx, types.NamespacedName{Name: pe.Spec.TargetNamespace}, ns)
	if apierrors.IsNotFound(err) {
		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: pe.Spec.TargetNamespace,
				Labels: map[string]string{
					miragev1alpha1.LabelManagedBy:      miragev1alpha1.ManagedByValue,
					miragev1alpha1.LabelOwnerUID:       string(pe.UID),
					miragev1alpha1.LabelOwnerName:      pe.Name,
					miragev1alpha1.LabelOwnerNamespace: pe.Namespace,
				},
			},
		}
		return r.Create(ctx, ns)
	}
	if err != nil {
		return err
	}
	if !ownsNamespace(pe, ns) {
		return &namespaceConflictError{name: ns.Name}
	}
	return nil
}

func (r *PreviewEnvironmentReconciler) workloadLabels(pe *miragev1alpha1.PreviewEnvironment) map[string]string {
	return map[string]string{
		appLabel:                           pe.Name,
		componentLabel:                     "preview",
		miragev1alpha1.LabelManagedBy:      miragev1alpha1.ManagedByValue,
		miragev1alpha1.LabelOwnerUID:       string(pe.UID),
		miragev1alpha1.LabelOwnerName:      pe.Name,
		miragev1alpha1.LabelOwnerNamespace: pe.Namespace,
	}
}

func (r *PreviewEnvironmentReconciler) ensureDeployment(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment) (*appsv1.Deployment, error) {
	replicas := defaultReplicas
	if pe.Spec.Replicas != nil {
		replicas = *pe.Spec.Replicas
	}
	port := pe.Spec.ContainerPort
	if port == 0 {
		port = defaultContainerPort
	}
	labels := r.workloadLabels(pe)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pe.Name,
			Namespace: pe.Spec.TargetNamespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, deploy, func() error {
		deploy.Labels = labels
		deploy.Spec.Replicas = &replicas
		if deploy.Spec.Selector == nil {
			deploy.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{appLabel: pe.Name}}
		}
		deploy.Spec.Template.ObjectMeta.Labels = labels
		deploy.Spec.Template.Spec.Containers = []corev1.Container{{
			Name:      "app",
			Image:     pe.Spec.Image,
			Env:       pe.Spec.Env,
			Resources: pe.Spec.Resources,
			Ports: []corev1.ContainerPort{{
				Name:          "http",
				ContainerPort: port,
			}},
		}}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := r.Get(ctx, types.NamespacedName{Name: deploy.Name, Namespace: deploy.Namespace}, deploy); err != nil {
		return nil, err
	}
	return deploy, nil
}

func (r *PreviewEnvironmentReconciler) ensureService(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment) error {
	port := pe.Spec.ContainerPort
	if port == 0 {
		port = defaultContainerPort
	}
	labels := r.workloadLabels(pe)
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pe.Name,
			Namespace: pe.Spec.TargetNamespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		svc.Labels = labels
		svc.Spec.Selector = map[string]string{appLabel: pe.Name}
		svc.Spec.Ports = []corev1.ServicePort{{
			Name:       "http",
			Port:       80,
			TargetPort: intstr.FromInt32(port),
			Protocol:   corev1.ProtocolTCP,
		}}
		return nil
	})
	return err
}

func (r *PreviewEnvironmentReconciler) ensureIngress(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment) error {
	if pe.Spec.Ingress == nil || pe.Spec.Ingress.Host == "" {
		return fmt.Errorf("ingress.enabled requires ingress.host")
	}
	labels := r.workloadLabels(pe)
	pathType := networkingv1.PathTypePrefix
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pe.Name,
			Namespace: pe.Spec.TargetNamespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, ing, func() error {
		ing.Labels = labels
		ing.Spec.IngressClassName = pe.Spec.Ingress.IngressClassName
		ing.Spec.Rules = []networkingv1.IngressRule{{
			Host: pe.Spec.Ingress.Host,
			IngressRuleValue: networkingv1.IngressRuleValue{
				HTTP: &networkingv1.HTTPIngressRuleValue{
					Paths: []networkingv1.HTTPIngressPath{{
						Path:     "/",
						PathType: &pathType,
						Backend: networkingv1.IngressBackend{
							Service: &networkingv1.IngressServiceBackend{
								Name: pe.Name,
								Port: networkingv1.ServiceBackendPort{Name: "http"},
							},
						},
					}},
				},
			},
		}}
		return nil
	})
	return err
}

func deploymentReady(d *appsv1.Deployment) bool {
	if d.Spec.Replicas != nil && *d.Spec.Replicas == 0 {
		return true
	}
	return d.Status.ReadyReplicas > 0 &&
		d.Status.UpdatedReplicas == d.Status.ReadyReplicas &&
		d.Status.AvailableReplicas == d.Status.ReadyReplicas &&
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

func (r *PreviewEnvironmentReconciler) patchStatus(
	ctx context.Context,
	pe *miragev1alpha1.PreviewEnvironment,
	phase string,
	readyStatus metav1.ConditionStatus,
	reason, message, url string,
	expiresAt *metav1.Time,
) error {
	latest := &miragev1alpha1.PreviewEnvironment{}
	if err := r.Get(ctx, types.NamespacedName{Name: pe.Name, Namespace: pe.Namespace}, latest); err != nil {
		return err
	}
	meta.SetStatusCondition(&latest.Status.Conditions, metav1.Condition{
		Type:               miragev1alpha1.ConditionReady,
		Status:             readyStatus,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: latest.Generation,
	})
	latest.Status.Phase = phase
	latest.Status.URL = url
	if expiresAt != nil {
		latest.Status.ExpiresAt = expiresAt
	}
	latest.Status.ObservedGeneration = latest.Generation
	if err := r.Status().Update(ctx, latest); err != nil {
		return err
	}
	pe.Status = latest.Status
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PreviewEnvironmentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&miragev1alpha1.PreviewEnvironment{}).
		Named("previewenvironment").
		Complete(r)
}
