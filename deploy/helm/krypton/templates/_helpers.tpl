{{/*
Common name + label helpers.
*/}}

{{- define "krypton.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "krypton.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "krypton.labels" -}}
app.kubernetes.io/name: {{ include "krypton.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: krypton
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
{{- end -}}

{{- define "krypton.selectorLabels" -}}
app.kubernetes.io/name: {{ include "krypton.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Per-component image lookup. Falls back to {{ .Values.image.registry }}/<binary>:{{ .Values.image.tag }}.
Usage: {{ include "krypton.image" (dict "ctx" . "binary" "manager" "override" .Values.images.manager) }}
*/}}
{{- define "krypton.image" -}}
{{- $ctx := .ctx -}}
{{- $override := .override | default dict -}}
{{- $repo := $override.repository | default (printf "%s/%s" $ctx.Values.image.registry .binary) -}}
{{- $tag := $override.tag | default $ctx.Values.image.tag -}}
{{- printf "%s:%s" $repo $tag -}}
{{- end -}}

{{- define "krypton.pullPolicy" -}}
{{- $override := .override | default dict -}}
{{- $override.pullPolicy | default .ctx.Values.image.pullPolicy -}}
{{- end -}}
