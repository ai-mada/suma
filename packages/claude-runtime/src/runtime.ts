import {query,type Options,type Query,type SDKMessage,type SDKUserMessage,type PermissionResult} from '@anthropic-ai/claude-agent-sdk/core';
import {randomUUID} from 'node:crypto';
import {RuntimeError,SDK_VERSION,CLI_VERSION,MAX_CALLBACKS,TOOL_NAMES,id,object,inputJSON,questions,sha256,parseRequest,same,permissionDigest,answerDigest,nativeAnswers,type RuntimeConfig,type ExecuteRequest,type Frame,type Emit,type HostRequest,type PermissionSource,type InputRequest} from './contract.js';
import {prepare,sdkOptions,verifyExecutable} from './config.js';

export type QueryFactory=(params:{prompt:string|AsyncIterable<SDKUserMessage>;options?:Options})=>Query;
export interface SDKResultFacts{result_subtype:string;result_is_error:boolean;turn_ended:boolean;tools_completed:boolean;callback_count:number;returned_count:number;pending_callbacks:number;callback_denial_count:number;native_permission_denial_count?:number}
type Usage={input_tokens?:number;cached_input_tokens?:number;cache_write_input_tokens?:number;output_tokens?:number};
type Pending={kind:'permission'|'input';source:PermissionSource|InputRequest;resolve:(value:PermissionResult)=>void;reject:(error:Error)=>void;abort:()=>void;signal:AbortSignal};
const counts=(v:unknown)=>typeof v==='number'&&Number.isSafeInteger(v)&&v>=0;
function usageOf(value:unknown,camel=false):Usage|undefined{if(!object(value))return;const out:Usage={};for(const [target,key]of Object.entries(camel?{input_tokens:'inputTokens',cached_input_tokens:'cacheReadInputTokens',cache_write_input_tokens:'cacheCreationInputTokens',output_tokens:'outputTokens'}:{input_tokens:'input_tokens',cached_input_tokens:'cache_read_input_tokens',cache_write_input_tokens:'cache_creation_input_tokens',output_tokens:'output_tokens'})){if(value[key]===undefined)continue;if(!counts(value[key]))throw new RuntimeError('usage_invalid');out[target as keyof Usage]=value[key] as number;}return Object.keys(out).length?out:undefined;}
function addUsage(left:Usage|undefined,right:Usage|undefined):Usage|undefined{if(!right)return left;if(!left)return{...right};const next={...left};for(const [key,value]of Object.entries(right)){const sum=(next[key as keyof Usage]||0)+value;if(!Number.isSafeInteger(sum))throw new RuntimeError('usage_invalid');next[key as keyof Usage]=sum;}return next;}
function resultUsage(message:Record<string,unknown>):Usage|undefined{if(message.modelUsage!==undefined){if(!object(message.modelUsage))throw new RuntimeError('usage_invalid');let total:Usage|undefined;for(const model of Object.values(message.modelUsage)){const current=usageOf(model,true);if(!current)throw new RuntimeError('usage_invalid');total=addUsage(total,current);}if(total)return total;}return usageOf(message.usage);}

/** Owns one SDK query. Native request IDs are retained, while platform IDs,
 * delivery intentions and durable single-writer evidence remain in Go. */
