<template>
  <a-dropdown>
    <a-button type="text">
      <GlobalOutlined /> {{ current }}
    </a-button>
    <template #overlay>
      <a-menu @click="onClick">
        <a-menu-item key="zh-CN">中文</a-menu-item>
        <a-menu-item key="en-US">English</a-menu-item>
      </a-menu>
    </template>
  </a-dropdown>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { GlobalOutlined } from '@ant-design/icons-vue'
import { useAppStore } from '@/stores/app'
import { useI18n } from '@/hooks/useI18n'
import type { SupportedLocale } from '@/locales'

const app = useAppStore()
const { locale } = useI18n()
const current = computed(() => (app.locale === 'zh-CN' ? '中' : 'EN'))

function onClick({ key }: { key: string }) {
  const l = key as SupportedLocale
  app.setLocale(l)
  locale.value = l
}
</script>
