import {lstat,readFile,readdir,mkdir,realpath} from 'node:fs/promises';
import {isAbsolute,resolve,join,relative,sep} from 'node:path';
import {createHash} from 'node:crypto';
import type {Options} from '@anthropic-ai/claude-agent-sdk';
import {CLI_VERSION,SDK_VERSION,RuntimeError,closed,required,digest,type RuntimeConfig,TOOL_NAMES} from './contract.js';

export function parseConfig(value:unknown):RuntimeConfig{
 const keys=['native_cli_executable','native_cli_sha256','home','source','model','credential_env_ref','configuration_sha256','sdk_version','cli_version'];closed(value,keys);required(value,keys);
 for(const key of ['native_cli_executable','home','source'])if(typeof value[key]!=='string'||!isAbsolute(value[key] as string)||/[\x00\r\n]/.test(value[key] as string)||resolve(value[key] as string)!==value[key])throw new RuntimeError('configuration_invalid');
 if(!digest(value.native_cli_sha256)||!digest(value.configuration_sha256)||value.sdk_version!==SDK_VERSION||value.cli_version!==CLI_VERSION||typeof value.model!=='string'||value.model.length>256||!/^claude-[a-z][a-z0-9-]*[0-9][a-z0-9.-]*$/.test(value.model)||typeof value.credential_env_ref!=='string'||!/^[A-Z][A-Z0-9_]{0,127}$/.test(value.credential_env_ref)||/^(?:SUMA_|HOME$|PATH$|TEMP$|TMP$|NODE_|CLAUDE_|ANTHROPIC_AUTH_TOKEN$)/.test(value.credential_env_ref))throw new RuntimeError('configuration_invalid');
 const homeRelative=relative(value.source as string,value.home as string);
 if(value.home===value.source||homeRelative!=='..'&&!homeRelative.startsWith('..'+sep)&&!isAbsolute(homeRelative))throw new RuntimeError('configuration_invalid');
 return value as unknown as RuntimeConfig;
}
export async function verifyExecutable(config:RuntimeConfig){const stat=await lstat(config.native_cli_executable);if(!stat.isFile()||stat.isSymbolicLink()||createHash('sha256').update(await readFile(config.native_cli_executable)).digest('hex')!==config.native_cli_sha256)throw new RuntimeError('executable_changed');}
export async function prepare(config:RuntimeConfig,environment:NodeJS.ProcessEnv=process.env){
 const source=await lstat(config.source);if(!source.isDirectory()||source.isSymbolicLink()||await realpath(config.source)!==resolve(config.source))throw new RuntimeError('workspace_source');await verifyExecutable(config);
 try{const home=await lstat(config.home);if(!home.isDirectory()||home.isSymbolicLink()||(await readdir(config.home)).length!==0)throw new RuntimeError('home_not_empty');}catch(error){if((error as NodeJS.ErrnoException).code!=='ENOENT')throw error;await mkdir(config.home,{recursive:true});}
 const credential=environment[config.credential_env_ref];if(typeof credential!=='string'||credential.length<1||credential.length>32768||/[\x00\r\n]/.test(credential))throw new RuntimeError('credential_unavailable');
 // A fresh process environment prevents ambient Runner tokens, OAuth paths,
 // proxy endpoints and SDK configuration from leaking into the vendor child.
 const env:Record<string,string>={};for(const name of ['SystemRoot','WINDIR','COMSPEC','PATHEXT','PATH','LANG','LC_ALL'])if(environment[name])env[name]=environment[name]!;
 Object.assign(env,{HOME:config.home,USERPROFILE:config.home,APPDATA:join(config.home,'appdata'),LOCALAPPDATA:join(config.home,'localappdata'),TEMP:join(config.home,'tmp'),TMP:join(config.home,'tmp'),TMPDIR:join(config.home,'tmp'),XDG_CONFIG_HOME:join(config.home,'config'),XDG_CACHE_HOME:join(config.home,'cache'),XDG_DATA_HOME:join(config.home,'data'),XDG_STATE_HOME:join(config.home,'state'),CLAUDE_CONFIG_DIR:join(config.home,'.claude'),ANTHROPIC_API_KEY:credential,DISABLE_UPDATES:'1',CLAUDE_CODE_DISABLE_BACKGROUND_TASKS:'1',CLAUDE_CODE_DISABLE_AGENT_VIEW:'1',CLAUDE_CODE_DISABLE_CRON:'1',CLAUDE_CODE_DISABLE_AUTO_MEMORY:'1',CLAUDE_CODE_DISABLE_CLAUDE_MDS:'1',CLAUDE_CODE_DISABLE_ARTIFACT:'1',CLAUDE_CODE_DISABLE_BUNDLED_SKILLS:'1',CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC:'1'});
 for(const dir of ['APPDATA','LOCALAPPDATA','TEMP','XDG_CONFIG_HOME','XDG_CACHE_HOME','XDG_DATA_HOME','XDG_STATE_HOME','CLAUDE_CONFIG_DIR'])await mkdir(env[dir],{recursive:true});
 return{env,credential};
}
/** Every option is host-owned; there is no task-controlled option spread. The
 * SDK owns its native control channel and the explicit external CLI path. */
export function sdkOptions(config:RuntimeConfig,env:Record<string,string>,abortController:AbortController,canUseTool:NonNullable<Options['canUseTool']>):Options{
 return{pathToClaudeCodeExecutable:config.native_cli_executable,cwd:config.source,env,model:config.model,permissionMode:'default',tools:[...TOOL_NAMES],allowedTools:[],canUseTool,abortController,persistSession:false,settingSources:[],mcpServers:{},strictMcpConfig:true,plugins:[],agents:{},hooks:{},includePartialMessages:false,settings:{disableAllHooks:true,permissions:{blockReadsOutsideWorkingDirectories:true,blockWritesOutsideWorkingDirectories:true}},extraArgs:{'bare':null,'safe-mode':null,'restricted':null},stderr:()=>{}};
}
