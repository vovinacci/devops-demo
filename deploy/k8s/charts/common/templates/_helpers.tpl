{{/*
Naming and label helpers shared by every service chart (RFC-0003 DK2).
Names are release-scoped so the umbrella renders unique objects per service:
  <release>-<chart>  e.g. platform-backend
*/}}

{{- define "common.name" -}}
{{- .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "common.fullname" -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "common.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "common.serviceAccountName" -}}
{{- include "common.fullname" . -}}
{{- end -}}

{{/*
selectorLabels: the immutable identity match set (pod selector + Service +
ServiceMonitor selector). Kept minimal so it never changes under a rollout.
*/}}
{{- define "common.selectorLabels" -}}
app.kubernetes.io/name: {{ include "common.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
labels: the full recommended label set, part-of the devops-demo platform.
*/}}
{{- define "common.labels" -}}
helm.sh/chart: {{ include "common.chart" . }}
{{ include "common.selectorLabels" . }}
app.kubernetes.io/part-of: devops-demo
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- with .Chart.AppVersion }}
app.kubernetes.io/version: {{ . | quote }}
{{- end }}
{{- end -}}

{{/*
image: repository@digest when a digest is pinned (infra images carry the same
digest as deploy/compose/docker-compose.yml), else repository:tag. Built
service images have no registry digest yet -- their tag is a PR-3 concern
(loaded into Kind), so PR-2 renders a placeholder tag.
*/}}
{{- define "common.image" -}}
{{- $img := .Values.image -}}
{{- if $img.digest -}}
{{ $img.repository }}@{{ $img.digest }}
{{- else -}}
{{ $img.repository }}:{{ $img.tag | default "dev" }}
{{- end -}}
{{- end -}}
