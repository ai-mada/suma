import {useId} from 'react';
import type {SteeringContext,SteeringInstruction,SteeringSession} from '../../../packages/contracts/src';
import {formatDateTime,formatNumber,tr,useLocale,UiError} from './i18n';
import {ErrorNotice} from './ui';
import './TurnSteeringCard.css';

export type TurnSteeringCardProps={context?:SteeringContext;selectedSessionID:string;loading?:boolean;stale?:boolean;disabled?:boolean;readOnly?:boolean;pending?:boolean;error?:unknown;onSelect:(sessionID:string)=>void;onSend:(sessionID:string)=>void;onRefresh?:()=>void;onRestore?:()=>void};
const t=(key:string,values?:Record<string,unknown>)=>tr('turn-steering.'+key,values);
const stamp=(value:string)=>formatDateTime(value,{dateStyle:'short',timeStyle:'medium',hour12:false});
const sourceLink=(project:string,run:string,node:string)=>'?'+new URLSearchParams({project,view:'development',sub:'runs',run,node});
const originalSession=(instruction:SteeringInstruction,session?:SteeringSession)=>session?.id===instruction.session_id&&session.project_id===instruction.project_id&&session.task_id===instruction.task_id&&session.run_id===instruction.run_id&&session.node_id===instruction.node_id&&session.attempt===instruction.attempt&&session.fence===instruction.fence?session:undefined;

/** A controlled presentation of one authorized message context. All reads,
 * current authorization and original write recovery belong to the parent. */
export function TurnSteeringCard({context,selectedSessionID,loading=false,stale=false,disabled=false,readOnly=false,pending=false,error,onSelect,onSend,onRefresh,onRestore}:TurnSteeringCardProps){
 useLocale();const choices=useId(),blocked=disabled||readOnly||loading||stale||pending||!!error;
 const sent=(id:string)=>!!context?.instructions.some(instruction=>instruction.session_id===id&&instruction.message.id===context.message.id);
 const target=context?.targets.find(value=>value.id===selectedSessionID),selectable=(value:SteeringSession)=>!!context?.can_send&&value.state==='active'&&value.allowed_actions.includes('send')&&!sent(value.id),canSend=!!target&&!blocked&&selectable(target);
 return <section className="turn-steering-card" data-turn-steering-card aria-busy={loading}>
  <header><h4>{t('title')}</h4></header><p className="turn-steering-help">{t('boundary')}</p>
  {!!error&&<ErrorNotice error={error}/>}{loading&&<p role="status">{t('loading')}</p>}{stale&&!loading&&<p className="turn-steering-notice" role="status">{t('stale')}</p>}{pending&&<p className="turn-steering-notice" role="status">{t('original_pending')}</p>}
  {context?<>
   <div className="turn-steering-message"><strong>{t('selected_message')}</strong><p data-turn-steering-message>{context.message.body}</p></div>
   <p className="turn-steering-help">{t('no_credentials')}</p>
   {!context.can_send&&context.reason&&<ErrorNotice error={new UiError('turn-steering.reason.'+context.reason)}/>}
   {!!context.targets.length&&<fieldset className="turn-steering-targets" disabled={blocked}><legend>{t('choose_target')}</legend>{context.targets.map(value=><label key={value.id} className="turn-steering-target"><input type="radio" name={choices} data-turn-steering-target={value.id} value={value.id} checked={value.id===selectedSessionID} disabled={!selectable(value)} onChange={()=>{if(!blocked&&selectable(value))onSelect(value.id);}}/><span><strong>{value.node_id}</strong><span>{t('execution_tuple',{run:value.run_id,attempt:formatNumber(value.attempt),fence:formatNumber(value.fence)})}</span><span className="turn-steering-meta">{t('worker',{worker:value.worker_id})} · {t(value.state==='closed'?'session_closed':'session_active')}</span>{sent(value.id)&&<span className="turn-steering-meta">{t('already_saved')}</span>}</span></label>)}</fieldset>}
   {!context.targets.length&&!loading&&<p className="turn-steering-help">{t('empty')}</p>}
  </>:!loading&&!error&&<p className="turn-steering-help">{t('empty')}</p>}
  <div className="turn-steering-actions">{pending?onRestore&&<button type="button" className="primary" data-turn-steering-restore disabled={loading||stale} onClick={onRestore}>{t('recover')}</button>:<>
   {!!context?.targets.length&&<button type="button" className="primary" data-turn-steering-send disabled={!canSend} onClick={()=>{if(canSend&&target)onSend(target.id);}}>{t('send')}</button>}
  </>}{onRefresh&&<button type="button" data-turn-steering-refresh disabled={loading} onClick={onRefresh}>{t('refresh')}</button>}</div>
  {context&&<details className="turn-steering-source"><summary>{t('context_details')}</summary><pre tabIndex={0}>{JSON.stringify({task_id:context.task_id,task_version:context.task_version,message_id:context.message.id,sequence:context.message.sequence,author_id:context.message.author_id,body_sha256:context.message.body_sha256,targets:context.targets},null,2)}</pre></details>}
  {!!context?.instructions.length&&<div className="turn-steering-history"><h5>{t('delivery_history')}</h5>{context.instructions.map(instruction=><TurnSteeringFacts key={instruction.id} instruction={instruction} session={context.targets.find(value=>value.id===instruction.session_id)}/>)}</div>}
 </section>;
}

