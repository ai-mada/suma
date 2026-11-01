'use strict';
const path=require('node:path');
const HASH=/^[a-f0-9]{64}$/,ID=/^[a-f0-9]{32}$/;
const ACP_ADAPTERS=Object.freeze({'qoder-acp':Object.freeze({runtime:'qoder',protocol:'acp-stdio-v1',authStrategy:'existing_cli_login',product:'Qoder CLI'}),'cursor-cli':Object.freeze({runtime:'cursor',protocol:'acp-stdio-v1',authStrategy:'explicit_api_key_env',product:'Cursor CLI'})});
const acpAdapter=id=>typeof id==='string'&&Object.hasOwn(ACP_ADAPTERS,id)?ACP_ADAPTERS[id]:undefined;
const plain=v=>!!v&&typeof v==='object'&&!Array.isArray(v);
const fields=(v,required,optional=[])=>plain(v)&&required.every(k=>Object.hasOwn(v,k))&&Object.keys(v).every(k=>required.includes(k)||optional.includes(k));
const text=(v,max)=>typeof v==='string'&&v.length>0&&v.trim()===v&&Buffer.byteLength(v)<=max&&!/[\p{Cc}\u2028\u2029\uD800-\uDFFF]/u.test(v);
const reservedPrefixes=['SUMA_','CODEX','OPENAI','ANTHROPIC','CLAUDE','AZURE_','GEMINI','GOOGLE_','QWEN','DASHSCOPE','DEEPSEEK','OPENCODE','DSH_','WORKBUDDY','CODEBUDDY','AWS_','HUGGINGFACE','HF_','GIT_','NODE_','PYTHON'];
const reserved=new Set(['HOME','USERPROFILE','PATH','APPDATA','LOCALAPPDATA','TEMP','TMP','TMPDIR','COMSPEC','SYSTEMROOT','WINDIR','PATHEXT','HOMEDRIVE','HOMEPATH','SYSTEMDRIVE','PROGRAMDATA','PROGRAMFILES','PUBLIC','LANG','GOFLAGS','GOCACHE','GOTMPDIR']);
const credentialRef=v=>typeof v==='string'&&/^[A-Z_][A-Z0-9_]{0,127}$/.test(v)&&!reserved.has(v)&&!reservedPrefixes.some(p=>v.startsWith(p));
function validExecutable(value,adapterID='qoder-acp'){const adapter=acpAdapter(adapterID);return !!adapter&&text(value,4096)&&path.isAbsolute(value)&&(adapter.runtime!=='cursor'||['node','node.exe'].includes(path.basename(value).toLowerCase()));}
function validConfiguration(v,adapterID='qoder-acp'){const adapter=acpAdapter(adapterID);return !!adapter&&fields(v,['executable','executable_sha256','reviewed_version','model','configuration_sha256'],['credential_env_ref'])&&validExecutable(v.executable,adapterID)&&HASH.test(v.executable_sha256)&&text(v.reviewed_version,256)&&text(v.model,256)&&HASH.test(v.configuration_sha256)&&(!Object.hasOwn(v,'credential_env_ref')||credentialRef(v.credential_env_ref))&&(adapter.authStrategy!=='explicit_api_key_env'||credentialRef(v.credential_env_ref));}
const errors=new Set(['acp_discovery_invalid','acp_discovery_unknown','acp_discovery_unexpected_activity','acp_discovery_model_unavailable','acp_discovery_not_stopped','acp_discovery_failed']);
function validDiscovery(v,adapterID='qoder-acp'){
 const adapter=acpAdapter(adapterID);
 if(!adapter||!fields(v,['protocol','runtime','state','executable_sha256','tool_version','evidence'],['agent_name','agent_version','catalogue','error_code'])||v.protocol!=='suma-acp-model-discovery-v1'||v.runtime!==adapter.runtime||!HASH.test(v.executable_sha256)||!text(v.tool_version,256)||['agent_name','agent_version'].some(k=>Object.hasOwn(v,k)&&!text(v[k],256)))return false;
 const e=v.evidence,booleans=['session_created','session_close_requested','session_close_acknowledged','host_process_stopped','host_process_tree_stopped'],numbers=['prompt_requests','configuration_writes','permission_requests','unexpected_activity'];
 if(!fields(e,[...booleans,...numbers])||booleans.some(k=>typeof e[k]!=='boolean')||numbers.some(k=>!Number.isSafeInteger(e[k])||e[k]<0||e[k]>(k==='permission_requests'?32:4096))||e.host_process_tree_stopped&&!e.host_process_stopped||e.session_close_acknowledged&&!e.session_close_requested||e.session_close_requested&&!e.session_created)return false;
 if(v.state!=='ready')return ['failed','unknown'].includes(v.state)&&!Object.hasOwn(v,'catalogue')&&errors.has(v.error_code);
 if(Object.hasOwn(v,'error_code')||!e.session_created||!e.host_process_stopped||!e.host_process_tree_stopped||numbers.some(k=>e[k]!==0)||e.session_close_requested&&!e.session_close_acknowledged)return false;
 const c=v.catalogue;
 if(!fields(c,['source','protocol_version','config_id','current_value','models'])||c.source!=='acp_session_config'||c.protocol_version!==1||!text(c.config_id,128)||typeof c.current_value!=='string'||!Array.isArray(c.models)||c.models.length<1||c.models.length>256)return false;
 const seen=new Set();for(const m of c.models){if(!fields(m,['id','name','configuration_sha256'])||!text(m.id,256)||!text(m.name,4096)||!HASH.test(m.configuration_sha256)||seen.has(m.id))return false;seen.add(m.id);}
 return c.current_value===''||text(c.current_value,256);
}
// Strict bounded JSON before interpreting a child-process result. Checking
// duplicate keys here prevents JSON.parse's last-key-wins from hiding facts.
function strictJSON(raw){
 const source=new TextDecoder('utf-8',{fatal:true}).decode(raw);let at=0;
 function ws(){while(/[\t\n\r ]/.test(source[at]||'x'))at++;}
 function string(){const start=at++;while(at<source.length){const c=source[at++];if(c==='"')return JSON.parse(source.slice(start,at));if(c==='\\')at++;}throw Error('json');}
 function value(depth){if(depth>32)throw Error('json');ws();const c=source[at];if(c==='"'){string();return;}if(c==='{'){at++;ws();const seen=new Set();if(source[at]==='}'){at++;return;}for(;;){ws();if(source[at]!=='"')throw Error('json');const k=string();if(seen.has(k))throw Error('json');seen.add(k);ws();if(source[at++]!==':')throw Error('json');value(depth+1);ws();const end=source[at++];if(end==='}')return;if(end!==',')throw Error('json');}}if(c==='['){at++;ws();if(source[at]===']'){at++;return;}for(;;){value(depth+1);ws();const end=source[at++];if(end===']')return;if(end!==',')throw Error('json');}}const m=/^(?:true|false|null|-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?)/.exec(source.slice(at));if(!m)throw Error('json');at+=m[0].length;}
 value(0);ws();if(at!==source.length)throw Error('json');return JSON.parse(source);
}
module.exports={HASH,ID,ACP_ADAPTERS,acpAdapter,plain,fields,text,credentialRef,validExecutable,validConfiguration,validDiscovery,strictJSON};
