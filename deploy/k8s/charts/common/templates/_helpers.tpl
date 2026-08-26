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
image: repository:tag, where an infra image's tag is the full pinned
reference from deploy/compose/docker-compose.yml -- `3.7.3@sha256:...`. A
name:tag@digest reference is valid and the digest is what actually resolves,
so ONE field carries both the human-readable version and the pin.

It used to be two fields (`tag` plus a separate `digest` this template
preferred). Renovate reads the tag string as an image reference and updates
the digest inside it, never the other key -- so a bump silently did nothing
and the two drifted apart. deploy/k8s/scripts/validate.sh now also checks each
of these against the compose original.

Built service images are loaded into Kind rather than pulled and carry the
placeholder `dev` tag.
*/}}
{{- define "common.image" -}}
{{- $img := .Values.image -}}
{{ $img.repository }}:{{ $img.tag | default "dev" }}
{{- end -}}
