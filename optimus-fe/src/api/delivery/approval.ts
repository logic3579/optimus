import type{AxiosInstance}from'axios'
import type{Envelope}from'@/types/api'
import type{DeliveryApproval,PendingDeliveryApproval}from'@/types/delivery'
export function makeDeliveryApprovalApi(client:AxiosInstance){const decide=async(stageId:number,decision:'approve'|'reject',comment:string)=>(await client.post<Envelope<DeliveryApproval>>(`/delivery/run-stages/${stageId}/${decision}`,{comment})).data.data;return{
  listPending:async()=>(await client.get<Envelope<PendingDeliveryApproval[]>>('/delivery/approvals/pending')).data.data,
  approve:(stageId:number,comment:string)=>decide(stageId,'approve',comment),
  reject:(stageId:number,comment:string)=>decide(stageId,'reject',comment),
}}
export type DeliveryApprovalApi=ReturnType<typeof makeDeliveryApprovalApi>
