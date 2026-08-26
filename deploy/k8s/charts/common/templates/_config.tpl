{{/*
Configuration and credentials (RFC-0003 DK8). Compose passes both as plain
`environment:` entries; Kubernetes splits them by KIND of data, and the split
is the lesson: non-secret settings go in a ConfigMap, credentials go in a
Secret. Both are consumed with `envFrom`, so the container sees exactly the
same variable names it sees under compose -- the service code is unchanged.

DEMO-GRADE POSTURE, stated plainly (ADR-0011): a Secret is base64-encoded,
NOT encrypted, and the values below are committed to this repository. That is
honest for a teaching stack whose passwords are `app`/`analytics`/`reports`,
and it is NOT how a real system stores credentials -- a real one sources them
from an external store (Vault, cloud KMS, Secrets Store CSI, External
Secrets). Separating them from the ConfigMap is what makes that later swap a
change of ONE object rather than an audit of every manifest.
*/}}

{{/*
common.configmap: non-secret environment settings (upstream URLs, tuning
knobs, feature flags). Rendered only when the service declares any.

Values are run through `tpl`, so an in-cluster address is written against the
release rather than hardcoded: "http://{{ .Release.Name }}-backend:8000"
resolves to the Service the umbrella actually created. Compose could hardcode
`http://api:8000` because the compose service name IS the DNS name; Helm names
objects <release>-<chart>, so the reference has to be computed.
*/}}
{{- define "common.configmap" -}}
{{- with .Values.config -}}
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ include "common.fullname" $ }}-config
  labels:
    {{- include "common.labels" $ | nindent 4 }}
data:
  {{- range $k, $v := . }}
  {{ $k }}: {{ tpl (toString $v) $ | quote }}
  {{- end }}
{{- end -}}
{{- end -}}

{{/*
common.secret: credential-bearing environment settings -- anything carrying a
password, including the DSNs that embed one. stringData (not data) keeps the
values readable in the chart; the API server base64-encodes them on write.
*/}}
{{- define "common.secret" -}}
{{- with .Values.secret -}}
apiVersion: v1
kind: Secret
metadata:
  name: {{ include "common.fullname" $ }}-secret
  labels:
    {{- include "common.labels" $ | nindent 4 }}
type: Opaque
stringData:
  {{- range $k, $v := . }}
  {{ $k }}: {{ tpl (toString $v) $ | quote }}
  {{- end }}
{{- end -}}
{{- end -}}

{{/*
common.fileconfigmap: whole config FILES, for the third-party services that
take a --config.file rather than environment variables (loki, blackbox). Under
compose these are bind mounts; here the file becomes a ConfigMap the pod
mounts as a volume. The content is read from the chart's own files/ directory
with .Files.Get -- Helm cannot read outside the chart, so the file is a copy,
and deploy/k8s/scripts/validate.sh diffs it against the compose original so
the copy cannot drift (the same construction as the toolchain gate).
*/}}
{{- define "common.fileconfigmap" -}}
{{- with .Values.configFiles -}}
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ include "common.fullname" $ }}-files
  labels:
    {{- include "common.labels" $ | nindent 4 }}
data:
  {{- range . }}
  {{ .name }}: |
    {{- $.Files.Get .path | nindent 4 }}
  {{- end }}
{{- end -}}
{{- end -}}

{{/*
common.envFrom: the container-side half. Referenced from both workload
templates so a service that gains a ConfigMap or Secret picks it up with no
edit to the workload shape.
*/}}
{{- define "common.envFrom" -}}
{{- if or .Values.config .Values.secret .Values.extraEnvFrom }}
envFrom:
  {{- if .Values.config }}
  - configMapRef:
      name: {{ include "common.fullname" . }}-config
  {{- end }}
  {{- if .Values.secret }}
  - secretRef:
      name: {{ include "common.fullname" . }}-secret
  {{- end }}
  {{- with .Values.extraEnvFrom }}
  {{- toYaml . | nindent 2 }}
  {{- end }}
{{- end }}
{{- end -}}

{{/*
common.configChecksum: a hash of everything this service's ConfigMap and
Secret contain, stamped onto the pod template.

Editing a ConfigMap or Secret does NOT restart the pods consuming it. With
envFrom the container reads its environment once, at start, so a `helm
upgrade` that changes only configuration reports success, leaves the pods
running, and the new value never takes effect -- a silent no-op that looks
exactly like a successful deploy. Changing the pod template is what makes
Kubernetes roll, and a checksum annotation is the smallest honest change.
*/}}
{{- define "common.configChecksum" -}}
{{- $parts := list -}}
{{- with .Values.config }}{{- $parts = append $parts (toYaml .) }}{{- end }}
{{- with .Values.secret }}{{- $parts = append $parts (toYaml .) }}{{- end }}
{{- range .Values.configFiles }}{{- $parts = append $parts ($.Files.Get .path) }}{{- end }}
{{- join "\n" $parts | sha256sum -}}
{{- end -}}
