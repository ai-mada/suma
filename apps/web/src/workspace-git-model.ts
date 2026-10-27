import type {WorkspaceGitChange,WorkspaceGitStatus,WorkspaceGitDiff} from '../../../packages/contracts/src';
/** Public DTOs come from the generated contract; only the UI source union is local. */
export type {WorkspaceGitStatus,WorkspaceGitDiff};
export type WorkspaceGitFile=WorkspaceGitChange;
export type WorkspaceGitSource='worktree'|'index';
const reasons=new Set(['not_repository','git_unavailable','unsupported_layout','unsafe_config']);
const gitStates=new Set([' ','M','A','D','R','C','U','T','?','!']);
const sha256=/^[a-f0-9]{64}$/;
const record=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const stamp=(value:unknown):value is string=>typeof value==='string'&&value.length<=64&&Number.isFinite(Date.parse(value));
const count=(value:unknown):value is number=>Number.isSafeInteger(value)&&Number(value)>=0;
// Mirror the base workspace's conservative path boundary for display safety.
// Server authorization and symlink checks remain authoritative; hiding paths
// here cannot grant file access or make a restricted response acceptable.
const restrictedParts=new Set(['.git','.suma','.cache','node_modules','.aws','.ssh','.pnpm-store','.codex','.agents','.kube','.azure','.config','.docker','.npmrc','.pypirc','.netrc','.git-credentials','credentials','credentials.json','secrets','secrets.json','id_rsa','id_ed25519']);
function relativePath(value:unknown):value is string{
 return typeof value==='string'&&value.length>0&&value.length<=4096&&new TextEncoder().encode(value).length<=4096&&!value.startsWith('/')&&!value.includes('\\')&&!value.includes(':')&&!/\p{Cc}/u.test(value)&&!value.split('/').some(raw=>{
  const part=raw.toLowerCase();
  return !part||part==='.'||part==='..'||part.endsWith('.')||part.endsWith(' ')||restrictedParts.has(part)||part.startsWith('.env')||part.startsWith('.suma-save-')||/\.(pem|key|p12|pfx)$/.test(part);
 });
}
export function validWorkspaceGitStatus(value:unknown,project:string):value is WorkspaceGitStatus{
 if(!record(value)||value.project_id!==project||typeof value.available!=='boolean'||typeof value.reason_code!=='string'||typeof value.branch!=='string'||typeof value.head!=='string'||typeof value.unborn!=='boolean'||typeof value.detached!=='boolean'||!Array.isArray(value.files)||!count(value.total)||!count(value.limit)||value.limit<1||value.limit>200||!count(value.restricted_files)||typeof value.truncated!=='boolean'||!stamp(value.observed_at))return false;
 if(value.limit!==200||value.files.length>value.limit||value.total<value.files.length||value.truncated!==(value.total>value.files.length))return false;
 if(!value.available)return reasons.has(value.reason_code)&&value.files.length===0&&value.branch===''&&value.head==='';
 if(value.reason_code!=='ready'||value.unborn&&value.detached||(value.detached?value.branch!=='':value.branch===''))return false;
 if(value.unborn?value.head!=='':!/^([a-f0-9]{40}|[a-f0-9]{64})$/.test(value.head))return false;
 const paths=new Set<string>();
 for(const file of value.files){
  if(!record(file)||!relativePath(file.path)||typeof file.previous_path!=='string'||file.previous_path!==''&&!relativePath(file.previous_path)||typeof file.index_status!=='string'||!gitStates.has(file.index_status)||typeof file.worktree_status!=='string'||!gitStates.has(file.worktree_status)||typeof file.untracked!=='boolean'||typeof file.conflicted!=='boolean'||paths.has(file.path))return false;
  paths.add(file.path);
 }
 return true;
}
export function gitSourceAvailable(file:WorkspaceGitFile,source:WorkspaceGitSource){
 return source==='index'?![' ','?','!'].includes(file.index_status):file.untracked||![' ','!'].includes(file.worktree_status);
}
export function preferredGitSource(file:WorkspaceGitFile):WorkspaceGitSource{return gitSourceAvailable(file,'worktree')?'worktree':'index';}
async function digest(content:string){return Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256',new TextEncoder().encode(content))),byte=>byte.toString(16).padStart(2,'0')).join('');}

/** Bind both text sides to the selected project/path/source and observed HEAD.
 * A mismatched or unknown 200 never becomes a code comparison. In particular,
 * an absent side is real empty text with its SHA256, not a missing receipt.
 * The service separately guards binding, HEAD and index changes with 409.
 */
export async function workspaceGitDiff(value:unknown,status:WorkspaceGitStatus,file:WorkspaceGitFile,source:WorkspaceGitSource):Promise<WorkspaceGitDiff|undefined>{
 if(!record(value)||value.project_id!==status.project_id||value.path!==file.path||value.previous_path!==file.previous_path||value.source!==source||value.head!==status.head||typeof value.binary!=='boolean'||!stamp(value.observed_at))return;
 const {original,modified,original_sha256,modified_sha256}=value;
 if(typeof original!=='string'||typeof modified!=='string'||typeof original_sha256!=='string'||typeof modified_sha256!=='string')return;
 const document:WorkspaceGitDiff={project_id:status.project_id,path:file.path,previous_path:file.previous_path,source,head:status.head,original,modified,original_sha256,modified_sha256,binary:value.binary,observed_at:value.observed_at};
 if(value.binary)return [original,modified,original_sha256,modified_sha256].every(text=>text==='')?document:undefined;
 if(!sha256.test(original_sha256)||!sha256.test(modified_sha256)||[original,modified].some(text=>text.length>1048576||new TextEncoder().encode(text).length>1048576))return;
 try{
  const [before,after]=await Promise.all([digest(original),digest(modified)]);
  return original_sha256===before&&modified_sha256===after?document:undefined;
 }catch{return;} // Missing secure hashing is an unverified observation, not text.
}
