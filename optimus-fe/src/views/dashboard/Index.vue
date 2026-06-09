<template>
  <a-card>
    <PageHeader :title="$t('menu.dashboard')" />
    <a-empty :description="$t('placeholder.coming_soon')" />
  </a-card>
</template>

<script setup lang="ts">
import PageHeader from '@/components/PageHeader.vue'
</script>
