import type {ToolContext} from '@opencode-ai/plugin/tool';
import {AttemptWorkspaceClient,workspaceBridge} from '@suma/agent-runtime/workspace-client';
import {writeFile} from 'node:fs/promises';
import {RuntimeError,stableCallIdentity,type ToolName} from './contract.js';

export interface HostClient{invoke(name:string,raw:string,step:number,callID:string):Promise<string>;close():Promise<void>}
export class ToolHost{
 private queue:Promise<unknown>=Promise.resolve();private fatal=false;
 constructor(private readonly requestID:string,private readonly connect:(signal:AbortSignal)=>Promise<HostClient>,private readonly recordFatal:()=>Promise<void>){}
 invoke(name:ToolName,args:unknown,context:ToolContext):Promise<string>{
  const operation=this.queue.then(async()=>{
   if(this.fatal||context.abort.aborted)throw new RuntimeError('workspace_fatal');
   try{
    const identity=stableCallIdentity(this.requestID,context,name);
    await context.ask({permission:'suma_workspace_'+name,patterns:['*'],always:['*'],metadata:{}});
    const client=await this.connect(context.abort);return await client.invoke('workspace.'+name,JSON.stringify(args),0,identity);
   }catch(error){this.fatal=true;await this.recordFatal().catch(()=>{});throw new RuntimeError('workspace_fatal');}
  });this.queue=operation.catch(()=>{});return operation;
 }
}
let host:ToolHost|undefined;
export function hostTools():ToolHost{
 if(host)return host;
 const requestID=process.env.SUMA_OPENCODE_REQUEST_ID;if(!requestID)throw new RuntimeError('workspace_identity');
 const bridge=workspaceBridge(JSON.parse(process.env.SUMA_OPENCODE_WORKSPACE_BRIDGE||'null'),requestID);if(bridge.conflict_repair_protocol)throw new RuntimeError('workspace_identity');
 let connecting:Promise<AttemptWorkspaceClient>|undefined;
 host=new ToolHost(requestID,signal=>connecting??=AttemptWorkspaceClient.connect(bridge,requestID,signal),async()=>{const path=process.env.SUMA_OPENCODE_TOOL_FAILURE;if(!path)process.exit(75);try{await writeFile(path,'{"v":1,"code":"workspace_fatal"}\n',{flag:'wx'});}catch(error){if((error as NodeJS.ErrnoException).code!=='EEXIST')process.exit(75);}});return host;
}
