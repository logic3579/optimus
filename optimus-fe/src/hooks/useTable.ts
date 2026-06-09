import { ref } from 'vue'

export interface PageRequest {
  page: number
  pageSize: number
}
export interface PageResult<T> {
  items: T[]
  total: number
}

export interface UseTableOptions<T> {
  fetcher: (req: PageRequest) => Promise<PageResult<T>>
  defaultPageSize?: number
}

export function useTable<T>(opts: UseTableOptions<T>) {
  const page = ref(1)
  const pageSize = ref(opts.defaultPageSize ?? 20)
  const items = ref<T[]>([]) as { value: T[] }
  const total = ref(0)
  const loading = ref(false)

  async function reload() {
    loading.value = true
    try {
      const r = await opts.fetcher({ page: page.value, pageSize: pageSize.value })
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

  return { page, pageSize, items, total, loading, reload, setPage, setPageSize }
}
