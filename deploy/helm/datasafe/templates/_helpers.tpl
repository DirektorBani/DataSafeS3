{{/*
Expand the name of the chart.
*/}}
{{- define "datasafe.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "datasafe.fullname" -}}
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
Chart labels
*/}}
{{- define "datasafe.labels" -}}
helm.sh/chart: {{ include "datasafe.chart" . }}
{{ include "datasafe.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "datasafe.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "datasafe.selectorLabels" -}}
app.kubernetes.io/name: {{ include "datasafe.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "datasafe.componentSelectorLabels" -}}
{{ include "datasafe.selectorLabels" . }}
app.kubernetes.io/component: {{ .component }}
{{- end }}

{{/*
Image reference with optional global registry prefix.
*/}}
{{- define "datasafe.image" -}}
{{- $registry := .global.imageRegistry -}}
{{- $repo := .image.repository -}}
{{- $tag := .image.tag | default "latest" -}}
{{- if $registry -}}
{{- printf "%s/%s:%s" $registry $repo $tag -}}
{{- else -}}
{{- printf "%s:%s" $repo $tag -}}
{{- end -}}
{{- end }}

{{- define "datasafe.storageServer.fullname" -}}
{{- printf "%s-storage-server" (include "datasafe.fullname" .) }}
{{- end }}

{{- define "datasafe.caddy.fullname" -}}
{{- printf "%s-caddy" (include "datasafe.fullname" .) }}
{{- end }}

{{- define "datasafe.postgres.fullname" -}}
{{- printf "%s-postgres" (include "datasafe.fullname" .) }}
{{- end }}

{{- define "datasafe.prometheus.fullname" -}}
{{- printf "%s-prometheus" (include "datasafe.fullname" .) }}
{{- end }}

{{- define "datasafe.grafana.fullname" -}}
{{- printf "%s-grafana" (include "datasafe.fullname" .) }}
{{- end }}

{{- define "datasafe.minio.fullname" -}}
{{- printf "%s-minio" (include "datasafe.fullname" .) }}
{{- end }}

{{/*
PostgreSQL host for storage-server env.
*/}}
{{- define "datasafe.postgres.host" -}}
{{- if .Values.postgres.enabled -}}
{{- include "datasafe.postgres.fullname" . -}}
{{- else if .Values.storageServer.config.postgres.host -}}
{{- .Values.storageServer.config.postgres.host -}}
{{- else -}}
{{- "localhost" -}}
{{- end -}}
{{- end }}

{{/*
Effective metadata backend.
*/}}
{{- define "datasafe.metadataBackend" -}}
{{- if .Values.postgres.enabled -}}
postgres
{{- else -}}
{{- .Values.storageServer.metadataBackend | default "bolt" -}}
{{- end -}}
{{- end }}

{{/*
OIDC redirect URL default from ingress console host.
*/}}
{{- define "datasafe.oidc.redirectUrl" -}}
{{- if .Values.oidc.redirectUrl -}}
{{- .Values.oidc.redirectUrl -}}
{{- else if .Values.ingress.enabled -}}
{{- $scheme := ternary "https" "http" .Values.ingress.console.tls -}}
{{- printf "%s://%s/api/v1/auth/oidc/callback" $scheme .Values.ingress.console.host -}}
{{- else -}}
http://localhost:8080/api/v1/auth/oidc/callback
{{- end -}}
{{- end }}

{{- define "datasafe.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "datasafe.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}
