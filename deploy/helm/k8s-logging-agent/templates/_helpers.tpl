{{- define "k8s-logging-agent.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end }}

{{- define "k8s-logging-agent.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := include "k8s-logging-agent.name" . -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end }}

{{- define "k8s-logging-agent.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end }}

{{- define "k8s-logging-agent.selectorLabels" -}}
app.kubernetes.io/name: {{ include "k8s-logging-agent.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "k8s-logging-agent.labels" -}}
helm.sh/chart: {{ include "k8s-logging-agent.chart" . }}
{{ include "k8s-logging-agent.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "k8s-logging-agent.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "k8s-logging-agent.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end }}
