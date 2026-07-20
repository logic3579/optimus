export interface Page<T>{items:T[];total:number;page:number;page_size:number}
export interface NamedRef{id:number;name:string}
export interface DatasourceSummary{id:number;name:string;base_url:string;auth_type:'none'|'basic'|'bearer';http_credential?:NamedRef;cluster?:NamedRef;tls_skip_verify:boolean;has_custom_ca:boolean;description:string;created_at:string;updated_at:string}
export type DatasourceDetail=DatasourceSummary
export interface SaveDatasource{name:string;base_url:string;auth_type:'none'|'basic'|'bearer';http_credential_id?:number;cluster_id?:number;tls_skip_verify:boolean;custom_ca_pem?:string;description:string;clear_http_credential?:boolean;clear_cluster?:boolean;clear_custom_ca?:boolean}
export interface Query{ref_id:string;promql:string}
export interface InstantBatch{datasource_id:number;time?:string;enrich_assets:boolean;queries:Query[]}
export interface RangeBatch{datasource_id:number;start:string;end:string;step:string;enrich_assets:boolean;queries:Query[]}
export interface Sample{timestamp:number;value:string}
export interface Series{labels:Record<string,string>;samples:Sample[]}
export interface NormalizedResult{result_type:'vector'|'matrix'|'scalar'|'string';series:Series[];scalar?:Sample;string?:Sample;warnings?:string[]}
export interface ItemError{code:number;message:string;message_key?:string}
export interface QueryItemResult{ref_id:string;result?:NormalizedResult;error?:ItemError}
export interface BatchResult{results:QueryItemResult[]}
export interface PanelInput{datasource_id:number;title:string;panel_type:'time_series'|'stat'|'table';promql:string;unit:string;legend:string;sort_order:number;width:6|12}
export interface Panel extends PanelInput{id:number;dashboard_id:number;created_at:string;updated_at:string}
export interface SaveDashboard{name:string;description:string;refresh_interval_s:number;time_range:'15m'|'1h'|'6h'|'24h'|'7d';panels:PanelInput[]}
export interface Dashboard extends Omit<SaveDashboard,'panels'>{id:number;created_by_user_id?:number;created_at:string;updated_at:string;panels:Panel[]}
export interface BuiltinVariable{name:string;label:string;required:boolean}
export interface BuiltinPanel{ref_id:string;title_key:string;panel_type:'time_series'|'stat'|'table';promql:string;unit:string;sort_order:number;width:6|12}
export interface BuiltinDashboard{code:string;title_key:string;variables:BuiltinVariable[];panels:BuiltinPanel[]}
