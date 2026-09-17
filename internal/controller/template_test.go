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
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	miragev1alpha1 "github.com/sauravrana646/mirage/api/v1alpha1"
)

func TestEffectiveServicesSingle(t *testing.T) {
	pe := &miragev1alpha1.PreviewEnvironment{
		ObjectMeta: metav1.ObjectMeta{Name: "pr-1"},
		Spec: miragev1alpha1.PreviewEnvironmentSpec{
			Image:         "ghcr.io/acme/app@sha256:abc",
			ContainerPort: 8080,
		},
	}
	svcs, err := effectiveServices(pe)
	if err != nil {
		t.Fatal(err)
	}
	if len(svcs) != 1 || svcs[0].WorkloadName != "pr-1" || svcs[0].Image != pe.Spec.Image {
		t.Fatalf("unexpected single service: %+v", svcs)
	}
}

func TestEffectiveServicesMulti(t *testing.T) {
	pe := &miragev1alpha1.PreviewEnvironment{
		ObjectMeta: metav1.ObjectMeta{Name: "pr-2"},
		Spec: miragev1alpha1.PreviewEnvironmentSpec{
			Services: []miragev1alpha1.PreviewServiceSpec{
				{Name: "api", Image: "ghcr.io/acme/api:1", Port: 8080},
				{Name: "web", Image: "ghcr.io/acme/web:1", Port: 3000, Ingress: &miragev1alpha1.IngressSpec{Enabled: true, Host: "web.preview.local"}},
			},
		},
	}
	svcs, err := effectiveServices(pe)
	if err != nil {
		t.Fatal(err)
	}
	if len(svcs) != 2 {
		t.Fatalf("want 2 services, got %d", len(svcs))
	}
	if svcs[0].Name != "api" || svcs[1].Ingress == nil || !svcs[1].Ingress.Enabled {
		t.Fatalf("unexpected multi services: %+v", svcs)
	}
}

func TestEffectiveServicesRequiresImageOrServices(t *testing.T) {
	pe := &miragev1alpha1.PreviewEnvironment{ObjectMeta: metav1.ObjectMeta{Name: "x"}}
	if _, err := effectiveServices(pe); err == nil {
		t.Fatal("expected error when image and services empty")
	}
}

func TestApplyTemplate(t *testing.T) {
	ttl := int64(3600)
	replicas := int32(2)
	pe := &miragev1alpha1.PreviewEnvironment{
		ObjectMeta: metav1.ObjectMeta{Name: "pr-9"},
		Spec:       miragev1alpha1.PreviewEnvironmentSpec{Image: "nginx:1"},
	}
	tpl := &miragev1alpha1.PreviewTemplate{
		Spec: miragev1alpha1.PreviewTemplateSpec{
			TTLSeconds:    &ttl,
			Replicas:      &replicas,
			NetworkPolicy: miragev1alpha1.NetworkPolicyPermissive,
			Resources:     &miragev1alpha1.ResourcePresetSpec{Preset: miragev1alpha1.ResourcePresetSmall},
			Ingress: &miragev1alpha1.IngressTemplateSpec{
				Enabled: true,
				Domain:  "preview.example.com",
			},
		},
	}
	applyTemplate(pe, tpl)
	if pe.Spec.TTLSeconds == nil || *pe.Spec.TTLSeconds != 3600 {
		t.Fatalf("ttl not applied: %v", pe.Spec.TTLSeconds)
	}
	if pe.Spec.NetworkPolicy != miragev1alpha1.NetworkPolicyPermissive {
		t.Fatalf("networkPolicy not applied")
	}
	if pe.Spec.Ingress == nil || pe.Spec.Ingress.Host != "pr-9.preview.example.com" {
		t.Fatalf("ingress host not derived: %+v", pe.Spec.Ingress)
	}
	cpu := pe.Spec.Resources.Requests[corev1.ResourceCPU]
	if !cpu.Equal(resource.MustParse("50m")) {
		t.Fatalf("preset resources not applied: %v", pe.Spec.Resources)
	}
}

func TestValidateRequireDigest(t *testing.T) {
	ok := &resolvedPreview{
		PE: &miragev1alpha1.PreviewEnvironment{
			Spec: miragev1alpha1.PreviewEnvironmentSpec{RequireDigest: true},
		},
		Services: []effectiveService{{Name: "api", Image: "ghcr.io/acme/api@sha256:deadbeef"}},
	}
	if msg := validateRequireDigest(ok); msg != "" {
		t.Fatalf("expected ok, got %q", msg)
	}
	bad := &resolvedPreview{
		PE: &miragev1alpha1.PreviewEnvironment{
			Spec: miragev1alpha1.PreviewEnvironmentSpec{RequireDigest: true},
		},
		Services: []effectiveService{{Name: "api", Image: "ghcr.io/acme/api:latest"}},
	}
	if msg := validateRequireDigest(bad); msg == "" {
		t.Fatal("expected digest error")
	}
	off := &resolvedPreview{
		PE:       &miragev1alpha1.PreviewEnvironment{Spec: miragev1alpha1.PreviewEnvironmentSpec{}},
		Services: []effectiveService{{Name: "api", Image: "ghcr.io/acme/api:latest"}},
	}
	if msg := validateRequireDigest(off); msg != "" {
		t.Fatalf("expected no check when requireDigest unset, got %q", msg)
	}
}
