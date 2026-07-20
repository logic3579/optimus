import type { AxiosInstance } from 'axios'
import type { Envelope, PageResp } from '@/types/api'

export type HTTPAuthType = 'basic' | 'bearer'
export interface HTTPCredentialSummary {
  id: number
  name: string
  auth_type: HTTPAuthType
  username?: string
  created_at: string
  updated_at: string
}
export interface HTTPCredentialCreateRequest {
  name: string
  auth_type: HTTPAuthType
  username?: string
  secret: string
}
export interface HTTPCredentialUpdateRequest {
  name?: string
  username?: string
  secret?: string
}
export interface HTTPCredentialListParams {
  page: number
  page_size: number
  q?: string
  auth_type?: HTTPAuthType
}

export function makeHTTPCredentialApi(client: AxiosInstance) {
  const base = '/credentials/http-credentials'
  return {
    list: async (params: HTTPCredentialListParams) =>
      (await client.get<Envelope<PageResp<HTTPCredentialSummary>>>(base, { params })).data.data,
    get: async (id: number) =>
      (await client.get<Envelope<HTTPCredentialSummary>>(`${base}/${id}`)).data.data,
    create: async (body: HTTPCredentialCreateRequest) =>
      (await client.post<Envelope<HTTPCredentialSummary>>(base, body)).data.data,
    update: async (id: number, body: HTTPCredentialUpdateRequest) =>
      (await client.put<Envelope<HTTPCredentialSummary>>(`${base}/${id}`, body)).data.data,
    remove: async (id: number) => { await client.delete<Envelope<null>>(`${base}/${id}`) },
  }
}

export type HTTPCredentialApi = ReturnType<typeof makeHTTPCredentialApi>