export class ClaudeSession{
 private readonly controller=new AbortController();private readonly pending=new Map<string,Pending>();private readonly seenNative=new Set<string>();private readonly seenTools=new Set<string>();private readonly activeTools=new Set<string>();private readonly seenUsage=new Set<string>();
 private sessionID='';private initialized=false;private queryInstance:Query|undefined;private done=false;private used=false;private cancelled=false;private stoppedInput:()=>void=()=>{};private fault:RuntimeError|undefined;
 private callbacks=0;private returned=0;private denied=0;private partial:Usage|undefined;
 private facts:SDKResultFacts={result_subtype:'',result_is_error:true,turn_ended:false,tools_completed:false,callback_count:0,returned_count:0,pending_callbacks:0,callback_denial_count:0};
 constructor(readonly config:RuntimeConfig,readonly request:ExecuteRequest,private readonly emit:Emit,private readonly queryFactory:QueryFactory=query,private readonly environment:NodeJS.ProcessEnv=process.env){if(request.configuration_sha256!==config.configuration_sha256)throw new RuntimeError('configuration_changed');}
 private frame(type:string,fields:Record<string,unknown>={}){if(!this.done)this.emit({v:1,type,request_id:this.request.request_id,...fields});}
 private fail(code:string):never{throw new RuntimeError(code);}
 private text(value:string,credential:string){let clean=value.split(credential).join('[redacted]');if(Buffer.byteLength(clean)>1024*1024)this.fail('message_too_large');let part='',size=0;for(const ch of clean){const bytes=Buffer.byteLength(ch);if(size+bytes>8192){this.frame('message',{session_id:this.sessionID,text:part});part='';size=0;}part+=ch;size+=bytes;}if(part)this.frame('message',{session_id:this.sessionID,text:part});}
 /** Handle only the original operation. A stale reply cannot select a new
  * callback, and a second exact reply is rejected rather than resolved twice. */
 handle(value:HostRequest){
  if(value.request_id!==this.request.request_id||this.done||!this.used)throw new RuntimeError('request_invalid');if(value.type==='cancel'){this.cancel();return;}
  if(value.type!=='permission_reply'&&value.type!=='input_reply')throw new RuntimeError('request_invalid');const pending=this.pending.get(value.source.callback_id);
  if(!pending||pending.kind!==(value.type==='permission_reply'?'permission':'input')||!same(pending.source,value.source))throw new RuntimeError('callback_changed');
  let reply:PermissionResult;if(value.type==='permission_reply'){
   permissionDigest(pending.source as PermissionSource,value.decision);if(value.decision.outcome==='allow_once')reply={behavior:'allow',updatedInput:JSON.parse(pending.source.input_json)};else{this.denied++;reply={behavior:'deny',message:value.decision.reason||'The current user declined this one action.'};}
  }else{answerDigest(pending.source as InputRequest,value.answer);if(value.answer.outcome==='answered')reply={behavior:'allow',updatedInput:{...JSON.parse(pending.source.input_json),answers:nativeAnswers(pending.source as InputRequest,value.answer)}};else{this.denied++;reply={behavior:'deny',message:value.answer.reason||'The current user cancelled this question.'};}}
  pending.signal.removeEventListener('abort',pending.abort);this.pending.delete(value.source.callback_id);this.returned++;pending.resolve(reply);
 }
 cancel(){if(this.done)return;this.cancelled=true;this.controller.abort();this.stoppedInput();for(const item of this.pending.values()){item.signal.removeEventListener('abort',item.abort);item.reject(new RuntimeError('execution_cancelled'));}this.pending.clear();}
 invalidate(){this.fault=new RuntimeError('request_invalid');this.cancel();}
 private async callback(tool:string,input:Record<string,unknown>,options:Parameters<NonNullable<Options['canUseTool']>>[2],credential:string):Promise<PermissionResult>{
  if(!this.initialized||!this.sessionID||this.done||this.cancelled||options.signal.aborted||this.facts.turn_ended||!TOOL_NAMES.includes(tool as typeof TOOL_NAMES[number])||options.agentID||options.mcpServer||!id(options.toolUseID)||!id(options.requestId)||this.seenNative.has(options.requestId)||this.callbacks>=MAX_CALLBACKS)this.fail('callback_invalid');
  const raw=inputJSON(input,credential),basis={session_id:this.sessionID,tool_use_id:options.toolUseID,request_id:options.requestId,callback_id:'cb-'+randomUUID(),configuration_sha256:this.config.configuration_sha256,params_sha256:sha256(raw)};
  const kind=tool==='AskUserQuestion'?'input':'permission';const source:PermissionSource|InputRequest=kind==='input'?{...basis,input_json:raw,questions:questions(raw)}:{...basis,tool_name:tool as PermissionSource['tool_name'],input_json:raw};this.seenNative.add(options.requestId);this.callbacks++;
  return await new Promise<PermissionResult>((resolve,reject)=>{const abort=()=>{this.pending.delete(source.callback_id);this.fault=new RuntimeError('callback_aborted');reject(this.fault);this.controller.abort();this.stoppedInput();};const pending={kind,source,resolve,reject,abort,signal:options.signal} satisfies Pending;this.pending.set(source.callback_id,pending);options.signal.addEventListener('abort',abort,{once:true});if(options.signal.aborted){abort();return;}this.frame(kind==='input'?'input_request':'permission_request',{source});});
 }
 private consume(raw:SDKMessage,credential:string){
  const message=raw as unknown as Record<string,unknown>;if(!object(message)||!id(message.session_id)||this.sessionID&&message.session_id!==this.sessionID||message.parent_tool_use_id!==undefined&&message.parent_tool_use_id!==null&&message.parent_tool_use_id!=='')this.fail('session_changed');
  if(this.facts.turn_ended)this.fail('activity_after_result');
  if(message.type==='system'&&message.subtype==='init'){
   if(this.initialized||message.model!==this.config.model||message.claude_code_version!==CLI_VERSION||message.permissionMode!=='default'||message.apiKeySource!=='ANTHROPIC_API_KEY'||!Array.isArray(message.tools)||message.tools.some(v=>!TOOL_NAMES.includes(v as typeof TOOL_NAMES[number]))||!Array.isArray(message.mcp_servers)||message.mcp_servers.length||Array.isArray(message.plugins)&&message.plugins.length)this.fail('initialization_mismatch');
   this.initialized=true;this.sessionID=message.session_id;this.frame('initialized',{session_id:this.sessionID,model:this.config.model,sdk_version:SDK_VERSION,cli_version:CLI_VERSION,configuration_sha256:this.config.configuration_sha256});return;
  }
  if(!this.initialized)this.fail('activity_before_init');
  if(message.type==='assistant'||message.type==='user'){
   if(!object(message.message)||!Array.isArray(message.message.content))this.fail('message_invalid');
   const native=message.message;if(message.type==='assistant'&&id(native.id)&&!this.seenUsage.has(native.id)){this.seenUsage.add(native.id);const usage=usageOf(native.usage);if(usage){delete usage.output_tokens;this.partial=addUsage(this.partial,usage);}}
   for(const block of native.content as unknown[]){if(!object(block))this.fail('message_invalid');if(block.type==='text'){if(message.type==='assistant'&&typeof block.text==='string')this.text(block.text,credential);}else if(block.type==='thinking'||block.type==='redacted_thinking')continue;else if(block.type==='tool_use'){
     if(message.type!=='assistant'||!id(block.id)||!TOOL_NAMES.includes(block.name as typeof TOOL_NAMES[number])||this.seenTools.has(block.id)||object(block.input)&&block.input.run_in_background===true)this.fail('tool_invalid');this.seenTools.add(block.id);this.activeTools.add(block.id);this.frame('activity_started',{session_id:this.sessionID,tool_use_id:block.id,tool_name:block.name});
    }else if(block.type==='tool_result'){
     if(message.type!=='user'||!id(block.tool_use_id)||!this.activeTools.has(block.tool_use_id)||block.is_error!==undefined&&typeof block.is_error!=='boolean')this.fail('tool_invalid');this.activeTools.delete(block.tool_use_id);this.frame('activity_completed',{session_id:this.sessionID,tool_use_id:block.tool_use_id,status:block.is_error===true?'error':'completed'});
    }else this.fail('message_invalid');}
   return;
  }
  if(message.type==='result'){
   if(typeof message.is_error!=='boolean'||typeof message.subtype!=='string'||!Array.isArray(message.permission_denials)||message.permission_denials.length>1000000||message.deferred_tool_use!==undefined&&message.deferred_tool_use!==null||message.queued_turn_count!==undefined&&message.queued_turn_count!==0||message.terminal_reason!==undefined&&message.terminal_reason!==''&&message.terminal_reason!=='completed'||this.pending.size||this.callbacks!==this.returned||this.activeTools.size)this.fail('result_incomplete');
   const usage=resultUsage(message);if(usage&&!(message.subtype!=='success'&&Object.values(usage).every(v=>v===0)))this.partial=usage;
   this.facts={result_subtype:message.subtype,result_is_error:message.is_error,turn_ended:true,tools_completed:this.activeTools.size===0,callback_count:this.callbacks,returned_count:this.returned,pending_callbacks:this.pending.size,callback_denial_count:this.denied,native_permission_denial_count:message.permission_denials.length};return;
  }
  // An automatic status or rate-limit observation does not authorize new work.
  if(message.type==='system'&&['status','api_retry','session_state_changed'].includes(String(message.subtype)))return;
  this.fail('unsupported_sdk_activity');
 }
 async run():Promise<Frame>{
  if(this.used)throw new RuntimeError('request_invalid');this.used=true;let terminal:Frame;
  try{const prepared=await prepare(this.config,this.environment);if(this.cancelled)this.fail('execution_cancelled');const stopped=new Promise<void>(resolve=>{this.stoppedInput=resolve;});
   const prompt=async function*(goal:string){yield{type:'user',session_id:'',parent_tool_use_id:null,message:{role:'user',content:goal}} as SDKUserMessage;await stopped;};
   this.queryInstance=this.queryFactory({prompt:prompt(this.request.goal),options:sdkOptions(this.config,prepared.env,this.controller,(tool,input,options)=>this.callback(tool,input,options,prepared.credential).catch(error=>{this.fault=error instanceof RuntimeError?error:new RuntimeError('callback_invalid');this.controller.abort();this.stoppedInput();throw error;}))});
   for await(const message of this.queryInstance){this.consume(message,prepared.credential);if(this.facts.turn_ended){this.stoppedInput();break;}}
   if(this.fault)throw this.fault;if(this.cancelled)this.fail('execution_cancelled');if(!this.facts.turn_ended)this.fail('result_missing');await verifyExecutable(this.config);
   const succeeded=this.facts.result_subtype==='success'&&!this.facts.result_is_error&&this.facts.callback_denial_count===0&&this.facts.native_permission_denial_count===0;terminal={v:1,type:succeeded?'completed':'failed',request_id:this.request.request_id,session_id:this.sessionID,evidence:this.facts,...(this.partial?{usage:this.partial}:{}),...(succeeded?{}:{code:'sdk_result_failed'})};
  }catch(error){const code=this.fault?.code||(error instanceof RuntimeError?error.code:'sdk_runtime_failed');terminal={v:1,type:this.cancelled&&!this.fault?'cancelled':'failed',request_id:this.request.request_id,session_id:this.sessionID,code,evidence:{...this.facts,callback_count:this.callbacks,returned_count:this.returned,pending_callbacks:this.pending.size,callback_denial_count:this.denied},...(this.partial?{usage:this.partial}:{})};}
  finally{this.stoppedInput();for(const item of this.pending.values()){item.signal.removeEventListener('abort',item.abort);item.reject(new RuntimeError('execution_closed'));}this.pending.clear();try{this.queryInstance?.close();}catch{this.fault=new RuntimeError('sdk_close_failed');}}
  // Local cancellation/failure may arrive during final disk verification or
  // SDK close. Recheck immediately before the sole terminal write.
  if(this.fault||this.cancelled)terminal={...terminal,type:this.fault?'failed':'cancelled',code:this.fault?.code||'execution_cancelled'};
  this.frame(terminal.type,Object.fromEntries(Object.entries(terminal).filter(([key])=>!['v','type','request_id'].includes(key))));this.done=true;return terminal;
 }
}

export function checkedRequest(value:unknown){return parseRequest(value);}
