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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	miragev1alpha1 "github.com/sauravrana646/mirage/api/v1alpha1"
)

func TestWorkloadLabelsReservedWin(t *testing.T) {
	const peName = "pr-labels"
	pe := &miragev1alpha1.PreviewEnvironment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      peName,
			Namespace: "mirage-system",
			UID:       types.UID("uid-123"),
		},
		Spec: miragev1alpha1.PreviewEnvironmentSpec{
			WorkloadLabels: map[string]string{
				appLabel:                           "different-name",
				componentLabel:                     depComponent,
				miragev1alpha1.LabelManagedBy:      "something-else",
				miragev1alpha1.LabelOwnerUID:       "fake",
				miragev1alpha1.LabelOwnerName:      "fake-name",
				miragev1alpha1.LabelOwnerNamespace: "fake-ns",
				miragev1alpha1.LabelService:        "different-service",
				miragev1alpha1.LabelDependency:     "postgres",
				"custom.io/team":                   "platform",
			},
		},
	}
	r := &PreviewEnvironmentReconciler{}
	labels := r.workloadLabels(pe, "api")

	if labels[appLabel] != peName {
		t.Fatalf("app label = %q, want %s", labels[appLabel], peName)
	}
	if labels[componentLabel] != previewComponent {
		t.Fatalf("component = %q, want %q", labels[componentLabel], previewComponent)
	}
	if labels[miragev1alpha1.LabelManagedBy] != miragev1alpha1.ManagedByValue {
		t.Fatalf("managed-by overwritten")
	}
	if labels[miragev1alpha1.LabelOwnerUID] != "uid-123" {
		t.Fatalf("owner-uid overwritten")
	}
	if labels[miragev1alpha1.LabelOwnerName] != peName {
		t.Fatalf("owner-name overwritten")
	}
	if labels[miragev1alpha1.LabelOwnerNamespace] != "mirage-system" {
		t.Fatalf("owner-namespace overwritten")
	}
	if labels[miragev1alpha1.LabelService] != "api" {
		t.Fatalf("service label = %q, want api", labels[miragev1alpha1.LabelService])
	}
	if _, ok := labels[miragev1alpha1.LabelDependency]; ok {
		t.Fatalf("dependency label must not remain on app workloads from user input")
	}
	if labels["custom.io/team"] != "platform" {
		t.Fatalf("custom label lost")
	}

	selector := map[string]string{appLabel: peName, miragev1alpha1.LabelService: "api"}
	for k, v := range selector {
		if labels[k] != v {
			t.Fatalf("selector key %s=%q missing from template labels", k, v)
		}
	}
}
