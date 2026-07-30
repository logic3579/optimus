import type { AxiosInstance } from 'axios'
import type { Envelope } from '@/types/api'
import type { DeliveryEnvironment, DeliveryEnvironmentInput, DeliveryEnvironmentUpdate, DeliveryProjectDetail, DeliveryProjectInput, DeliveryProjectPage, DeliveryProjectUpdate } from '@/types/delivery'

export function makeDeliveryProjectApi(client:AxiosInstance){const base='/delivery/projects';return{
  list:async(params:{q?:string;page?:number;page_size?:number}={})=>(await client.get<Envelope<DeliveryProjectPage>>(base,{params})).data.data,
  get:async(id:number)=>(await client.get<Envelope<DeliveryProjectDetail>>(`${base}/${id}`)).data.data,
  create:async(body:DeliveryProjectInput)=>(await client.post<Envelope<DeliveryProjectDetail>>(base,body)).data.data,
  update:async(id:number,body:DeliveryProjectUpdate)=>(await client.put<Envelope<DeliveryProjectDetail>>(`${base}/${id}`,body)).data.data,
  remove:async(id:number)=>{await client.delete<Envelope<null>>(`${base}/${id}`)},
  listEnvironments:async(id:number)=>(await client.get<Envelope<DeliveryEnvironment[]>>(`${base}/${id}/environments`)).data.data,
  bindEnvironment:async(id:number,body:DeliveryEnvironmentInput)=>(await client.post<Envelope<DeliveryEnvironment>>(`${base}/${id}/environments`,body)).data.data,
  updateEnvironment:async(id:number,environmentId:number,body:DeliveryEnvironmentUpdate)=>(await client.put<Envelope<DeliveryEnvironment>>(`${base}/${id}/environments/${environmentId}`,body)).data.data,
  unbindEnvironment:async(id:number,environmentId:number)=>{await client.delete<Envelope<null>>(`${base}/${id}/environments/${environmentId}`)},
}}
export type DeliveryProjectApi=ReturnType<typeof makeDeliveryProjectApi>
