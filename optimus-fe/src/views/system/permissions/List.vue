<template>
  <a-card>
    <PageHeader :title="$t('system.permissions.title')" />

    <a-input
      v-model:value="filter"
      :placeholder="$t('system.permissions.filter_placeholder')"
      style="max-width: 360px; margin-bottom: 16px;"
      allow-clear
    />

    <a-collapse v-model:active-key="activeKeys" :bordered="false">
      <a-collapse-panel v-for="group in filteredGroups" :key="group.category" :header="$t(`perm.category.${group.category}`)">
        <a-list :data-source="group.items" size="small">
          <template #renderItem="{ item }">
            <a-list-item>
              <a-list-item-meta>
                <template #title>
                  <code>{{ item.code }}</code>
                  <span style="margin-left: 12px;">{{ $t(`perm.${item.code.replace(/:/g, '.')}`) }}</span>
                </template>
                <template #description>{{ item.description }}</template>
              </a-list-item-meta>
            </a-list-item>
          </template>
        </a-list>
      </a-collapse-panel>
    </a-collapse>
  </a-card>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import { useI18n } from '@/hooks/useI18n'
import { isBizError } from '@/utils/http-error'
import PageHeader from '@/components/PageHeader.vue'
import type { PermissionApi } from '@/api/permission'
import type { Permission } from '@/types/api'

const { t } = useI18n()
const permissionApi = inject<PermissionApi>('permissionApi')!

const all = ref<Permission[]>([])
const filter = ref('')
const activeKeys = ref<string[]>([])

interface PermGroup {
  category: string
  items: Permission[]
}

const filteredGroups = computed<PermGroup[]>(() => {
  const f = filter.value.trim().toLowerCase()
  const matched = f
    ? all.value.filter(p => p.code.toLowerCase().includes(f) || p.name.toLowerCase().includes(f) || p.description.toLowerCase().includes(f))
    : all.value
  const byCategory = new Map<string, Permission[]>()
  for (const p of matched) {
    const arr = byCategory.get(p.category) ?? []
    arr.push(p)
    byCategory.set(p.category, arr)
  }
  return Array.from(byCategory.entries())
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([category, items]) => ({ category, items: items.sort((a, b) => a.code.localeCompare(b.code)) }))
})

onMounted(async () => {
  try {
    all.value = await permissionApi.list()
    activeKeys.value = Array.from(new Set(all.value.map(p => p.category)))
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  }
})
</script>
