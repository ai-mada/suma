import type {WorkspaceEvidence} from './api';
import type {CandidateBinding as Binding,CandidateImportReceipt as ImportReceipt} from '../../../packages/contracts/src';
import {formatNumber,tr as t,useLocale} from './i18n';
import './candidate-transfer.css';

const sourceFields=['run_id','node_id','attempt','fence','worker_id'];
const poolIDs=['tenant_id','project_id','pool_id','pool_runner_id','pool_slot_id','pool_grant_id','pool_credential_id','dispatch_id','dispatch_state','lease_until'];
const poolNumbers=['pool_grant_version','pool_credential_version'];
const bindingFields=['schema_version','transfer_id','project_id','state','source','base_commit','head_commit','evidence_sha256','bundle_sha256','size_bytes'];
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const closed=(value:Record<string,unknown>,required:string[],optional:string[]=[])=>required.every(key=>Object.hasOwn(value,key))&&Object.keys(value).every(key=>required.includes(key)||optional.includes(key));
const identity=(value:unknown,max:number):value is string=>typeof value==='string'&&value.length>0&&new TextEncoder().encode(value).length<=max&&value.trim()===value&&!/[\/\\\u0000-\u001f\u007f]/.test(value);
const positive=(value:unknown):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>0;
const commit=(value:unknown)=>typeof value==='string'&&/^[a-f0-9]{40}([a-f0-9]{24})?$/.test(value);
const hash=(value:unknown)=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);

// These guards only permit a versioned presentation. Current transfer authority
// and Git verification remain server/Runner facts, never browser decisions.
function binding(value:unknown):value is Binding{
 if(!object(value)||!closed(value,bindingFields)||value.schema_version!==1||value.state!=='sealed'||!identity(value.transfer_id,64)||!identity(value.project_id,64)||!commit(value.base_commit)||!commit(value.head_commit)||value.base_commit===value.head_commit||!hash(value.evidence_sha256)||!hash(value.bundle_sha256)||!positive(value.size_bytes)||value.size_bytes>64*1024*1024)return false;
 const source=value.source;
 return object(source)&&closed(source,sourceFields,[...poolIDs,...poolNumbers])&&identity(source.run_id,100)&&identity(source.node_id,150)&&identity(source.worker_id,100)&&positive(source.attempt)&&positive(source.fence)&&poolIDs.every(key=>!Object.hasOwn(source,key)||identity(source[key],150))&&poolNumbers.every(key=>!Object.hasOwn(source,key)||positive(source[key]));
}
function imported(value:unknown):value is ImportReceipt{
 return object(value)&&closed(value,['binding','imported','target_worker_id'])&&binding(value.binding)&&typeof value.imported==='boolean'&&identity(value.target_worker_id,100);
}
function producerMatches(value:Binding,evidence:WorkspaceEvidence){return value.base_commit===evidence.base_commit&&value.head_commit===evidence.head_commit&&value.evidence_sha256===evidence.evidence_sha256;}
function consumerMatches(value:Binding,evidence:WorkspaceEvidence){return value.head_commit===evidence.base_commit&&value.source.node_id===evidence.candidate_from&&value.source.attempt===evidence.candidate_attempt&&value.source.fence===evidence.candidate_fence&&value.evidence_sha256===evidence.candidate_evidence_sha256;}

function BindingFacts({value,target}:{value:Binding;target?:string}){
 return <div className="candidate-transfer-record"><p className="candidate-transfer-state">{t(target?'development.transfer.imported':'development.transfer.sealed')}</p><dl>
  <dt>{t('development.transfer.source_worker')}</dt><dd><code>{value.source.worker_id}</code></dd>
  {target&&<><dt>{t('development.transfer.target_worker')}</dt><dd><code>{target}</code></dd></>}
  <dt>{t('development.transfer.bundle_digest')}</dt><dd><code>{value.bundle_sha256}</code></dd>
  <dt>{t('development.transfer.bundle_size')}</dt><dd>{t('development.transfer.bytes',{size:formatNumber(value.size_bytes)})}</dd>
 </dl><details><summary>{t('development.transfer.provenance')}</summary><dl>
  <dt>{t('development.transfer.project')}</dt><dd><code>{value.project_id}</code></dd>
  <dt>{t('development.transfer.transfer_id')}</dt><dd><code>{value.transfer_id}</code></dd>
  <dt>{t('development.transfer.source_run')}</dt><dd><code>{value.source.run_id}</code></dd>
  <dt>{t('development.candidate.source_node')}</dt><dd><code>{value.source.node_id}</code></dd>
  <dt>{t('development.candidate.source_attempt')}</dt><dd>{formatNumber(value.source.attempt)}</dd>
  <dt>{t('development.candidate.source_fence')}</dt><dd>{formatNumber(value.source.fence)}</dd>
  <dt>{t('development.evidence.base_commit')}</dt><dd><code>{value.base_commit}</code></dd>
  <dt>{t('development.candidate.pinned_commit')}</dt><dd><code>{value.head_commit}</code></dd>
  <dt>{t('development.candidate.source_evidence')}</dt><dd><code>{value.evidence_sha256}</code></dd>
 </dl>{[...poolIDs,...poolNumbers].some(key=>Object.hasOwn(value.source,key))&&<><h5>{t('development.transfer.source_identity')}</h5><pre>{JSON.stringify(value.source,null,2)}</pre></>}</details></div>;
}

export function CandidateTransferFacts({evidence}:{evidence:WorkspaceEvidence}){
 useLocale();
 const sourcePresent=Object.hasOwn(evidence,'candidate_bundle'),importPresent=Object.hasOwn(evidence,'candidate_import');
 if(!sourcePresent&&!importPresent)return null;
 const source=binding(evidence.candidate_bundle)&&producerMatches(evidence.candidate_bundle,evidence)?evidence.candidate_bundle:undefined;
 const receipt=imported(evidence.candidate_import)&&consumerMatches(evidence.candidate_import.binding,evidence)?evidence.candidate_import:undefined;
 const invalid=(sourcePresent&&!source)||(importPresent&&!receipt);
 return <div className="candidate-transfer-facts"><h4>{t('development.transfer.title')}</h4>
  {invalid&&<p className="warning">{t('development.transfer.invalid')}</p>}
  {source&&<BindingFacts key={JSON.stringify([source])} value={source}/>}
  {receipt?.imported&&<BindingFacts key={JSON.stringify([receipt.binding,receipt.target_worker_id])} value={receipt.binding} target={receipt.target_worker_id}/>}
  {(source||receipt)&&!receipt?.imported&&<p className="muted">{t('development.transfer.no_import')}</p>}
  {(source||receipt)&&<p className="muted">{t('development.transfer.boundary')}</p>}
 </div>;
}
