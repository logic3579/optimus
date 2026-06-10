<script setup lang="ts">
import { computed } from 'vue'
import { Codemirror } from 'vue-codemirror'
import { yaml } from '@codemirror/lang-yaml'
import { oneDark } from '@codemirror/theme-one-dark'
import { EditorView } from '@codemirror/view'
import { EditorState } from '@codemirror/state'

/**
 * Read-only YAML viewer backed by CodeMirror 6. `readonly` defaults to `true`
 * so callers can `<YamlViewer v-model="text" />` without worrying about
 * accidental edits. Pass `:readonly="false"` to make the editor writable.
 *
 * The component emits `update:modelValue` so it composes with `v-model`,
 * which keeps a future "edit YAML" workflow trivial to wire up.
 */
const props = withDefaults(
  defineProps<{ modelValue: string; readonly?: boolean }>(),
  { readonly: true }
)
const emit = defineEmits<{ (e: 'update:modelValue', v: string): void }>()

const extensions = computed(() => [
  yaml(),
  oneDark,
  EditorView.lineWrapping,
  EditorState.readOnly.of(props.readonly === true),
])

function onUpdate(v: string): void {
  emit('update:modelValue', v)
}
</script>

<template>
  <Codemirror
    :model-value="modelValue"
    :extensions="extensions"
    :disabled="readonly !== false"
    :style="{ minHeight: '400px', fontFamily: 'monospace', fontSize: '12px' }"
    @update:model-value="onUpdate"
  />
</template>
