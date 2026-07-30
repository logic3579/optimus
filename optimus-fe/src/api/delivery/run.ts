import type{AxiosInstance}from'axios'
import type{Envelope}from'@/types/api'
import type{DeliveryRun,DeliveryRunInput,DeliveryRunPage}from'@/types/delivery'
const key=(value:string)=>({headers:{'Idempotency-Key':value}})
export function makeDeliveryRunApi(client:AxiosInstance){return{
  list:async(projectId:number,params:{page?:number;page_size?:number}={})=>(await client.get<Envelope<DeliveryRunPage>>(`/delivery/projects/${projectId}/runs`,{params})).data.data,
  get:async(id:number)=>(await client.get<Envelope<DeliveryRun>>(`/delivery/runs/${id}`)).data.data,
  create:async(projectId:number,body:DeliveryRunInput,idempotencyKey:string)=>(await client.post<Envelope<DeliveryRun>>(`/delivery/projects/${projectId}/runs`,body,key(idempotencyKey))).data.data,
  cancel:async(id:number)=>(await client.post<Envelope<DeliveryRun>>(`/delivery/runs/${id}/cancel`)).data.data,
  reconcile:async(id:number)=>(await client.post<Envelope<DeliveryRun>>(`/delivery/runs/${id}/reconcile`)).data.data,
  retry:async(id:number,idempotencyKey:string)=>(await client.post<Envelope<DeliveryRun>>(`/delivery/runs/${id}/retry`,undefined,key(idempotencyKey))).data.data,
}}
export type DeliveryRunApi=ReturnType<typeof makeDeliveryRunApi>
