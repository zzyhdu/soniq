{{/*
Expand the chart name.
*/}}
{{- define "soniq.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "soniq.fullname" -}}
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

{{/*
Chart label value.
*/}}
{{- define "soniq.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Target namespace for rendered resources.
*/}}
{{- define "soniq.namespace" -}}
{{- default .Release.Namespace .Values.namespace.name -}}
{{- end -}}

{{/*
Common labels for all Soniq resources.
*/}}
{{- define "soniq.commonLabels" -}}
helm.sh/chart: {{ include "soniq.chart" . }}
app.kubernetes.io/part-of: soniq
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- with .Values.commonLabels }}
{{ toYaml . }}
{{- end }}
{{- end -}}

{{/*
ConfigMap name.
*/}}
{{- define "soniq.configName" -}}
{{- default (printf "%s-config" (include "soniq.fullname" .)) .Values.config.name -}}
{{- end -}}

{{/*
Secret name.
*/}}
{{- define "soniq.secretName" -}}
{{- default (printf "%s-secret" (include "soniq.fullname" .)) .Values.secret.name -}}
{{- end -}}

{{/*
ServiceAccount name.
*/}}
{{- define "soniq.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "soniq.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{/*
Component-specific names.
*/}}
{{- define "soniq.apiName" -}}
{{- printf "%s-api" (include "soniq.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "soniq.workerName" -}}
{{- printf "%s-worker" (include "soniq.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "soniq.migrateName" -}}
{{- printf "%s-migrate" (include "soniq.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Common egress rules for Soniq NetworkPolicy resources.
*/}}
{{- define "soniq.networkPolicyEgress" -}}
- ports:
    - protocol: UDP
      port: 53
    - protocol: TCP
      port: 53
- ports:
{{- range .Values.networkPolicy.allowedEgressTCPPorts }}
    - protocol: TCP
      port: {{ . }}
{{- end }}
{{- end -}}
