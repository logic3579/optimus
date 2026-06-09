<template>
  <div class="profile-page">
    <PageHeader :title="$t('profile.title')" />

    <a-card :title="$t('profile.title')" class="u-mb-16">
      <a-form :model="profile" layout="vertical" @finish="onSaveProfile">
        <a-form-item :label="$t('profile.display_name')" name="display_name">
          <a-input v-model:value="profile.display_name" />
        </a-form-item>
        <a-form-item :label="$t('profile.email')" name="email" :rules="[{ type: 'email' }]">
          <a-input v-model:value="profile.email" />
        </a-form-item>
        <a-form-item :label="$t('profile.avatar_url')" name="avatar_url">
          <a-input v-model:value="profile.avatar_url" />
        </a-form-item>
        <a-form-item>
          <a-button type="primary" html-type="submit" :loading="profileSaving">{{ $t('common.save') }}</a-button>
        </a-form-item>
      </a-form>
    </a-card>

    <a-card :title="$t('profile.change_password')">
      <a-form :model="pw" layout="vertical" @finish="onChangePassword">
        <a-form-item :label="$t('profile.old_password')" name="old_password" :rules="[{ required: true }]">
          <a-input-password v-model:value="pw.old_password" />
        </a-form-item>
        <a-form-item :label="$t('profile.new_password')" name="new_password" :rules="[{ required: true, min: 8 }]">
          <a-input-password v-model:value="pw.new_password" />
        </a-form-item>
        <a-form-item :label="$t('profile.confirm_password')" name="confirm" :rules="[{ validator: validateConfirm }]">
          <a-input-password v-model:value="pw.confirm" />
        </a-form-item>
        <a-form-item>
          <a-button type="primary" html-type="submit" :loading="pwSaving">{{ $t('profile.change_password') }}</a-button>
        </a-form-item>
      </a-form>
    </a-card>
  </div>
</template>

<script setup lang="ts">
import { inject, reactive, ref, onMounted } from 'vue'
import { message } from 'ant-design-vue'
import { useI18n } from '@/hooks/useI18n'
import { useAuthStore } from '@/stores/auth'
import { isBizError } from '@/utils/http-error'
import { formDiff } from '@/utils/form-diff'
import PageHeader from '@/components/PageHeader.vue'
import type { MeApi } from '@/api/me'

const { t } = useI18n()
const meApi = inject<MeApi>('meApi')!
const auth = useAuthStore()

const profile = reactive({
  display_name: auth.user?.display_name ?? '',
  email: auth.user?.email ?? '',
  avatar_url: auth.user?.avatar_url ?? ''
})

const initialProfile = ref({
  display_name: auth.user?.display_name ?? '',
  email: auth.user?.email ?? '',
  avatar_url: auth.user?.avatar_url ?? ''
})

const pw = reactive({ old_password: '', new_password: '', confirm: '' })
const profileSaving = ref(false)
const pwSaving = ref(false)

onMounted(async () => {
  if (!auth.user) {
    try {
      const me = await meApi.get()
      auth.setUser(me)
      profile.display_name = me.display_name
      profile.email = me.email
      profile.avatar_url = me.avatar_url
      initialProfile.value = {
        display_name: me.display_name,
        email: me.email,
        avatar_url: me.avatar_url
      }
    } catch { /* guard already redirects on 401 */ }
  }
})

async function onSaveProfile() {
  const patch = formDiff(initialProfile.value, {
    display_name: profile.display_name,
    email: profile.email,
    avatar_url: profile.avatar_url
  })
  if (Object.keys(patch).length === 0) {
    message.info(t('profile.update_ok'))
    return
  }
  profileSaving.value = true
  try {
    const updated = await meApi.update(patch)
    auth.setUser(updated)
    initialProfile.value = {
      display_name: updated.display_name,
      email: updated.email,
      avatar_url: updated.avatar_url
    }
    message.success(t('profile.update_ok'))
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    profileSaving.value = false
  }
}

async function validateConfirm(_: unknown, value: string) {
  if (value !== pw.new_password) {
    return Promise.reject(t('profile.password_mismatch'))
  }
  return Promise.resolve()
}

async function onChangePassword() {
  pwSaving.value = true
  try {
    await meApi.changePassword({ old_password: pw.old_password, new_password: pw.new_password })
    pw.old_password = ''
    pw.new_password = ''
    pw.confirm = ''
    message.success(t('profile.password_changed'))
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    pwSaving.value = false
  }
}
</script>

<style scoped lang="scss">
.profile-page {
  max-width: 640px;
  margin: 0 auto;
}
</style>
