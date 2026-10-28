import type {WorkspaceTreeEntry,WorkspaceTreePage} from '../../../packages/contracts/src';
import {UiError} from './i18n';
import {wellFormed} from './i18n/catalog';

export const workspaceTreeLimit=100;
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const bytes=(value:string)=>new TextEncoder().encode(value).length;
const plainText=(value:unknown):value is string=>typeof value==='string'&&wellFormed(value)&&!/[\u0000-\u001f\u007f-\u009f]/.test(value);
export function workspaceDirectory(value:unknown):value is string {
 return plainText(value)&&bytes(value)<=1024&&!/[\\:]/.test(value)&&(value===''||!value.startsWith('/')&&!value.endsWith('/')&&value.split('/').every(part=>!!part&&part!=='.'&&part!=='..'));
}
export function workspaceParent(directory:string){return directory.split('/').slice(0,-1).join('/');}
export function workspaceBreadcrumbs(directory:string){return directory?directory.split('/').map((name,index,parts)=>({name,path:parts.slice(0,index+1).join('/')})):[];}
export function workspaceTreePath(project:string,directory:string,before='') {
 if(!workspaceDirectory(directory)||!plainText(before)||bytes(before)>1024)throw new UiError('workspace-tree.invalid_response');
 return '/workspace/tree?'+new URLSearchParams({project_id:project,directory,limit:String(workspaceTreeLimit),...(before?{before}:{})});
}
export function validWorkspaceTreePage(value:unknown,project:string,directory:string,before=''):value is WorkspaceTreePage {
 if(!object(value)||Object.keys(value).some(key=>!['project_id','directory','entries','limit','next_cursor','snapshot_sha256','observed_at'].includes(key))||value.project_id!==project||value.directory!==directory||!workspaceDirectory(value.directory)||value.limit!==workspaceTreeLimit||!Array.isArray(value.entries)||value.entries.length>workspaceTreeLimit||!plainText(value.next_cursor)||bytes(value.next_cursor)>1024||!!before&&value.next_cursor===before||typeof value.snapshot_sha256!=='string'||!/^[a-f0-9]{64}$/.test(value.snapshot_sha256)||typeof value.observed_at!=='string'||!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(value.observed_at)||!Number.isFinite(Date.parse(value.observed_at)))return false;
 const seen=new Set<string>();
 for(const entry of value.entries){
  if(!object(entry)||Object.keys(entry).some(key=>!['path','name','kind'].includes(key))||!workspaceDirectory(entry.path)||!entry.path||entry.name!==entry.path.split('/').at(-1)||workspaceParent(entry.path)!==directory||entry.kind!=='file'&&entry.kind!=='directory'||seen.has(entry.path))return false;
  seen.add(entry.path);
 }
 return true;
}
/** Merge only a page from the exact same observed directory snapshot. */
export function appendWorkspaceTreePage(previous:WorkspaceTreePage,next:WorkspaceTreePage,before:string):WorkspaceTreePage {
 if(!before||previous.next_cursor!==before||previous.project_id!==next.project_id||previous.directory!==next.directory)throw new UiError('workspace-tree.invalid_response');
 if(previous.snapshot_sha256!==next.snapshot_sha256)throw new UiError('workspace-tree.changed');
 const seen=new Set(previous.entries.map(entry=>entry.path));
 if(next.entries.some(entry=>seen.has(entry.path)))throw new UiError('workspace-tree.invalid_response');
 return {...next,entries:[...previous.entries,...next.entries]};
}
export function filterWorkspaceEntries(entries:WorkspaceTreeEntry[],filter:string){const wanted=filter.trim().toLocaleLowerCase();return wanted?entries.filter(entry=>entry.name.toLocaleLowerCase().includes(wanted)):entries;}
