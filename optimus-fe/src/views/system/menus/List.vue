<template>
  <a-card>
    <PageHeader :title="$t('system.menus.title')" />

    <div class="filter-row u-mb-16">
      <a-button v-permission="'system:menu:write'" type="primary" @click="openCreateRoot">
        {{ $t('system.menus.create_root') }}
      </a-button>
    </div>

    <a-spin :spinning="loading">
      <a-tree
        v-if="tree.length"
        :tree-data="treeData"
        :default-expand-all="true"
        :draggable="true"
        :field-names="{ children: 'children', title: 'code', key: 'id' }"
        @drop="onDrop"
      >
        <template #title="node">
          <span class="menu-row">
            <span class="menu-code">{{ node.code }}</span>
            <code v-if="node.path" class="menu-path">{{ node.path }}</code>
            <span class="menu-name">{{ $t(node.name) }}</span>
            <span class="menu-actions">
              <a v-permission="'system:menu:write'" @click.stop="openEdit(node as MenuNode)">{{ $t('system.menus.action_edit') }}</a>
              <a v-permission="'system:menu:write'" @click.stop="openCreateChild(node as MenuNode)">{{ $t('system.menus.action_add_child') }}</a>
              <a-popconfirm
                :title="$t('confirm.delete_title')"
                :description="$t('confirm.delete_desc')"
                @confirm="remove(node as MenuNode)"
              >
                <a v-permission="'system:menu:delete'" @click.stop>{{ $t('system.menus.action_delete') }}</a>
              </a-popconfirm>
            </span>
          </span>
        </template>
      </a-tree>
      <a-empty v-else />
    </a-spin>

    <MenuEditModal
      v-model:open="editOpen"
      :initial="editing"
      :parent-id="parentForCreate"
      :tree="tree"
      @saved="reload"
    />
  </a-card>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import { useI18n } from '@/hooks/useI18n'
import { isBizError } from '@/utils/http-error'
import PageHeader from '@/components/PageHeader.vue'
import MenuEditModal from './components/MenuEditModal.vue'
import { computeDropTarget } from './computeDropTarget'
import type { MenuApi } from '@/api/menu'
import type { MenuNode } from '@/types/api'

const { t } = useI18n()
const menuApi = inject<MenuApi>('menuApi')!

const tree = ref<MenuNode[]>([])
const loading = ref(false)
const editOpen = ref(false)
const editing = ref<MenuNode | null>(null)
const parentForCreate = ref<number | null>(null)

const treeData = computed(() => tree.value)

async function reload() {
  loading.value = true
  try {
    tree.value = await menuApi.list()
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    loading.value = false
  }
}

function openCreateRoot() {
  editing.value = null
  parentForCreate.value = null
  editOpen.value = true
}
function openCreateChild(parent: MenuNode) {
  editing.value = null
  parentForCreate.value = parent.id
  editOpen.value = true
}
function openEdit(node: MenuNode) {
  editing.value = node
  parentForCreate.value = null
  editOpen.value = true
}

async function remove(node: MenuNode) {
  try {
    await menuApi.remove(node.id)
    message.success(t('system.menus.delete_ok'))
    await reload()
  } catch (e) {
    message.error(isBizError(e) ? e.message : t('network.error'))
  }
}

interface AntdvDropEvent {
  dragNode: { dataRef?: MenuNode; key: number }
  node: { dataRef?: MenuNode; key: number }
  dropPosition: number
  dropToGap: boolean
}

async function onDrop(info: AntdvDropEvent) {
  const dragId = info.dragNode.dataRef?.id ?? info.dragNode.key
  const dropId = info.node.dataRef?.id ?? info.node.key
  try {
    const target = computeDropTarget(tree.value, dragId, dropId, info.dropPosition, info.dropToGap)
    await menuApi.update(dragId, target)
    message.success(t('system.menus.drop_ok'))
    await reload()
  } catch (e) {
    if (e instanceof Error && e.message.toLowerCase().includes('descendant')) {
      message.warning(t('system.menus.drop_invalid'))
      return
    }
    message.error(isBizError(e) ? e.message : t('network.error'))
  }
}

onMounted(reload)
</script>

<style scoped lang="scss">
.filter-row {
  display: flex;
  gap: 12px;
}
.menu-row {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}
.menu-code { font-weight: 500; }
.menu-path { color: #888; }
.menu-name { color: #555; }
.menu-actions {
  margin-left: 16px;
  display: inline-flex;
  gap: 8px;
}
</style>
