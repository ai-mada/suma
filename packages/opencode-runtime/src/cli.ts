#!/usr/bin/env node
import {createInterface} from 'node:readline';
import {mkdtemp,rm,mkdir} from 'node:fs/promises';
import {dirname,join,resolve,basename} from 'node:path';
import {VERSION,SDK_VERSION,RuntimeError,object,safeID,closed,parseExecute,errorCode,type Frame,type RuntimeConfig} from './contract.js';
import {parseConfig,prepareHome,verifyAssets,verifyExecutable,rejectManagedConfig,childEnvironment,generatedConfig,probeExecutable} from './config.js';
import {startServer,ServerStartError} from './server.js';
import {runSession} from './session.js';
import {options,display,help} from './display.js';

const write=(frame:unknown)=>process.stdout.write(JSON.stringify(frame)+'\n');
const parsed=options(process.argv.slice(2),process.env),args=parsed.args,locale=parsed.locale;
if(parsed.error){process.stderr.write(display('language_invalid',locale));process.exit(2);}
if(args.length===1&&args[0]==='--version'){process.stdout.write('suma-opencode-runtime '+VERSION+'\n');process.exit(0);}
if(args.length===1&&args[0]==='--help'){process.stdout.write(help(locale));process.exit(0);}
let config:RuntimeConfig;
try{if(args.length&&!(args.length===1&&args[0]==='--probe'))throw new RuntimeError('arguments_invalid');config=parseConfig(JSON.parse(process.env.SUMA_OPENCODE_CONFIG||'null'));}catch{process.stderr.write(display('configuration_invalid',locale));process.exit(2);}
if(args[0]==='--probe'){
 try{
  await rejectManagedConfig();await verifyAssets(config);await verifyExecutable(config);await mkdir(dirname(config.home),{recursive:true});
  const scratch=await mkdtemp(join(dirname(config.home),'.suma-opencode-probe-'));
  try{const local={...config,home:scratch};await probeExecutable(local,childEnvironment(local,generatedConfig(local)));}
  finally{if(dirname(resolve(scratch))!==resolve(dirname(config.home))||!basename(scratch).startsWith('.suma-opencode-probe-'))throw new RuntimeError('probe_cleanup');await rm(scratch,{recursive:true,force:true});}
  write({v:1,type:'probe',protocol:'opencode-sdk-v1',runtime_version:VERSION,sdk_version:SDK_VERSION,server_version:SDK_VERSION,configuration_sha256:config.configuration_sha256,provider_id:config.provider_id,model:config.model,workspace_tools_version:'attempt-workspace-v1'});
 }catch{process.stderr.write(display('probe_failed',locale));process.exitCode=1;}
}else{
 let active:AbortController|undefined,requestID='',used=false,terminal=false,protocolError=false;
 const emit=(frame:Frame)=>{if(terminal)return;if(['completed','failed','cancelled'].includes(frame.type)){if(protocolError)frame={v:1,type:'failed',request_id:requestID,code:'request_invalid',server_parent_stopped:frame.server_parent_stopped};terminal=true;process.exitCode=frame.type==='completed'||frame.type==='cancelled'?0:1;}write(frame);};
 const invalid=(id:string)=>{if(active){protocolError=true;active.abort();return;}if(safeID(id))emit({v:1,type:'failed',request_id:id,code:'request_invalid',server_parent_stopped:true});else {process.stderr.write(display('request_invalid',locale));process.exitCode=2;}};
 const lines=createInterface({input:process.stdin,crlfDelay:Infinity});
 lines.on('line',line=>{
  if(Buffer.byteLength(line)>1024*1024){invalid(requestID);return;}let value:unknown;
  try{value=JSON.parse(line);if(object(value)&&value.type==='cancel'){closed(value,['v','type','request_id']);if(value.v!==1||!safeID(value.request_id)||value.request_id!==requestID||!active)throw new RuntimeError('request_invalid');active.abort();return;}
   const request=parseExecute(value);if(used)throw new RuntimeError('request_invalid');used=true;requestID=request.request_id;active=new AbortController();const controller=active;
   void (async()=>{let serverStarted=false;try{const prepared=await prepareHome(config,request);if(controller.signal.aborted)throw new RuntimeError('aborted');const server=await startServer(config,prepared.env,controller.signal);serverStarted=true;await runSession(request,config,server,prepared.generated,emit,controller.signal);}catch(error){emit({v:1,type:'failed',request_id:requestID,code:errorCode(error),server_parent_stopped:error instanceof ServerStartError?error.serverParentStopped:!serverStarted});}finally{active=undefined;lines.close();}})();
  }catch{invalid(object(value)&&safeID(value.request_id)?value.request_id:requestID);}
 });
 lines.on('close',()=>{if(!used&&!terminal){process.stderr.write(display('execute_required',locale));process.exitCode=2;}});
 for(const signal of ['SIGINT','SIGTERM'] as const)process.on(signal,()=>{active?.abort();if(!active)lines.close();});
}
