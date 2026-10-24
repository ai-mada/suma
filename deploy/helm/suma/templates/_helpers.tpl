{{- define "suma.name" -}}
{{- printf "%s-suma" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- define "suma.selector" -}}
app.kubernetes.io/name: suma
app.kubernetes.io/instance: {{ .Release.Name | quote }}
{{- end -}}
