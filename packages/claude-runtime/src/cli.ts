#!/usr/bin/env node
import {StringDecoder} from 'node:string_decoder';
import {parseConfig} from './config.js';
import {VERSION,RuntimeError,parseLine,parseRequest,type RuntimeConfig,type Frame} from './contract.js';
import {ClaudeSession} from './runtime.js';
import {locale,display,type Locale} from './display.js';

let language:Locale='zh-CN';try{language=locale(process.env);}catch{process.stderr.write(display('locale_invalid',language)+'\n');process.exit(2);}
const args=process.argv.slice(2);
if(args.length===1&&args[0]==='--version'){process.stdout.write('suma-claude-runtime '+VERSION+'\n');process.exit(0);}
if(args.length===1&&args[0]==='--help'){process.stdout.write(display('help',language));process.exit(0);}
if(args.length){process.stderr.write(display('arguments_invalid',language)+'\n');process.exit(2);}
let config:RuntimeConfig;try{config=parseConfig(parseLine(process.env.SUMA_CLAUDE_SDK_CONFIG||'null'));}catch{process.stderr.write(display('configuration_invalid',language)+'\n');process.exit(2);}
let session:ClaudeSession|undefined,used=false,terminal=false,buffer='',lineBytes=0,tailPending=false;
const write=(frame:Frame)=>{if(terminal)return;if(['completed','failed','cancelled'].includes(frame.type)){terminal=true;process.exitCode=frame.type==='completed'?0:1;process.stdin.pause();}process.stdout.write(JSON.stringify(frame)+'\n');};
const invalid=()=>{if(session){session.invalidate();return;}process.stderr.write(display('request_invalid',language)+'\n');process.exitCode=2;process.stdin.pause();};
const line=(raw:string)=>{if(terminal)return;try{const request=parseRequest(parseLine(raw));if(request.type==='execute'){if(used)throw new RuntimeError('request_invalid');used=true;session=new ClaudeSession(config,request,write);void session.run().catch(()=>{if(!terminal){write({v:1,type:'failed',request_id:request.request_id,session_id:'',code:'sdk_runtime_failed'});}});}else{if(!session)throw new RuntimeError('request_invalid');session.handle(request);}}catch{invalid();}};
// Limit each physical frame while bytes arrive, before readline could buffer
// an arbitrarily large or unterminated line. StringDecoder preserves UTF-8
// boundaries; invalid UTF-8 is rejected by a round-trip check per frame below.
const decoder=new StringDecoder('utf8');let rawChunks:Buffer[]=[];
process.stdin.on('data',(chunk:Buffer)=>{if(terminal)return;let start=0;for(let i=0;i<chunk.length;i++)if(chunk[i]===10){const bytes=chunk.subarray(start,i);lineBytes+=bytes.length;if(lineBytes>1024*1024){invalid();return;}rawChunks.push(bytes);const original=Buffer.concat(rawChunks),raw=buffer+decoder.write(bytes);if(!Buffer.from(raw,'utf8').equals(original)){invalid();return;}line(raw.endsWith('\r')?raw.slice(0,-1):raw);buffer='';rawChunks=[];lineBytes=0;tailPending=false;start=i+1;}
 if(start<chunk.length){const bytes=chunk.subarray(start);lineBytes+=bytes.length;if(lineBytes>1024*1024){invalid();return;}rawChunks.push(bytes);buffer+=decoder.write(bytes);tailPending=true;}
});
process.stdin.on('end',()=>{if(terminal)return;if(tailPending||buffer||decoder.end()){invalid();return;}if(session)session.invalidate();else invalid();});
process.stdin.on('error',invalid);
for(const signal of ['SIGINT','SIGTERM'] as const)process.once(signal,()=>{if(session)session.cancel();else process.exitCode=1;});
