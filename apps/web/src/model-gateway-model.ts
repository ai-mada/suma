import type {ModelBinding,ModelEvidence,ModelRequestEvidence,ModelRequestUsage} from '../../../packages/contracts/src';
import {validModelBinding} from './model-execution-model';
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const fields=(value:Record<string,unknown>,allowed:string[])=>Object.keys(value).every(key=>allowed.includes(key));
const hash=(value:unknown):value is string=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
const text=(value:unknown,max=256):value is string=>typeof value==='string'&&new TextEncoder().encode(value).length<=max&&!/[\u0000-\u001f\u007f-\u009f]/.test(value);
const count=(value:unknown):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>=0;
export const usageCounters=['input_tokens','output_tokens','cached_input_tokens','cache_write_input_tokens','reasoning_output_tokens'] as const;
function request(value:unknown):value is ModelRequestEvidence{
 if(!object(value)||!fields(value,['state','step','request_sha256','response_sha256','provider_model','provider_request_id','output_allowance','usage_state','usage'])||!['started','completed','rejected','unknown'].includes(String(value.state))||!count(value.step)||value.step>127||!hash(value.request_sha256)||value.response_sha256!==undefined&&!hash(value.response_sha256)||value.provider_model!==undefined&&!text(value.provider_model)||value.provider_request_id!==undefined&&!text(value.provider_request_id,256)||!count(value.output_allowance)||value.output_allowance<1||value.output_allowance>131072||!['complete','partial','unreported'].includes(String(value.usage_state)))return false;
 return value.usage===undefined||object(value.usage)&&fields(value.usage,[...usageCounters])&&Object.values(value.usage).every(count);
}
/** Only bounded Go gateway facts enter the view. SDK aggregate usage or a
 * process exit cannot synthesize missing host request states or counters. */
export function validGatewayEvidence(value:unknown):value is ModelEvidence{
 if(!object(value)||!fields(value,['gateway_protocol','gateway_closed','binding_sha256','deployment_sha256','parameters_sha256','model','requests'])||value.gateway_protocol!=='suma-model-gateway-v1'||typeof value.gateway_closed!=='boolean'||!hash(value.binding_sha256)||!hash(value.deployment_sha256)||!hash(value.parameters_sha256)||!text(value.model)||!value.model||!Array.isArray(value.requests)||value.requests.length>128||!value.requests.every(request))return false;
 return value.requests.every((item,index)=>item.step===index);
}
export type GatewayFacts={coverage:'complete'|'pending'|'mismatch'|'unrecorded';evidence?:ModelEvidence;usageState:'complete'|'partial'|'unreported';completed:number;rejected:number;pending:number;modelChanged:boolean;totals:Partial<Record<keyof ModelRequestUsage,bigint>>};
export function gatewayFacts(value:unknown,binding?:ModelBinding|null):GatewayFacts{
 const base:GatewayFacts={coverage:value===undefined||value===null?'unrecorded':'mismatch',usageState:'unreported',completed:0,rejected:0,pending:0,modelChanged:false,totals:{}};
 if(!validGatewayEvidence(value))return base;
 base.evidence=value;
 if(binding!=null&&(!validModelBinding(binding)||binding.binding_sha256!==value.binding_sha256||binding.deployment.deployment_sha256!==value.deployment_sha256||binding.parameters_sha256!==value.parameters_sha256||binding.deployment.model!==value.model))return base;
 const completed=value.requests.filter(item=>item.state==='completed');base.completed=completed.length;base.rejected=value.requests.filter(item=>item.state==='rejected').length;base.pending=value.requests.length-base.completed-base.rejected;
 base.coverage=value.gateway_closed&&base.pending===0&&completed.every(item=>hash(item.response_sha256))?'complete':'pending';
 base.modelChanged=completed.some(item=>!!item.provider_model&&item.provider_model!==value.model);
 const known=completed.some(item=>item.usage&&Object.keys(item.usage).length>0),full=base.rejected===0&&completed.every(item=>item.usage_state==='complete'&&count(item.usage?.input_tokens)&&count(item.usage?.output_tokens));
 base.usageState=base.coverage==='complete'&&full?'complete':known?'partial':'unreported';
 for(const key of usageCounters)if(completed.length>0&&completed.every(item=>count(item.usage?.[key])))base.totals[key]=completed.reduce((total,item)=>total+BigInt(item.usage![key]!),0n);
 return base;
}
