import type {DesktopToolConnection} from './desktop-tool-connector-model';
import {validClaudeSDKCapability} from './claude-sdk-execution-model';
export type DesktopClaudeSDKConfiguration={model:string;credential_env_ref:string};
export type DesktopClaudeSDKSetup=DesktopClaudeSDKConfiguration&{executable_sha256:string};
export type DesktopClaudeSDKConnectedProof=Required<Pick<DesktopToolConnection,'worker_id'|'model'|'credential_env_ref'|'tool_version'|'executable_sha256'|'runtime_executable_sha256'|'configuration_sha256'|'claude_sdk_execution'>>;
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value)&&[Object.prototype,null].includes(Object.getPrototypeOf(value));
const closed=(value:Record<string,unknown>,keys:readonly string[])=>keys.every(key=>Object.hasOwn(value,key))&&Reflect.ownKeys(value).every(key=>typeof key==='string'&&keys.includes(key));
const hash=(value:unknown):value is string=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
// These are the existing SDK administrator constraints, not arbitrary process
// environment assignment. Only a name crosses the renderer boundary.
const reserved=new Set(['HOME','USERPROFILE','APPDATA','LOCALAPPDATA','PATH','SYSTEMROOT','WINDIR','COMSPEC','PROGRAMDATA','TEMP','TMP','TMPDIR','ANTHROPIC_AUTH_TOKEN','LANG','LC_ALL','PATHEXT','SYSTEMDRIVE','ALLUSERSPROFILE','PROGRAMFILES','PROGRAMW6432','COMMONPROGRAMFILES','COMMONPROGRAMW6432','HOMEDRIVE','HOMEPATH','PUBLIC','OS']);
export function validDesktopClaudeSDKModel(value:unknown):value is string{
 return typeof value==='string'&&value.length<=256&&/^claude-[a-z][a-z0-9-]*[0-9][a-z0-9.-]*$/.test(value);
}
export function validDesktopClaudeSDKCredentialEnvRef(value:unknown):value is string{
 return typeof value==='string'&&/^[A-Z][A-Z0-9_]{0,127}$/.test(value)&&!['SUMA_','NODE_','NPM_','CLAUDE_','LC_','LD_','DYLD_'].some(prefix=>value.startsWith(prefix))&&!reserved.has(value);
}
export function validDesktopClaudeSDKConfiguration(value:unknown):value is DesktopClaudeSDKConfiguration{
 if(!object(value)||!closed(value,['model','credential_env_ref']))return false;
 return validDesktopClaudeSDKModel(value.model)&&validDesktopClaudeSDKCredentialEnvRef(value.credential_env_ref);
}
export function validDesktopClaudeSDKSetup(value:unknown):value is DesktopClaudeSDKSetup{
 if(!object(value)||!closed(value,['executable_sha256','model','credential_env_ref'])||!hash(value.executable_sha256))return false;
 return validDesktopClaudeSDKConfiguration({model:value.model,credential_env_ref:value.credential_env_ref});
}
export function makeDesktopClaudeSDKSetup(configuration:unknown,executableSHA:string):DesktopClaudeSDKSetup|undefined{
 if(!validDesktopClaudeSDKConfiguration(configuration)||!hash(executableSHA))return;
 return {...configuration,executable_sha256:executableSHA};
}
/** The trusted desktop response echoes the original persisted variable name,
 * never its value. A current status can therefore prove an exact original
 * configuration after reload or a lost ACK without guessing from the draft. */
export function matchesDesktopClaudeSDKSetup(setup:unknown,connection:unknown,connectedOnly=true):boolean{
 if(!validDesktopClaudeSDKSetup(setup)||!object(connection)||connection.adapter_id!=='claude-code-sdk'||typeof connection.worker_id!=='string'||!/^desktop-[a-f0-9]{32}$/.test(connection.worker_id)||!(connectedOnly?connection.phase==='connected':['prepared','starting','connected'].includes(String(connection.phase)))||connection.tool_version!=='2.1.314'||connection.model!==setup.model||connection.credential_env_ref!==setup.credential_env_ref||connection.executable_sha256!==setup.executable_sha256||!hash(connection.runtime_executable_sha256)||!hash(connection.configuration_sha256)||!validClaudeSDKCapability(connection.claude_sdk_execution))return false;
 return connection.claude_sdk_execution.model===setup.model&&connection.claude_sdk_execution.configuration_sha256===connection.configuration_sha256;
}
