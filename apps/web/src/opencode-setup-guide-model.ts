import {wellFormed} from './i18n/catalog';

export type OpenCodeSetupInput={node:string;bundle:string;manifest:string;executable:string;provider:string;model:string;endpoint:string;credential:string;checks:string};
const fields=['node','bundle','manifest','executable','provider','model','endpoint','credential','checks'] as const;
const encoder=new TextEncoder();
const text=(value:string,max:number)=>!!value&&value.trim()===value&&wellFormed(value)&&encoder.encode(value).length<=max&&!/[\p{Cc}\u2028\u2029]/u.test(value);
const absolute=(value:string)=>text(value,4096)&&(/^[A-Za-z]:[\\/]/.test(value)||/^\/(?!\/)/.test(value)||/^\\\\[^\\/]+[\\/][^\\/]+[\\/]/.test(value));
const native=(value:string)=>absolute(value)&&! /\.(?:cmd|bat|ps1|sh|js|mjs|cjs|json)$/i.test(value);
const reserved=new Set(['HOME','USERPROFILE','APPDATA','LOCALAPPDATA','PATH','SYSTEMROOT','WINDIR','PROGRAMDATA','TEMP','TMP','TMPDIR']);

/** Local administrator draft only; no file, SDK or provider observation. */
export function openCodeSetupConfiguration(value:unknown):Record<string,unknown>|undefined{
 if(!value||typeof value!=='object'||Array.isArray(value)||(Object.getPrototypeOf(value)!==Object.prototype&&Object.getPrototypeOf(value)!==null))return;
 const row=value as Record<string,unknown>;
 if(Reflect.ownKeys(row).length!==fields.length||!fields.every(key=>Object.hasOwn(row,key)&&typeof row[key]==='string'))return;
 const input=row as OpenCodeSetupInput;
 if(!native(input.node)||!absolute(input.bundle)||!/\.mjs$/i.test(input.bundle)||!absolute(input.manifest)||!/\.json$/i.test(input.manifest)||!native(input.executable)||!/^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$/.test(input.provider)||!/^[A-Za-z0-9][A-Za-z0-9._:/@+\-]{0,255}$/.test(input.model)||!text(input.endpoint,2048)||!/^https?:\/\//.test(input.endpoint))return;
 try{
  const url=new URL(input.endpoint),authority=input.endpoint.slice(input.endpoint.indexOf('://')+3).split('/')[0];
  if(!url.hostname||url.username||url.password||authority.includes('@')||input.endpoint.includes('?')||input.endpoint.includes('#')||(url.protocol!=='https:'&&!(url.protocol==='http:'&&['localhost','127.0.0.1','[::1]'].includes(url.hostname))))return;
 }catch{return;}
 if(input.credential&&(!/^[A-Z_][A-Z0-9_]{0,127}$/.test(input.credential)||/^(?:SUMA_|OPENCODE|NODE_|NPM_)/.test(input.credential)||reserved.has(input.credential)))return;
 if(input.checks.length>2048||!wellFormed(input.checks))return;
 const checks=input.checks.trim()?input.checks.split(',').map(id=>id.trim()):[];
 if(checks.length>16||new Set(checks).size!==checks.length||checks.some(id=>!/^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$/.test(id)||id==='opencode-coder'))return;
 return {'opencode-coder':{executable:input.node,args:[input.bundle],adapter:'opencode',protocol:'opencode-sdk-v1',input:'opencode-sdk-jsonl',read_only:false,native_workspace:{checks,runtime_manifest:input.manifest},opencode:{executable:input.executable,provider_id:input.provider,model:input.model,base_url:input.endpoint,...(input.credential?{api_key_env_ref:input.credential}:{})}}};
}
