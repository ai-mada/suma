import {spawn} from 'node:child_process';
import {randomBytes} from 'node:crypto';
import {join} from 'node:path';
import {RuntimeError,type RuntimeConfig} from './contract.js';
import type {ManagedServer} from './session.js';
import {verifyExecutable} from './config.js';

export class ServerStartError extends RuntimeError{constructor(code:string,readonly serverStopped:boolean){super(code);}}
async function killWindowsTree(pid:number,environment:NodeJS.ProcessEnv):Promise<boolean>{
 const executable=join(environment.SystemRoot||environment.SYSTEMROOT||'C:\\Windows','System32','taskkill.exe');
 return await new Promise<boolean>(resolve=>{const taskkill=spawn(executable,['/pid',String(pid),'/T','/F'],{windowsHide:true,stdio:'ignore',env:environment});const timer=setTimeout(()=>{taskkill.kill();resolve(false);},3000);taskkill.once('error',()=>{clearTimeout(timer);resolve(false);});taskkill.once('close',code=>{clearTimeout(timer);resolve(code===0);});});
}
export async function startServer(config:RuntimeConfig,environment:NodeJS.ProcessEnv,signal:AbortSignal):Promise<ManagedServer>{
 await verifyExecutable(config);
 const password=randomBytes(32).toString('hex'),authorization='Basic '+Buffer.from('suma:'+password).toString('base64');
 // Inherit the Go supervisor's process group. A detached server could survive
 // wrapper failure; the outer Runner's Finish/Stop is the tree authority.
 const child=spawn(config.executable,['serve','--hostname=127.0.0.1','--port=0'],{cwd:config.source,env:{...environment,OPENCODE_SERVER_USERNAME:'suma',OPENCODE_SERVER_PASSWORD:password},windowsHide:true,stdio:['pipe','pipe','pipe']});child.stdin.on('error',()=>{});child.stdin.end();
 let stopping=false,unexpected=false,closed=false;let closeError=false;
 const didClose=new Promise<void>(resolve=>{child.once('error',()=>{closeError=true;});child.once('close',()=>{closed=true;if(!stopping)unexpected=true;resolve();});});
 let stopTask:Promise<void>|undefined;
 const stop=()=>stopTask??=(async()=>{
  stopping=true;if(!child.pid){await didClose;return;}
  let treeConfirmed=false;
  if(process.platform==='win32'){if(!closed)treeConfirmed=await killWindowsTree(child.pid,environment);if(!treeConfirmed&&!closed)child.kill();}
  else child.kill('SIGTERM');
  let timer:ReturnType<typeof setTimeout>|undefined;const graceful=new Promise<void>(resolve=>{timer=setTimeout(resolve,1500);});await Promise.race([didClose,graceful]);if(timer)clearTimeout(timer);
  if(!closed)child.kill('SIGKILL');
  await Promise.race([didClose,new Promise<never>((_,reject)=>{const timer=setTimeout(()=>reject(new RuntimeError('server_stop_unconfirmed')),3000);timer.unref();})]);
  if(process.platform!=='win32')treeConfirmed=closed;
  if(!closed||!treeConfirmed)throw new RuntimeError('server_stop_unconfirmed');
  if(unexpected||closeError)throw new RuntimeError('server_exit_unexpected');
 })();
 let startupTimer:ReturnType<typeof setTimeout>|undefined;let outputSize=0,buffer='';
 const aborted=()=>{void stop().catch(()=>{});};signal.addEventListener('abort',aborted,{once:true});
 try{
  const url=await new Promise<string>((resolve,reject)=>{
   startupTimer=setTimeout(()=>reject(new RuntimeError('server_start_timeout')),10000);
   child.once('error',()=>reject(new RuntimeError('server_start_failed')));child.once('close',()=>reject(new RuntimeError('server_start_failed')));
   const bounded=(chunk:Buffer)=>{outputSize+=chunk.length;if(outputSize>32768)reject(new RuntimeError('server_start_output'));};
   child.stderr.on('data',bounded);
   child.stdout.on('data',(chunk:Buffer)=>{bounded(chunk);if(outputSize>32768)return;buffer+=chunk.toString('utf8');let newline:number;while((newline=buffer.indexOf('\n'))>=0){const line=buffer.slice(0,newline).trim();buffer=buffer.slice(newline+1);const match=/^opencode server listening on (http:\/\/127\.0\.0\.1:([1-9][0-9]{0,4}))\/?$/.exec(line);if(match&&Number(match[2])<=65535){resolve(match[1]);return;}}});
   if(signal.aborted)reject(new RuntimeError('aborted'));
  });
  if(signal.aborted)throw new RuntimeError('aborted');return {url,authorization,stop};
 }catch(error){let stopped=false;try{await stop();stopped=true;}catch{}throw new ServerStartError(error instanceof RuntimeError?error.code:'server_start_failed',stopped);}
 finally{if(startupTimer)clearTimeout(startupTimer);signal.removeEventListener('abort',aborted);}
}
