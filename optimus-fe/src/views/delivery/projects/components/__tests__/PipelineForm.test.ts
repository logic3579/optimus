import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAuthStore } from '@/stores/auth'
import { permissionDirective } from '@/directives/permission'
import type { DeliveryEnvironment, DeliveryPipeline } from '@/types/delivery'
import PipelineForm from '../PipelineForm.vue'

vi.mock('@/hooks/useI18n',()=>({useI18n:()=>({t:(key:string,args?:{version?:number})=>args?.version?`${key}:${args.version}`:key})}))
vi.mock('ant-design-vue',()=>({Modal:{confirm:vi.fn((options:{onOk():void})=>options.onOk())}}))
const environments:DeliveryEnvironment[]=[1,2].map(id=>({id,project_id:3,environment_key:`e${id}`,display_name:`Env ${id}`,application_id:id,application_name:`App ${id}`,chart_repo_id:1,chart_name:'app',installed:true,cluster_id:1,namespace:'ns',release_name:'app',created_at:'',updated_at:''}))
const current:DeliveryPipeline={id:5,project_id:3,version:2,created_by_user_id:1,published_at:'',is_current:true,stages:environments.map((e,index)=>({id:index+1,environment_id:e.id,order:index+1,approval_required:index===1,timeout:'10m'}))}
describe('PipelineForm',()=>{beforeEach(()=>{setActivePinia(createPinia());useAuthStore().setPermissions(['delivery:pipeline:write'])})
function mounted(){const publish=vi.fn().mockResolvedValue({...current,version:3});const wrapper=mount(PipelineForm,{props:{projectId:3,environments,current},global:{provide:{deliveryPipelineApi:{publish}},directives:{permission:permissionDirective},stubs:{'a-alert':true,'a-checkbox':true,'a-input':true,'a-button':{template:'<button><slot/></button>'}}}});return{wrapper,publish,vm:wrapper.vm as unknown as{stages:{environment_id:number;approval_required:boolean;timeout:string}[];error:string;nextVersion:number;move(i:number,d:number):void;publish():Promise<void>}}}
it('preserves order controls, approvals, and confirms the next immutable version',async()=>{const{vm,publish}=mounted();expect(vm.nextVersion).toBe(3);expect(vm.stages[1]?.approval_required).toBe(true);vm.move(1,-1);expect(vm.stages.map(x=>x.environment_id)).toEqual([2,1]);await vm.publish();expect(publish).toHaveBeenCalledWith(3,vm.stages)})
it('rejects empty and out-of-bound timeouts',async()=>{const{vm,publish}=mounted();vm.stages[0]!.timeout='30s';await vm.publish();expect(vm.error).toBe('delivery.pipeline.timeout_invalid');expect(publish).not.toHaveBeenCalled();vm.stages=[];await vm.publish();expect(vm.error).toBe('delivery.pipeline.empty')})})
