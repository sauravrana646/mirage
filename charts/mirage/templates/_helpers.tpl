{{/*
Expand the name of the chart.
*/}}
{{- define "mirage.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "mirage.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "mirage.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "mirage.labels" -}}
helm.sh/chart: {{ include "mirage.chart" . }}
{{ include "mirage.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: mirage
{{- end }}

{{/*
Selector labels
*/}}
{{- define "mirage.selectorLabels" -}}
app.kubernetes.io/name: {{ include "mirage.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
control-plane: controller-manager
{{- end }}

{{/*
ServiceAccount name — defaults to controller-manager.
*/}}
{{- define "mirage.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default "controller-manager" .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Namespace for namespaced resources.
*/}}
{{- define "mirage.namespace" -}}
{{- default .Release.Namespace .Values.namespaceOverride }}
{{- end }}

{{/*
Manager container image reference (supports tag and/or digest).
*/}}
{{- define "mirage.image" -}}
{{- $tag := default .Chart.AppVersion .Values.image.tag }}
{{- if .Values.image.digest }}
{{- printf "%s@%s" .Values.image.repository .Values.image.digest }}
{{- else }}
{{- printf "%s:%s" .Values.image.repository $tag }}
{{- end }}
{{- end }}

{{/*
Webhook service name
*/}}
{{- define "mirage.webhookServiceName" -}}
{{- printf "%s-webhook-service" (include "mirage.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Metrics service name
*/}}
{{- define "mirage.metricsServiceName" -}}
{{- printf "%s-controller-manager-metrics-service" (include "mirage.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Webhook certificate secret name (must match cert-manager Certificate)
*/}}
{{- define "mirage.webhookCertSecretName" -}}
webhook-server-cert
{{- end }}

{{/*
cert-manager CA inject annotation value: namespace/certificate-name
*/}}
{{- define "mirage.webhookCAInject" -}}
{{- printf "%s/%s-serving-cert" (include "mirage.namespace" .) (include "mirage.fullname" .) }}
{{- end }}
