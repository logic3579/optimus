<template>
  <a-form :model="form" layout="vertical">
    <a-form-item label="Name"><a-input v-model:value="form.name" /></a-form-item>
    <a-form-item label="Base URL"><a-input v-model:value="form.base_url" /></a-form-item>
    <a-form-item label="Authentication">
      <a-select v-model:value="form.auth_type" :options="authOptions" />
    </a-form-item>
    <a-form-item v-if="form.auth_type !== 'none'" label="HTTP credential">
      <a-select v-model:value="form.http_credential_id" :options="credentialOptions" allow-clear />
    </a-form-item>
    <a-form-item label="Cluster (optional)"><a-select v-model:value="form.cluster_id" :options="clusterOptions" allow-clear /></a-form-item>
    <a-form-item label="Custom CA (public PEM)"><a-textarea v-model:value="form.custom_ca_pem" :rows="5" /></a-form-item>
    <a-form-item label="Skip TLS verification"><a-switch v-model:checked="form.tls_skip_verify" /></a-form-item>
    <a-alert v-if="form.tls_skip_verify" data-testid="tls-warning" type="error" message="TLS certificate verification is disabled" show-icon />
    <a-form-item label="Description"><a-textarea v-model:value="form.description" /></a-form-item>
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
import { computed, reactive, watch } from 'vue'
import type { HTTPCredentialSummary } from '@/api/credentials/http-credential'
import type { DatasourceDetail, NamedRef, SaveDatasource } from '@/types/observability'

const props = defineProps<{ initial?: DatasourceDetail | null; credentials: HTTPCredentialSummary[]; clusters: NamedRef[] }>()
const form = reactive<SaveDatasource>({ name: '', base_url: '', auth_type: 'none', tls_skip_verify: false, description: '' })
const authOptions = [{ value: 'none', label: 'None' }, { value: 'basic', label: 'Basic' }, { value: 'bearer', label: 'Bearer' }]
const credentialOptions = computed(() => props.credentials.filter(item => item.auth_type === form.auth_type).map(item => ({ value: item.id, label: item.name })))
const clusterOptions = computed(() => props.clusters.map(item => ({ value: item.id, label: item.name })))
function reset() {
  Object.assign(form, { name: props.initial?.name ?? '', base_url: props.initial?.base_url ?? '', auth_type: props.initial?.auth_type ?? 'none', http_credential_id: props.initial?.http_credential?.id, cluster_id: props.initial?.cluster?.id, tls_skip_verify: props.initial?.tls_skip_verify ?? false, custom_ca_pem: '', description: props.initial?.description ?? '' })
}
watch(() => props.initial, reset, { immediate: true })
watch(() => form.auth_type, value => { if (value === 'none') form.http_credential_id = undefined; else if (!props.credentials.some(item => item.id === form.http_credential_id && item.auth_type === value)) form.http_credential_id = undefined })
function payload(): SaveDatasource {
  if (!validateDatasourceURL(form.base_url)) throw new Error('observability.datasource.invalid_url')
  return { ...form, http_credential_id: form.auth_type === 'none' ? undefined : form.http_credential_id }
}
defineExpose({ form, credentialOptions, clusterOptions, payload })
</script>
