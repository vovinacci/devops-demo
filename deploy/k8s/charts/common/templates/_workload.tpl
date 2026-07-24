{{/*
common.deployment: the D6 service shape for stateless workloads. Every app
service (backend, analytics, reports, reports-ui, canary, ...) renders this
from a few values -- the uniformity is enforced by this one template, not by
copies staying in sync (RFC-0003 DK2, ADR-0015).
*/}}
{{- define "common.deployment" -}}
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "common.fullname" . }}
  labels:
    {{- include "common.labels" . | nindent 4 }}
spec:
  replicas: {{ .Values.replicaCount | default 1 }}
  selector:
    matchLabels:
      {{- include "common.selectorLabels" . | nindent 6 }}
  template:
    metadata:
      labels:
        {{- include "common.selectorLabels" . | nindent 8 }}
    spec:
      serviceAccountName: {{ include "common.serviceAccountName" . }}
      containers:
        - name: {{ .Chart.Name }}
          image: {{ include "common.image" . }}
          imagePullPolicy: {{ .Values.image.pullPolicy | default "IfNotPresent" }}
          {{- with .Values.command }}
          command:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          {{- with .Values.args }}
          args:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          {{- if or .Values.containerPort .Values.extraPorts }}
          ports:
            {{- include "common.containerPorts" . | trim | nindent 12 }}
          {{- end }}
          {{- with .Values.env }}
          env:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          {{- include "common.probes" . | trim | nindent 10 }}
          {{- with .Values.resources }}
          resources:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          {{- if or .Values.volumeMounts .Values.pvcMounts }}
          volumeMounts:
            {{- with .Values.volumeMounts }}
            {{- toYaml . | nindent 12 }}
            {{- end }}
            {{- range .Values.pvcMounts }}
            - name: {{ .name }}
              mountPath: {{ .mountPath }}
            {{- end }}
          {{- end }}
      {{- if or .Values.volumes .Values.pvcMounts }}
      volumes:
        {{- with .Values.volumes }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
        {{- range .Values.pvcMounts }}
        - name: {{ .name }}
          persistentVolumeClaim:
            claimName: {{ include "common.fullname" $ }}-{{ .pvcName | default .name }}
        {{- end }}
      {{- end }}
{{- end -}}

{{/*
common.statefulset: stable identity + durable per-replica storage for the
three Postgres instances (RFC-0003 DK7, ADR-0018). Pairs with a headless
common.service (clusterIP: None). Probes are exec pg_isready (type=exec).
*/}}
{{- define "common.statefulset" -}}
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: {{ include "common.fullname" . }}
  labels:
    {{- include "common.labels" . | nindent 4 }}
spec:
  serviceName: {{ include "common.fullname" . }}
  replicas: {{ .Values.replicaCount | default 1 }}
  selector:
    matchLabels:
      {{- include "common.selectorLabels" . | nindent 6 }}
  template:
    metadata:
      labels:
        {{- include "common.selectorLabels" . | nindent 8 }}
    spec:
      serviceAccountName: {{ include "common.serviceAccountName" . }}
      containers:
        - name: {{ .Chart.Name }}
          image: {{ include "common.image" . }}
          imagePullPolicy: {{ .Values.image.pullPolicy | default "IfNotPresent" }}
          {{- if or .Values.containerPort .Values.extraPorts }}
          ports:
            {{- include "common.containerPorts" . | trim | nindent 12 }}
          {{- end }}
          {{- with .Values.env }}
          env:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          {{- include "common.probes" . | trim | nindent 10 }}
          {{- with .Values.resources }}
          resources:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          {{- with .Values.volumeMounts }}
          volumeMounts:
            {{- toYaml . | nindent 12 }}
          {{- end }}
  volumeClaimTemplates:
    {{- range .Values.persistence.volumeClaims }}
    - metadata:
        name: {{ .name }}
      spec:
        accessModes:
          - {{ .accessMode | default "ReadWriteOnce" }}
        resources:
          requests:
            storage: {{ .size | default "1Gi" }}
        {{- with .storageClassName }}
        storageClassName: {{ . }}
        {{- end }}
    {{- end }}
{{- end -}}
