<template>
  <a-result status="500" :title="$t('errors.500_title')">
    <template #extra>
      <a-button type="primary" @click="$router.push('/')">{{ $t('errors.back_home') }}</a-button>
    </template>
  </a-result>
</template>
