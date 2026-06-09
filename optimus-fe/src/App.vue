<template>
  <a-config-provider :locale="antd" :theme="{ algorithm }">
    <component :is="layout">
      <router-view />
    </component>
  </a-config-provider>
</template>

<script setup lang="ts">
import { computed, watch, type Component } from 'vue'
import { theme as antdTheme } from 'ant-design-vue'
import { useRoute } from 'vue-router'
import { useAppStore } from '@/stores/app'
import { useI18n } from '@/hooks/useI18n'
import { antdLocale } from '@/locales'
import DefaultLayout from '@/layouts/DefaultLayout.vue'
import BlankLayout from '@/layouts/BlankLayout.vue'

const app = useAppStore()
const route = useRoute()
const { locale } = useI18n()

const antd = computed(() => antdLocale(app.locale))
const algorithm = computed(() =>
  app.theme === 'dark' ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm
)

const layout = computed<Component>(() =>
  route.meta?.layout === 'blank' ? BlankLayout : DefaultLayout
)

watch(
  () => app.locale,
  l => {
    locale.value = l
  },
  { immediate: true }
)
</script>
