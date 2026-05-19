{{/*
Expand the name of the chart.
*/}}
{{- define "authproxy.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Fully qualified app name. Honors fullnameOverride, else combines release
name with chart name unless the release name already contains the chart name.
*/}}
{{- define "authproxy.fullname" -}}
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
Chart version label, sanitized for label values.
*/}}
{{- define "authproxy.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Common labels emitted on every resource.
*/}}
{{- define "authproxy.labels" -}}
helm.sh/chart: {{ include "authproxy.chart" . }}
{{ include "authproxy.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/*
Selector labels — stable across upgrades, used by Service + Deployment.
*/}}
{{- define "authproxy.selectorLabels" -}}
app.kubernetes.io/name: {{ include "authproxy.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
ServiceAccount name to use.
*/}}
{{- define "authproxy.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "authproxy.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{/*
Container image reference. Falls back to Chart.appVersion when tag is unset.
*/}}
{{- define "authproxy.image" -}}
{{- $tag := default .Chart.AppVersion .Values.image.tag -}}
{{- printf "%s:%s" .Values.image.repository $tag -}}
{{- end -}}

{{/*
Comma-separated list of enabled services in the order accepted by
`cmd/server serve` (admin-api,api,public,worker). Used for the container
args. Returns empty string when nothing is enabled (chart validation
prevents an install in that state).

Output is a single string, no newline.
*/}}
{{- define "authproxy.enabledServices" -}}
{{- $svc := .Values.services -}}
{{- $list := list -}}
{{- if $svc.adminApi.enabled }}{{- $list = append $list "admin-api" }}{{- end }}
{{- if $svc.api.enabled }}{{- $list = append $list "api" }}{{- end }}
{{- if $svc.public.enabled }}{{- $list = append $list "public" }}{{- end }}
{{- if $svc.worker.enabled }}{{- $list = append $list "worker" }}{{- end }}
{{- join "," $list -}}
{{- end -}}

{{/*
Build a list of {name, port} tuples for the enabled services that have a
listening port (i.e. all of public/api/admin-api and worker's health
endpoint). Used by the Service and Deployment templates so we don't
expose ports for disabled services.

Returns YAML — call with `| fromYaml | .ports`.
*/}}
{{- define "authproxy.enabledPorts" -}}
ports:
{{- if .Values.services.public.enabled }}
  - name: public
    port: {{ .Values.ports.public }}
{{- end }}
{{- if .Values.services.api.enabled }}
  - name: api
    port: {{ .Values.ports.api }}
{{- end }}
{{- if .Values.services.adminApi.enabled }}
  - name: admin-api
    port: {{ .Values.ports.adminApi }}
{{- end }}
{{- if .Values.services.worker.enabled }}
  - name: worker-health
    port: {{ .Values.ports.workerHealth }}
{{- end }}
{{- end -}}
