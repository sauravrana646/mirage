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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	miragev1alpha1 "github.com/sauravrana646/mirage/api/v1alpha1"
)

var argoApplicationGVK = schema.GroupVersionKind{
	Group:   "argoproj.io",
	Version: "v1alpha1",
	Kind:    "Application",
}

func (r *PreviewEnvironmentReconciler) reconcileArgo(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment, expiresAt *metav1.Time) (ctrl.Result, error) {
	if pe.Spec.ArgoCD == nil {
		return ctrl.Result{}, r.patchStatus(ctx, pe, statusPatch{
			Phase: miragev1alpha1.PhaseFailed, Ready: metav1.ConditionFalse,
			Reason: miragev1alpha1.ReasonInvalidSpec, Message: "spec.argoCD is required when backend=argocd", ExpiresAt: expiresAt,
		})
	}
	app, err := r.ensureArgoApplication(ctx, pe)
	if err != nil {
		r.record(pe, corev1.EventTypeWarning, miragev1alpha1.ReasonRolloutFailed, err.Error())
		_ = r.patchStatus(ctx, pe, statusPatch{
			Phase: miragev1alpha1.PhaseFailed, Ready: metav1.ConditionFalse,
			Reason: miragev1alpha1.ReasonRolloutFailed, Message: err.Error(), ExpiresAt: expiresAt,
		})
		return ctrl.Result{}, err
	}

	health, _, _ := unstructured.NestedString(app.Object, "status", "health", "status")
	sync, _, _ := unstructured.NestedString(app.Object, "status", "sync", "status")
	appName := app.GetNamespace() + "/" + app.GetName()
	url := previewURL(pe)

	phase := miragev1alpha1.PhasePending
	ready := metav1.ConditionFalse
	reason := miragev1alpha1.ReasonArgoSyncing
	msg := fmt.Sprintf("Argo CD sync=%s health=%s", sync, health)
	requeue := ctrl.Result{RequeueAfter: requeueFast}

	if health == "Healthy" && sync == "Synced" {
		phase = miragev1alpha1.PhaseReady
		ready = metav1.ConditionTrue
		reason = miragev1alpha1.ReasonArgoHealthy
		msg = "Argo CD Application healthy and synced"
		r.record(pe, corev1.EventTypeNormal, reason, msg)
		if expiresAt != nil {
			requeue = ctrl.Result{RequeueAfter: expiresAt.Sub(expiresAt.Time) + expiresAt.Time.Sub(expiresAt.Time)}
			requeue = ctrl.Result{RequeueAfter: requeueFast}
			if d := expiresAt.Sub(metav1.Now().Time); d > 0 {
				requeue = ctrl.Result{RequeueAfter: d}
			}
		} else {
			requeue = ctrl.Result{RequeueAfter: requeueFast}
		}
	} else if health == "Degraded" {
		phase = miragev1alpha1.PhaseFailed
		reason = miragev1alpha1.ReasonRolloutFailed
	}

	if err := r.patchStatus(ctx, pe, statusPatch{
		Phase: phase, Ready: ready, Reason: reason, Message: msg, URL: url, ExpiresAt: expiresAt, ArgoApp: appName,
	}); err != nil {
		return ctrl.Result{}, err
	}
	return requeue, nil
}

func (r *PreviewEnvironmentReconciler) ensureArgoApplication(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment) (*unstructured.Unstructured, error) {
	spec := pe.Spec.ArgoCD
	argoNS := spec.ArgoNamespace
	if argoNS == "" {
		argoNS = "argocd"
	}
	destNS := spec.DestinationNamespace
	if destNS == "" {
		destNS = pe.Spec.TargetNamespace
	}
	project := spec.Project
	if project == "" {
		project = "default"
	}
	revision := spec.TargetRevision
	if revision == "" && pe.Spec.Source != nil && pe.Spec.Source.CommitSHA != "" {
		revision = pe.Spec.Source.CommitSHA
	}
	if revision == "" {
		revision = "HEAD"
	}

	app := &unstructured.Unstructured{}
	app.SetGroupVersionKind(argoApplicationGVK)
	app.SetName(pe.Name)
	app.SetNamespace(argoNS)

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, app, func() error {
		labels := r.workloadLabels(pe)
		app.SetLabels(labels)
		_ = unstructured.SetNestedField(app.Object, project, "spec", "project")
		_ = unstructured.SetNestedMap(app.Object, map[string]interface{}{
			"repoURL":        spec.RepoURL,
			"path":           spec.Path,
			"targetRevision": revision,
		}, "spec", "source")
		_ = unstructured.SetNestedMap(app.Object, map[string]interface{}{
			"server":    "https://kubernetes.default.svc",
			"namespace": destNS,
		}, "spec", "destination")
		_ = unstructured.SetNestedMap(app.Object, map[string]interface{}{
			"automated":   map[string]interface{}{"prune": true, "selfHeal": true},
			"syncOptions": []interface{}{"CreateNamespace=true"},
		}, "spec", "syncPolicy")
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := r.Get(ctx, types.NamespacedName{Name: app.GetName(), Namespace: app.GetNamespace()}, app); err != nil {
		return nil, err
	}
	return app, nil
}

func (r *PreviewEnvironmentReconciler) deleteArgoApplication(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment) error {
	argoNS := "argocd"
	if pe.Spec.ArgoCD != nil && pe.Spec.ArgoCD.ArgoNamespace != "" {
		argoNS = pe.Spec.ArgoCD.ArgoNamespace
	}
	app := &unstructured.Unstructured{}
	app.SetGroupVersionKind(argoApplicationGVK)
	app.SetName(pe.Name)
	app.SetNamespace(argoNS)
	if err := r.Delete(ctx, app); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}
