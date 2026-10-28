// Saved rows retain raw Event identity, separate from typed display projection.
import {useLayoutEffect,useRef} from 'react';
import type {ExecutionAttempt} from '../../../packages/contracts/src';
import {tr as t,useLocale,formatDateTime,formatNumber} from './i18n';
import {toolFactsForAttempt} from './tool-model';
import {ToolEventContent} from './ToolFacts';
import {toolError} from './tool-hooks';
import {useAttemptToolHistory} from './use-attempt-tool-history';
import {savedToolDisplayMatchesSource} from './attempt-tool-history';
import {WorkspaceOperations,WorkspaceOperationRecord} from './WorkspaceOperations';
import {workspaceOperationsForAttempt} from './workspace-operations-model';
export function AttemptToolEventHistory({project,attempt,refresh=0}:{project:string;attempt:ExecutionAttempt;refresh?:number|string}){
 useLocale();
 const scope={project_id:project,run_id:attempt.run_id,node_id:attempt.node_id,attempt:attempt.attempt,fence:attempt.fence,worker_id:attempt.worker_id};
 const history=useAttemptToolHistory(scope,refresh),root=useRef<HTMLElement>(null),heading=useRef<HTMLHeadingElement>(null),focusedRow=useRef<HTMLElement|null>(null);
 // Replacement removes evidence that may own focus. Locale-only render leaves
 // surviving native disclosures/focus untouched, including same-page refresh.
 // Leaving the region releases ownership; async replacement cannot reclaim it.
 useLayoutEffect(()=>{if(focusedRow.current&&!focusedRow.current.isConnected){const active=document.activeElement;if(!active||active===document.body||!active.isConnected)heading.current?.focus({preventScroll:true});focusedRow.current=null;}});
 const page=history.page;
 const projection=page?workspaceOperationsForAttempt(page.items,scope,attempt.result?.tool_evidence?.profile.profile_sha256):undefined;
 const openSource=(sequence:number)=>{const row=root.current?.querySelector<HTMLElement>('[data-saved-tool-event="'+sequence+'"]'),details=row?.querySelector<HTMLDetailsElement>('[data-saved-raw]');if(details){details.open=true;details.querySelector('summary')?.focus({preventScroll:true});row?.scrollIntoView?.({block:'nearest'});}};
 return <section ref={root} className="attempt-tool-history" aria-label={t('development.savedFacts.title')} aria-busy={history.loading} onFocusCapture={event=>{focusedRow.current=(event.target as HTMLElement).closest<HTMLElement>('[data-saved-tool-event]');}} onBlurCapture={event=>{const next=event.relatedTarget;if(!(next instanceof Node)||!root.current?.contains(next))focusedRow.current=null;}}>
  <h5 ref={heading} tabIndex={-1}>{t('development.savedFacts.title')}</h5>
  <p className="muted">{t('development.savedFacts.boundary')}</p>
  <div className="attempt-tool-history-actions">
   <button onClick={history.latest}>{t('development.savedFacts.latest')}</button>
   <label>{t('development.savedFacts.pageSize')}<select aria-label={t('development.savedFacts.pageSize')} value={history.limit} onChange={event=>history.setLimit(Number(event.target.value))}>{[5,25,50,100].map(value=><option key={value} value={value}>{formatNumber(value)}</option>)}</select></label>
  </div>
  <p className="muted">{t('development.savedFacts.sizeReset')}</p>
  <div role="status" aria-live="polite">
   {history.loading&&<p>{t(page?'development.savedFacts.loadingPrevious':'development.savedFacts.loading')}</p>}
   {history.denied&&<p className="warning">{t(history.denied===404?'development.savedFacts.missing':'development.savedFacts.denied')}</p>}
   {!!history.failure&&page&&<p className="warning">{t('development.savedFacts.unconfirmed')}</p>}
   {!!history.failure&&<p className="warning">{toolError(history.failure)}</p>}
   {!history.loading&&!history.failure&&page&&<p>{t(page.has_more?(page.items.length?'development.savedFacts.continues':'development.savedFacts.emptyContinues'):'development.savedFacts.exhausted')}</p>}
  </div>
  {!!history.failure&&<button onClick={history.retry}>{t('development.savedFacts.retry')}</button>}
  {page&&<>
   <small>{t('development.savedFacts.window',{watermark:formatNumber(page.watermark),scanned:formatNumber(page.scanned_through),count:formatNumber(page.items.length)})}</small>
   {projection&&<WorkspaceOperations scope={scope} projection={projection} workspaceEvidence={attempt.result?.workspace_evidence??undefined} context="history" unconfirmed={!!history.failure} onSource={openSource}/>}
   <div className="tool-event-list">{page.items.map(event=>{
    const fact=savedToolDisplayMatchesSource(event)?toolFactsForAttempt([event],{id:attempt.node_id,...attempt})[0]:undefined;
    const hostFact=projection?.facts.find(item=>item.serverSequence===event.sequence);
    const key=JSON.stringify([event]);
    return <article key={key} data-saved-tool-event={event.sequence}>
     <small>{t('development.savedFacts.sequence',{sequence:formatNumber(event.sequence)})}</small>
     <time dateTime={event.at}>{formatDateTime(event.at,{dateStyle:'short',timeStyle:'medium'})} · {event.at}</time>
     {fact?<ToolEventContent event={fact}/>:hostFact?<WorkspaceOperationRecord fact={hostFact}/>:<p className="muted">{t('development.savedFacts.rawOnly')}</p>}
     <details data-saved-raw><summary>{t('development.savedFacts.rawEvent')}</summary>
      <p>{t('development.savedFacts.rawData')}</p><pre className="output" tabIndex={0} aria-label={t('development.savedFacts.rawData')} data-raw-event-data>{event.raw_data}</pre>
      <p>{t('development.savedFacts.envelope')}</p><pre className="output" tabIndex={0} aria-label={t('development.savedFacts.envelope')}>{JSON.stringify(event,null,2)}</pre>
     </details>
    </article>;
   })}</div>
   <div className="attempt-tool-history-actions">
    <button disabled={history.loading||!!history.failure||!history.previous.length} onClick={history.previousPage}>{t('development.savedFacts.previous')}</button>
    <button disabled={history.loading||!!history.failure||!page.has_more} onClick={history.next}>{t('development.savedFacts.next')}</button>
   </div>
   {history.dropped&&<p className="muted">{t('development.savedFacts.earlierDropped')}</p>}
  </>}
 </section>;
}
