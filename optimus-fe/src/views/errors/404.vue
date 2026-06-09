<template>
  <a-result status="404" :title="$t('errors.404_title')">
    <template #extra>
      <a-button type="primary" @click="$router.push('/')">{{ $t('errors.back_home') }}</a-button>
    </template>
  </a-result>
</template>
