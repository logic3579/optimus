<template>
  <a-layout class="default-layout">
    <a-layout-sider v-model:collapsed="collapsed" :trigger="null" collapsible>
      <AppSidebar :collapsed="collapsed" />
    </a-layout-sider>
    <a-layout>
      <a-layout-header class="header">
        <AppHeader :collapsed="collapsed" @toggle="collapsed = !collapsed" />
      </a-layout-header>
      <a-layout-content class="content">
        <router-view />
      </a-layout-content>
    </a-layout>
  </a-layout>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useAppStore } from '@/stores/app'
import AppSidebar from '@/components/AppSidebar.vue'
import AppHeader from '@/components/AppHeader.vue'

const app = useAppStore()
const collapsed = ref(app.sidebarCollapsed)
watch(collapsed, v => {
  if (v !== app.sidebarCollapsed) app.toggleSidebar()
})
watch(
  () => app.sidebarCollapsed,
  v => {
    collapsed.value = v
  }
)
</script>

<style scoped lang="scss">
.default-layout {
  min-height: 100vh;
}
.header {
  background: #fff;
  padding: 0 16px;
  border-bottom: 1px solid var(--ant-color-border, #f0f0f0);
}
.content {
  padding: 16px;
  background: var(--ant-color-bg-layout, #f5f5f5);
}
</style>
