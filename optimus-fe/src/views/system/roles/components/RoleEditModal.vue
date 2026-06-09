<template>
  <a-modal
    :open="open"
    :title="isEdit ? $t('system.roles.edit') : $t('system.roles.create')"
    :confirm-loading="saving"
    @ok="onOk"
    @cancel="emit('update:open', false)"
  >
    <a-form ref="formRef" :model="form" layout="vertical" :rules="rules">
      <a-form-item v-if="!isEdit" :label="$t('system.roles.form_code')" name="code">
        <a-input v-model:value="form.code" />
      </a-form-item>
      <a-form-item :label="$t('system.roles.form_name')" name="name">
        <a-input v-model:value="form.name" />
      </a-form-item>
      <a-form-item :label="$t('system.roles.form_description')" name="description">
        <a-textarea v-model:value="form.description" :rows="3" />
      </a-form-item>
    </a-form>
  </a-modal>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch, inject } from 'vue'
import { message, type FormInstance } from 'ant-design-vue'
import { useI18n } from '@/hooks/useI18n'
import { isBizError } from '@/utils/http-error'
import { formDiff } from '@/utils/form-diff'
import type { RoleApi } from '@/api/role'
import type { RoleSummary } from '@/types/api'

const props = defineProps<{
  open: boolean
  initial?: RoleSummary | null
}>()
const emit = defineEmits<{
  (e: 'update:open', v: boolean): void
  (e: 'saved'): void
}>()

const { t } = useI18n()
const roleApi = inject<RoleApi>('roleApi')!

const isEdit = computed(() => !!props.initial)
const saving = ref(false)
const formRef = ref<FormInstance>()

const form = reactive({ code: '', name: '', description: '' })
let initialSnapshot = { name: '', description: '' }

const rules = computed(() => ({
  code: [{ required: true, min: 2, max: 64, message: t('form.required') }],
  name: [{ required: true, max: 128, message: t('form.required') }]
}))

watch(
  () => props.open,
  (open) => {
    if (!open) return
    formRef.value?.resetFields()
    if (props.initial) {
      form.code = props.initial.code
      form.name = props.initial.name
      form.description = props.initial.description
      initialSnapshot = { name: props.initial.name, description: props.initial.description }
    } else {
      form.code = ''
      form.name = ''
      form.description = ''
    }
  },
  { immediate: true }
)

async function onOk() {
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  saving.value = true
  try {
    if (isEdit.value && props.initial) {
      const patch = formDiff(initialSnapshot, { name: form.name, description: form.description })
      if (Object.keys(patch).length > 0) {
        await roleApi.update(props.initial.id, patch)
      }
      message.success(t('system.roles.update_ok'))
    } else {
      await roleApi.create({
        code: form.code,
        name: form.name,
        description: form.description || undefined
      })
      message.success(t('system.roles.create_ok'))
    }
    emit('saved')
    emit('update:open', false)
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    saving.value = false
  }
}
</script>
