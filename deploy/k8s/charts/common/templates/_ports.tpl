{{/*
Container and Service ports. The primary port is named (default "http") so
probes and the ServiceMonitor can target it by name; extraPorts carry any
additional listeners (e.g. the backend's gRPC :50051, RFC-0001 D3).
*/}}

{{- define "common.containerPorts" -}}
{{- if .Values.containerPort }}
- name: {{ .Values.portName | default "http" }}
  containerPort: {{ .Values.containerPort }}
  protocol: TCP
{{- end }}
{{- range .Values.extraPorts }}
- name: {{ .name }}
  containerPort: {{ .containerPort }}
  protocol: {{ .protocol | default "TCP" }}
{{- end }}
{{- end -}}

{{- define "common.servicePorts" -}}
{{- if .Values.containerPort }}
- name: {{ .Values.portName | default "http" }}
  port: {{ .Values.service.port | default .Values.containerPort }}
  targetPort: {{ .Values.portName | default "http" }}
  protocol: TCP
{{- end }}
{{- range .Values.extraPorts }}
- name: {{ .name }}
  port: {{ .containerPort }}
  targetPort: {{ .name }}
  protocol: {{ .protocol | default "TCP" }}
{{- end }}
{{- end -}}
