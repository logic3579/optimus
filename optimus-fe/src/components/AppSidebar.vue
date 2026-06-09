<template>
  <div class="app-sidebar">
    <div class="logo">{{ collapsed ? 'O' : 'Optimus' }}</div>
    <a-menu
      :selected-keys="[currentKey]"
      :open-keys="openKeys"
      mode="inline"
      theme="dark"
      :items="items"
      @click="onClick"
      @open-change="onOpenChange"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useMenuStore } from '@/stores/menu'
import { useI18n } from '@/hooks/useI18n'
import type { MeMenuNode } from '@/types/api'
import type { ItemType } from 'ant-design-vue'

defineProps<{ collapsed: boolean }>()
const menu = useMenuStore()
const route = useRoute()
const router = useRouter()
const { t } = useI18n()

function buildItems(nodes: MeMenuNode[]): ItemType[] {
  return nodes.map(n => {
    const base = { key: n.code, label: t(n.name) }
    if (n.children?.length) return { ...base, children: buildItems(n.children) } as ItemType
    return base as ItemType
  })
}

const items = computed(() => buildItems(menu.tree))

const codeByPath = computed(() => {
  const map = new Map<string, string>()
  const walk = (ns: MeMenuNode[]) => {
    for (const n of ns) {
      if (n.path) map.set(n.path, n.code)
      if (n.children?.length) walk(n.children)
    }
  }
  walk(menu.tree)
  return map
})

const currentKey = computed(() => codeByPath.value.get(route.path) ?? '')
const openKeys = ref<string[]>([])

watch(
  () => menu.tree,
  ts => {
    // open every group by default
    openKeys.value = ts.filter(n => n.children?.length).map(n => n.code)
  },
  { immediate: true }
)

function onClick({ key }: { key: string }) {
  const node = findNode(menu.tree, key)
  if (node?.path) router.push(node.path)
}

function onOpenChange(keys: (string | number)[]) {
  openKeys.value = keys.map(k => String(k))
}

function findNode(ns: MeMenuNode[], code: string): MeMenuNode | undefined {
  for (const n of ns) {
    if (n.code === code) return n
    const found = n.children ? findNode(n.children, code) : undefined
    if (found) return found
  }
}
</script>

<style scoped lang="scss">
.app-sidebar {
  height: 100%;
  background: #001529;
  display: flex;
  flex-direction: column;
}
.logo {
  height: 48px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-weight: 600;
  letter-spacing: 1px;
}
</style>
