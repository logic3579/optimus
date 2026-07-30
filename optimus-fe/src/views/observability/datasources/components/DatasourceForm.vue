<template>
  <a-form :model="form" layout="vertical">
    <a-form-item :label="t('observability_ui.forms.name')"><a-input v-model:value="form.name" /></a-form-item>
    <a-form-item :label="t('observability_ui.forms.base_url')"><a-input v-model:value="form.base_url" /></a-form-item>
    <a-form-item :label="t('observability_ui.credentials.auth_type')">
      <a-select v-model:value="form.auth_type" :options="authOptions" />
    </a-form-item>
    <a-form-item v-if="form.auth_type !== 'none'" :label="t('menu.credentials.http_credentials')">
      <a-select v-model:value="form.http_credential_id" :options="credentialOptions" allow-clear />
    </a-form-item>
    <a-form-item :label="t('observability_ui.forms.cluster_optional')"><a-select v-model:value="form.cluster_id" :options="clusterOptions" allow-clear /></a-form-item>
    <a-form-item v-if="initial?.has_custom_ca" :label="t('observability_ui.forms.existing_ca')">
      <a-radio-group v-model:value="caMode" :options="caOptions" />
    </a-form-item>
    <a-form-item v-if="!initial?.has_custom_ca || caMode === 'replace'" :label="t('observability_ui.forms.custom_ca')"><a-textarea v-model:value="form.custom_ca_pem" :rows="5" /></a-form-item>
    <a-form-item :label="t('observability_ui.forms.skip_tls')"><a-switch v-model:checked="form.tls_skip_verify" /></a-form-item>
    <a-alert v-if="form.tls_skip_verify" data-testid="tls-warning" type="error" :message="t('observability_ui.tls_warning')" show-icon />
    <a-form-item :label="t('observability_ui.forms.description')"><a-textarea v-model:value="form.description" /></a-form-item>
  </a-form>
</template>

<script lang="ts">
export function validateDatasourceURL(value: string): boolean {
  try {
    const url = new URL(value)
    return (url.protocol === 'http:' || url.protocol === 'https:') && !url.username && !url.password && !url.search && !url.hash
  } catch { return false }
}
</script>
<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from '@/hooks/useI18n'
import type { HTTPCredentialSummary } from '@/api/credentials/http-credential'
import type { DatasourceDetail, NamedRef, SaveDatasource } from '@/types/observability'

const props = defineProps<{ initial?: DatasourceDetail | null; credentials: HTTPCredentialSummary[]; clusters: NamedRef[] }>()
const { t } = useI18n()
const form = reactive<SaveDatasource>({ name: '', base_url: '', auth_type: 'none', tls_skip_verify: false, description: '' })
const caMode = ref<'keep' | 'replace' | 'clear'>('keep')
const caOptions = computed(() => [{ value: 'keep', label: t('observability_ui.ca.keep') }, { value: 'replace', label: t('observability_ui.ca.replace') }, { value: 'clear', label: t('observability_ui.ca.clear') }])
const authOptions = computed(() => [{ value: 'none', label: t('observability_ui.units.none') }, { value: 'basic', label: t('observability_ui.credentials.basic') }, { value: 'bearer', label: t('observability_ui.credentials.bearer') }])
const credentialOptions = computed(() => props.credentials.filter(item => item.auth_type === form.auth_type).map(item => ({ value: item.id, label: item.name })))
const clusterOptions = computed(() => props.clusters.map(item => ({ value: item.id, label: item.name })))
function reset() {
  caMode.value = props.initial?.has_custom_ca ? 'keep' : 'replace'
  Object.assign(form, { name: props.initial?.name ?? '', base_url: props.initial?.base_url ?? '', auth_type: props.initial?.auth_type ?? 'none', http_credential_id: props.initial?.http_credential?.id, cluster_id: props.initial?.cluster?.id, tls_skip_verify: props.initial?.tls_skip_verify ?? false, custom_ca_pem: '', description: props.initial?.description ?? '' })
}
watch(() => props.initial, reset, { immediate: true })
watch(() => form.auth_type, value => { if (value === 'none') form.http_credential_id = undefined; else if (!props.credentials.some(item => item.id === form.http_credential_id && item.auth_type === value)) form.http_credential_id = undefined })
function payload(): SaveDatasource {
  if (!validateDatasourceURL(form.base_url)) throw validationError('observability.datasource.invalid_url')
  if (form.auth_type !== 'none' && !form.http_credential_id) {
    throw validationError('observability.datasource.credential_required')
  }
  if (props.initial?.has_custom_ca && caMode.value === 'replace' && !form.custom_ca_pem?.trim()) {
    throw validationError('observability.datasource.ca_required')
  }
  const result: SaveDatasource = {
    name: form.name, base_url: form.base_url, auth_type: form.auth_type,
    tls_skip_verify: form.tls_skip_verify, description: form.description,
  }
  if (form.auth_type !== 'none' && form.http_credential_id) result.http_credential_id = form.http_credential_id
  if (form.cluster_id) result.cluster_id = form.cluster_id
  if (props.initial?.http_credential && form.auth_type === 'none') result.clear_http_credential = true
  if (props.initial?.cluster && !form.cluster_id) result.clear_cluster = true
  if (props.initial?.has_custom_ca) {
    if (caMode.value === 'clear') result.clear_custom_ca = true
    if (caMode.value === 'replace' && form.custom_ca_pem) result.custom_ca_pem = form.custom_ca_pem
  } else if (form.custom_ca_pem) result.custom_ca_pem = form.custom_ca_pem
  return result
}
function validationError(messageKey: string): Error & { message_key: string } {
  return Object.assign(new Error(messageKey), { message_key: messageKey })
}
defineExpose({ form, caMode, credentialOptions, clusterOptions, payload })
</script>
