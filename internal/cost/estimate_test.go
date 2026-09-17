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

package cost

import (
	"math"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	miragev1alpha1 "github.com/sauravrana646/mirage/api/v1alpha1"
)

func TestSumRequests_defaultsSingleService(t *testing.T) {
	pe := &miragev1alpha1.PreviewEnvironment{
		Spec: miragev1alpha1.PreviewEnvironmentSpec{Image: "app:1", TargetNamespace: "preview-a"},
	}
	got := SumRequests(pe)
	if got.CPUMillis != 50 {
		t.Fatalf("cpu millis: got %d want 50", got.CPUMillis)
	}
	defMem := resource.MustParse(DefaultMemoryRequest)
	wantMem := defMem.Value()
	if got.MemoryBytes != wantMem {
		t.Fatalf("memory: got %d want %d", got.MemoryBytes, wantMem)
	}
	if got.StorageBytes != 0 {
		t.Fatalf("storage should be 0, got %d", got.StorageBytes)
	}
	if got.IngressCount != 0 {
		t.Fatalf("ingress: got %d want 0", got.IngressCount)
	}
}

func TestSumRequests_explicitAndReplicas(t *testing.T) {
	reps := int32(3)
	pe := &miragev1alpha1.PreviewEnvironment{
		Spec: miragev1alpha1.PreviewEnvironmentSpec{
			Image:    "app:1",
			Replicas: &reps,
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
			},
			Ingress: &miragev1alpha1.IngressSpec{Enabled: true, Host: "a.example.com"},
		},
	}
	got := SumRequests(pe)
	if got.CPUMillis != 300 {
		t.Fatalf("cpu millis: got %d want 300", got.CPUMillis)
	}
	mem128 := resource.MustParse("128Mi")
	wantMem := mem128.Value() * 3
	if got.MemoryBytes != wantMem {
		t.Fatalf("memory: got %d want %d", got.MemoryBytes, wantMem)
	}
	if got.IngressCount != 1 {
		t.Fatalf("ingress: got %d want 1", got.IngressCount)
	}
	if got.ReplicaPods != 3 {
		t.Fatalf("replicas: got %d want 3", got.ReplicaPods)
	}
}

func TestSumRequests_zeroReplicas(t *testing.T) {
	zero := int32(0)
	pe := &miragev1alpha1.PreviewEnvironment{
		Spec: miragev1alpha1.PreviewEnvironmentSpec{
			Image:    "app:1",
			Replicas: &zero,
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
			},
			Ingress: &miragev1alpha1.IngressSpec{Enabled: true, Host: "a.example.com"},
		},
	}
	got := SumRequests(pe)
	if got.CPUMillis != 0 || got.MemoryBytes != 0 {
		t.Fatalf("zero replicas should contribute 0 cpu/mem, got cpu=%d mem=%d", got.CPUMillis, got.MemoryBytes)
	}
	if got.ReplicaPods != 0 {
		t.Fatalf("replica pods: got %d want 0", got.ReplicaPods)
	}
	// Ingress is still enabled on the service even with 0 pods.
	if got.IngressCount != 1 {
		t.Fatalf("ingress: got %d want 1", got.IngressCount)
	}
}

func TestSumRequests_multiServiceFallback(t *testing.T) {
	svcReps := int32(2)
	pe := &miragev1alpha1.PreviewEnvironment{
		Spec: miragev1alpha1.PreviewEnvironmentSpec{
			TargetNamespace: "preview-ms",
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("50m"),
					corev1.ResourceMemory: resource.MustParse("64Mi"),
				},
			},
			Ingress: &miragev1alpha1.IngressSpec{Enabled: true, Host: "primary.example.com"},
			Services: []miragev1alpha1.PreviewServiceSpec{
				{Name: "api", Image: "api:1"}, // inherits Spec.Resources + top-level ingress
				{
					Name: "worker", Image: "worker:1", Replicas: &svcReps,
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("200m"),
							corev1.ResourceMemory: resource.MustParse("256Mi"),
						},
					},
				},
				{
					Name: "web", Image: "web:1",
					Ingress: &miragev1alpha1.IngressSpec{Enabled: true, Host: "web.example.com"},
				},
			},
		},
	}
	got := SumRequests(pe)
	// api: 50m*1, worker: 200m*2, web: 50m*1 = 50+400+50 = 500
	if got.CPUMillis != 500 {
		t.Fatalf("cpu millis: got %d want 500", got.CPUMillis)
	}
	mem64 := resource.MustParse("64Mi")
	mem256 := resource.MustParse("256Mi")
	wantMem := mem64.Value() + mem256.Value()*2 + mem64.Value()
	if got.MemoryBytes != wantMem {
		t.Fatalf("memory: got %d want %d", got.MemoryBytes, wantMem)
	}
	// primary ingress on index 0 + web ingress
	if got.IngressCount != 2 {
		t.Fatalf("ingress: got %d want 2", got.IngressCount)
	}
}

