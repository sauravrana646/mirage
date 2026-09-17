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

	"k8s.io/apimachinery/pkg/types"

	miragev1alpha1 "github.com/sauravrana646/mirage/api/v1alpha1"
	"github.com/sauravrana646/mirage/internal/resolve"
)

// resolvedPreview is the PreviewEnvironment after template defaults are applied.
type resolvedPreview struct {
	PE               *miragev1alpha1.PreviewEnvironment
	TemplateName     string
	RuntimeClassName string
	PSA              string
	Services         []effectiveService
}

// effectiveService aliases the shared resolve type used by resource builders.
type effectiveService = resolve.EffectiveService

func (r *PreviewEnvironmentReconciler) resolvePreview(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment) (*resolvedPreview, error) {
	out := &resolvedPreview{PE: pe.DeepCopy()}
	if pe.Spec.TemplateRef != nil && pe.Spec.TemplateRef.Name != "" {
		tplNS := pe.Spec.TemplateRef.Namespace
		if tplNS == "" {
			tplNS = pe.Namespace
		}
		tpl := &miragev1alpha1.PreviewTemplate{}
		if err := r.Get(ctx, types.NamespacedName{Name: pe.Spec.TemplateRef.Name, Namespace: tplNS}, tpl); err != nil {
			return nil, fmt.Errorf("%s: %w", miragev1alpha1.ReasonTemplateNotFound, err)
		}
		resolve.ApplyTemplate(out.PE, tpl)
		out.TemplateName = tpl.Name
		if tpl.Spec.Security != nil {
			out.RuntimeClassName = tpl.Spec.Security.RuntimeClassName
			if tpl.Spec.Security.Profile != "" {
				out.PSA = string(tpl.Spec.Security.Profile)
			}
		}
	}
	if out.PSA == "" {
		out.PSA = string(miragev1alpha1.SecurityProfileRestricted)
	}
	if pe.Spec.RuntimeClassName != "" {
		out.RuntimeClassName = pe.Spec.RuntimeClassName
	}
	svcs, err := resolve.EffectiveServices(out.PE)
	if err != nil {
		return nil, err
	}
	out.Services = svcs
	return out, nil
}

// Thin wrappers keep existing controller tests stable.
func applyTemplate(pe *miragev1alpha1.PreviewEnvironment, tpl *miragev1alpha1.PreviewTemplate) {
	resolve.ApplyTemplate(pe, tpl)
}

func effectiveServices(pe *miragev1alpha1.PreviewEnvironment) ([]effectiveService, error) {
	return resolve.EffectiveServices(pe)
}
