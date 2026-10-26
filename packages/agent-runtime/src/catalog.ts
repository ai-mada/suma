import {spawn} from 'node:child_process';
import {access} from 'node:fs/promises';
import {delimiter,join} from 'node:path';
import {nativeDisplay,requireNativeLocale,type NativeDisplay,type NativeMessageKey} from './display.js';
export interface AdapterInfo {id:string;installed:boolean;executable?:string;version?:string;operations:string[];verified:string[];reason?:string;display?:NativeDisplay}
class ProbeError extends Error{constructor(message:string,readonly displayKey:NativeMessageKey,readonly displayParams:Record<string,string|number>={}){super(message)}}
const definitions=[{id:'codex',binary:'codex',help:['exec','--help'],required:['--json','--sandbox'],operations:['jsonl_exec','read_only_sandbox']},{id:'opencode',binary:'opencode',help:['run','--help'],required:['--format'],operations:['headless_run']},{id:'hermes',binary:'hermes',help:['--help'],required:['chat'],operations:['interactive_chat']},{id:'harness',binary:'dsh',help:['--profile','multica','--probe'],required:[],operations:['multica_stdio_v1']},{id:'codebuddy',binary:'codebuddy',help:['--help'],required:['--input-format','--output-format','--verbose'],operations:['stream_json_print']}];
async function installed(binary:string):Promise<string|undefined>{for(const ext of process.platform==='win32'?['.exe','','.cmd','.bat']:['']){for(const dir of (process.env.PATH??'').split(delimiter)){const p=join(dir,binary+ext);try{await access(p);return p}catch{}}}return undefined;}
function probe(file:string,args:string[],stdoutOnly=false):Promise<string>{return new Promise((resolve,reject)=>{
 if(/\.(cmd|bat)$/i.test(file)){reject(new ProbeError('Shell wrapper is not probed automatically','native.catalog.shell_wrapper'));return;}
 const child=spawn(file,args,{shell:false,windowsHide:true,stdio:['ignore','pipe','pipe']});const chunks:Buffer[]=[];let size=0,settled=false;
 const fail=(error:unknown)=>{if(settled)return;settled=true;clearTimeout(timer);reject(error);};
 const timer=setTimeout(()=>{child.kill();fail(new ProbeError('CLI probe timed out','native.catalog.probe_timeout'));},5000);
 const collect=(stdout:boolean)=>(b:Buffer)=>{if(settled)return;size+=b.length;if(size>32768){child.kill();fail(new ProbeError('CLI probe exceeded output limit','native.catalog.probe_output_limit'));return;}if(stdout||!stdoutOnly)chunks.push(b);};
 child.stdout.on('data',collect(true));child.stderr.on('data',collect(false));child.on('error',fail);
 child.on('close',code=>{if(settled)return;if(code!==0){fail(new ProbeError(`CLI probe exited ${code}`,'native.catalog.probe_exited',{status:String(code)}));return;}try{const text=new TextDecoder('utf8',{fatal:true}).decode(Buffer.concat(chunks));settled=true;clearTimeout(timer);resolve(text.trim());}catch(error){fail(error);}});
})}

// Reject duplicate keys, including escaped aliases in nested objects, before
// JSON.parse discards them. Only a single bounded upstream probe is accepted.
function validateHarnessProbe(text:string):void {
 let offset=0;const rootNumbers=new Map<string,string>();
 const whitespace=()=>{while(/[\t\r\n ]/.test(text[offset]??'!'))offset++;};
 const string=():string=>{if(text[offset]!=='"')throw Error('string required');const start=offset++;while(offset<text.length){const char=text[offset++];if(char==='"')return JSON.parse(text.slice(start,offset)) as string;if(char==='\\')offset++;}throw Error('unterminated string');};
 const value=(depth:number):void=>{
  if(depth>32)throw Error('probe nesting limit');whitespace();const kind=text[offset];
  if(kind==='{'){offset++;whitespace();const keys=new Set<string>();if(text[offset]==='}'){offset++;return;}for(;;){whitespace();const key=string();if(keys.has(key))throw Error('duplicate probe key');keys.add(key);whitespace();if(text[offset++]!==':')throw Error('separator required');whitespace();const start=offset;value(depth+1);if(depth===0&&(key==='v'||key==='protocol_version'))rootNumbers.set(key,text.slice(start,offset));whitespace();const separator=text[offset++];if(separator==='}')return;if(separator!==',')throw Error('separator required');}}
  if(kind==='['){offset++;whitespace();if(text[offset]===']'){offset++;return;}for(;;){value(depth+1);whitespace();const separator=text[offset++];if(separator===']')return;if(separator!==',')throw Error('separator required');}}
  if(kind==='"'){string();return;}
  const scalar=/^(?:true|false|null|-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?)/.exec(text.slice(offset));if(!scalar)throw Error('value required');offset+=scalar[0].length;
 };
 value(0);whitespace();if(offset!==text.length)throw Error('trailing probe data');const frame:unknown=JSON.parse(text);
 if(!frame||typeof frame!=='object'||Array.isArray(frame))throw Error('object required');const record=frame as Record<string,unknown>,plugin=record.plugin_version;
 if(rootNumbers.get('v')!=='1'||rootNumbers.get('protocol_version')!=='1'||record.type!=='probe'||record.runtime!=='dsh'||typeof plugin!=='string'||!plugin.trim()||plugin.length>64||/[\u0000-\u001f\u007f]/.test(plugin))throw Error('incompatible profile probe');
}
// A display reason never changes installation, health or verified capabilities.
// Foreign OS errors retain their exact reason and get context only, not a claim
// that the OS diagnostic itself was translated.
// These are read-only observations, never Go Runner inventory, model health,
// executable admission, or proof of a coding run. execution_enabled stays false.
export async function inspectAdapters(locale?:string):Promise<AdapterInfo[]>{
 const selectedLocale=requireNativeLocale(locale),result:AdapterInfo[]=[];
 for(const d of definitions){
  const file=await installed(d.binary);if(!file){result.push({id:d.id,installed:false,operations:[],verified:[],reason:'CLI not found on PATH',display:nativeDisplay('native.catalog.not_found',selectedLocale)});continue;}
  let version:string|undefined;const verified:string[]=[];
  try{
   version=(await probe(file,['--version'])).slice(0,256);verified.push('version');
   if(d.id==='harness'){
    try{validateHarnessProbe(await probe(file,d.help,true));}catch{throw new ProbeError('Harness multica runtime profile is unavailable or incompatible','native.catalog.requirements_mismatch');}
    verified.push('multica_profile_probe');result.push({id:d.id,installed:true,executable:file,version,operations:d.operations,verified});continue;
   }
   const help=await probe(file,d.help);verified.push('help');const compatible=d.required.every(x=>help.includes(x));
   result.push({id:d.id,installed:true,executable:file,version,operations:compatible?d.operations:[],verified,...(compatible?{}:{reason:'Installed CLI help does not match adapter requirements',display:nativeDisplay('native.catalog.requirements_mismatch',selectedLocale)})});
  }catch(error){result.push({id:d.id,installed:true,executable:file,...(version!==undefined?{version}:{}),operations:[],verified,reason:error instanceof Error?error.message:'CLI probe failed',display:nativeDisplay(error instanceof ProbeError?error.displayKey:'native.catalog.probe_failed',selectedLocale,error instanceof ProbeError?error.displayParams:{})});}
 }
 return result;
}
