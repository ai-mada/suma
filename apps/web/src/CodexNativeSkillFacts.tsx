import {Fragment} from 'react';
import type {ExecutionSkillBundle,ExecutionSkillEvidence} from '../../../packages/contracts/src';
import {tr,useLocale} from './i18n';
import {validExecutionSkillBundle,validExecutionSkillEvidence,executionSkillEvidenceMatches} from './codex-native-skills-model';
import './CodexNativeSkillFacts.css';

export type CodexNativeSkillFactsProps={bundle?:ExecutionSkillBundle|null;evidence?:ExecutionSkillEvidence|null;mode:'plan'|'attempt'|'text'};
const t=(key:string,values?:Record<string,unknown>)=>tr('codex-native-skills.'+key,values);
const flags=['materialized','catalog_acknowledged','invocation_requested','turn_acknowledged','files_verified'] as const;

/** Configuration is not execution evidence. Only a matching original receipt
 * can confirm a fact; none of these facts asserts model use or effectiveness. */
export function CodexNativeSkillFacts({bundle,evidence,mode}:CodexNativeSkillFactsProps){
 useLocale();const fixed=validExecutionSkillBundle(bundle)?bundle:undefined;
 const validProof=validExecutionSkillEvidence(evidence)?evidence:undefined;
 const proof=mode==='attempt'&&fixed&&validProof&&executionSkillEvidenceMatches(fixed,validProof)?validProof:undefined;
 const diagnostic=mode==='text'?'text_mode':bundle&&!fixed?'invalid_bundle':mode==='plan'?fixed?'configured':'none_selected':!evidence?'not_recorded':!validProof?'invalid_evidence':!fixed?'bundle_unavailable':!proof?'mismatched_evidence':undefined;
 return <section className="cns-facts" aria-label={t('title')} data-codex-native-skills>
  <header><h4>{t('title')}</h4>{fixed&&mode!=='text'&&<span>{t('count',{value:fixed.documents.length})}</span>}</header>
  {diagnostic&&<p className="cns-muted" role="status">{t(diagnostic)}</p>}
  {proof?.failure_code&&<p role="status" data-codex-skill-failure>{t('failure_recorded')}</p>}
  {mode==='attempt'&&<><dl className="cns-checks" data-skill-facts>{flags.map(flag=><Fragment key={flag}><dt>{t(flag)}</dt><dd data-verified={proof?.[flag]===true}><span aria-hidden="true">{proof?.[flag]===true?'✓':'—'}</span> {t(proof?.[flag]===true?'confirmed':'not_verified')}</dd></Fragment>)}</dl><p className="cns-muted">{t('fact_boundary')}</p></>}
  {fixed&&mode!=='text'&&<details key={mode+':'+fixed.bundle_sha256} className="cns-details"><summary>{t('details')}</summary>
   <dl className="cns-identities"><dt>{t('bundle_sha256')}</dt><dd><code>{fixed.bundle_sha256}</code></dd></dl>
   <ul className="cns-documents">{fixed.documents.map(document=><li key={document.id}><div className="cns-document-heading"><strong>{document.title}</strong><span>{t('revision',{value:document.revision})}</span></div><dl className="cns-identities"><dt>{t('id')}</dt><dd><code>{document.id}</code></dd><dt>{t('name')}</dt><dd><code>{document.name}</code></dd><dt>{t('revision_sha256')}</dt><dd><code>{document.sha256}</code></dd><dt>{t('document_sha256')}</dt><dd><code>{document.document_sha256}</code></dd></dl></li>)}</ul>
   {proof?.failure_code&&<dl className="cns-identities"><dt>{t('failure_code')}</dt><dd><code>{proof.failure_code}</code></dd></dl>}
  </details>}
 </section>;
}
