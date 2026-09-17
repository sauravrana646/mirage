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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	miragev1alpha1 "github.com/sauravrana646/mirage/api/v1alpha1"
)

const (
	argoHealthPollInterval = 30 * time.Second
	argoNameHashLen        = 8
	argoDNSLabelMax        = 63
)

var argoApplicationGVK = schema.GroupVersionKind{
	Group:   "argoproj.io",
	Version: "v1alpha1",
	Kind:    "Application",
}

type argoOwnershipConflictError struct {
	name, existingOwner string
}

func (e *argoOwnershipConflictError) Error() string {
	return fmt.Sprintf("Argo CD Application %q is owned by %s", e.name, e.existingOwner)
}

func (r *PreviewEnvironmentReconciler) reconcileArgo(ctx context.Context, resolved *resolvedPreview, orig *miragev1alpha1.PreviewEnvironment, expiresAt *metav1.Time) (ctrl.Result, error) {
	pe := resolved.PE
	if pe.Spec.ArgoCD == nil || pe.Spec.ArgoCD.RepoURL == "" || pe.Spec.ArgoCD.Path == "" {
		msg := "spec.argoCD.repoURL and path are required when backend=argocd"
		r.record(orig, corev1.EventTypeWarning, miragev1alpha1.ReasonInvalidSpec, msg)
		return ctrl.Result{}, r.patchStatus(ctx, orig, statusPatch{
			Phase: miragev1alpha1.PhaseFailed, Ready: metav1.ConditionFalse,
			Reason: miragev1alpha1.ReasonInvalidSpec, Message: msg, ExpiresAt: expiresAt,
			Template: resolved.TemplateName,
		})
	}
	app, err := r.ensureArgoApplication(ctx, pe)
	if err != nil {
		reason := miragev1alpha1.ReasonRolloutFailed
		if _, ok := err.(*argoOwnershipConflictError); ok {
			reason = miragev1alpha1.ReasonInvalidSpec
		}
		r.record(orig, corev1.EventTypeWarning, reason, err.Error())
		_ = r.patchStatus(ctx, orig, statusPatch{
			Phase: miragev1alpha1.PhaseFailed, Ready: metav1.ConditionFalse,
			Reason: reason, Message: err.Error(), ExpiresAt: expiresAt,
			Template: resolved.TemplateName,
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
		r.record(orig, corev1.EventTypeNormal, reason, msg)
		// Keep polling health even when Ready (Applications can become Degraded).
		requeue = ctrl.Result{RequeueAfter: argoHealthPollInterval}
		if expiresAt != nil {
			untilExpiry := time.Until(expiresAt.Time)
			if untilExpiry < time.Second {
				untilExpiry = time.Second
			}
			if untilExpiry < requeue.RequeueAfter {
				requeue.RequeueAfter = untilExpiry
			}
		}
	} else if health == "Degraded" {
		phase = miragev1alpha1.PhaseFailed
		reason = miragev1alpha1.ReasonRolloutFailed
	}

	if err := r.patchStatus(ctx, orig, statusPatch{
		Phase: phase, Ready: ready, Reason: reason, Message: msg, URL: url, ExpiresAt: expiresAt, ArgoApp: appName,
		Template: resolved.TemplateName,
	}); err != nil {
		return ctrl.Result{}, err
	}
	observePreviewResources(orig, time.Now())
	return requeue, nil
}

func (r *PreviewEnvironmentReconciler) ensureArgoApplication(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment) (*unstructured.Unstructured, error) {
	spec := pe.Spec.ArgoCD
	if spec == nil {
		return nil, fmt.Errorf("spec.argoCD is required when backend=argocd")
	}
	nn, err := r.desiredArgoApplicationRef(ctx, pe)
	if err != nil {
		return nil, err
	}
	destNS := pe.Spec.TargetNamespace
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
	app.SetName(nn.Name)
	app.SetNamespace(nn.Namespace)

	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, app, func() error {
		if conflict := argoOwnershipConflict(pe, app); conflict != nil {
			return conflict
		}
		labels := r.workloadLabels(pe, pe.Name)
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

// desiredArgoApplicationRef prefers status.argoApplication when it points at an
// existing Application we own in the configured Argo namespace. This preserves
// legacy <pe.Name> Applications across upgrades without creating duplicates.
func (r *PreviewEnvironmentReconciler) desiredArgoApplicationRef(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment) (types.NamespacedName, error) {
	expectedNS := argoNamespace(pe)
	if ref := strings.TrimSpace(pe.Status.ArgoApplication); ref != "" {
		ns, name, ok := splitNamespacedName(ref)
		if ok {
			if ns != expectedNS {
				return types.NamespacedName{}, &argoOwnershipConflictError{
					name:          ref,
					existingOwner: "a different Argo namespace than configured",
				}
			}
			existing := &unstructured.Unstructured{}
			existing.SetGroupVersionKind(argoApplicationGVK)
			err := r.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, existing)
			if err == nil {
				if conflict := argoOwnershipConflict(pe, existing); conflict != nil {
					return types.NamespacedName{}, conflict
				}
				return types.NamespacedName{Namespace: ns, Name: name}, nil
			}
			if !apierrors.IsNotFound(err) && !meta.IsNoMatchError(err) {
				return types.NamespacedName{}, err
			}
			// Status points at a missing Application — fall through to generated name.
		}
	}
	return types.NamespacedName{Namespace: expectedNS, Name: argoApplicationName(pe)}, nil
}

func argoOwnershipConflict(pe *miragev1alpha1.PreviewEnvironment, app *unstructured.Unstructured) error {
	if app == nil || app.GetUID() == "" && len(app.GetLabels()) == 0 {
		return nil
	}
	labels := app.GetLabels()
	if labels == nil {
		return nil
	}
	ownerUID := labels[miragev1alpha1.LabelOwnerUID]
	if ownerUID == "" {
		// Unlabeled Application occupying our deterministic name — treat as conflict.
		if app.GetUID() != "" {
			return &argoOwnershipConflictError{
				name:          app.GetNamespace() + "/" + app.GetName(),
				existingOwner: "an unlabeled Application",
			}
		}
		return nil
	}
	if ownerUID != string(pe.UID) {
		owner := labels[miragev1alpha1.LabelOwnerNamespace] + "/" + labels[miragev1alpha1.LabelOwnerName]
		if owner == "/" {
			owner = ownerUID
		}
		return &argoOwnershipConflictError{
			name:          app.GetNamespace() + "/" + app.GetName(),
			existingOwner: owner,
		}
	}
	return nil
}

func argoNamespace(pe *miragev1alpha1.PreviewEnvironment) string {
	if pe.Spec.ArgoCD != nil && pe.Spec.ArgoCD.ArgoNamespace != "" {
		return pe.Spec.ArgoCD.ArgoNamespace
	}
	return miragev1alpha1.DefaultArgoNamespace
}

// argoApplicationName returns a DNS-1123 label unique per PreviewEnvironment
// identity (namespace/name), staying within 63 characters.
func argoApplicationName(pe *miragev1alpha1.PreviewEnvironment) string {
	key := pe.Namespace + "/" + pe.Name
	sum := sha256.Sum256([]byte(key))
	hash := hex.EncodeToString(sum[:])[:argoNameHashLen]
	base := sanitizeDNSLabel(pe.Namespace + "-" + pe.Name)
	// leave room for "-" + hash
	maxBase := argoDNSLabelMax - 1 - argoNameHashLen
	if len(base) > maxBase {
		base = strings.Trim(base[:maxBase], "-")
	}
	if base == "" {
		base = "pe"
	}
	return base + "-" + hash
}

func sanitizeDNSLabel(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	b.Grow(len(s))
	prevDash := false
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-'
		if !ok {
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
			continue
		}
		b.WriteRune(r)
		prevDash = r == '-'
	}
	out := strings.Trim(b.String(), "-")
	return out
}

// deleteArgoApplication removes Mirage-managed Applications for this PE.
// Template-backed backends may leave pe.Spec.Backend empty, so cleanup always
// runs. Only Applications with matching mirage ownership labels are deleted —
// never unlabeled or foreign Applications (including legacy name probes).
func (r *PreviewEnvironmentReconciler) deleteArgoApplication(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment) error {
	candidates := argoDeleteCandidates(pe)
	var firstErr error
	for _, nn := range candidates {
		app := &unstructured.Unstructured{}
		app.SetGroupVersionKind(argoApplicationGVK)
		err := r.Get(ctx, nn, app)
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			continue
		}
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if !argoOwnedBy(pe, app) {
			continue
		}
		if err := r.Delete(ctx, app); err != nil && !apierrors.IsNotFound(err) && !meta.IsNoMatchError(err) {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func argoOwnedBy(pe *miragev1alpha1.PreviewEnvironment, app *unstructured.Unstructured) bool {
	labels := app.GetLabels()
	if labels == nil {
		return false
	}
	if labels[miragev1alpha1.LabelManagedBy] != miragev1alpha1.ManagedByValue {
		return false
	}
	return labels[miragev1alpha1.LabelOwnerUID] == string(pe.UID)
}

func argoDeleteCandidates(pe *miragev1alpha1.PreviewEnvironment) []types.NamespacedName {
	seen := map[string]struct{}{}
	var out []types.NamespacedName
	add := func(ns, name string) {
		if ns == "" || name == "" {
			return
		}
		key := ns + "/" + name
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, types.NamespacedName{Namespace: ns, Name: name})
	}

	if ref := strings.TrimSpace(pe.Status.ArgoApplication); ref != "" {
		if ns, name, ok := splitNamespacedName(ref); ok {
			add(ns, name)
		}
	}
	add(argoNamespace(pe), argoApplicationName(pe))
	// Legacy name used before unique naming (pe.Name in argo namespace).
	add(argoNamespace(pe), pe.Name)
	return out
}

func splitNamespacedName(ref string) (ns, name string, ok bool) {
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}
