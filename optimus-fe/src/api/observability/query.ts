import type{AxiosInstance}from'axios';import type{Envelope}from'@/types/api';import type{BatchResult,InstantBatch,RangeBatch}from'@/types/observability'
export function makeObservabilityQueryApi(c:AxiosInstance){return{instant:async(v:InstantBatch,signal?:AbortSignal)=>(await c.post<Envelope<BatchResult>>('/observability/query',v,{signal})).data.data,range:async(v:RangeBatch,signal?:AbortSignal)=>(await c.post<Envelope<BatchResult>>('/observability/query-range',v,{signal})).data.data}}
export type ObservabilityQueryApi=ReturnType<typeof makeObservabilityQueryApi>
