{{/*
Chart name.
*/}}
{{- define "crypto-broker-operator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels.
*/}}
{{- define "crypto-broker-operator.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/name: {{ include "crypto-broker-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/component: operator
{{- end }}

{{/*
Selector labels.
*/}}
{{- define "crypto-broker-operator.selectorLabels" -}}
app.kubernetes.io/name: crypto-broker-operator
{{- end }}

{{/*
Service account name.
*/}}
{{- define "crypto-broker-operator.serviceAccountName" -}}
{{- default "crypto-broker-operator" .Values.serviceAccount.name }}
{{- end }}
