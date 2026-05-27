{{/*
Name helpers — shared across this chart's own templates. Subcharts
have their own helpers and aren't affected by these.
*/}}

{{- define "authproxy-demo.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name "demo" | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "authproxy-demo.demoShell.name" -}}
{{- printf "%s-demo-shell" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "authproxy-demo.goOauth2Server.name" -}}
{{- printf "%s-go-oauth2-server" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "authproxy-demo.authproxy.serviceName" -}}
{{/* Matches the inner authproxy chart's fullname when installed under
     this release. The inner chart's _helpers.tpl `authproxy.fullname`
     uses .Release.Name + "-" + .Chart.Name, hence -authproxy. */}}
{{- printf "%s-authproxy" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "authproxy-demo.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | quote }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
{{- end -}}

{{/*
Resolve the demo-shell image tag. Defaults to .Chart.AppVersion so the
umbrella's appVersion line is a single source of truth.
*/}}
{{- define "authproxy-demo.demoShell.imageTag" -}}
{{- default .Chart.AppVersion .Values.demoShell.image.tag -}}
{{- end -}}
