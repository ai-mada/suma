#!/usr/bin/env node
import {createInterface} from 'node:readline';
import {runAgent,type AgentRequest,type RuntimeConfig} from './index.js';
const args=process.argv.slice(2);
if(args.length){
 if(args.length===1&&args[0]==='--version'){process.stdout.write('suma-agent-runtime 0.1.0\n');process.exit(0);}
 if(args.length===1&&args[0]==='--help'){process.stdout.write('Suma agent runtime: newline-delimited JSONL requests on stdin and bounded JSONL events on stdout.\nConfigure SUMA_MODEL, SUMA_MODEL_BASE_URL and optional SUMA_MODEL_API_KEY.\nProject tools require SUMA_CONTROL_URL, SUMA_PROJECT_ID and a separate agent-scope SUMA_AGENT_TOKEN. Runner credentials are never used for agent tools.\n--version / --help perform read-only probes without a model request.\n');process.exit(0);}
 process.stderr.write('Unsupported arguments; use --help.\n');process.exit(2);
}
const config:RuntimeConfig={model:process.env.SUMA_MODEL,baseURL:process.env.SUMA_MODEL_BASE_URL,apiKey:process.env.SUMA_MODEL_API_KEY,controlURL:process.env.SUMA_CONTROL_URL,controlToken:process.env.SUMA_AGENT_TOKEN,projectID:process.env.SUMA_PROJECT_ID};
let current:AbortController|undefined;const output=(e:unknown)=>{if(e && typeof e==='object' && 'type' in e && e.type==='error')process.exitCode=1;process.stdout.write(JSON.stringify(e)+'\n')};
const lines=createInterface({input:process.stdin,crlfDelay:Infinity});
lines.on('line',line=>{if(line.length>65536){output({type:'error',code:'request_limit',message:'Request exceeded 64 KiB'});return;}let request:AgentRequest & {cancel?:boolean};try{request=JSON.parse(line)}catch{output({type:'error',code:'invalid_json'});return;}if(request.cancel){current?.abort();return;}if(current){output({type:'error',code:'busy'});return;}current=new AbortController();void runAgent(request,config,output,current.signal).finally(()=>{current=undefined})});
process.on('SIGINT',()=>{current?.abort();lines.close()});process.on('SIGTERM',()=>{current?.abort();lines.close()});
