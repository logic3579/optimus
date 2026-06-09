import type { AxiosInstance } from 'axios'
import type {
  Envelope,
  MenuNode, MenuCreateRequest, MenuUpdateRequest
} from '@/types/api'

export function makeMenuApi(client: AxiosInstance) {
  return {
    list: async () => {
      const r = await client.get<Envelope<MenuNode[]>>('/menus')
      return r.data.data
    },
    create: async (body: MenuCreateRequest) => {
      const r = await client.post<Envelope<MenuNode>>('/menus', body)
      return r.data.data
    },
    update: async (id: number, body: MenuUpdateRequest) => {
      const r = await client.put<Envelope<MenuNode>>(`/menus/${id}`, body)
      return r.data.data
    },
    remove: async (id: number) => {
      await client.delete<Envelope<null>>(`/menus/${id}`)
    }
  }
}

export type MenuApi = ReturnType<typeof makeMenuApi>
