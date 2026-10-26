import {createHash} from 'node:crypto';
import {Client,StreamableHTTPClientTransport,type FetchLike} from '@modelcontextprotocol/client';

export interface WorkspaceBridge {version:1;endpoint:string;token:string;policy_sha256:string;request_id:string}
export class WorkspaceBridgeError extends Error {constructor(readonly code:string){super('Agent request failed; inspect provider and control configuration');}}
const object=(value:unknown):value is Record<string,unknown>=>value!==null&&typeof value==='object'&&!Array.isArray(value);
const fields:Record<string,string[]>={list:['directory','limit','cursor'],read:['path','start_line','max_lines'],write:['path','content','expected_sha256','expected_absent'],remove:['path','expected_sha256'],check:['check_id'],edit:['path','expected_sha256','edits']};
const ordinaryErrors=new Set(['args','arguments','path','missing','text','output_limit','conflict','operation','check','checks_stale','cursor'].map(code=>'native_workspace_'+code));
export function workspaceBridge(value:unknown,requestID:string):WorkspaceBridge {
 if(!object(value)||Object.keys(value).sort().join(',')!=='endpoint,policy_sha256,request_id,token,version'||value.version!==1||typeof value.endpoint!=='string'||typeof value.token!=='string'||!/^[A-Za-z0-9._~-]{16,512}$/.test(value.token)||typeof value.policy_sha256!=='string'||!/^[a-f0-9]{64}$/.test(value.policy_sha256)||typeof value.request_id!=='string'||!value.request_id||value.request_id.length>256||/\p{Cc}/u.test(value.request_id)||value.request_id!==requestID)throw new WorkspaceBridgeError('workspace_config');
 let endpoint:URL;try{endpoint=new URL(value.endpoint);}catch{throw new WorkspaceBridgeError('workspace_config');}
 if(endpoint.protocol!=='http:'||endpoint.hostname!=='127.0.0.1'||!endpoint.port||endpoint.username||endpoint.password||endpoint.search||endpoint.hash||endpoint.href!==value.endpoint)throw new WorkspaceBridgeError('workspace_config');
 return value as unknown as WorkspaceBridge;
}

