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
{{- $_ := required "database.urlSecret.name is required" .Values.database.urlSecret.name -}}
{{- $_ := required "oidc.issuer is required" .Values.oidc.issuer -}}
{{- $_ := required "oidc.clientID is required" .Values.oidc.clientID -}}
{{- $_ := required "oidc.clientSecret.name is required" .Values.oidc.clientSecret.name -}}
{{- $_ := required "session.keySecret.name is required" .Values.session.keySecret.name -}}
{{- $_ := required "anthropic.apiKeySecret.name is required" .Values.anthropic.apiKeySecret.name -}}
{{- if not .Values.auth.allowed -}}
{{- fail "auth.allowed must list at least one claim value" -}}
{{- end -}}
{{- end -}}

{{/* matlistan.theme is "true" when any theme value is set. */}}
{{- define "matlistan.theme" -}}
{{- $t := .Values.theme | default dict -}}
{{- if or $t.light $t.dark -}}true{{- end -}}
{{- end -}}

{{/* Env shared by serve and generate. */}}
{{- define "matlistan.env" -}}
{{- $v := .Values -}}
- name: MATLISTAN_DATABASE_URL
  valueFrom:
    secretKeyRef:
      name: {{ $v.database.urlSecret.name }}
      key: {{ $v.database.urlSecret.key }}
{{- if $v.database.caSecret.name }}
- name: MATLISTAN_DATABASE_CA_FILE
  value: /etc/matlistan/db-ca/ca.crt
{{- end }}
- name: MATLISTAN_LOCALE
  value: {{ $v.locale | quote }}
- name: MATLISTAN_TIMEZONE
  value: {{ $v.timezone | quote }}
- name: MATLISTAN_ANTHROPIC_API_KEY_FILE
  value: /etc/matlistan/secrets/anthropic-api-key
{{- with $v.anthropic.model }}
- name: MATLISTAN_MODEL
  value: {{ . | quote }}
{{- end }}
{{- end -}}

{{/* Projected secret volume; mode 0440 with fsGroup 65532 lets only the app read it. */}}
{{- define "matlistan.secretVolume" -}}
{{- $v := .Values -}}
- name: secrets
  projected:
    defaultMode: 0440
    sources:
      - secret:
          name: {{ $v.anthropic.apiKeySecret.name }}
          items:
            - key: {{ $v.anthropic.apiKeySecret.key }}
              path: anthropic-api-key
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
{{- if $v.database.caSecret.name }}
- name: db-ca
  secret:
    secretName: {{ $v.database.caSecret.name }}
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
