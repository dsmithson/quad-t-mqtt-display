{{- define "azurebuildmonitor.name" -}}
{{- .Chart.Name -}}
{{- end -}}

{{- define "azurebuildmonitor.fullname" -}}
{{- printf "%s" .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "azurebuildmonitor.labels" -}}
app.kubernetes.io/name: {{ include "azurebuildmonitor.name" . }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}
