import {createHash} from 'node:crypto';

export interface ModelDeployment {deployment_id:string;revision:number;deployment_sha256:string;protocol:string;model:string;endpoint_origin:string;endpoint_config_sha256:string;context_window:number}
export interface ModelParameters {max_steps:number;max_output_tokens:number}
export interface ModelBudget {token_limit?:number;cost_limit_micros?:number;currency?:string}
export interface ModelRate {id:string;version:number;currency:string;input_rate_micros_per_million:number;output_rate_micros_per_million:number}
export interface ModelBinding {protocol:string;deployment:ModelDeployment;parameters:ModelParameters;parameters_sha256:string;binding_sha256:string;input_token_reservation:number;output_token_reservation:number;budget?:ModelBudget;rate?:ModelRate}
export interface LocalModelExecution {binding:ModelBinding;base_url:string;api_key:string}
export class ModelBindingError extends Error {readonly code='model_binding_invalid';constructor(){super('Native model execution does not match its approved local deployment and binding');}}
const fail=():never=>{throw new ModelBindingError();};
const object=(value:unknown):Record<string,unknown>=>value!==null&&typeof value==='object'&&!Array.isArray(value)?value as Record<string,unknown>:fail();
function fields(value:unknown,names:string[],optional:string[]=[]):Record<string,unknown>{const raw=object(value);if(names.some(name=>!Object.hasOwn(raw,name))||Object.keys(raw).some(name=>!names.includes(name)&&!optional.includes(name)))fail();return raw;}
const integer=(value:unknown,min=0,max=Number.MAX_SAFE_INTEGER):number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>=min&&value<=max?value:fail();
const text=(value:unknown,max=256):string=>typeof value==='string'&&value.length>0&&Buffer.byteLength(value)<=max&&value.trim()===value&&!/[\p{Cc}\p{Cs}]/u.test(value)?value:fail();
const sha=(value:unknown):string=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value)?value:fail();
const id=(value:unknown):string=>typeof value==='string'&&/^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$/.test(value)?value:fail();
// Match encoding/json's escaped string scalars and declared struct field order.
export const modelJSON=(value:unknown):string=>JSON.stringify(value).replace(/[<>&\u2028\u2029]/g,c=>'\\u'+c.charCodeAt(0).toString(16).padStart(4,'0'));
export const modelHash=(value:unknown):string=>createHash('sha256').update(typeof value==='string'?value:modelJSON(value),'utf8').digest('hex');

