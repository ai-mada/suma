import type {DesktopToolConnection} from './desktop-tool-connector-model';
import {wellFormed} from './i18n/catalog';
import {openCodeEndpointOrigin} from './opencode-execution-model';
export type DesktopOpenCodeConfiguration={provider_id:string;model:string;base_url:string;api_key_env_ref?:string};
export type DesktopOpenCodeSetup=DesktopOpenCodeConfiguration&{executable_sha256:string};
export type DesktopOpenCodeConnectedProof=Required<Pick<DesktopToolConnection,'worker_id'|'model'|'tool_version'|'executable_sha256'|'configuration_sha256'|'provider_id'|'endpoint_origin'>>;
const object=(v:unknown):v is Record<string,unknown>=>!!v&&typeof v==='object'&&!Array.isArray(v)&&(Object.getPrototypeOf(v)===Object.prototype||Object.getPrototypeOf(v)===null);
const closed=(v:Record<string,unknown>,required:string[],optional:string[]=[])=>required.every(k=>Object.hasOwn(v,k))&&Reflect.ownKeys(v).every(k=>typeof k==='string'&&(required.includes(k)||optional.includes(k)));
const hash=(v:unknown):v is string=>typeof v==='string'&&/^[a-f0-9]{64}$/.test(v);
const reserved=new Set(['HOME','USERPROFILE','APPDATA','LOCALAPPDATA','PATH','SYSTEMROOT','WINDIR','PROGRAMDATA','TEMP','TMP','TMPDIR']);
export function validDesktopOpenCodeConfiguration(value:unknown):value is DesktopOpenCodeConfiguration{
 if(!object(value)||!closed(value,['provider_id','model','base_url'],['api_key_env_ref'])||typeof value.provider_id!=='string'||!/^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$/.test(value.provider_id)||typeof value.model!=='string'||!/^[A-Za-z0-9][A-Za-z0-9._:/@+\-]{0,255}$/.test(value.model)||typeof value.base_url!=='string'||!wellFormed(value.base_url)||!value.base_url||value.base_url.trim()!==value.base_url||new TextEncoder().encode(value.base_url).length>2048||/[\p{Cc}\u2028\u2029?#]/u.test(value.base_url))return false;
 if(Object.hasOwn(value,'api_key_env_ref')&&(typeof value.api_key_env_ref!=='string'||!/^[A-Z_][A-Z0-9_]{0,127}$/.test(value.api_key_env_ref)||/^(SUMA_|OPENCODE|NODE_|NPM_)/.test(value.api_key_env_ref)||reserved.has(value.api_key_env_ref)))return false;
 return openCodeEndpointOrigin(value.base_url)!==undefined;
}
export function validDesktopOpenCodeSetup(value:unknown):value is DesktopOpenCodeSetup{
 if(!object(value)||!closed(value,['executable_sha256','provider_id','model','base_url'],['api_key_env_ref'])||!hash(value.executable_sha256))return false;
 const {executable_sha256:_,...configuration}=value;return validDesktopOpenCodeConfiguration(configuration);
}
export function makeDesktopOpenCodeSetup(configuration:unknown,executableSHA:string):DesktopOpenCodeSetup|undefined{
 if(!validDesktopOpenCodeConfiguration(configuration)||!hash(executableSHA))return;
 return {...configuration,executable_sha256:executableSHA};
}
/** Local comparison only: a scanned path never becomes a renderer launch grant. */
export async function openCodeConfigurationSHA(configuration:DesktopOpenCodeConfiguration,executable:string):Promise<string|undefined>{
 if(!validDesktopOpenCodeConfiguration(configuration)||typeof executable!=='string'||!wellFormed(executable)||executable.trim()!==executable||new TextEncoder().encode(executable).length>4096||/[\p{Cc}\u2028\u2029]/u.test(executable)||!(/^[A-Za-z]:[\\/]/.test(executable)||/^\/(?!\/)/.test(executable)||/^\\\\[^\\]+\\[^\\]+\\/.test(executable))||/^\\\\[?.]\\/.test(executable))return;
 try{
  const subtle=globalThis.crypto?.subtle;if(!subtle)return;
  const {provider_id,model,base_url,api_key_env_ref}=configuration;
  const raw=JSON.stringify({policy_version:1,sdk_version:'1.19.4',provider:{executable,provider_id,model,base_url,...(api_key_env_ref?{api_key_env_ref}:{})}}).replace(/[<>&\u2028\u2029]/g,value=>'\\u'+value.charCodeAt(0).toString(16).padStart(4,'0'));
  const digest=await subtle.digest('SHA-256',new TextEncoder().encode(raw));
  return Array.from(new Uint8Array(digest),value=>value.toString(16).padStart(2,'0')).join('');
 }catch{return;}
}
