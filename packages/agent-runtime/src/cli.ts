#!/usr/bin/env node
import {createInterface} from 'node:readline';
import {runAgent,type AgentRequest,type RuntimeConfig} from './index.js';
import {nativeOptions,nativeHelp,nativeDisplay} from './display.js';
const options=nativeOptions(process.argv.slice(2),process.env);
if(options.error){process.stderr.write(nativeDisplay(options.error,options.locale)!.localized_message+'\n');process.exit(2);}
if(options.action==='version'){process.stdout.write('suma-agent-runtime 0.1.0\n');process.exit(0);}
if(options.action==='help'){process.stdout.write(nativeHelp(options.locale));process.exit(0);}
const config:RuntimeConfig={model:process.env.SUMA_MODEL,baseURL:process.env.SUMA_MODEL_BASE_URL,apiKey:process.env.SUMA_MODEL_API_KEY,controlURL:process.env.SUMA_CONTROL_URL,controlToken:process.env.SUMA_AGENT_TOKEN,projectID:process.env.SUMA_PROJECT_ID,locale:options.locale};
let current:AbortController|undefined;const output=(e:unknown)=>{if(e && typeof e==='object' && 'type' in e && e.type==='error')process.exitCode=1;process.stdout.write(JSON.stringify(e)+'\n')};
const lines=createInterface({input:process.stdin,crlfDelay:Infinity});
lines.on('line',line=>{if(line.length>65536){output({type:'error',code:'request_limit',message:'Request exceeded 64 KiB',display:nativeDisplay('native.cli.request_limit',options.locale)});return;}let request:AgentRequest & {cancel?:boolean};try{request=JSON.parse(line);if(!request||typeof request!=='object'||Array.isArray(request))throw Error('object required')}catch{output({type:'error',code:'invalid_json',display:nativeDisplay('native.cli.invalid_json',options.locale)});return;}if(request.cancel){current?.abort();return;}if(current){output({type:'error',code:'busy',display:nativeDisplay('native.cli.busy',options.locale)});return;}current=new AbortController();void runAgent(request,config,output,current.signal).finally(()=>{current=undefined})});
process.on('SIGINT',()=>{current?.abort();lines.close()});process.on('SIGTERM',()=>{current?.abort();lines.close()});
