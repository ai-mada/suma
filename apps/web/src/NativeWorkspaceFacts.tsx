import {useState} from 'react';
import type {WorkspaceEvidence} from './api';
import type {NativeWorkspaceEvidence as NativeEvidence,NativeWorkspaceFileEffect as FileEffect,NativeWorkspaceCheckResult as CheckResult,NativeWorkspaceSeal as Seal} from '../../../packages/contracts/src';
import {formatNumber,tr as t,useLocale} from './i18n';
import {wellFormed} from './i18n/catalog';
import './NativeWorkspaceFacts.css';

const object=(value:unknown):value is Record<string,unknown>=>value!==null&&typeof value==='object'&&!Array.isArray(value);
const closed=(value:Record<string,unknown>,required:string[],optional:string[]=[])=>required.every(key=>Object.hasOwn(value,key))&&Object.keys(value).every(key=>required.includes(key)||optional.includes(key));
const integer=(value:unknown):value is number=>typeof value==='number'&&Number.isSafeInteger(value);
const natural=(value:unknown):value is number=>integer(value)&&value>=0;
const hash=(value:unknown):value is string=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
const commit=(value:unknown):value is string=>typeof value==='string'&&/^[a-f0-9]{40}([a-f0-9]{24})?$/.test(value);
const id=(value:unknown):value is string=>typeof value==='string'&&/^[A-Za-z0-9_.:-]{1,128}$/.test(value);
const bytes=(value:string)=>new TextEncoder().encode(value).length;
function path(value:unknown):value is string{return typeof value==='string'&&wellFormed(value)&&bytes(value)>0&&bytes(value)<=4096&&!/[\\:\u0000-\u001f\u007f]/.test(value)&&!value.startsWith('/')&&value.split('/').every(part=>part!==''&&part!=='.'&&part!=='..');}
function file(value:unknown,generation:number):value is FileEffect{
 return object(value)&&closed(value,['path','operation_id','removed','generation'],['previous_sha256','sha256'])&&path(value.path)&&id(value.operation_id)&&typeof value.removed==='boolean'&&natural(value.generation)&&value.generation>0&&value.generation<=generation&&(!Object.hasOwn(value,'previous_sha256')||hash(value.previous_sha256))&&(value.removed?hash(value.previous_sha256)&&!Object.hasOwn(value,'sha256'):hash(value.sha256));
}
function check(value:unknown,generation:number):value is CheckResult{
 return object(value)&&closed(value,['operation_id','check_id','profile_sha256','generation','workspace_sha256','exit_code','stopped','outcome_unknown','output','output_truncated'])&&id(value.operation_id)&&id(value.check_id)&&hash(value.profile_sha256)&&natural(value.generation)&&value.generation<=generation&&hash(value.workspace_sha256)&&integer(value.exit_code)&&typeof value.stopped==='boolean'&&typeof value.outcome_unknown==='boolean'&&typeof value.output==='string'&&wellFormed(value.output)&&bytes(value.output)<=32768&&typeof value.output_truncated==='boolean';
}
function seal(value:unknown,policy:string):value is Seal{
 return object(value)&&closed(value,['schema_version','operation_id','policy_sha256','state','previous_commit','changed_paths'],['commit','tree'])&&value.schema_version===1&&id(value.operation_id)&&value.policy_sha256===policy&&['prepared','unknown','sealed','no_change'].includes(String(value.state))&&commit(value.previous_commit)&&natural(value.changed_paths)&&value.changed_paths<=2000&&(!Object.hasOwn(value,'commit')||commit(value.commit))&&(!Object.hasOwn(value,'tree')||commit(value.tree));
}
// Versioned display validation does not replace Runner or platform acceptance.
// Unknown fields are never dumped as JSON or promoted into development facts.
function native(value:unknown):value is NativeEvidence{
 if(!object(value)||!closed(value,['schema_version','policy_sha256','generation','state','files','checks'],['seal'])||value.schema_version!==1||!hash(value.policy_sha256)||!natural(value.generation)||!['open','closed','unknown'].includes(String(value.state))||!Array.isArray(value.files)||value.files.length>2000||!Array.isArray(value.checks)||value.checks.length>2048)return false;
 if(!value.files.every(item=>file(item,value.generation as number))||!value.checks.every(item=>check(item,value.generation as number)))return false;
 if(new Set(value.files.map(item=>(item as FileEffect).path.toLowerCase())).size!==value.files.length||new Set(value.checks.map(item=>(item as CheckResult).check_id)).size!==value.checks.length)return false;
 return value.seal===undefined||value.seal===null||seal(value.seal,value.policy_sha256);
}
const knownSuccess=(check:CheckResult,generation:number)=>check.generation===generation&&check.exit_code===0&&!check.stopped&&!check.outcome_unknown;
function checkState(check:CheckResult,generation:number){return check.outcome_unknown?'unknown':check.stopped?'stopped':check.generation!==generation?'earlier':check.exit_code!==0?'failed':'success';}
function Output({check}:{check:CheckResult}){
 const [open,setOpen]=useState(false);
 return <details className="native-workspace-output" onToggle={event=>setOpen(event.currentTarget.open)}><summary>{t('native-workspace.output')}</summary>{open&&<><pre tabIndex={0} aria-label={t('native-workspace.output')}>{check.output}</pre>{!check.output&&<p className="muted">{t('native-workspace.output_empty')}</p>}{check.output_truncated&&<p className="muted">{t('native-workspace.output_truncated')}</p>}</>}</details>;
}
function OriginalFacts({value}:{value:NativeEvidence}){
 return <details className="native-workspace-original"><summary>{t('native-workspace.original')}</summary><dl><dt>{t('native-workspace.policy')}</dt><dd><code>{value.policy_sha256}</code></dd><dt>{t('native-workspace.generation')}</dt><dd>{formatNumber(value.generation)}</dd><dt>{t('native-workspace.raw_state')}</dt><dd><code>{value.state}</code></dd></dl>
  {value.files.map(effect=><div className="native-workspace-record" key={effect.path}><h5><code>{effect.path}</code></h5><p>{t(effect.removed?'native-workspace.removed':'native-workspace.written')}</p><dl><dt>{t('native-workspace.operation')}</dt><dd><code>{effect.operation_id}</code></dd><dt>{t('native-workspace.generation')}</dt><dd>{formatNumber(effect.generation)}</dd>{effect.previous_sha256&&<><dt>{t('native-workspace.previous_sha')}</dt><dd><code>{effect.previous_sha256}</code></dd></>}{effect.sha256&&<><dt>{t('native-workspace.file_sha')}</dt><dd><code>{effect.sha256}</code></dd></>}</dl></div>)}
  {value.checks.map(item=><div className="native-workspace-record" key={item.check_id}><h5><code>{item.check_id}</code></h5><p>{value.state==='unknown'?t('native-workspace.checks_unknown'):t('native-workspace.check_'+checkState(item,value.generation))}</p><dl><dt>{t('native-workspace.operation')}</dt><dd><code>{item.operation_id}</code></dd><dt>{t('native-workspace.generation')}</dt><dd>{formatNumber(item.generation)}</dd><dt>{t('native-workspace.profile_sha')}</dt><dd><code>{item.profile_sha256}</code></dd><dt>{t('native-workspace.workspace_sha')}</dt><dd><code>{item.workspace_sha256}</code></dd><dt>{t('native-workspace.exit_code')}</dt><dd>{formatNumber(item.exit_code)}</dd><dt>{t('native-workspace.stopped')}</dt><dd><code>{String(item.stopped)}</code></dd><dt>{t('native-workspace.unknown')}</dt><dd><code>{String(item.outcome_unknown)}</code></dd></dl><Output check={item}/></div>)}
  {value.seal&&<div className="native-workspace-record"><h5>{t('native-workspace.seal_record')}</h5><dl><dt>{t('native-workspace.operation')}</dt><dd><code>{value.seal.operation_id}</code></dd><dt>{t('native-workspace.raw_state')}</dt><dd><code>{value.seal.state}</code></dd><dt>{t('native-workspace.previous_commit')}</dt><dd><code>{value.seal.previous_commit}</code></dd>{value.seal.commit&&<><dt>{t('native-workspace.commit')}</dt><dd><code>{value.seal.commit}</code></dd></>}{value.seal.tree&&<><dt>{t('native-workspace.tree')}</dt><dd><code>{value.seal.tree}</code></dd></>}<dt>{t('native-workspace.changed_paths')}</dt><dd>{formatNumber(value.seal.changed_paths)}</dd></dl></div>}
 </details>;
}
export function NativeWorkspaceFacts({evidence}:{evidence:WorkspaceEvidence}){
 useLocale();if(!Object.hasOwn(evidence,'native_workspace'))return null;
 const value=evidence.native_workspace;
 if(!native(value))return <section className="native-workspace-facts"><h4>{t('native-workspace.title')}</h4><p className="warning">{t('native-workspace.invalid')}</p></section>;
 const needsReconciliation=value.state==='unknown'||value.seal?.state==='unknown'||value.checks.some(item=>item.outcome_unknown);
 const successful=value.checks.filter(item=>knownSuccess(item,value.generation)).length;
 const basisComplete=value.state==='closed'&&commit(evidence.head_commit)&&value.seal?.commit===evidence.head_commit&&commit(value.seal.tree)&&successful===value.checks.length;
 const sealState=value.seal?.state;
 const state=needsReconciliation?'unknown':sealState==='sealed'?(basisComplete&&value.seal!.changed_paths>0&&value.seal!.commit!==value.seal!.previous_commit?'sealed':'incomplete'):sealState==='no_change'?(basisComplete&&value.seal!.changed_paths===0&&value.seal!.commit===value.seal!.previous_commit?'no_change':'incomplete'):sealState==='prepared'?'prepared':value.state==='open'?'open':'unsealed';
 return <section className="native-workspace-facts"><h4>{t('native-workspace.title')}</h4><p className={'native-workspace-state'+(['unknown','incomplete'].includes(state)?' warning':'')}>{t('native-workspace.state_'+state)}</p><p className="native-workspace-summary"><span>{t('native-workspace.files',{count:formatNumber(value.files.length)})}</span><span>{needsReconciliation?t('native-workspace.checks_unknown'):value.checks.length?t('native-workspace.checks',{passed:formatNumber(successful),count:formatNumber(value.checks.length)}):t('native-workspace.no_checks')}</span></p><p className="muted">{t('native-workspace.boundary')}</p><OriginalFacts key={JSON.stringify(value)+String(evidence.head_commit)} value={value}/></section>;
}