export class AttemptWorkspaceClient {
 private readonly client:Client;
 private pending?:{name:string;operation:string;raw:string;rpcID?:string|number};
 readonly tools:{name:string;description?:string;parameters:Record<string,unknown>}[]=[];
 private constructor(private readonly bridge:WorkspaceBridge,private readonly signal:AbortSignal){
  this.client=new Client({name:'suma-agent-runtime',version:'0.1.0'},{capabilities:{},defaultCacheTtlMs:0,cachePartition:bridge.request_id,listMaxPages:1,inputRequired:{autoFulfill:false}});
 }
 static async connect(value:unknown,requestID:string,signal:AbortSignal):Promise<AttemptWorkspaceClient>{
  const instance=new AttemptWorkspaceClient(workspaceBridge(value,requestID),signal);
  try{
   const transport=new StreamableHTTPClientTransport(new URL(instance.bridge.endpoint),{fetch:instance.fetch,requestInit:{redirect:'error'},onInsufficientScope:'throw',reconnectionOptions:{maxRetries:0,initialReconnectionDelay:1000,maxReconnectionDelay:1000,reconnectionDelayGrowFactor:1}});
   await instance.client.connect(transport,{signal});
   const server=instance.client.getServerVersion();if(server?.name!=='suma-native-workspace'||server.version!=='attempt-workspace-v1')throw new WorkspaceBridgeError('workspace_catalog');
   const result=await instance.client.listTools(undefined,{signal});
   const seen=new Set<string>();
   for(const tool of result.tools){
    const suffix=tool.name.replace(/^suma_workspace_/,'');const schema=tool.inputSchema;
    if(!Object.hasOwn(fields,suffix)||tool.name!=='suma_workspace_'+suffix||seen.has(suffix)||schema.type!=='object'||schema.additionalProperties!==false||!object(schema.properties)||Object.keys(schema.properties).sort().join(',')!==[...fields[suffix]].sort().join(','))throw new WorkspaceBridgeError('workspace_catalog');
    seen.add(suffix);instance.tools.push({name:'workspace.'+suffix,description:tool.description,parameters:schema});
   }
   if(seen.size!==Object.keys(fields).length||result.nextCursor)throw new WorkspaceBridgeError('workspace_catalog');
   return instance;
  }catch(error){await instance.close().catch(()=>{});if(signal.aborted)throw signal.reason;throw error instanceof WorkspaceBridgeError?error:new WorkspaceBridgeError('workspace_transport');}
 }
 // The SDK remains the protocol owner. This bounded fetch only binds the
 // local endpoint and preserves the model's original JSON argument lexemes.
 private fetch:FetchLike=async(input,init)=>{
  const endpoint=input instanceof Request?input.url:String(input);
  if(endpoint!==this.bridge.endpoint||this.signal.aborted)throw new WorkspaceBridgeError('workspace_transport');
  const headers=new Headers(init?.headers);headers.set('Authorization','Bearer '+this.bridge.token);headers.delete('Cookie');headers.delete('Origin');
  let body=init?.body;
  if(body!==undefined&&body!==null){
   if(typeof body!=='string'||Buffer.byteLength(body)>2*1024*1024)throw new WorkspaceBridgeError('workspace_transport');
   const message:unknown=JSON.parse(body);
   if(!object(message)||message.jsonrpc!=='2.0'||typeof message.method!=='string')throw new WorkspaceBridgeError('workspace_transport');
   if(message.method==='tools/call'){
    const pending=this.pending,params=message.params;
    if(!pending||!object(params)||params.name!==pending.name||!object(params._meta)||params._meta['suma/operation_id']!==pending.operation||(typeof message.id!=='number'&&typeof message.id!=='string')||(pending.rpcID!==undefined&&pending.rpcID!==message.id)||JSON.stringify(params.arguments)!==JSON.stringify(JSON.parse(pending.raw)))throw new WorkspaceBridgeError('workspace_transport');
    pending.rpcID=message.id;
    const outer=JSON.stringify({...message,params:undefined}),parameters=JSON.stringify({...params,arguments:undefined});
    body=outer.slice(0,-1)+',"params":'+parameters.slice(0,-1)+',"arguments":'+pending.raw+'}}';
   }
  }
  const signal=AbortSignal.any([this.signal,...(init?.signal?[init.signal]:[])]);
  const response=await fetch(this.bridge.endpoint,{...init,body,headers,signal,redirect:'error'});
  const chunks:Uint8Array[]=[];let size=0;const reader=response.body?.getReader();
  if(reader)try{for(;;){const {value,done}=await reader.read();if(done)break;size+=value.length;if(size>2*1024*1024){await reader.cancel();throw new WorkspaceBridgeError('workspace_transport');}chunks.push(value);}}finally{reader.releaseLock();}
  const bytes=Buffer.concat(chunks);new TextDecoder('utf8',{fatal:true}).decode(bytes);
  return new Response(bytes.length?bytes:null,{status:response.status,statusText:response.statusText,headers:response.headers});
 };
 async invoke(name:string,raw:string,step:number,callID:string):Promise<string>{
  if(this.pending||this.signal.aborted||!this.tools.some(tool=>tool.name===name))throw new WorkspaceBridgeError('workspace_fatal');
  const parsed:unknown=JSON.parse(raw);if(!object(parsed))throw new WorkspaceBridgeError('workspace_fatal');
  const operation=createHash('sha256').update(JSON.stringify([this.bridge.request_id,step,callID])).digest('hex');
  this.pending={name:'suma_'+name.replaceAll('.','_'),operation,raw};
  try{
   const result=await this.client.callTool({name:this.pending.name,arguments:parsed,_meta:{'suma/operation_id':operation}},{signal:this.signal,timeout:65000});
   if(Buffer.byteLength(JSON.stringify(result))>65536)throw new WorkspaceBridgeError('workspace_fatal');
   if(result.isError){const structured=result.structuredContent,error=object(structured)?structured.error:undefined;if(!object(error)||error.fatal!==false||typeof error.code!=='string'||!ordinaryErrors.has(error.code))throw new WorkspaceBridgeError('workspace_fatal');}
   const content=JSON.stringify(result.structuredContent??{content:result.content,isError:result.isError===true});if(Buffer.byteLength(content)>65536)throw new WorkspaceBridgeError('workspace_fatal');return content;
  }catch(error){if(this.signal.aborted)throw this.signal.reason;throw error instanceof WorkspaceBridgeError?error:new WorkspaceBridgeError('workspace_transport');}
  finally{this.pending=undefined;}
 }
 async close(){await this.client.close();}
}
