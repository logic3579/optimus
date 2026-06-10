import { h } from 'vue'
import { CheckCircleFilled, CloseCircleFilled } from '@ant-design/icons-vue'
import type { ColumnType } from 'ant-design-vue/es/table'
import dayjs from 'dayjs'
import relativeTime from 'dayjs/plugin/relativeTime'
import type { EventSummary, NamespaceSummary, NodeSummary } from '@/types/api'

dayjs.extend(relativeTime)

/**
 * Per-kind column descriptors for the cluster-resources page.
 *
 * Ready/Schedulable booleans render as filled antd icons rather than text
 * because most operators eyeball the column for green/red — the words "yes/no"
 * read identically and lose that affordance. Roles are joined with commas
 * because a node typically has 0-2 roles.
 */
const ageColumn: ColumnType = {
  title: 'Age',
  dataIndex: 'age',
  key: 'age',
  width: 100,
  customRender: ({ text }: { text: string | undefined }) =>
    text ? dayjs(text).fromNow(true) : '-',
}

export const namespaceColumns: ColumnType[] = [
  { title: 'Name', dataIndex: 'name', key: 'name', ellipsis: true },
  { title: 'Phase', dataIndex: 'phase', key: 'phase', width: 140 },
  ageColumn,
]

// Reference NamespaceSummary so the import is genuinely used (the column
// descriptors above pull the field names out of records typed by the caller,
// but exporting the alias keeps the consumer side clean).
export type NamespaceRow = NamespaceSummary

const boolIcon = (ok: boolean) =>
  h(ok ? CheckCircleFilled : CloseCircleFilled, {
    style: { color: ok ? '#52c41a' : '#ff4d4f' },
  })

export const nodeColumns: ColumnType[] = [
  { title: 'Name', dataIndex: 'name', key: 'name', ellipsis: true },
  {
    title: 'Ready',
    key: 'ready',
    width: 80,
    customRender: ({ record }: { record: NodeSummary }) => boolIcon(!!record.ready),
  },
  {
    title: 'Schedulable',
    key: 'schedulable',
    width: 110,
    customRender: ({ record }: { record: NodeSummary }) => boolIcon(!!record.schedulable),
  },
  {
    title: 'Roles',
    key: 'roles',
    customRender: ({ record }: { record: NodeSummary }) =>
      (record.roles ?? []).join(', ') || '-',
  },
  { title: 'KubeletVersion', dataIndex: 'kubelet_version', key: 'kubelet_version', width: 140 },
  { title: 'CPU', dataIndex: 'cpu_capacity', key: 'cpu_capacity', width: 90 },
  { title: 'Mem', dataIndex: 'mem_capacity', key: 'mem_capacity', width: 110 },
  ageColumn,
]

export const eventColumns: ColumnType[] = [
  { title: 'Type', dataIndex: 'type', key: 'type', width: 100 },
  { title: 'Reason', dataIndex: 'reason', key: 'reason', width: 160 },
  {
    title: 'Object',
    key: 'object',
    width: 240,
    customRender: ({ record }: { record: EventSummary }) =>
      `${record.involved_kind}/${record.involved_name}`,
  },
  { title: 'Message', dataIndex: 'message', key: 'message', ellipsis: true },
  { title: 'Count', dataIndex: 'count', key: 'count', width: 80 },
  {
    title: 'LastTimestamp',
    dataIndex: 'last_timestamp',
    key: 'last_timestamp',
    width: 120,
    customRender: ({ text }: { text: string | undefined }) =>
      text ? dayjs(text).fromNow(true) : '-',
  },
]
