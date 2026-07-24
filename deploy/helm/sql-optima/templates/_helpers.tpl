{{- define "sql-optima.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "sql-optima.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name (include "sql-optima.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "sql-optima.labels" -}}
app.kubernetes.io/name: {{ include "sql-optima.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "sql-optima.image" -}}
{{- $tag := .Values.image.tag -}}
{{- if not $tag -}}
{{- $tag = .Chart.AppVersion -}}
{{- end -}}
{{ printf "%s:%s" .Values.image.repository $tag }}
{{- end -}}

{{/*
Secret holding DB_PASSWORD / JWT_SECRET (chart-managed or existing).
*/}}
{{- define "sql-optima.secretName" -}}
{{- if .Values.timescale.existingSecret -}}
{{- .Values.timescale.existingSecret -}}
{{- else if .Values.auth.existingSecret -}}
{{- .Values.auth.existingSecret -}}
{{- else -}}
{{- printf "%s-secrets" (include "sql-optima.fullname" .) -}}
{{- end -}}
{{- end -}}

{{/*
Timescale host: bundled subchart service, or external host from values.
*/}}
{{- define "sql-optima.timescaleHost" -}}
{{- if .Values.timescaledb.enabled -}}
{{- printf "%s-timescaledb" .Release.Name -}}
{{- else -}}
{{- .Values.timescale.host -}}
{{- end -}}
{{- end -}}

{{- define "sql-optima.timescalePort" -}}
{{- if .Values.timescaledb.enabled -}}
{{- .Values.timescaledb.service.port | default 5432 -}}
{{- else -}}
{{- .Values.timescale.port -}}
{{- end -}}
{{- end -}}

{{- define "sql-optima.timescaleDatabase" -}}
{{- if .Values.timescaledb.enabled -}}
{{- .Values.timescaledb.auth.database | default .Values.timescale.database -}}
{{- else -}}
{{- .Values.timescale.database -}}
{{- end -}}
{{- end -}}

{{- define "sql-optima.timescaleUser" -}}
{{- if .Values.timescaledb.enabled -}}
{{- .Values.timescaledb.auth.username | default .Values.timescale.user -}}
{{- else -}}
{{- .Values.timescale.user -}}
{{- end -}}
{{- end -}}

{{- define "sql-optima.dbPasswordSecretName" -}}
{{- if .Values.timescale.existingSecret -}}
{{- .Values.timescale.existingSecret -}}
{{- else -}}
{{- printf "%s-secrets" (include "sql-optima.fullname" .) -}}
{{- end -}}
{{- end -}}

{{- define "sql-optima.dbPasswordSecretKey" -}}
{{- .Values.timescale.existingSecretPasswordKey | default "DB_PASSWORD" -}}
{{- end -}}
