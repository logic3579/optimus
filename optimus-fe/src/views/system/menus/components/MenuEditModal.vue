<template>
  <a-modal
    :open="open"
    :title="isEdit ? $t('system.menus.edit') : (props.parentId ? $t('system.menus.create_child') : $t('system.menus.create_root'))"
    :confirm-loading="saving"
    width="640px"
    @ok="onOk"
    @cancel="emit('update:open', false)"
  >
    <a-form ref="formRef" :model="form" layout="vertical" :rules="rules">
      <a-form-item :label="$t('system.menus.form_parent')" name="parent_id">
        <a-tree-select
          v-model:value="form.parent_id"
          :tree-data="parentOptions"
          allow-clear
          :placeholder="$t('system.menus.form_parent_root')"
          tree-default-expand-all
        />
      </a-form-item>
      <a-form-item :label="$t('system.menus.form_code')" name="code">
        <a-input v-model:value="form.code" :disabled="isEdit" />
      </a-form-item>
      <a-form-item :label="$t('system.menus.form_name')" name="name">
        <a-input v-model:value="form.name" />
      </a-form-item>
      <a-form-item :label="$t('system.menus.form_path')" name="path">
        <a-input v-model:value="form.path" />
      </a-form-item>
      <a-form-item :label="$t('system.menus.form_component')" name="component">
        <a-input v-model:value="form.component" />
      </a-form-item>
      <a-form-item :label="$t('system.menus.form_icon')" name="icon">
        <a-input v-model:value="form.icon" />
      </a-form-item>
      <a-form-item :label="$t('system.menus.form_permission_code')" name="permission_code">
        <a-input v-model:value="form.permission_code" />
      </a-form-item>
      <a-form-item :label="$t('system.menus.form_sort_order')" name="sort_order">
        <a-input-number v-model:value="form.sort_order" />
      </a-form-item>
      <a-form-item :label="$t('system.menus.form_hidden')" name="hidden">
        <a-switch v-model:checked="form.hidden" />
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
import type { MenuApi } from '@/api/menu'
import type { MenuNode, MenuUpdateRequest } from '@/types/api'

interface TreeSelectOption {
  value: number | null
  label: string
  children?: TreeSelectOption[]
}

const props = defineProps<{
  open: boolean
  initial?: MenuNode | null      // null/undefined → create mode
  parentId?: number | null        // when creating a child of an existing node
  tree: MenuNode[]                // current full tree, used to build parent select
}>()
const emit = defineEmits<{
  (e: 'update:open', v: boolean): void
  (e: 'saved'): void
}>()

const { t } = useI18n()
const menuApi = inject<MenuApi>('menuApi')!

const isEdit = computed(() => !!props.initial)
const saving = ref(false)
const formRef = ref<FormInstance>()

const form = reactive({
  parent_id: null as number | null,
  code: '',
  name: '',
  path: '',
  component: '',
  icon: '',
  permission_code: '',
  sort_order: 10,
  hidden: false
})
let snapshot = { ...form }

const rules = computed(() => ({
  code: [{ required: true, min: 2, max: 64, message: t('form.required') }],
  name: [{ required: true, max: 128, message: t('form.required') }]
}))

function buildParentOptions(nodes: MenuNode[]): TreeSelectOption[] {
  return nodes.map(n => ({
    value: n.id,
    label: n.code,
    children: n.children ? buildParentOptions(n.children) : undefined
  }))
}
const parentOptions = computed(() => buildParentOptions(props.tree))

watch(
  () => props.open,
  (open) => {
    if (!open) return
    formRef.value?.resetFields()
    if (props.initial) {
      form.parent_id = props.initial.parent_id ?? null
      form.code = props.initial.code
      form.name = props.initial.name
      form.path = props.initial.path
      form.component = props.initial.component
      form.icon = props.initial.icon
      form.permission_code = props.initial.permission_code ?? ''
      form.sort_order = props.initial.sort_order
      form.hidden = props.initial.hidden
    } else {
      form.parent_id = props.parentId ?? null
      form.code = ''
      form.name = ''
      form.path = ''
      form.component = ''
      form.icon = ''
      form.permission_code = ''
      form.sort_order = 10
      form.hidden = false
    }
    snapshot = { ...form }
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
      const patch = formDiff(snapshot, { ...form }) as MenuUpdateRequest
      if (Object.keys(patch).length > 0) {
        // Coerce '' → undefined for optional fields so backend doesn't see empty strings
        if (patch.permission_code === '') patch.permission_code = undefined
        await menuApi.update(props.initial.id, patch)
      }
      message.success(t('system.menus.update_ok'))
    } else {
      await menuApi.create({
        parent_id: form.parent_id,
        code: form.code,
        name: form.name,
        path: form.path || undefined,
        component: form.component || undefined,
        icon: form.icon || undefined,
        permission_code: form.permission_code || undefined,
        sort_order: form.sort_order,
        hidden: form.hidden
      })
      message.success(t('system.menus.create_ok'))
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
