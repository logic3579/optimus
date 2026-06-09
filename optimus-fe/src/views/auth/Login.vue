<template>
  <a-card class="login-card">
    <h1 class="title">{{ $t('auth.login_title') }}</h1>
    <a-form :model="form" layout="vertical" @finish="onSubmit">
      <a-form-item :label="$t('auth.username')" name="username" :rules="[{ required: true }]">
        <a-input v-model:value="form.username" autocomplete="username" />
      </a-form-item>
      <a-form-item :label="$t('auth.password')" name="password" :rules="[{ required: true }]">
        <a-input-password v-model:value="form.password" autocomplete="current-password" />
      </a-form-item>
      <a-form-item>
        <a-button type="primary" html-type="submit" :loading="loading" block>
          {{ $t('auth.login') }}
        </a-button>
      </a-form-item>
    </a-form>
  </a-card>
</template>

<script setup lang="ts">
import { reactive, ref, inject } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { message } from 'ant-design-vue'
import { useAuthStore } from '@/stores/auth'
import { isBizError } from '@/utils/http-error'
import type { AuthApi } from '@/api/auth'

const form = reactive({ username: '', password: '' })
const loading = ref(false)
const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const authApi = inject<AuthApi>('authApi')!

async function onSubmit() {
  loading.value = true
  try {
    const pair = await authApi.login({ username: form.username, password: form.password })
    authStore.setActiveTokens(pair.access_token, pair.refresh_token)
    const redirect = (route.query.redirect as string) || '/dashboard'
    router.push(redirect)
  } catch (e) {
    if (isBizError(e)) {
      message.error(e.messageKey ? `auth.${e.messageKey}` : e.message)
    } else {
      message.error('network.error')
    }
  } finally {
    loading.value = false
  }
}
</script>

<style scoped lang="scss">
.login-card {
  width: 360px;
  box-shadow: 0 2px 16px rgba(0, 0, 0, 0.06);
}
.title {
  font-size: 20px;
  margin: 0 0 16px;
  text-align: center;
}
</style>
