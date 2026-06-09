import { ref, type Ref } from 'vue'

export interface PageRequest {
  page: number
  pageSize: number
}
export interface PageResult<T> {
  items: T[]
  total: number
}

export interface UseTableOptions<T, F = Record<string, unknown>> {
  fetcher: (req: PageRequest & { filters?: F }) => Promise<PageResult<T>>
  defaultPageSize?: number
  defaultFilters?: F
}

export function useTable<T, F = Record<string, unknown>>(opts: UseTableOptions<T, F>) {
  const page = ref(1)
  const pageSize = ref(opts.defaultPageSize ?? 20)
  const items = ref<T[]>([]) as Ref<T[]>
  const total = ref(0)
  const loading = ref(false)
  const filters = ref<F>(opts.defaultFilters ?? ({} as F)) as Ref<F>

  async function reload() {
    loading.value = true
    try {
      const r = await opts.fetcher({
        page: page.value,
        pageSize: pageSize.value,
        filters: filters.value
      })
      items.value = r.items
      total.value = r.total
    } finally {
      loading.value = false
    }
  }

  async function setPage(p: number) {
    page.value = p
    await reload()
  }

  async function setPageSize(s: number) {
    pageSize.value = s
    page.value = 1
    await reload()
  }

  async function setFilters(patch: Partial<F>) {
    filters.value = { ...filters.value, ...patch } as F
    page.value = 1
    await reload()
  }

  return { page, pageSize, items, total, loading, filters, reload, setPage, setPageSize, setFilters }
}
