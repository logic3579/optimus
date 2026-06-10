import type { ColumnType } from 'ant-design-vue/es/table'
import dayjs from 'dayjs'
import relativeTime from 'dayjs/plugin/relativeTime'

dayjs.extend(relativeTime)

/**
 * Per-kind antd-vue column descriptors for the config page (ConfigMap + Secret).
 *
 * The Secret columns are returned without an explicit "actions" column —
 * the page injects an actions column dynamically because the Reveal button
 * needs access to component-local state (modal open/close, message API).
 */
const ageColumn: ColumnType = {
  title: 'Age',
  dataIndex: 'age',
  key: 'age',
  width: 100,
  customRender: ({ text }: { text: string | undefined }) =>
    text ? dayjs(text).fromNow(true) : '-',
}

export const configmapColumns: ColumnType[] = [
  { title: 'Name', dataIndex: 'name', key: 'name', ellipsis: true },
  { title: 'Namespace', dataIndex: 'namespace', key: 'namespace', width: 160 },
  { title: 'DataCount', dataIndex: 'data_count', key: 'data_count', width: 120 },
  ageColumn,
]

export const secretBaseColumns: ColumnType[] = [
  { title: 'Name', dataIndex: 'name', key: 'name', ellipsis: true },
  { title: 'Namespace', dataIndex: 'namespace', key: 'namespace', width: 160 },
  { title: 'Type', dataIndex: 'type', key: 'type', width: 220 },
  { title: 'DataCount', dataIndex: 'data_count', key: 'data_count', width: 120 },
  ageColumn,
]
