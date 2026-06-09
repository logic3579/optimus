<template>
  <a-popconfirm
    :title="confirm ?? $t('common.ok')"
    :ok-text="$t('common.ok')"
    :cancel-text="$t('common.cancel')"
    @confirm="$emit('confirm')"
  >
    <slot />
  </a-popconfirm>
</template>

<script setup lang="ts">
defineProps<{ confirm?: string }>()
defineEmits<{ confirm: [] }>()
</script>
