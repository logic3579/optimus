<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useLogStream } from '@/api/k8s/log'

/**
 * Pod log viewer. Owns the controls (container/tail/follow/previous +
 * connect-disconnect button) and renders the streamed lines inside a plain
 * <pre>. Lines accumulate in `stream.lines` (a Ref<string[]>) and are
 * truncated to MAX_LINES to bound memory; virtual-scroll is a future
 * improvement once profiling indicates it's needed.
 */
const props = defineProps<{
  clusterId: number
  namespace: string
  pod: string
  containers: string[]
}>()

const MAX_LINES = 50_000
const container = ref<string>(props.containers[0] ?? '')
const tail = ref<number>(200)
const follow = ref<boolean>(true)
const previous = ref<boolean>(false)

const stream = useLogStream()
const text = computed(() => stream.lines.value.join('\n'))

const { t } = useI18n()
// The i18n-key checker scans for static $t(key) calls, so we map status →
// key explicitly. This also documents the full set of states the BE may
// emit (idle | connecting | open | closed | error). Keys referenced
// indirectly so the parity script treats them as used:
//   k8s.log.idle, k8s.log.connecting, k8s.log.open, k8s.log.closed, k8s.log.error
const statusLabel = computed<string>(() => {
  switch (stream.status.value) {
    case 'idle': return t(`k8s.log.idle`)
    case 'connecting': return t(`k8s.log.connecting`)
    case 'open': return t(`k8s.log.open`)
    case 'closed': return t(`k8s.log.closed`)
    case 'error': return t(`k8s.log.error`)
  }
  return ''
})

const isActive = computed(
  () => stream.status.value === 'open' || stream.status.value === 'connecting'
)
const isReconnect = computed(
  () => stream.status.value === 'closed' || stream.status.value === 'error'
)

function connect(): void {
  if (stream.lines.value.length > MAX_LINES) {
    stream.lines.value = stream.lines.value.slice(-Math.floor(MAX_LINES / 2))
  }
  void stream.open({
    clusterId: props.clusterId,
    namespace: props.namespace,
    pod: props.pod,
    container: container.value,
    follow: follow.value,
    tailLines: tail.value,
    previous: previous.value,
  })
}

// Cap accumulated lines to bound memory. We slice to half the cap so the cap
// hit doesn't fire on every appended line.
watch(
  () => stream.lines.value.length,
  (n) => {
    if (n > MAX_LINES) {
      stream.lines.value = stream.lines.value.slice(-Math.floor(MAX_LINES / 2))
    }
  }
)

// When the parent swaps the available containers (e.g. pod selection changes),
// keep the picker pointed at a valid value.
watch(
  () => props.containers,
  (list) => {
    if (!list.includes(container.value)) {
      container.value = list[0] ?? ''
    }
  },
  { deep: true }
)
</script>

<template>
  <div class="log-viewer">
    <div class="controls">
      <a-select v-model:value="container" style="width: 200px" :placeholder="$t('k8s.log.container')">
        <a-select-option v-for="c in containers" :key="c" :value="c">{{ c }}</a-select-option>
      </a-select>
      <a-input-number v-model:value="tail" :min="50" :max="50000" :placeholder="$t('k8s.log.tail_lines')" />
      <a-checkbox v-model:checked="follow">{{ $t('k8s.log.follow') }}</a-checkbox>
      <a-checkbox v-model:checked="previous">{{ $t('k8s.log.previous') }}</a-checkbox>
      <a-button v-if="!isActive" type="primary" @click="connect">
        {{ isReconnect ? $t('k8s.log.reconnect') : $t('k8s.log.connect') }}
      </a-button>
      <a-button v-else @click="stream.close">
        {{ $t('k8s.log.disconnect') }}
      </a-button>
      <a-tag
        :color="stream.status.value === 'open' ? 'green' : stream.status.value === 'error' ? 'red' : 'default'"
      >
        {{ statusLabel }}
      </a-tag>
    </div>
    <pre class="log-body">{{ text }}</pre>
  </div>
</template>

<style scoped>
.controls {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-bottom: 8px;
  flex-wrap: wrap;
}
.log-body {
  max-height: 60vh;
  overflow: auto;
  background: #0d1117;
  color: #e6edf3;
  padding: 12px;
  font-family: monospace;
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-all;
  margin: 0;
  border-radius: 4px;
}
</style>
