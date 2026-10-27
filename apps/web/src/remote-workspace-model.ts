import type {WorkspaceObservation,WorkspaceObservationArguments,WorkspaceObservationScope,WorkspaceObservationTargetPage} from '../../../packages/contracts/src';

export const remoteScopeKey=(scope:WorkspaceObservationScope)=>JSON.stringify([scope.run_id,scope.node_id,scope.attempt,scope.fence,scope.worker_id,scope.tool_profile_id,scope.profile_sha256]);
const id=(v:unknown):v is string=>typeof v==='string'&&/^[A-Za-z0-9][A-Za-z0-9_.:-]{0,149}$/.test(v);
const sha=(v:unknown):v is string=>typeof v==='string'&&/^[a-f0-9]{64}$/.test(v);
const integer=(v:unknown,min=0):v is number=>typeof v==='number'&&Number.isSafeInteger(v)&&v>=min;
export const remotePath=(v:unknown,empty=false):v is string=>typeof v==='string'&&(empty&&v===''||v.length>0&&new TextEncoder().encode(v).length<=1024&&!/[\\:\u0000-\u001f\u007f]/.test(v)&&!v.startsWith('/')&&v.split('/').every(part=>part!==''&&part!=='.'&&part!=='..'&&!part.endsWith('.')&&!part.endsWith(' ')));
export function validRemoteScope(v:WorkspaceObservationScope):boolean{return !!v&&id(v.run_id)&&id(v.node_id)&&id(v.worker_id)&&id(v.tool_profile_id)&&integer(v.attempt,1)&&integer(v.fence,1)&&sha(v.profile_sha256);}
export function validRemoteTargets(v:WorkspaceObservationTargetPage,project:string):boolean{return !!v&&v.project_id===project&&Array.isArray(v.items)&&v.items.length<=50&&integer(v.total)&&integer(v.limit,1)&&typeof v.truncated==='boolean'&&Number.isFinite(Date.parse(v.observed_at))&&v.items.every(item=>validRemoteScope(item)&&typeof item.run_name==='string'&&typeof item.node_name==='string'&&typeof item.can_write==='boolean');}
export const remoteArgumentsKey=(kind:string,args:WorkspaceObservationArguments,expected='')=>JSON.stringify([kind,args.directory||'',args.limit||100,args.cursor||'',args.path||'',args.start_line||1,args.max_lines||200,!!args.full_file,expected]);
/** Responses must match the exact current project, execution and requested read. */
export function validRemoteObservation(v:WorkspaceObservation,project:string,scope:WorkspaceObservationScope,kind:string,args:WorkspaceObservationArguments,expected='',requestID?:string,metadata=false):boolean{
 if(!v||!id(v.id)||requestID&&v.id!==requestID||v.project_id!==project||!integer(v.version,1)||!validRemoteScope(v.scope)||remoteScopeKey(v.scope)!==remoteScopeKey(scope)||v.operation!==kind||remoteArgumentsKey(v.operation,v.arguments,v.expected_sha256||'')!==remoteArgumentsKey(kind,args,expected)||!['requested','leased','completed','failed','expired','cancelled'].includes(v.state)||!Number.isFinite(Date.parse(v.expires_at)))return false;
 if(v.state!=='completed'||metadata)return !v.result||metadata;
 const result=v.result;if(!result||!sha(v.workspace_policy_sha256))return false;
 if(kind==='list'){const list=result.list;return !!list&&!result.read&&Array.isArray(list.entries)&&list.entries.length<=200&&typeof list.next_cursor==='string'&&list.next_cursor.length<=4096&&list.complete===(list.next_cursor==='')&&list.entries.every(entry=>remotePath(entry.path)&&['file','directory'].includes(entry.kind));}
 const read=result.read;return !!read&&!result.list&&read.path===args.path&&remotePath(read.path)&&typeof read.content==='string'&&new TextEncoder().encode(read.content).length<=48*1024&&sha(read.sha256)&&integer(read.bytes)&&read.bytes<=1024*1024&&read.start_line===(args.start_line||1)&&typeof read.complete==='boolean'&&(expected===''||read.sha256===expected);
}

export function remoteFileLanguage(path:string){const extension=path.split('.').pop()?.toLowerCase();return ({ts:'typescript',tsx:'typescript',js:'javascript',jsx:'javascript',go:'go',py:'python',json:'json',md:'markdown',css:'css',html:'html',rs:'rust',java:'java',yaml:'yaml',yml:'yaml'} as Record<string,string>)[extension||'']||'plaintext';}
