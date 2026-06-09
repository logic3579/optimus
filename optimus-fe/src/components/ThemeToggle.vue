<template>
  <a-button type="text" @click="toggle">
    <BulbOutlined v-if="app.theme === 'dark'" />
    <BulbFilled v-else />
  </a-button>
</template>

<script setup lang="ts">
import { BulbOutlined, BulbFilled } from '@ant-design/icons-vue'
import { useAppStore } from '@/stores/app'

const app = useAppStore()
function toggle() {
  app.setTheme(app.theme === 'dark' ? 'light' : 'dark')
}
</script>
