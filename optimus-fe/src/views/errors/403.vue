<template>
  <a-result status="403" :title="$t('errors.403_title')">
    <template #extra>
      <a-button type="primary" @click="$router.push('/')">{{ $t('errors.back_home') }}</a-button>
    </template>
  </a-result>
</template>
