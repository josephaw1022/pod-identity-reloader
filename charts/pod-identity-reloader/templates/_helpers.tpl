{{- define "pod-identity-reloader.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "pod-identity-reloader.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- include "pod-identity-reloader.name" . }}
{{- end }}
{{- end }}

{{- define "pod-identity-reloader.labels" -}}
helm.sh/chart: {{ include "pod-identity-reloader.name" . }}-{{ .Chart.Version | replace "+" "_" }}
app.kubernetes.io/name: {{ include "pod-identity-reloader.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "pod-identity-reloader.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "pod-identity-reloader.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}
