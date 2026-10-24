#!/usr/bin/env node
import {createInterface} from 'node:readline';
import {runAgent,type AgentRequest,type RuntimeConfig} from './index.js';
const config:RuntimeConfig={model:process.env.SUMA_MODEL,baseURL:process.env.SUMA_MODEL_BASE_URL,apiKey:process.env.SUMA_MODEL_API_KEY,controlURL:process.env.SUMA_CONTROL_URL,controlToken:process.env.SUMA_RUNNER_TOKEN,projectID:process.env.SUMA_PROJECT_ID};
let current:AbortController|undefined;const output=(e:unknown)=>{if(e && typeof e==='object' && 'type' in e && e.type==='error')process.exitCode=1;process.stdout.write(JSON.stringify(e)+'\n')};
const lines=createInterface({input:process.stdin,crlfDelay:Infinity});
lines.on('line',line=>{if(line.length>65536){output({type:'error',code:'request_limit',message:'Request exceeded 64 KiB'});return;}let request:AgentRequest & {cancel?:boolean};try{request=JSON.parse(line)}catch{output({type:'error',code:'invalid_json'});return;}if(request.cancel){current?.abort();return;}if(current){output({type:'error',code:'busy'});return;}current=new AbortController();void runAgent(request,config,output,current.signal).finally(()=>{current=undefined})});
process.on('SIGINT',()=>{current?.abort();lines.close()});process.on('SIGTERM',()=>{current?.abort();lines.close()});
