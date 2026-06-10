import type { ColumnType } from 'ant-design-vue/es/table'
import dayjs from 'dayjs'
import relativeTime from 'dayjs/plugin/relativeTime'

dayjs.extend(relativeTime)

/**
 * Per-kind antd-vue column descriptors for the workloads page.
 *
 * The `record` row data shape matches the matching `*Summary` types in
 * `@/types/api`. Column fields stay snake_case to mirror BE JSON without
 * a transformation layer.
 *
 * `age` is rendered as a relative-time string ("3d", "5h") via dayjs. The
 * BE returns ISO-8601 timestamps; falsy values render as "-" so empty cells
 * don't show the literal "Invalid Date".
 */
const ageColumn: ColumnType = {
  title: 'Age',
  dataIndex: 'age',
  key: 'age',
  width: 100,
  customRender: ({ text }: { text: string | undefined }) =>
    text ? dayjs(text).fromNow(true) : '-',
}

const baseHead: ColumnType[] = [
  { title: 'Name', dataIndex: 'name', key: 'name', ellipsis: true },
  { title: 'Namespace', dataIndex: 'namespace', key: 'namespace', width: 160 },
]

export const deploymentColumns: ColumnType[] = [
  ...baseHead,
  {
    title: 'Ready',
    key: 'ready',
    width: 100,
    customRender: ({ record }: { record: { replicas_ready: number; replicas_desired: number } }) =>
      `${record.replicas_ready}/${record.replicas_desired}`,
  },
  { title: 'Updated', dataIndex: 'replicas_updated', key: 'replicas_updated', width: 100 },
  { title: 'Available', dataIndex: 'replicas_available', key: 'replicas_available', width: 100 },
  { title: 'Strategy', dataIndex: 'strategy', key: 'strategy', width: 140 },
  ageColumn,
]

export const statefulsetColumns: ColumnType[] = [
  ...baseHead,
  {
    title: 'Ready',
    key: 'ready',
    width: 100,
    customRender: ({ record }: { record: { ready_replicas: number; replicas: number } }) =>
      `${record.ready_replicas}/${record.replicas}`,
  },
  { title: 'Service', dataIndex: 'service_name', key: 'service_name', width: 200 },
  ageColumn,
]

export const daemonsetColumns: ColumnType[] = [
  ...baseHead,
  { title: 'Desired', dataIndex: 'desired_number', key: 'desired_number', width: 90 },
  { title: 'Current', dataIndex: 'current_number', key: 'current_number', width: 90 },
  { title: 'Ready', dataIndex: 'ready_number', key: 'ready_number', width: 90 },
  { title: 'Available', dataIndex: 'available_number', key: 'available_number', width: 100 },
  { title: 'Misscheduled', dataIndex: 'misscheduled', key: 'misscheduled', width: 120 },
  ageColumn,
]

export const jobColumns: ColumnType[] = [
  ...baseHead,
  { title: 'Completions', dataIndex: 'completions', key: 'completions', width: 110 },
  { title: 'Succeeded', dataIndex: 'succeeded', key: 'succeeded', width: 100 },
  { title: 'Failed', dataIndex: 'failed', key: 'failed', width: 90 },
  {
    title: 'Start',
    dataIndex: 'start_time',
    key: 'start_time',
    width: 110,
    customRender: ({ text }: { text?: string | null }) =>
      text ? dayjs(text).fromNow(true) : '-',
  },
  ageColumn,
]

export const cronjobColumns: ColumnType[] = [
  ...baseHead,
  { title: 'Schedule', dataIndex: 'schedule', key: 'schedule', width: 160 },
  {
    title: 'Last schedule',
    dataIndex: 'last_schedule_time',
    key: 'last_schedule_time',
    width: 130,
    customRender: ({ text }: { text?: string | null }) =>
      text ? dayjs(text).fromNow(true) : '-',
  },
  { title: 'Active', dataIndex: 'active_jobs', key: 'active_jobs', width: 90 },
  {
    title: 'Suspended',
    dataIndex: 'suspended',
    key: 'suspended',
    width: 110,
    customRender: ({ text }: { text: boolean }) => (text ? 'Yes' : 'No'),
  },
  ageColumn,
]

export const replicasetColumns: ColumnType[] = [
  ...baseHead,
  {
    title: 'Ready',
    key: 'ready',
    width: 100,
    customRender: ({ record }: { record: { ready_replicas: number; replicas: number } }) =>
      `${record.ready_replicas}/${record.replicas}`,
  },
  {
    title: 'Owner',
    key: 'owner',
    ellipsis: true,
    customRender: ({ record }: { record: { owner_kind?: string; owner_name?: string } }) => {
      if (!record.owner_kind && !record.owner_name) return '-'
      return `${record.owner_kind ?? '?'}/${record.owner_name ?? '?'}`
    },
  },
  ageColumn,
]

export const podColumns: ColumnType[] = [
  ...baseHead,
  { title: 'Phase', dataIndex: 'phase', key: 'phase', width: 110 },
  {
    title: 'Ready',
    key: 'ready',
    width: 90,
    customRender: ({ record }: { record: { ready_containers: number; total_containers: number } }) =>
      `${record.ready_containers}/${record.total_containers}`,
  },
  { title: 'Restarts', dataIndex: 'restart_count', key: 'restart_count', width: 100 },
  { title: 'Node', dataIndex: 'node_name', key: 'node_name', ellipsis: true },
  { title: 'IP', dataIndex: 'pod_ip', key: 'pod_ip', width: 140 },
  ageColumn,
]

export const columnsByKind: Record<string, ColumnType[]> = {
  deployments: deploymentColumns,
  statefulsets: statefulsetColumns,
  daemonsets: daemonsetColumns,
  jobs: jobColumns,
  cronjobs: cronjobColumns,
  replicasets: replicasetColumns,
  pods: podColumns,
}
