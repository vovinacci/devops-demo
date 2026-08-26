{{/*
Object templates are "clean blocks": each renders one manifest with no leading
or trailing document separator. The per-service templates/manifests.yaml joins
them with `---` lines. Conditional templates render an empty string when opted
out (an empty doc between separators is harmless to helm and kubeconform).
*/}}

{{/*
common.serviceaccount: one SA per service, so a later RFC can attach RBAC /
workload identity per service without reworking the topology.
*/}}
{{- define "common.serviceaccount" -}}
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {{ include "common.serviceAccountName" . }}
  labels:
    {{- include "common.labels" . | nindent 4 }}
{{- end -}}

{{/*
common.service: ClusterIP by default; headless (clusterIP: None) for the
Postgres StatefulSets' stable DNS identity. Rendered only when the service
opts in (loadgen, a long-running k6 job with no listener, sets enabled=false).
*/}}
{{- define "common.service" -}}
{{- if .Values.service.enabled -}}
apiVersion: v1
kind: Service
metadata:
  name: {{ include "common.fullname" . }}
  labels:
    {{- include "common.labels" . | nindent 4 }}
spec:
  type: {{ .Values.service.type | default "ClusterIP" }}
  {{- if .Values.service.headless }}
  clusterIP: None
  {{- end }}
  selector:
    {{- include "common.selectorLabels" . | nindent 4 }}
  ports:
    {{- include "common.servicePorts" . | trim | nindent 4 }}
{{- end -}}
{{- end -}}

{{/*
common.servicemonitor: the /metrics scrape target (RFC-0003 DK6). Default on;
opted out per-service (frontend meets no D6 endpoint, ADR-0013; mailpit / loki
/ blackbox expose no D6 /metrics or are covered by a Probe CR in PR-4). This
is a Prometheus Operator CR (monitoring.coreos.com/v1) -- the CI gate
validates it against a vendored CRD schema (DK9), never --ignore-missing.
*/}}
{{- define "common.servicemonitor" -}}
{{- if .Values.d6.serviceMonitor.enabled -}}
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: {{ include "common.fullname" . }}
  labels:
    {{- include "common.labels" . | nindent 4 }}
spec:
  {{- /*
  Without this, Prometheus names the job after the SERVICE -- platform-reports
  rather than reports -- and every query written against the compose stack
  silently matches nothing. The Reports JVM and Reports UI dashboards go blank
  and the AnalyticsStreamDown alert can never fire, while Prometheus itself
  looks perfectly healthy. jobLabel names a label ON the Service whose value
  becomes the job, so the job label is the service's own name and the rules
  and dashboards are portable across both stacks.
  */}}
  jobLabel: {{ .Values.d6.serviceMonitor.jobLabel | default "app.kubernetes.io/name" }}
  selector:
    matchLabels:
      {{- include "common.selectorLabels" . | nindent 6 }}
  endpoints:
    - port: {{ .Values.d6.serviceMonitor.port | default (.Values.portName | default "http") }}
      path: {{ .Values.d6.serviceMonitor.path | default "/metrics" }}
      interval: {{ .Values.d6.serviceMonitor.interval | default "30s" }}
{{- end -}}
{{- end -}}

{{/*
common.pvc: standalone PersistentVolumeClaims that outlive the pod -- the
reports service's generated-artifact volume (RFC-0001 D2, RFC-0003 DK7,
ADR-0018): files on a ReadWriteOnce volume, never BLOBs in Postgres. Multiple
claims are separated with their own `---`.
*/}}
{{- define "common.pvc" -}}
{{- range $i, $pvc := .Values.extraPVCs -}}
{{- if $i }}
---
{{ end -}}
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: {{ include "common.fullname" $ }}-{{ $pvc.name }}
  labels:
    {{- include "common.labels" $ | nindent 4 }}
spec:
  accessModes:
    - {{ $pvc.accessMode | default "ReadWriteOnce" }}
  resources:
    requests:
      storage: {{ $pvc.size | default "1Gi" }}
  {{- with $pvc.storageClassName }}
  storageClassName: {{ . }}
  {{- end }}
{{- end -}}
{{- end -}}
