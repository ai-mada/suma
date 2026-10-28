import {createOpencodeClient} from '@opencode-ai/sdk/v2/client';
import {randomBytes} from 'node:crypto';
import {readFile} from 'node:fs/promises';
import {join} from 'node:path';
import {isDeepStrictEqual} from 'node:util';
import {RuntimeError,SDK_VERSION,TOOL_NAMES,object,safeID,validateFinal,errorCode,type ExecuteRequest,type RuntimeConfig,type Emit,type Frame} from './contract.js';
export interface ManagedServer{url:string;authorization:string;readonly parentPID?:number;stop():Promise<void>}

// SDK parsing owns the protocol. This fetch only restricts transport, response
// size and redirects; it never retries a possibly accepted mutation.
export function localFetch(origin:string,authorization:string):typeof fetch{
 const allowed=new URL(origin);if(allowed.protocol!=='http:'||allowed.hostname!=='127.0.0.1'||!allowed.port||allowed.username||allowed.password)throw new RuntimeError('server_origin');
 return async(input,init)=>{
  const request=new Request(input,init),url=new URL(request.url);
  if(url.origin!==allowed.origin||url.username||url.password)throw new RuntimeError('server_origin');
  const headers=new Headers(request.headers);headers.set('Authorization',authorization);headers.delete('Cookie');headers.delete('Origin');
  const response=await fetch(new Request(request,{headers,redirect:'error'}));
  if(response.headers.get('content-type')?.startsWith('text/event-stream')){
   let eventSize=0,newlines=0;const bounded=new TransformStream<Uint8Array,Uint8Array>({transform(chunk,controller){for(const byte of chunk){eventSize++;if(byte===10)newlines++;else if(byte!==13)newlines=0;if(newlines===2){eventSize=0;newlines=0;}if(eventSize>262144)throw new RuntimeError('event_limit');}controller.enqueue(chunk);}});
   return new Response(response.body?.pipeThrough(bounded),{status:response.status,statusText:response.statusText,headers:response.headers});
  }
  let size=0;const chunks:Uint8Array[]=[];const reader=response.body?.getReader();
  if(reader)try{for(;;){const {value,done}=await reader.read();if(done)break;size+=value.byteLength;if(size>2*1024*1024){await reader.cancel();throw new RuntimeError('response_limit');}chunks.push(value);}}finally{reader.releaseLock();}
  const bytes=Buffer.concat(chunks);new TextDecoder('utf8',{fatal:true}).decode(bytes);
  return new Response(bytes.length?bytes:null,{status:response.status,statusText:response.statusText,headers:response.headers});
 };
}
export function assertEffectiveConfig(actual:unknown,expected:unknown){
 if(!object(actual)||!object(expected))throw new RuntimeError('configuration_mismatch');
 const copy={...actual};for(const name of ['mode','plugin_origins']){if(name in copy){const item=copy[name];if(Array.isArray(item)?item.length!==0:!object(item)||Object.keys(item).length!==0)throw new RuntimeError('configuration_mismatch');delete copy[name];}}
 if(!isDeepStrictEqual(copy,expected))throw new RuntimeError('configuration_mismatch');
}
function usage(value:unknown):Record<string,number>|undefined{
 if(!object(value)||!object(value.info)||!object(value.info.tokens))return;
 // Upstream initializes missing usage counters to zero. Do not mistake those
 // defaults for observed zero usage or a complete bill.
 const tokens=value.info.tokens;if(!Number.isSafeInteger(tokens.input)||!Number.isSafeInteger(tokens.output)||Number(tokens.input)<=0||Number(tokens.output)<=0)return;
 const result:Record<string,number>={input_tokens:Number(tokens.input),output_tokens:Number(tokens.output)};
 for(const [key,count] of [['reasoning_output_tokens',tokens.reasoning],['cached_input_tokens',object(tokens.cache)?tokens.cache.read:undefined],['cache_write_input_tokens',object(tokens.cache)?tokens.cache.write:undefined]] as const){if(Number.isSafeInteger(count)&&Number(count)>0)result[key]=Number(count);}
 return result;
}
export async function runSession(request:ExecuteRequest,config:RuntimeConfig,server:ManagedServer,expected:unknown,emit:Emit,signal:AbortSignal):Promise<void>{
 const frame=(value:Omit<Frame,'v'|'request_id'>)=>emit({v:1,request_id:request.request_id,...value} as Frame);
 const client=createOpencodeClient({baseUrl:server.url,directory:config.source,fetch:localFetch(server.url,server.authorization),throwOnError:true});
 let sessionID='',userMessageID='',abortAcknowledged=false,parentStopped=false,final:unknown,failure:unknown,createSent=false;
 const promptAbort=new AbortController();let abortTask:Promise<void>|undefined;
 const observationAbort=new AbortController();let observing=true,observation:Promise<void>|undefined;
 const abort=()=>{if(!abortTask)abortTask=(async()=>{try{if(sessionID){const result=await client.session.abort({sessionID},{signal:AbortSignal.timeout(3000),throwOnError:true});abortAcknowledged=result.data===true;}}catch{}finally{promptAbort.abort();}})();};
 signal.addEventListener('abort',abort,{once:true});
 try{
  if(signal.aborted)throw new RuntimeError('aborted');
  const health=await client.global.health({signal:signal,throwOnError:true});if(health.data?.healthy!==true||health.data.version!==SDK_VERSION)throw new RuntimeError('server_version');
  const current=await client.config.get({}, {signal,throwOnError:true});assertEffectiveConfig(current.data,expected);
  frame({type:'ready',sdk_version:SDK_VERSION,server_version:SDK_VERSION,configuration_sha256:config.configuration_sha256,provider_id:config.provider_id,model:config.model});
  if(signal.aborted)throw new RuntimeError('aborted');createSent=true;
  const session=await client.session.create({title:'Suma attempt '+request.request_id,agent:'suma',model:{id:config.model,providerID:config.provider_id},permission:[{permission:'*',pattern:'*',action:'deny'},...TOOL_NAMES.map(name=>({permission:'suma_workspace_'+name,pattern:'*',action:'allow' as const}))]},{signal,throwOnError:true});
  if(!session.data||!safeID(session.data.id))throw new RuntimeError('session_identity');sessionID=session.data.id;
  // OpenCode identifiers accept the msg_ prefix; use a fresh persisted identity
  // before POST, then correlate the returned assistant's parent exactly.
  userMessageID='msg_'+Date.now().toString(16)+randomBytes(8).toString('hex');
  frame({type:'started',session_id:sessionID,user_message_id:userMessageID});
  if(signal.aborted){abort();throw new RuntimeError('aborted');}
  // Events are bounded progress observations. Their EOF/idle/step-finish never
  // supplies terminal proof and an observation gap never replays a request.
  observation=(async()=>{
   const events=await client.event.subscribe({}, {signal:observationAbort.signal,sseMaxRetryAttempts:1,onSseError:()=>{}});const seen=new Set<string>();
   for await(const event of events.stream){if(!observing||seen.size>=4096)continue;const value:unknown=event;if(!object(value)||value.type!=='message.part.updated'||!object(value.properties)||!object(value.properties.part))continue;const part=value.properties.part;
    if(part.type!=='tool'||part.sessionID!==sessionID||typeof part.tool!=='string'||!object(part.state)||!safeID(part.messageID)||!safeID(part.callID))continue;const tool=part.tool.replace(/^suma_workspace_/,'');if(!TOOL_NAMES.includes(tool as typeof TOOL_NAMES[number])||part.tool!=='suma_workspace_'+tool)continue;
    const status=part.state.status==='running'?'started':part.state.status==='completed'?'completed':part.state.status==='error'?'failed':undefined;if(!status)continue;const key=JSON.stringify([part.messageID,part.callID,status]);if(seen.has(key))continue;seen.add(key);frame({type:'progress',session_id:sessionID,kind:'tool',status,tool});
   }
  })().catch(()=>{});
  frame({type:'progress',session_id:sessionID,kind:'model',status:'started'});
  const result=await client.session.prompt({sessionID,messageID:userMessageID,agent:'suma',model:{providerID:config.provider_id,modelID:config.model},parts:[{type:'text',text:request.goal}]},{signal:promptAbort.signal,throwOnError:true});
  final=result.data;validateFinal(final,{sessionID,userMessageID,providerID:config.provider_id,model:config.model});
  const fatal=await readFile(join(config.home,'tool-failure.json'),'utf8').catch((error:NodeJS.ErrnoException)=>{if(error.code==='ENOENT')return undefined;throw error;});if(fatal!==undefined)throw new RuntimeError('workspace_fatal');
  frame({type:'progress',session_id:sessionID,kind:'model',status:'completed'});
 }catch(error){failure=error;}
 finally{
  if(signal.aborted){abort();await abortTask;}
  observing=false;observationAbort.abort();await observation;
  signal.removeEventListener('abort',abort);
  try{await server.stop();parentStopped=true;}catch{failure=new RuntimeError('server_parent_stop_unconfirmed');}
 }
 if(signal.aborted){if(parentStopped&&(!sessionID&&!createSent||!!sessionID&&abortAcknowledged))frame({type:'cancelled',session_id:sessionID,abort_acknowledged:abortAcknowledged,server_parent_stopped:true});else frame({type:'failed',code:'cancel_unconfirmed',server_parent_stopped:parentStopped});return;}
 if(failure!==undefined||!parentStopped){frame({type:'failed',code:errorCode(failure),server_parent_stopped:parentStopped});return;}
 const proof=validateFinal(final,{sessionID,userMessageID,providerID:config.provider_id,model:config.model}),counters=usage(final);
 const completedAt=new Date(proof.completed_at).toISOString().replace(/\.000Z$/,'Z').replace(/(\.\d*?[1-9])0+Z$/,'$1Z');
 frame({type:'completed',session_id:sessionID,user_message_id:userMessageID,assistant_message_id:proof.assistant_message_id,provider_id:config.provider_id,model:config.model,finish:'stop',completed_at:completedAt,server_parent_stopped:true,...(counters?{usage:counters}:{}),usage_state:counters?'partial':'unreported'});
}
