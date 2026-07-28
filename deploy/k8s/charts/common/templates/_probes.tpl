{{/*
Probes: the load-bearing D6 payoff (RFC-0003 DK4). A D6-compliant service
wires livenessProbe from /healthz and readinessProbe from /readyz with no
per-service special-casing -- those are the defaults below.

Each probe (liveness / readiness / startup) is an independent, per-service
value with a `type`: httpGet | tcpSocket | grpc | exec | none. This is the
opt-out mechanism DK2 requires:

  - The nginx `frontend` (the ONE documented D6 exception, ADR-0013) sets
    liveness.type=tcpSocket and readiness.type=none: an honest "the static
    server answers on :80" check, never a fake httpGet /healthz that nginx's
    SPA fallback would answer 200 with the app shell (a vacuous probe).
  - `postgres_exporter` / `loadgen` have no HTTP health endpoint either and
    use tcpSocket / none the same way -- opt-out is a named posture, not a
    hole.
  - Postgres StatefulSets use type=exec (pg_isready).
  - The backend adds a startup probe of type=grpc against the gRPC Health
    Checking Protocol on :50051 (RFC-0001 D3) while keeping the D6 HTTP
    liveness/readiness pair -- k8s allows one handler per probe, so the gRPC
    health check rides the startupProbe rather than displacing /healthz.
*/}}

{{- define "common.probeHandler" -}}
{{- $type := .type | default "httpGet" -}}
{{- if eq $type "httpGet" }}
httpGet:
  path: {{ .path | default "/healthz" }}
  port: {{ .port | default "http" }}
{{- else if eq $type "tcpSocket" }}
tcpSocket:
  port: {{ .port | default "http" }}
{{- else if eq $type "grpc" }}
grpc:
  port: {{ .port }}
{{- else if eq $type "exec" }}
exec:
  command:
    {{- toYaml .command | nindent 4 }}
{{- else if ne $type "none" }}
{{- /*
An unrecognised type matches no branch and would emit a probe with timing
fields and no handler. kubeconform -strict accepts that (every handler is
optional in the schema) while the API server rejects it, so the typo would
survive the gate and surface only at install. Fail at render instead.
*/}}
{{- fail (printf "common.probeHandler: unsupported probe type %q (want httpGet|tcpSocket|grpc|exec|none)" $type) }}
{{- end }}
{{- with .initialDelaySeconds }}
initialDelaySeconds: {{ . }}
{{- end }}
{{- with .periodSeconds }}
periodSeconds: {{ . }}
{{- end }}
{{- with .timeoutSeconds }}
timeoutSeconds: {{ . }}
{{- end }}
{{- with .failureThreshold }}
failureThreshold: {{ . }}
{{- end }}
{{- with .successThreshold }}
successThreshold: {{ . }}
{{- end }}
{{- end -}}

{{- define "common.probes" -}}
{{- $d6 := .Values.d6 | default dict -}}
{{- with $d6.liveness }}
{{- if ne (.type | default "httpGet") "none" }}
livenessProbe:
  {{- include "common.probeHandler" . | nindent 2 }}
{{- end }}
{{- end }}
{{- with $d6.readiness }}
{{- if ne (.type | default "httpGet") "none" }}
readinessProbe:
  {{- include "common.probeHandler" . | nindent 2 }}
{{- end }}
{{- end }}
{{- with $d6.startup }}
{{- if ne (.type | default "none") "none" }}
startupProbe:
  {{- include "common.probeHandler" . | nindent 2 }}
{{- end }}
{{- end }}
{{- end -}}
