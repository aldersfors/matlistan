{{- define "matlistan.fullname" -}}
{{- if contains "matlistan" .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-matlistan" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "matlistan.selectorLabels" -}}
app.kubernetes.io/name: matlistan
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "matlistan.labels" -}}
{{ include "matlistan.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{- end -}}

{{- define "matlistan.validate" -}}
{{- $_ := required "baseURL is required" .Values.baseURL -}}
{{- $_ := required "database.urlSecret.name is required (or set database.cnpg.enabled)" (include "matlistan.dbURLSecret" .) -}}
{{- $_ := required "oidc.issuer is required" .Values.oidc.issuer -}}
{{- $_ := required "oidc.clientID is required" .Values.oidc.clientID -}}
{{- $_ := required "oidc.clientSecret.name is required" .Values.oidc.clientSecret.name -}}
{{- $_ := required "session.keySecret.name is required" .Values.session.keySecret.name -}}
{{- if and (eq .Values.llm.provider "openai") (or .Values.anthropic.apiKeySecret.name .Values.anthropic.model) -}}
{{- fail "anthropic.* is set but llm.provider is openai: remove the anthropic block so its key is never sent to another provider" -}}
{{- end -}}
{{- if and .Values.networkPolicy.llm.port (not .Values.networkPolicy.llm.cidrs) -}}
{{- fail "networkPolicy.llm.port needs networkPolicy.llm.cidrs, or it opens that port to every destination" -}}
{{- end -}}
{{- if and (eq .Values.llm.provider "openai") (not .Values.llm.model) -}}
{{- fail "llm.model is required for llm.provider openai" -}}
{{- end -}}
{{- $keyless := and (eq .Values.llm.provider "openai") .Values.llm.baseURL (not (contains "api.openai.com" .Values.llm.baseURL)) -}}
{{- if and (not $keyless) (not (include "matlistan.llmKeySecret" .)) -}}
{{- fail "llm.apiKeySecret.name is required (except for an OpenAI-compatible server without a key)" -}}
{{- end -}}
{{- if not .Values.auth.allowed -}}
{{- fail "auth.allowed must list at least one claim value" -}}
{{- end -}}
{{- if and (include "matlistan.pushCertManaged" .) (not (.Capabilities.APIVersions.Has "cert-manager.io/v1")) -}}
{{- fail "push.enabled needs cert-manager (cert-manager.io/v1) to make the VAPID key, or set push.vapidKeySecret.name to an existing Secret" -}}
{{- end -}}
{{- end -}}

{{/* The CNPG cluster's name, and the database secrets: explicit names win, otherwise
the CNPG cluster's own <cluster>-app and <cluster>-ca when database.cnpg is on. */}}
{{- define "matlistan.dbCluster" -}}
{{- printf "%s-db" (include "matlistan.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "matlistan.dbURLSecret" -}}
{{- if .Values.database.urlSecret.name -}}
{{- .Values.database.urlSecret.name -}}
{{- else if .Values.database.cnpg.enabled -}}
{{- printf "%s-app" (include "matlistan.dbCluster" .) -}}
{{- end -}}
{{- end -}}

{{- define "matlistan.dbCASecret" -}}
{{- if .Values.database.caSecret.name -}}
{{- .Values.database.caSecret.name -}}
{{- else if .Values.database.cnpg.enabled -}}
{{- printf "%s-ca" (include "matlistan.dbCluster" .) -}}
{{- end -}}
{{- end -}}

{{/* matlistan.theme is "true" when any theme value is set. */}}
{{- define "matlistan.theme" -}}
{{- $t := .Values.theme | default dict -}}
{{- if or $t.light $t.dark -}}true{{- end -}}
{{- end -}}

{{/* The model key secret and its key, and the model: llm wins over the deprecated
anthropic values. */}}
{{- define "matlistan.llmKeySecret" -}}
{{- if .Values.llm.apiKeySecret.name -}}{{ .Values.llm.apiKeySecret.name }}
{{- else if eq .Values.llm.provider "anthropic" -}}{{ .Values.anthropic.apiKeySecret.name }}
{{- end -}}
{{- end -}}

{{- define "matlistan.llmKeyKey" -}}
{{- if .Values.llm.apiKeySecret.name -}}{{ .Values.llm.apiKeySecret.key }}{{- else -}}{{ .Values.anthropic.apiKeySecret.key }}{{- end -}}
{{- end -}}

{{- define "matlistan.llmModel" -}}
{{- if .Values.llm.model -}}{{ .Values.llm.model }}
{{- else if eq .Values.llm.provider "anthropic" -}}{{ .Values.anthropic.model }}
{{- end -}}
{{- end -}}

{{/* Env shared by serve and generate. */}}
{{- define "matlistan.env" -}}
{{- $v := .Values -}}
- name: MATLISTAN_DATABASE_URL
  valueFrom:
    secretKeyRef:
      name: {{ include "matlistan.dbURLSecret" . }}
      key: {{ $v.database.urlSecret.key }}
{{- if include "matlistan.dbCASecret" . }}
- name: MATLISTAN_DATABASE_CA_FILE
  value: /etc/matlistan/db-ca/ca.crt
{{- end }}
- name: MATLISTAN_LOCALE
  value: {{ $v.locale | quote }}
- name: MATLISTAN_TIMEZONE
  value: {{ $v.timezone | quote }}
- name: MATLISTAN_PROVIDER
  value: {{ $v.llm.provider | quote }}
{{- with include "matlistan.llmModel" . }}
- name: MATLISTAN_MODEL
  value: {{ . | quote }}
{{- end }}
{{- with $v.llm.maxOutputTokens }}
- name: MATLISTAN_MAX_OUTPUT_TOKENS
  value: {{ . | quote }}
{{- end }}
{{- with $v.llm.baseURL }}
- name: MATLISTAN_OPENAI_BASE_URL
  value: {{ . | quote }}
{{- end }}
{{- if include "matlistan.llmKeySecret" . }}
- name: MATLISTAN_API_KEY_FILE
  value: /etc/matlistan/secrets/llm-api-key
{{- end }}
{{- if include "matlistan.pushKeySecret" . }}
- name: MATLISTAN_VAPID_KEY_FILE
  value: /etc/matlistan/secrets/vapid-key
{{- with $v.push.subject }}
- name: MATLISTAN_VAPID_SUBJECT
  value: {{ . | quote }}
{{- end }}
{{- end }}
{{- end -}}

{{/* Projected secret volume; mode 0440 with fsGroup 65532 lets only the app read it. */}}
{{- define "matlistan.secretVolume" -}}
{{- $v := .Values -}}
{{- if or (not .jobOnly) .llmKey .pushKey }}
- name: secrets
  projected:
    defaultMode: 0440
    sources:
      {{- with .llmKey }}
      - secret:
          name: {{ . }}
          items:
            - key: {{ $.llmKeyKey }}
              path: llm-api-key
      {{- end }}
      {{- with .pushKey }}
      - secret:
          name: {{ . }}
          items:
            - key: {{ $v.push.vapidKeySecret.key }}
              path: vapid-key
      {{- end }}
      {{- if not .jobOnly }}
      - secret:
          name: {{ $v.oidc.clientSecret.name }}
          items:
            - key: {{ $v.oidc.clientSecret.key }}
              path: client-secret
      - secret:
          name: {{ $v.session.keySecret.name }}
          items:
            - key: {{ $v.session.keySecret.key }}
              path: session-key
      {{- end }}
{{- end }}
{{- with .caSecret }}
- name: db-ca
  secret:
    secretName: {{ . }}
    defaultMode: 0444
    items:
      - key: {{ $v.database.caSecret.key }}
        path: ca.crt
{{- end }}
{{- end -}}

{{- define "matlistan.podSecurity" -}}
automountServiceAccountToken: false
securityContext:
  runAsNonRoot: true
  runAsUser: 65532
  runAsGroup: 65532
  fsGroup: 65532
  seccompProfile:
    type: RuntimeDefault
{{- end -}}

{{- define "matlistan.containerSecurity" -}}
securityContext:
  readOnlyRootFilesystem: true
  allowPrivilegeEscalation: false
  capabilities:
    drop: ["ALL"]
{{- end -}}

{{/* The Secret holding the VAPID key: the named one, or the Certificate's. Empty when
push is off. */}}
{{- define "matlistan.pushKeySecret" -}}
{{- if .Values.push.enabled -}}
{{- if .Values.push.vapidKeySecret.name -}}{{ .Values.push.vapidKeySecret.name }}
{{- else -}}{{ printf "%s-vapid" (include "matlistan.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "matlistan.pushCertManaged" -}}
{{- if and .Values.push.enabled (not .Values.push.vapidKeySecret.name) -}}true{{- end -}}
{{- end -}}