function deployment(value:unknown):ModelDeployment {
 const raw=fields(value,['deployment_id','revision','deployment_sha256','protocol','model','endpoint_origin','endpoint_config_sha256','context_window']);
 const result={deployment_id:id(raw.deployment_id),revision:integer(raw.revision,1),deployment_sha256:sha(raw.deployment_sha256),protocol:text(raw.protocol),model:text(raw.model),endpoint_origin:text(raw.endpoint_origin,2048),endpoint_config_sha256:sha(raw.endpoint_config_sha256),context_window:integer(raw.context_window,1,10000000)};
 const endpoint=new URL(result.endpoint_origin);if(result.protocol!=='openai-chat-completions-v1'||endpoint.origin!==result.endpoint_origin||!safeModelEndpoint(result.endpoint_origin)||modelHash({...result,deployment_sha256:''})!==result.deployment_sha256)fail();return result;
}
function budget(value:unknown):ModelBudget {
 const raw=fields(value,[],['token_limit','cost_limit_micros','currency']);const result:ModelBudget={};
 if(Object.hasOwn(raw,'token_limit'))result.token_limit=integer(raw.token_limit);
 if(Object.hasOwn(raw,'cost_limit_micros')){result.cost_limit_micros=integer(raw.cost_limit_micros);if(raw.currency!=='CNY'&&raw.currency!=='USD')return fail();result.currency=raw.currency;}
 else if(Object.hasOwn(raw,'currency'))fail();if(result.token_limit===undefined&&result.cost_limit_micros===undefined)fail();return result;
}
function rate(value:unknown):ModelRate {const raw=fields(value,['id','version','currency','input_rate_micros_per_million','output_rate_micros_per_million']);if(raw.currency!=='CNY'&&raw.currency!=='USD')return fail();return {id:id(raw.id),version:integer(raw.version,1),currency:raw.currency,input_rate_micros_per_million:integer(raw.input_rate_micros_per_million),output_rate_micros_per_million:integer(raw.output_rate_micros_per_million)};}
export function validateModelBinding(value:unknown):ModelBinding {
 const raw=fields(value,['protocol','deployment','parameters','parameters_sha256','binding_sha256','input_token_reservation','output_token_reservation'],['budget','rate']);const p=fields(raw.parameters,['max_steps','max_output_tokens']);
 const result:ModelBinding={protocol:text(raw.protocol),deployment:deployment(raw.deployment),parameters:{max_steps:integer(p.max_steps,1,128),max_output_tokens:integer(p.max_output_tokens,1,131072)},parameters_sha256:sha(raw.parameters_sha256),binding_sha256:sha(raw.binding_sha256),input_token_reservation:integer(raw.input_token_reservation,1),output_token_reservation:integer(raw.output_token_reservation,1),...(Object.hasOwn(raw,'budget')?{budget:budget(raw.budget)}:{}),...(Object.hasOwn(raw,'rate')?{rate:rate(raw.rate)}:{})};
 if(result.protocol!=='suma-model-execution-v1'||result.parameters_sha256!==modelHash(result.parameters)||result.binding_sha256!==modelHash({...result,binding_sha256:''})||result.input_token_reservation!==result.deployment.context_window*result.parameters.max_steps||result.output_token_reservation!==result.parameters.max_output_tokens||!Number.isSafeInteger(result.input_token_reservation+result.output_token_reservation))fail();
 if(result.budget?.token_limit!==undefined&&result.input_token_reservation+result.output_token_reservation>result.budget.token_limit)fail();
 if(result.budget?.cost_limit_micros!==undefined){if(!result.rate||result.rate.currency!==result.budget.currency)return fail();const numerator=BigInt(result.input_token_reservation)*BigInt(result.rate.input_rate_micros_per_million)+BigInt(result.output_token_reservation)*BigInt(result.rate.output_rate_micros_per_million);if((numerator+999999n)/1000000n>BigInt(result.budget.cost_limit_micros))fail();}
 return result;
}
export function safeModelEndpoint(value:string):string {
 const endpoint=new URL(value);if(endpoint.username||endpoint.password||endpoint.search||endpoint.hash||value.trim()!==value||endpoint.protocol!=='https:'&&!(endpoint.protocol==='http:'&&['localhost','127.0.0.1','[::1]'].includes(endpoint.hostname)))fail();return endpoint.href.replace(/\/+$/,'');
}
export function resolveModelExecution(requestBinding:unknown,local:unknown):LocalModelExecution|undefined {
 if(requestBinding===undefined&&local===undefined)return undefined;
 try{const raw=fields(local,['binding','base_url','api_key']);const selected=validateModelBinding(requestBinding);const approved=validateModelBinding(raw.binding);const endpoint=safeModelEndpoint(text(raw.base_url,4096));const apiKey=text(raw.api_key,16384);if(modelJSON(selected)!==modelJSON(approved)||endpoint!==raw.base_url||new URL(endpoint).origin!==selected.deployment.endpoint_origin||modelHash(endpoint)!==selected.deployment.endpoint_config_sha256)fail();return {binding:selected,base_url:endpoint,api_key:apiKey};}catch{fail();}
}
export const modelExecutionCapability=()=>({protocol:'suma-model-execution-v1',provider_protocol:'openai-chat-completions-v1',parameters:['max_steps','max_output_tokens']});
