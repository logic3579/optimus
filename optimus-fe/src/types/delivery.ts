import type { PageResp } from '@/types/api'

export type RunState = 'queued' | 'running' | 'waiting_approval' | 'cancel_requested' | 'reconciling' | 'succeeded' | 'failed' | 'rejected' | 'canceled' | 'timed_out' | 'outcome_unknown'
export type StageState = 'pending' | 'waiting_approval' | 'queued' | 'running' | 'reconciling' | 'succeeded' | 'failed' | 'rejected' | 'canceled' | 'timed_out' | 'outcome_unknown'
export type ApprovalDecision = 'approved' | 'rejected'

export interface DeliveryProjectSummary { id:number;name:string;description:string;owner_user_id?:number;environment_count:number;created_at:string;updated_at:string }
export interface DeliveryEnvironment { id:number;project_id:number;environment_key:string;display_name:string;application_id:number;application_name:string;chart_repo_id:number;chart_name:string;installed:boolean;cluster_id:number;namespace:string;release_name:string;created_at:string;updated_at:string }
export interface DeliveryProjectDetail extends DeliveryProjectSummary { environments:DeliveryEnvironment[] }
export type DeliveryProjectPage = PageResp<DeliveryProjectSummary>
export interface DeliveryProjectInput { name:string;description:string;owner_user_id?:number }
export interface DeliveryProjectUpdate { name?:string;description?:string;owner_user_id?:number }
export interface DeliveryEnvironmentInput { environment_key:string;display_name:string;application_id:number }
export interface DeliveryEnvironmentUpdate { environment_key?:string;display_name?:string }

export interface DeliveryPipelineStageInput { environment_id:number;approval_required:boolean;timeout:string }
export interface DeliveryPipelineStage extends DeliveryPipelineStageInput { id:number;order:number }
export interface DeliveryPipeline { id:number;project_id:number;version:number;created_by_user_id:number;published_at:string;is_current:boolean;stages:DeliveryPipelineStage[] }
export interface DeliveryArtifactVersion { chart_repo_id:number;chart_name:string;version:string }

export interface DeliveryStage { id:number;environment_id:number;environment_key:string;environment_name:string;application_id:number;cluster_id:number;namespace:string;release_name:string;order:number;executor:'helm_upgrade_existing_release';approval_required:boolean;timeout:string;state:StageState;result_revision?:number;result_digest?:string;started_at?:string;finished_at?:string;error_code?:number;error_message_key?:string;correlation_id?:string }
export interface DeliveryRun { id:number;project_id:number;pipeline_id:number;pipeline_version:number;chart_repo_id:number;chart_name:string;chart_version:string;chart_digest:string;initiator_user_id:number;state:RunState;retry_of_run_id?:number;started_at?:string;finished_at?:string;error_code?:number;error_message_key?:string;correlation_id?:string;created_at:string;updated_at:string;stages:DeliveryStage[] }
export type DeliveryRunPage = PageResp<DeliveryRun>
export interface DeliveryRunInput { chart_repo_id:number;chart_name:string;chart_version:string }

export interface PendingDeliveryApproval { id:number;run_id:number;run_stage_id:number;project_id:number;project_name:string;environment_key:string;environment_name:string;stage_order:number;chart_name:string;chart_version:string;chart_digest:string;initiator_user_id:number;requested_at:string }
export interface DeliveryApproval { id:number;run_id:number;run_stage_id:number;decision:ApprovalDecision;decided_by_user_id:number;comment:string;decided_at:string }

export interface DeliveryEvent {
  id: number
  run_id: number
  run_stage_id?: number
  event_type: string
  old_state?: string
  new_state?: string
  actor_type: 'user' | 'system'
  actor_id?: number
  occurred_at: string
  error_code?: number
  error_message_key?: string
  correlation_id?: string
  metadata: Record<string, unknown>
}
