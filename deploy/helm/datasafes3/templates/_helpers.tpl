{{- define "datasafes3.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "datasafes3.fullname" -}}
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

{{- define "datasafes3.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "datasafes3.labels" -}}
helm.sh/chart: {{ include "datasafes3.chart" . }}
{{ include "datasafes3.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "datasafes3.selectorLabels" -}}
app.kubernetes.io/name: {{ include "datasafes3.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "datasafes3.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "datasafes3.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{- define "datasafes3.serverTag" -}}
{{- if .Values.storageServer.image.tag }}
{{- .Values.storageServer.image.tag }}
{{- else }}
{{- printf "v%s" .Chart.AppVersion }}
{{- end }}
{{- end }}

{{- define "datasafes3.consoleTag" -}}
{{- if .Values.console.image.tag }}
{{- .Values.console.image.tag }}
{{- else }}
{{- printf "v%s" .Chart.AppVersion }}
{{- end }}
{{- end }}

{{- define "datasafes3.serverImage" -}}
{{- printf "%s:%s" .Values.storageServer.image.repository (include "datasafes3.serverTag" .) }}
{{- end }}

{{- define "datasafes3.replicas" -}}
{{- if .Values.cluster.enabled }}
{{- .Values.cluster.replicas }}
{{- else }}
{{- .Values.storageServer.replicas }}
{{- end }}
{{- end }}

{{- define "datasafes3.metadataBackend" -}}
{{- if .Values.cluster.enabled }}
postgres
{{- else }}
{{- .Values.metadata.backend }}
{{- end }}
{{- end }}

{{- define "datasafes3.postgresHost" -}}
{{- if .Values.postgres.external.host }}
{{- .Values.postgres.external.host }}
{{- else if .Values.postgres.enabled }}
{{- printf "%s-postgres" (include "datasafes3.fullname" .) }}
{{- end }}
{{- end }}

{{- define "datasafes3.requireAuth" -}}
{{- if .Values.production }}
{{- if not .Values.auth.existingSecret }}
{{- fail "production: true requires auth.existingSecret (no passwords in the chart)" }}
{{- end }}
{{- else if not .Values.auth.existingSecret }}
{{- if or (not .Values.auth.accessKey) (not .Values.auth.secretKey) (not .Values.auth.adminPassword) (not .Values.auth.jwtSecret) }}
{{- fail "set auth.existingSecret or auth.accessKey, auth.secretKey, auth.adminPassword, and auth.jwtSecret" }}
{{- end }}
{{- end }}
{{- if .Values.cluster.enabled }}
{{- if and (not .Values.postgres.enabled) (not .Values.postgres.external.host) }}
{{- fail "cluster.enabled requires postgres.enabled or postgres.external.host" }}
{{- end }}
{{- end }}
{{- end }}
