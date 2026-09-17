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
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
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
)

func init() {
	metrics.Registry.MustRegister(previewsReady, previewsDeleted, previewsExpired, previewsPaused)
}