func TestSumRequestsFrom_templatePresetAndIngress(t *testing.T) {
	pe := &miragev1alpha1.PreviewEnvironment{
		ObjectMeta: metav1.ObjectMeta{Name: "pr-42"},
		Spec: miragev1alpha1.PreviewEnvironmentSpec{
			Image: "app:1",
			TemplateRef: &miragev1alpha1.TemplateRef{
				Name: "team-default",
			},
		},
	}
	tpl := &miragev1alpha1.PreviewTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "team-default"},
		Spec: miragev1alpha1.PreviewTemplateSpec{
			Resources: &miragev1alpha1.ResourcePresetSpec{Preset: miragev1alpha1.ResourcePresetMedium},
			Ingress: &miragev1alpha1.IngressTemplateSpec{
				Enabled: true,
				Domain:  "preview.example.com",
			},
		},
	}
	got := SumRequestsFrom(pe, tpl)
	// medium preset requests: 200m / 256Mi
	if got.CPUMillis != 200 {
		t.Fatalf("cpu millis: got %d want 200 (medium preset)", got.CPUMillis)
	}
	wantMem := resource.MustParse("256Mi")
	if got.MemoryBytes != wantMem.Value() {
		t.Fatalf("memory: got %d want %d", got.MemoryBytes, wantMem.Value())
	}
	if got.IngressCount != 1 {
		t.Fatalf("ingress from template: got %d want 1", got.IngressCount)
	}
	// Original PE must be unchanged.
	if !isEmptyResources(pe.Spec.Resources) || pe.Spec.Ingress != nil {
		t.Fatalf("SumRequestsFrom mutated input PE")
	}
}

func TestSumRequests_dependenciesEnabled(t *testing.T) {
	pe := &miragev1alpha1.PreviewEnvironment{
		Spec: miragev1alpha1.PreviewEnvironmentSpec{
			Image: "app:1",
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
			},
			Dependencies: &miragev1alpha1.PreviewDependenciesSpec{
				Postgres: &miragev1alpha1.PostgresDependencySpec{Enabled: true},
				Redis:    &miragev1alpha1.RedisDependencySpec{Enabled: true},
				Kafka:    &miragev1alpha1.KafkaDependencySpec{Enabled: true},
			},
		},
	}
	got := SumRequests(pe)
	// app 100 + pg 50 + redis 25 + kafka 100 = 275
	wantCPU := int64(100 + 50 + 25 + 100)
	if got.CPUMillis != wantCPU {
		t.Fatalf("cpu millis: got %d want %d", got.CPUMillis, wantCPU)
	}
	appMem := resource.MustParse("128Mi")
	pgMem := resource.MustParse(DepPostgresMemRequest)
	redisMem := resource.MustParse(DepRedisMemRequest)
	kafkaMem := resource.MustParse(DepKafkaMemRequest)
	wantMem := appMem.Value() + pgMem.Value() + redisMem.Value() + kafkaMem.Value()
	if got.MemoryBytes != wantMem {
		t.Fatalf("memory: got %d want %d", got.MemoryBytes, wantMem)
	}
	if got.ReplicaPods != 4 { // app + 3 deps
		t.Fatalf("replica pods: got %d want 4", got.ReplicaPods)
	}
}

func TestSumRequests_disabledDependenciesIgnored(t *testing.T) {
	pe := &miragev1alpha1.PreviewEnvironment{
		Spec: miragev1alpha1.PreviewEnvironmentSpec{
			Image: "app:1",
			Dependencies: &miragev1alpha1.PreviewDependenciesSpec{
				Postgres: &miragev1alpha1.PostgresDependencySpec{Enabled: false},
				Redis:    &miragev1alpha1.RedisDependencySpec{Enabled: false},
			},
		},
	}
	got := SumRequests(pe)
	if got.CPUMillis != 50 {
		t.Fatalf("disabled deps should not add cost: cpu=%d", got.CPUMillis)
	}
	if got.ReplicaPods != 1 {
		t.Fatalf("replica pods: got %d want 1", got.ReplicaPods)
	}
}

func TestEstimateCost_formula(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	created := metav1.NewTime(now.Add(-2 * time.Hour))
	expires := metav1.NewTime(now.Add(22 * time.Hour))
	pe := &miragev1alpha1.PreviewEnvironment{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: created},
		Spec: miragev1alpha1.PreviewEnvironmentSpec{
			Image: "app:1",
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1000m"), // 1 core
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
			},
			Ingress: &miragev1alpha1.IngressSpec{Enabled: true},
		},
		Status: miragev1alpha1.PreviewEnvironmentStatus{ExpiresAt: &expires},
	}
	est := EstimateCost(pe, now)
	if est.Hours != 2 {
		t.Fatalf("hours: got %v want 2", est.Hours)
	}
	wantCPU := 2 * 1.0 * CPUHourlyUSDPerCore
	wantMem := 2 * 1.0 * MemoryHourlyUSDPerGiB
	wantIng := 2 * 1.0 * IngressHourlyUSD
	wantTotal := wantCPU + wantMem + wantIng
	if !almostEqual(est.CPUUSD, wantCPU) || !almostEqual(est.MemoryUSD, wantMem) || !almostEqual(est.IngressUSD, wantIng) {
		t.Fatalf("component USD mismatch: cpu=%v mem=%v ing=%v", est.CPUUSD, est.MemoryUSD, est.IngressUSD)
	}
	if !almostEqual(est.TotalUSD, wantTotal) {
		t.Fatalf("total: got %v want %v", est.TotalUSD, wantTotal)
	}
	if est.ExpiresAt == nil || !est.ExpiresAt.Equal(expires.Time) {
		t.Fatalf("expiresAt not set")
	}
	if est.Resources.StorageBytes != 0 || est.StorageUSD != 0 {
		t.Fatalf("storage should be zero")
	}
}

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func isEmptyResources(r corev1.ResourceRequirements) bool {
	return len(r.Limits) == 0 && len(r.Requests) == 0
}
