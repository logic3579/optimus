<template>
  <div class="app-header">
    <a-button type="text" @click="$emit('toggle')">
      <MenuFoldOutlined v-if="!collapsed" />
      <MenuUnfoldOutlined v-else />
    </a-button>
    <div class="u-flex-1" />
    <ClusterPicker />
    <LangSwitch />
    <ThemeToggle />
    <a-dropdown>
      <a-button type="text">
        <UserOutlined />
        {{ auth.user?.display_name || auth.user?.username || 'user' }}
      </a-button>
      <template #overlay>
        <a-menu @click="onMenuClick">
          <a-menu-item key="profile">{{ $t('common.profile') }}</a-menu-item>
          <a-menu-divider />
          <a-menu-item key="logout">{{ $t('common.logout') }}</a-menu-item>
        </a-menu>
      </template>
    </a-dropdown>
  </div>
</template>

<script setup lang="ts">
import { MenuFoldOutlined, MenuUnfoldOutlined, UserOutlined } from '@ant-design/icons-vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useMenuStore } from '@/stores/menu'
import LangSwitch from './LangSwitch.vue'
import ThemeToggle from './ThemeToggle.vue'
import ClusterPicker from './layout/ClusterPicker.vue'

defineProps<{ collapsed: boolean }>()
defineEmits<{ toggle: [] }>()

const auth = useAuthStore()
const router = useRouter()

function onMenuClick({ key }: { key: string }) {
  if (key === 'profile') router.push('/profile')
  if (key === 'logout') {
    auth.reset()
    useMenuStore().reset()
    router.push('/login')
  }
}
</script>

<style scoped lang="scss">
.app-header {
  height: 48px;
  display: flex;
  align-items: center;
  gap: 8px;
}
.u-flex-1 {
  flex: 1;
}
</style>
