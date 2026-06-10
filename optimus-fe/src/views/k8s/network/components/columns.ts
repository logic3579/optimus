import { h } from 'vue'
import { Tag } from 'ant-design-vue'
import type { ColumnType } from 'ant-design-vue/es/table'
import dayjs from 'dayjs'
import relativeTime from 'dayjs/plugin/relativeTime'
import type { ServicePort, ServiceSummary, IngressSummary } from '@/types/api'

dayjs.extend(relativeTime)

/**
 * Per-kind antd-vue column descriptors for the network page (Service + Ingress).
 *
 * Field names stay snake_case to mirror the BE JSON shape directly. Ports and
 * Hosts render as `<a-tag>` per entry so the list reads at a glance, while
 * LoadBalancerIPs uses a comma-joined fallback because per-IP tagging adds
 * little signal there.
 */
const ageColumn: ColumnType = {
  title: 'Age',
  dataIndex: 'age',
  key: 'age',
  width: 100,
  customRender: ({ text }: { text: string | undefined }) =>
    text ? dayjs(text).fromNow(true) : '-',
}

export const serviceColumns: ColumnType[] = [
  { title: 'Name', dataIndex: 'name', key: 'name', ellipsis: true },
  { title: 'Namespace', dataIndex: 'namespace', key: 'namespace', width: 160 },
  { title: 'Type', dataIndex: 'type', key: 'type', width: 130 },
  { title: 'ClusterIP', dataIndex: 'cluster_ip', key: 'cluster_ip', width: 160 },
  {
    title: 'Ports',
    key: 'ports',
    customRender: ({ record }: { record: ServiceSummary }) =>
      h(
        'span',
        null,
        (record.ports ?? []).map((p: ServicePort) =>
          h(
            Tag,
            { color: 'blue', key: `${p.port}/${p.protocol}` },
            () => `${p.port}/${p.protocol}`,
          ),
        ),
      ),
  },
  ageColumn,
]

export const ingressColumns: ColumnType[] = [
  { title: 'Name', dataIndex: 'name', key: 'name', ellipsis: true },
  { title: 'Namespace', dataIndex: 'namespace', key: 'namespace', width: 160 },
  { title: 'Class', dataIndex: 'ingress_class', key: 'ingress_class', width: 130 },
  {
    title: 'Hosts',
    key: 'hosts',
    customRender: ({ record }: { record: IngressSummary }) =>
      h(
        'span',
        null,
        (record.hosts ?? []).map((host: string) =>
          h(Tag, { color: 'geekblue', key: host }, () => host),
        ),
      ),
  },
  {
    title: 'LoadBalancerIPs',
    key: 'load_balancer_ips',
    customRender: ({ record }: { record: IngressSummary }) =>
      (record.load_balancer_ips ?? []).join(', ') || '-',
  },
  ageColumn,
]

export const columnsByKind: Record<'services' | 'ingresses', ColumnType[]> = {
  services: serviceColumns,
  ingresses: ingressColumns,
}
