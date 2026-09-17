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

// Package cost estimates PreviewEnvironment spend from requested resources and age.
// Rates are hardcoded illustrative constants — not cloud billing. OpenCost (or similar)
// can replace this with actual allocation later.
package cost

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	miragev1alpha1 "github.com/sauravrana646/mirage/api/v1alpha1"
	"github.com/sauravrana646/mirage/internal/resolve"
)

// LimitRange-like defaults when requests are unset (matches mirage-defaults LimitRange).
const (
	DefaultCPURequest    = "50m"
	DefaultMemoryRequest = "64Mi"
)

// Dependency request footprints match bitnami containers in internal/controller/dependencies.go.
const (
	DepPostgresCPURequest = "50m"
	DepPostgresMemRequest = "128Mi"
	DepRedisCPURequest    = "25m"
	DepRedisMemRequest    = "64Mi"
	DepKafkaCPURequest    = "100m"
	DepKafkaMemRequest    = "256Mi"
)

// Illustrative USD/hour rates used for deterministic estimates (not invoices).
const (
	CPUHourlyUSDPerCore    = 0.04    // ~$0.04 per vCPU-hour
	MemoryHourlyUSDPerGiB  = 0.005   // ~$0.005 per GiB-hour
	StorageHourlyUSDPerGiB = 0.00014 // unused until PVC/deps; kept for formula symmetry
	IngressHourlyUSD       = 0.01    // flat per enabled ingress host-hour
)

// Resources is the summed request footprint of a preview.
type Resources struct {
	CPUMillis    int64 // millicores
	MemoryBytes  int64
	StorageBytes int64 // 0 until storage-backed deps exist
	IngressCount int
	ReplicaPods  int32
}

// Estimate is a deterministic cost projection for one preview.
type Estimate struct {
	Resources  Resources
	Age        time.Duration
	Hours      float64
	CPUUSD     float64
	MemoryUSD  float64
	StorageUSD float64
	IngressUSD float64
	TotalUSD   float64
	ExpiresAt  *time.Time
}

// SumRequests totals CPU/memory requests across effective workloads and enabled deps.
// Empty requests use DefaultCPURequest / DefaultMemoryRequest.
// Replica counts of 0 are preserved (not coerced to 1).
// For templateRef defaults, use SumRequestsFrom with a loaded PreviewTemplate.
func SumRequests(pe *miragev1alpha1.PreviewEnvironment) Resources {
	return SumRequestsFrom(pe, nil)
}

// SumRequestsFrom deep-copies pe, applies tpl when non-nil (same as the controller),
// then sums effective service requests plus enabled dependency footprints.
func SumRequestsFrom(pe *miragev1alpha1.PreviewEnvironment, tpl *miragev1alpha1.PreviewTemplate) Resources {
	if pe == nil {
		return Resources{}
	}
	working := pe.DeepCopy()
	if tpl != nil {
		resolve.ApplyTemplate(working, tpl)
	}
	out := Resources{}
	svcs, err := resolve.EffectiveServices(working)
	if err != nil {
		// Incomplete PE (no image/services): still count deps if present.
		addDependencyRequests(&out, working.Spec.Dependencies)
		return out
	}
	for _, svc := range svcs {
		cpu, mem := requestOrDefault(svc.Resources)
		out.CPUMillis += cpu * int64(svc.Replicas)
		out.MemoryBytes += mem * int64(svc.Replicas)
		out.ReplicaPods += svc.Replicas
		if svc.Ingress != nil && svc.Ingress.Enabled {
			out.IngressCount++
		}
	}
	addDependencyRequests(&out, working.Spec.Dependencies)
	return out
}

func addDependencyRequests(out *Resources, deps *miragev1alpha1.PreviewDependenciesSpec) {
	if deps == nil {
		return
	}
	if deps.Postgres != nil && deps.Postgres.Enabled {
		cpu := resource.MustParse(DepPostgresCPURequest)
		mem := resource.MustParse(DepPostgresMemRequest)
		out.CPUMillis += cpu.MilliValue()
		out.MemoryBytes += mem.Value()
		out.ReplicaPods++
	}
	if deps.Redis != nil && deps.Redis.Enabled {
		cpu := resource.MustParse(DepRedisCPURequest)
		mem := resource.MustParse(DepRedisMemRequest)
		out.CPUMillis += cpu.MilliValue()
		out.MemoryBytes += mem.Value()
		out.ReplicaPods++
	}
	if deps.Kafka != nil && deps.Kafka.Enabled {
		cpu := resource.MustParse(DepKafkaCPURequest)
		mem := resource.MustParse(DepKafkaMemRequest)
		out.CPUMillis += cpu.MilliValue()
		out.MemoryBytes += mem.Value()
		out.ReplicaPods++
	}
}

// EstimateCost projects spend from requests and age. now is used when createdAt is zero.
func EstimateCost(pe *miragev1alpha1.PreviewEnvironment, now time.Time) Estimate {
	return EstimateCostFrom(pe, nil, now)
}

// EstimateCostFrom is EstimateCost after optional template application.
func EstimateCostFrom(pe *miragev1alpha1.PreviewEnvironment, tpl *miragev1alpha1.PreviewTemplate, now time.Time) Estimate {
	res := SumRequestsFrom(pe, tpl)
	created := now
	if pe != nil && !pe.CreationTimestamp.IsZero() {
		created = pe.CreationTimestamp.Time
	}
	age := now.Sub(created)
	if age < 0 {
		age = 0
	}
	hours := age.Hours()
	cpuCores := float64(res.CPUMillis) / 1000.0
	memGiB := float64(res.MemoryBytes) / (1024 * 1024 * 1024)
	storageGiB := float64(res.StorageBytes) / (1024 * 1024 * 1024)

	est := Estimate{
		Resources:  res,
		Age:        age,
		Hours:      hours,
		CPUUSD:     hours * cpuCores * CPUHourlyUSDPerCore,
		MemoryUSD:  hours * memGiB * MemoryHourlyUSDPerGiB,
		StorageUSD: hours * storageGiB * StorageHourlyUSDPerGiB,
		IngressUSD: hours * float64(res.IngressCount) * IngressHourlyUSD,
	}
	est.TotalUSD = est.CPUUSD + est.MemoryUSD + est.StorageUSD + est.IngressUSD
	if pe != nil && pe.Status.ExpiresAt != nil {
		t := pe.Status.ExpiresAt.Time
		est.ExpiresAt = &t
	}
	return est
}

// CPUCores returns requested CPU as fractional cores.
func (r Resources) CPUCores() float64 {
	return float64(r.CPUMillis) / 1000.0
}

// MemoryGiB returns requested memory in GiB.
func (r Resources) MemoryGiB() float64 {
	return float64(r.MemoryBytes) / (1024 * 1024 * 1024)
}

func requestOrDefault(rr corev1.ResourceRequirements) (cpuMillis, memBytes int64) {
	cpu := resource.MustParse(DefaultCPURequest)
	mem := resource.MustParse(DefaultMemoryRequest)
	if rr.Requests != nil {
		if q, ok := rr.Requests[corev1.ResourceCPU]; ok {
			cpu = q
		}
		if q, ok := rr.Requests[corev1.ResourceMemory]; ok {
			mem = q
		}
	}
	return cpu.MilliValue(), mem.Value()
}
