<template>
  <div class="values-editor">
    <div class="actions">
      <a-button size="small" :loading="loadingDefaults" @click="onLoadDefaults">
        {{ t('apps.install.btn.loadDefaults') }}
      </a-button>
      <a-button size="small" @click="onFormat">{{ t('apps.install.btn.format') }}</a-button>
    </div>
    <Codemirror
      :model-value="modelValue"
      :extensions="extensions"
      :style="{ minHeight: '420px', fontFamily: 'monospace', fontSize: '13px' }"
      @update:model-value="onUpdate"
    />
    <a-typography-text v-if="parseError" type="danger" class="parse-err">
      {{ parseError }}
    </a-typography-text>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, ref } from 'vue'
import { message, Modal } from 'ant-design-vue'
import { Codemirror } from 'vue-codemirror'
import { yaml } from '@codemirror/lang-yaml'
import { oneDark } from '@codemirror/theme-one-dark'
import { EditorView } from '@codemirror/view'
import jsYaml from 'js-yaml'
import { useI18n } from '@/hooks/useI18n'
import { isBizError } from '@/utils/http-error'
import type { AppsRepoApi } from '@/api/apps/repo'

/**
 * ValuesEditor — CodeMirror-backed YAML editor with two extra actions:
 *
 *   1. Load defaults   — fetches the chart's default values.yaml from the BE
 *                        (overwrites with confirm prompt if buffer non-empty).
 *   2. Format          — round-trips through js-yaml, re-emits canonical
 *                        2-space indentation. Only succeeds when the document
 *                        decodes to a YAML map (top-level mapping).
 *
 * No localStorage draft cache per spec §8.4: closing the wizard discards the
 * buffer intentionally.
 */
const props = defineProps<{
  modelValue: string
  repoId?: number
  chartName?: string
  chartVersion?: string
}>()
const emit = defineEmits<{ (e: 'update:modelValue', v: string): void }>()

const { t } = useI18n()
const repoApi = inject<AppsRepoApi>('appsRepoApi')!

const loadingDefaults = ref(false)
const parseError = ref<string | null>(null)

const extensions = computed(() => [yaml(), oneDark, EditorView.lineWrapping])

function onUpdate(v: string): void {
  emit('update:modelValue', v)
}

async function onLoadDefaults(): Promise<void> {
  if (!props.repoId || !props.chartName || !props.chartVersion) {
    message.warning(t('apps.install.msg.pickChartFirst'))
    return
  }
  if (props.modelValue.trim() !== '') {
    const confirmed = await new Promise<boolean>((resolve) => {
      Modal.confirm({
        title: t('apps.install.confirm.loadDefaults.title'),
        content: t('apps.install.confirm.loadDefaults.body'),
        onOk: () => resolve(true),
        onCancel: () => resolve(false),
      })
    })
    if (!confirmed) return
  }
  loadingDefaults.value = true
  try {
    const r = await repoApi.getDefaultValues(props.repoId, props.chartName, props.chartVersion)
    emit('update:modelValue', r.values_yaml)
    parseError.value = null
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    loadingDefaults.value = false
  }
}

function onFormat(): void {
  if (props.modelValue.trim() === '') return
  try {
    const obj = jsYaml.load(props.modelValue)
    if (obj && typeof obj === 'object' && !Array.isArray(obj)) {
      const dumped = jsYaml.dump(obj, { indent: 2, lineWidth: 120, noRefs: true })
      emit('update:modelValue', dumped)
      parseError.value = null
    } else {
      parseError.value = t('apps.install.msg.valuesNotMap')
    }
  } catch (e) {
    parseError.value = e instanceof Error ? e.message : String(e)
  }
}
</script>

<style scoped lang="scss">
.values-editor .actions {
  display: flex;
  gap: 8px;
  margin-bottom: 8px;
}
.values-editor .parse-err {
  display: block;
  margin-top: 8px;
}
</style>
