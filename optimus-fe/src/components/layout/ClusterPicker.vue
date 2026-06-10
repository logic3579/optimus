<template>
  <a-dropdown v-if="visible" trigger="click" placement="bottomRight">
    <a-button :loading="loading">
      <span
        v-if="k8s.currentClusterId"
        :class="['dot', currentDotClass]"
      />
      <span class="picker-label">
        {{ k8s.currentClusterName || $t('k8s.cluster.no_cluster_selected') }}
      </span>
      <down-outlined />
    </a-button>
    <template #overlay>
      <a-menu>
        <template v-if="clusters.length > 0">
          <a-menu-item
            v-for="c in clusters"
            :key="c.id"
            @click="select(c)"
          >
            <span :class="['dot', dotClass(c.last_health_ok)]" />
            {{ c.name }}
          </a-menu-item>
        </template>
        <a-menu-item v-else key="__empty" disabled>
          {{ $t('k8s.cluster.no_clusters') }}
        </a-menu-item>
        <a-menu-divider />
        <a-menu-item key="__refresh" @click="refresh">
          <reload-outlined /> {{ $t('common.reset') }}
        </a-menu-item>
      </a-menu>
    </template>
  </a-dropdown>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { DownOutlined, ReloadOutlined } from '@ant-design/icons-vue'
import { useAuthStore } from '@/stores/auth'
import { useK8sStore } from '@/stores/k8s'
import type { ClusterApi } from '@/api/k8s/cluster'
import type { Cluster } from '@/types/api'

const auth = useAuthStore()
const k8s = useK8sStore()
const router = useRouter()
const clusterApi = inject<ClusterApi>('clusterApi')!

const visible = computed(() => auth.permissions.some(p => p.startsWith('k8s:')))
const clusters = ref<Cluster[]>([])
const loading = ref(false)

const currentDotClass = computed(() => {
  const c = clusters.value.find(x => x.id === k8s.currentClusterId)
  return dotClass(c?.last_health_ok ?? null)
})

function dotClass(ok: boolean | null | undefined): string {
  if (ok === true) return 'ok'
  if (ok === false) return 'fail'
  return 'unknown'
}

async function refresh() {
  if (!visible.value) return
  loading.value = true
  try {
    const res = await clusterApi.list({ page_size: 200 })
    clusters.value = res.items
    // If the persisted cluster id is still in the list, refresh its display
    // name (it might have been renamed since localStorage was last written).
    if (k8s.currentClusterId) {
      const match = clusters.value.find(c => c.id === k8s.currentClusterId)
      if (match && match.name !== k8s.currentClusterName) {
        k8s.setCluster(match.id, match.name)
      }
    }
  } catch {
    // Swallow — the picker is decorative on top of permission gating.
    // Errors will surface on the list page itself.
  } finally {
    loading.value = false
  }
}

function select(c: Cluster) {
  k8s.setCluster(c.id, c.name)
  const path = router.currentRoute.value.path
  // Re-enter k8s detail routes so list views refetch against the new cluster.
  // The clusters CRUD page itself is cluster-agnostic — skip it.
  if (path.startsWith('/k8s/') && path !== '/k8s/clusters') {
    router.replace({ path, query: { ...router.currentRoute.value.query, _r: Date.now().toString() } })
  }
}

onMounted(() => { if (visible.value) void refresh() })
</script>

<style scoped lang="scss">
.dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-right: 6px;
  vertical-align: middle;

  &.ok      { background: #52c41a; }
  &.fail    { background: #ff4d4f; }
  &.unknown { background: #d9d9d9; }
}
.picker-label {
  margin-right: 4px;
}
</style>
