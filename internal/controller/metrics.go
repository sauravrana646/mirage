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
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	miragev1alpha1 "github.com/sauravrana646/mirage/api/v1alpha1"
	"github.com/sauravrana646/mirage/internal/cost"
)

var (
	previewsReady = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "mirage_preview_ready_total",
		Help: "Number of times a PreviewEnvironment reached Ready",
	})
	previewsDeleted = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "mirage_preview_deleted_total",
		Help: "Number of PreviewEnvironments fully deleted",
	})
	previewsExpired = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "mirage_preview_expired_total",
		Help: "Number of PreviewEnvironments deleted due to TTL",
	})
	previewsPaused = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "mirage_preview_paused_total",
		Help: "Number of reconcile observations while suspended",
	})

	// Resource gauges are labeled by preview identity. Cardinality is expected to stay
	// low (one series set per active PreviewEnvironment name/namespace).
	previewCPURequested = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "mirage_preview_cpu_requested",
		Help: "Requested CPU cores for a PreviewEnvironment (requests; defaults applied when unset)",
	}, []string{"name", "namespace", "target_namespace"})
	previewMemoryRequested = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "mirage_preview_memory_requested_bytes",
		Help: "Requested memory bytes for a PreviewEnvironment (requests; defaults applied when unset)",
	}, []string{"name", "namespace", "target_namespace"})
	previewStorageRequested = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "mirage_preview_storage_requested_bytes",
		Help: "Requested storage bytes for a PreviewEnvironment (0 until PVC/deps are tracked)",
	}, []string{"name", "namespace", "target_namespace"})
	previewAgeSeconds = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "mirage_preview_age_seconds",
		Help: "Age of a PreviewEnvironment in seconds since CreationTimestamp",
	}, []string{"name", "namespace", "target_namespace"})
	activePreviews = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "mirage_active_previews",
		Help: "Number of PreviewEnvironments currently tracked by the operator metrics",
	})

	trackedPreviews sync.Map // key: namespace/name
)

func init() {
	metrics.Registry.MustRegister(
		previewsReady, previewsDeleted, previewsExpired, previewsPaused,
		previewCPURequested, previewMemoryRequested, previewStorageRequested,
		previewAgeSeconds, activePreviews,
	)
}

func previewMetricLabels(pe *miragev1alpha1.PreviewEnvironment) prometheus.Labels {
	return prometheus.Labels{
		"name":             pe.Name,
		"namespace":        pe.Namespace,
		"target_namespace": pe.Spec.TargetNamespace,
	}
}

func previewTrackKey(pe *miragev1alpha1.PreviewEnvironment) string {
	return pe.Namespace + "/" + pe.Name
}

// observePreviewResources sets per-preview request/age gauges and bumps active count once.
func observePreviewResources(pe *miragev1alpha1.PreviewEnvironment, now time.Time) {
	if pe == nil {
		return
	}
	key := previewTrackKey(pe)
	if _, loaded := trackedPreviews.LoadOrStore(key, pe.Spec.TargetNamespace); !loaded {
		activePreviews.Inc()
	} else {
		trackedPreviews.Store(key, pe.Spec.TargetNamespace)
	}

	res := cost.SumRequests(pe)
	labels := previewMetricLabels(pe)
	previewCPURequested.With(labels).Set(res.CPUCores())
	previewMemoryRequested.With(labels).Set(float64(res.MemoryBytes))
	previewStorageRequested.With(labels).Set(float64(res.StorageBytes))
	age := now.Sub(pe.CreationTimestamp.Time).Seconds()
	if age < 0 || pe.CreationTimestamp.IsZero() {
		age = 0
	}
	previewAgeSeconds.With(labels).Set(age)
}

// clearPreviewResources removes gauge series for a deleted preview.
func clearPreviewResources(pe *miragev1alpha1.PreviewEnvironment) {
	if pe == nil {
		return
	}
	key := previewTrackKey(pe)
	if _, loaded := trackedPreviews.LoadAndDelete(key); loaded {
		activePreviews.Dec()
	}
	labels := previewMetricLabels(pe)
	previewCPURequested.Delete(labels)
	previewMemoryRequested.Delete(labels)
	previewStorageRequested.Delete(labels)
	previewAgeSeconds.Delete(labels)
}
