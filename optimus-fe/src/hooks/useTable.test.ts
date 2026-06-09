import { describe, it, expect, vi } from 'vitest'
import { useTable } from './useTable'

describe('useTable', () => {
  it('starts with page 1, default pageSize, empty items, total 0', () => {
    const t = useTable<{ id: number }>({
      fetcher: vi.fn().mockResolvedValue({ items: [], total: 0 })
    })
    expect(t.page.value).toBe(1)
    expect(t.pageSize.value).toBe(20)
    expect(t.items.value).toEqual([])
    expect(t.total.value).toBe(0)
    expect(t.loading.value).toBe(false)
  })

  it('reload populates items and total', async () => {
    const fetcher = vi.fn().mockResolvedValue({ items: [{ id: 1 }, { id: 2 }], total: 2 })
    const t = useTable<{ id: number }>({ fetcher })
    await t.reload()
    expect(fetcher).toHaveBeenCalledWith({ page: 1, pageSize: 20 })
    expect(t.items.value).toEqual([{ id: 1 }, { id: 2 }])
    expect(t.total.value).toBe(2)
  })

  it('setPage triggers reload with the new page', async () => {
    const fetcher = vi.fn().mockResolvedValue({ items: [], total: 0 })
    const t = useTable<{ id: number }>({ fetcher })
    await t.setPage(3)
    expect(fetcher).toHaveBeenCalledWith({ page: 3, pageSize: 20 })
    expect(t.page.value).toBe(3)
  })

  it('setPageSize resets to page 1', async () => {
    const fetcher = vi.fn().mockResolvedValue({ items: [], total: 0 })
    const t = useTable<{ id: number }>({ fetcher })
    await t.setPage(5)
    await t.setPageSize(50)
    expect(t.page.value).toBe(1)
    expect(t.pageSize.value).toBe(50)
    expect(fetcher).toHaveBeenLastCalledWith({ page: 1, pageSize: 50 })
  })

  it('fetcher error sets loading false and re-throws', async () => {
    const err = new Error('boom')
    const fetcher = vi.fn().mockRejectedValue(err)
    const t = useTable<{ id: number }>({ fetcher })
    await expect(t.reload()).rejects.toBe(err)
    expect(t.loading.value).toBe(false)
  })
})
