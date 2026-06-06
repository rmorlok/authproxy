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

{{- define "authproxy-demo.goOauth2Server.publicHost" -}}
{{- printf "oauth2.%s" .Values.global.hostname -}}
{{- end -}}

{{- define "authproxy-demo.authproxy.serviceName" -}}
{{/* Matches the inner authproxy chart's fullname when installed under
     this release. The inner chart's _helpers.tpl `authproxy.fullname`
     uses .Release.Name + "-" + .Chart.Name, hence -authproxy. */}}
{{- printf "%s-authproxy" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "authproxy-demo.grafana.serviceName" -}}
{{/* Matches the grafana subchart's fullname helper default. */}}
{{- printf "%s-grafana" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "authproxy-demo.grafanaAuthProxy.datasourceName" -}}
{{- default "AuthProxy" .Values.grafanaAuthProxy.datasource.name -}}
{{- end -}}

{{- define "authproxy-demo.grafanaAuthProxy.datasourceUid" -}}
{{- default "authproxy-app-metrics" .Values.grafanaAuthProxy.datasource.uid -}}
{{- end -}}

{{- define "authproxy-demo.grafanaAuthProxy.baseUrl" -}}
{{- if .Values.grafanaAuthProxy.datasource.authproxyBaseUrl -}}
{{- .Values.grafanaAuthProxy.datasource.authproxyBaseUrl -}}
{{- else -}}
{{- printf "http://%s:%v" (include "authproxy-demo.authproxy.serviceName" .) .Values.grafanaAuthProxy.datasource.authproxyApiPort -}}
{{- end -}}
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
