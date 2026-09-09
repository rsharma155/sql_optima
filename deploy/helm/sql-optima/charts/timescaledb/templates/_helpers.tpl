{{- define "timescaledb.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "timescaledb.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name (include "timescaledb.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "timescaledb.labels" -}}
app.kubernetes.io/name: {{ include "timescaledb.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/component: timescaledb
{{- end -}}

{{- define "timescaledb.selectorLabels" -}}
app.kubernetes.io/name: {{ include "timescaledb.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: timescaledb
{{- end -}}

{{- define "timescaledb.secretName" -}}
{{- if .Values.auth.existingSecret -}}
{{- .Values.auth.existingSecret -}}
{{- else -}}
{{- printf "%s-credentials" (include "timescaledb.fullname" .) -}}
{{- end -}}
{{- end -}}

{{- define "timescaledb.image" -}}
{{ printf "%s:%s" .Values.image.repository .Values.image.tag }}
{{- end -}}
