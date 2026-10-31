'use strict';
const crypto=require('node:crypto'),path=require('node:path');
const {HASH,ID,fields,text,strictJSON}=require('./local-acp-validation.cjs');
const reserved=new Set(['CODEX_HOME','HOME','USERPROFILE','PATH','APPDATA','LOCALAPPDATA','TEMP','TMP','TMPDIR','SYSTEMROOT','WINDIR','COMSPEC','PATHEXT','NODE_OPTIONS','NODE_PATH']);
const credentialRef=v=>typeof v==='string'&&/^[A-Za-z_][A-Za-z0-9_]{0,127}$/.test(v)&&!v.toUpperCase().startsWith('SUMA_')&&!reserved.has(v.toUpperCase());
const goJSON=value=>JSON.stringify(value).replace(/[<>&\u2028\u2029]/g,c=>'\\u'+c.charCodeAt(0).toString(16).padStart(4,'0'));
function validProviderConfiguration(v,platform=process.platform){
 if(!fields(v,['provider_id','base_url'],['api_key_env_ref','windows_sandbox'])||typeof v.provider_id!=='string'||!/^[A-Za-z][A-Za-z0-9_-]{0,63}$/.test(v.provider_id)||!text(v.base_url,2048)||Object.hasOwn(v,'api_key_env_ref')&&!credentialRef(v.api_key_env_ref)||Object.hasOwn(v,'windows_sandbox')&&!['unelevated','elevated','mxc'].includes(v.windows_sandbox)||platform==='win32'&&!Object.hasOwn(v,'windows_sandbox'))return false;
 try{const u=new URL(v.base_url);return !!u.hostname&&!u.username&&!u.password&&!u.search&&!v.base_url.includes('?')&&!u.hash&&!v.base_url.includes('#')&&(u.protocol==='https:'||u.protocol==='http:'&&['127.0.0.1','localhost','[::1]'].includes(u.hostname));}catch{return false;}
}
function provider(v){return {provider_id:v.provider_id,base_url:v.base_url,...(Object.hasOwn(v,'api_key_env_ref')?{api_key_env_ref:v.api_key_env_ref}:{}),...(Object.hasOwn(v,'windows_sandbox')?{windows_sandbox:v.windows_sandbox}:{})};}
function configurationDigest(v){if(!validProviderConfiguration(provider(v)))throw Error('Invalid Codex provider configuration');return crypto.createHash('sha256').update(goJSON({version:3,approval_policy:'on-request',provider:provider(v)})).digest('hex');}
function validConfiguration(v){return fields(v,['executable','executable_sha256','reviewed_version','provider_id','base_url','configuration_sha256'],['api_key_env_ref','windows_sandbox'])&&text(v.executable,4096)&&path.isAbsolute(v.executable)&&HASH.test(v.executable_sha256)&&text(v.reviewed_version,256)&&validProviderConfiguration(provider(v))&&v.configuration_sha256===configurationDigest(v);}
const identifier=(v,optional=false)=>typeof v==='string'&&(optional&&v===''||/^[A-Za-z0-9][A-Za-z0-9_.:/@+\-]{0,127}$/.test(v));
function validCatalog(v){
 if(!fields(v,['protocol','configuration_sha256','models','catalog_sha256'])||v.protocol!=='codex-app-server-v1'||!HASH.test(v.configuration_sha256)||!HASH.test(v.catalog_sha256)||!Array.isArray(v.models)||!v.models.length||v.models.length>256)return false;
 let previous='';for(const m of v.models){if(!fields(m,['model','reasoning_efforts','default_reasoning_effort'])||!identifier(m.model)||m.model<=previous||!Array.isArray(m.reasoning_efforts)||m.reasoning_efforts.length>32||!identifier(m.default_reasoning_effort,true))return false;previous=m.model;let before='';for(const e of m.reasoning_efforts){if(!identifier(e)||e<=before)return false;before=e;}if(m.reasoning_efforts.length?!m.reasoning_efforts.includes(m.default_reasoning_effort):m.default_reasoning_effort!=='')return false;}
 const canonical={protocol:v.protocol,configuration_sha256:v.configuration_sha256,models:v.models.map(m=>({model:m.model,reasoning_efforts:m.reasoning_efforts,default_reasoning_effort:m.default_reasoning_effort})),catalog_sha256:''};return crypto.createHash('sha256').update(goJSON(canonical)).digest('hex')===v.catalog_sha256;
}
const errors=new Set(['codex_discovery_invalid','codex_discovery_unknown','codex_discovery_installation_changed','codex_discovery_credential_unavailable','codex_discovery_catalogue_unavailable','codex_discovery_not_stopped','codex_discovery_failed']);
function validDiscovery(v){
 if(!fields(v,['protocol','state','executable_sha256','tool_version','configuration_sha256'],['catalogue','error_code'])||v.protocol!=='suma-codex-model-discovery-v1'||!HASH.test(v.executable_sha256)||!text(v.tool_version,256)||!HASH.test(v.configuration_sha256))return false;
 return v.state==='ready'?!Object.hasOwn(v,'error_code')&&validCatalog(v.catalogue)&&v.catalogue.configuration_sha256===v.configuration_sha256:['failed','unknown'].includes(v.state)&&!Object.hasOwn(v,'catalogue')&&errors.has(v.error_code);
}
module.exports={HASH,ID,fields,text,strictJSON,credentialRef,provider,validProviderConfiguration,configurationDigest,validConfiguration,validCatalog,validDiscovery};
