<template>
  <div class="page-header">
    <h2 class="title">{{ title }}</h2>
    <div class="actions"><slot /></div>
  </div>
</template>

<script setup lang="ts">
defineProps<{ title: string }>()
</script>

<style scoped lang="scss">
.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
}
.title {
  margin: 0;
  font-size: 18px;
}
.actions {
  display: flex;
  gap: 8px;
}
</style>
