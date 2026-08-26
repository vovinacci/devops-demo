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
  {{- with .Values.strategy }}
  strategy:
    {{- toYaml . | nindent 4 }}
  {{- end }}
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
          {{- include "common.envFrom" . | trim | nindent 10 }}
          {{- include "common.probes" . | trim | nindent 10 }}
          {{- with .Values.resources }}
          resources:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          {{- if or .Values.volumeMounts .Values.pvcMounts .Values.configFiles }}
          volumeMounts:
            {{- with .Values.volumeMounts }}
            {{- toYaml . | nindent 12 }}
            {{- end }}
            {{- range .Values.pvcMounts }}
            - name: {{ .name }}
              mountPath: {{ .mountPath }}
            {{- end }}
            {{- /*
            The file-ConfigMap mounts itself: the volume has to reference an
            object named <release>-<chart>-files, which a values file cannot
            compute. Services declare the files and the directory; the wiring
            is the library's job.
            */}}
            {{- if .Values.configFiles }}
            - name: config-files
              mountPath: {{ required "configFilesMountPath is required when configFiles is set" .Values.configFilesMountPath }}
              readOnly: true
            {{- end }}
          {{- end }}
      {{- if or .Values.volumes .Values.pvcMounts .Values.configFiles }}
      volumes:
        {{- with .Values.volumes }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
        {{- range .Values.pvcMounts }}
        - name: {{ .name }}
          persistentVolumeClaim:
            claimName: {{ include "common.fullname" $ }}-{{ .pvcName | default .name }}
        {{- end }}
        {{- if .Values.configFiles }}
        - name: config-files
          configMap:
            name: {{ include "common.fullname" . }}-files
        {{- end }}
      {{- end }}
{{- end -}}

{{/*
common.daemonset: one pod per node, for workloads whose job is to observe the
node they run on (the Alloy log shipper, RFC-0003 DK6). Same container shape
as common.deployment -- probes, resources, envFrom, config files -- minus the
things that make no sense per-node: there is no replica count to set and no
rollout surge to configure, because the scheduler decides the count.
*/}}
{{- define "common.daemonset" -}}
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: {{ include "common.fullname" . }}
  labels:
    {{- include "common.labels" . | nindent 4 }}
spec:
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
          {{- include "common.envFrom" . | trim | nindent 10 }}
          {{- include "common.probes" . | trim | nindent 10 }}
          {{- with .Values.resources }}
          resources:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          {{- if or .Values.volumeMounts .Values.configFiles }}
          volumeMounts:
            {{- with .Values.volumeMounts }}
            {{- toYaml . | nindent 12 }}
            {{- end }}
            {{- if .Values.configFiles }}
            - name: config-files
              mountPath: {{ required "configFilesMountPath is required when configFiles is set" .Values.configFilesMountPath }}
              readOnly: true
            {{- end }}
          {{- end }}
      {{- if or .Values.volumes .Values.configFiles }}
      volumes:
        {{- with .Values.volumes }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
        {{- if .Values.configFiles }}
        - name: config-files
          configMap:
            name: {{ include "common.fullname" . }}-files
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
          {{- include "common.envFrom" . | trim | nindent 10 }}
          {{- include "common.probes" . | trim | nindent 10 }}
          {{- with .Values.resources }}
          resources:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          {{- with .Values.volumeMounts }}
          volumeMounts:
            {{- toYaml . | nindent 12 }}
          {{- end }}
      {{- with .Values.volumes }}
      volumes:
        {{- toYaml . | nindent 8 }}
      {{- end }}
  {{- /*
  Parenthesised lookup: a chart with no `persistence` key at all would
  otherwise nil-pointer here, and an empty list would emit a null key.
  */}}
  {{- with (.Values.persistence).volumeClaims }}
  volumeClaimTemplates:
    {{- range . }}
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
  {{- end }}
{{- end -}}
