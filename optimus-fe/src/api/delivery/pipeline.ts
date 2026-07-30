import type{AxiosInstance}from'axios'
import type{Envelope}from'@/types/api'
import type{DeliveryArtifactVersion,DeliveryPipeline,DeliveryPipelineStageInput,DeliveryResolvedArtifact,DeliveryRunInput}from'@/types/delivery'
export function makeDeliveryPipelineApi(client:AxiosInstance){const base=(id:number)=>`/delivery/projects/${id}`;return{
  get:async(id:number)=>(await client.get<Envelope<DeliveryPipeline>>(`${base(id)}/pipeline`)).data.data,
  publish:async(id:number,stages:DeliveryPipelineStageInput[])=>(await client.put<Envelope<DeliveryPipeline>>(`${base(id)}/pipeline`,{stages})).data.data,
  listArtifacts:async(id:number)=>(await client.get<Envelope<DeliveryArtifactVersion[]>>(`${base(id)}/artifacts`)).data.data,
  resolveArtifact:async(id:number,artifact:DeliveryRunInput)=>(await client.post<Envelope<DeliveryResolvedArtifact>>(`${base(id)}/artifacts/resolve`,artifact)).data.data,
}}
export type DeliveryPipelineApi=ReturnType<typeof makeDeliveryPipelineApi>