export function TurnSteeringFacts({instruction,session}:{instruction:SteeringInstruction;session?:SteeringSession}){
 useLocale();const observation=instruction.observation,source=originalSession(instruction,session),notWritten=source?.state==='closed'&&!observation?.write_started,uncertain=!!observation?.failure_code&&!observation.native_rejected;
 return <section className="turn-steering-facts" data-turn-steering-facts={instruction.id}>
  <p className="turn-steering-meta"><a href={sourceLink(instruction.project_id,instruction.run_id,instruction.node_id)}>{instruction.run_id} / {instruction.node_id}</a> · {t('attempt',{attempt:formatNumber(instruction.attempt),fence:formatNumber(instruction.fence)})}</p>
  <ul className="turn-steering-delivery">
   <li data-turn-steering-status="saved">{t('delivery.saved')} <span className="turn-steering-meta">{stamp(instruction.created_at)}</span></li>
   {observation?.observed&&<li data-turn-steering-status="observed">{t('delivery.observed')}</li>}
   {observation?.write_started&&<li data-turn-steering-status="started">{t('delivery.started')}</li>}
   {observation?.write_written&&<li data-turn-steering-status="written">{t('delivery.written')}</li>}
   {observation?.native_accepted&&<li data-turn-steering-status="accepted">{t('delivery.accepted')}</li>}
   {observation?.native_rejected&&<li data-turn-steering-status="rejected">{t('delivery.rejected')}</li>}
   {observation?.turn_closed&&<li data-turn-steering-status="closed">{t('delivery.closed')}</li>}
   {notWritten&&<li data-turn-steering-status="not_written">{t('delivery.not_written')}</li>}
  </ul>
  {uncertain&&<p className="turn-steering-notice" role="status" data-turn-steering-status="unknown">{t('delivery.unknown')}</p>}
  <p className="turn-steering-help">{t('acceptance_boundary')}</p>
  <details className="turn-steering-source"><summary>{t('instruction_details')}</summary><p className="turn-steering-meta">{t('expires_at')}: <time dateTime={instruction.expires_at} title={instruction.expires_at}>{stamp(instruction.expires_at)}</time></p><pre tabIndex={0}>{JSON.stringify({instruction_id:instruction.id,session_id:instruction.session_id,project_id:instruction.project_id,task_id:instruction.task_id,run_id:instruction.run_id,node_id:instruction.node_id,attempt:instruction.attempt,fence:instruction.fence,version:instruction.version,message_id:instruction.message.id,message_sequence:instruction.message.sequence,message_sha256:instruction.message.body_sha256,actor_id:instruction.actor_id,created_at:instruction.created_at,expires_at:instruction.expires_at,source:source?.source,observation},null,2)}</pre></details>
 </section>;
}
