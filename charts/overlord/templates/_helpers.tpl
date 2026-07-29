{{/*
Expand the name of the chart.
*/}}
{{- define "overlord.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "overlord.fullname" -}}
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
{{- define "overlord.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "overlord.labels" -}}
helm.sh/chart: {{ include "overlord.chart" . }}
{{ include "overlord.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "overlord.selectorLabels" -}}
app.kubernetes.io/name: {{ include "overlord.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "overlord.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "overlord.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Environment shared by the application and the migration job. Both need identical
database configuration, so it lives here rather than being duplicated.
*/}}
{{- define "overlord.env" -}}
{{- if .Values.overlord.db_url }}
- name: DB_URL
  valueFrom:
    secretKeyRef:
      name: {{ include "overlord.fullname" . }}-secret
      key: db_url
{{- end }}
{{- if .Values.service.port }}
- name: PORT
  valueFrom:
    secretKeyRef:
      name: {{ include "overlord.fullname" . }}-secret
      key: port
{{- end }}
- name: DB_TYPE
  value: {{ .Values.overlord.db_type | quote }}
- name: GRPC_SERVER_ADDRESS
  value: {{ .Values.overlord.grpc_server_address | quote }}
- name: ENVIRONMENT
  value: {{ .Values.overlord.environment | quote }}
- name: LOG_LEVEL
  value: {{ .Values.overlord.log_level | quote }}
- name: LOG_ENCODING
  value: {{ .Values.overlord.log_encoding | quote }}
{{- with .Values.env }}
{{- toYaml . }}
{{- end }}
{{- end }}
