import {useId,useState} from 'react';
import type {CodexUserInputQuestion,CodexUserInputSelection,ToolInput} from '../../../packages/contracts/src';
import {formatDateTime,formatNumber,tr,useLocale,UiError} from './i18n';
import {ErrorNotice} from './ui';
import {canAnswerToolInput,validToolInputAnswers} from './tool-input-model';
import './ToolInputCard.css';

export type ToolInputCardProps={input:ToolInput;disabled?:boolean;readOnly?:boolean;onAnswer:(answers:CodexUserInputSelection[])=>void};
const t=(key:string,values?:Record<string,unknown>)=>tr('tool-inputs.'+key,values);
const stamp=(value:string)=>formatDateTime(value,{dateStyle:'short',timeStyle:'medium',hour12:false});

export function ToolInputCard(props:ToolInputCardProps){
 const value=props.input,spec=JSON.stringify([value.id,value.project_id,value.run_id,value.node_id,value.attempt,value.fence,value.worker_id,value.profile_sha256,value.request.codex.binding_sha256,value.request.codex.params_sha256,value.request.codex.questions]);
 return <ToolInputDraft key={spec} {...props}/>;
}
function ToolInputDraft({input,disabled=false,readOnly=false,onAnswer}:ToolInputCardProps){
 useLocale();const group=useId(),[draft,setDraft]=useState<Record<string,CodexUserInputSelection|undefined>>({}),questions=input.request.codex.questions,editable=!readOnly&&canAnswerToolInput(input),locked=disabled||!editable;
 if(questions.some(question=>question.is_secret))return <ErrorNotice error={new UiError('tool-inputs.secret_unsupported')}/>;
 const answers=questions.flatMap(question=>draft[question.id]?[draft[question.id]!]:[]),complete=validToolInputAnswers(input.request,answers);
 function select(question:CodexUserInputQuestion,kind:'option'|'other'|'skip',option?:string){if(locked)return;setDraft(current=>({...current,[question.id]:{question_id:question.id,kind,...(kind==='option'?{option}:{}),...(kind!=='skip'&&current[question.id]?.text?{text:current[question.id]!.text}:{})}}));}
 function text(question:CodexUserInputQuestion,value:string){if(locked)return;setDraft(current=>{const old=current[question.id],kind=question.options.length?old?.kind:'text';if(kind!=='option'&&kind!=='other'&&kind!=='text')return current;return {...current,[question.id]:{question_id:question.id,kind,...(kind==='option'?{option:old?.option}:{}),...(value!==''?{text:value}:{})}};});}
 function submit(){if(!locked&&complete)onAnswer(answers);}
 function skip(){if(locked)return;const skipped=questions.map(question=>({question_id:question.id,kind:'skip'}));if(!validToolInputAnswers(input.request,skipped))return;setDraft(Object.fromEntries(skipped.map(answer=>[answer.question_id,answer])));onAnswer(skipped);}
 return <section className="tool-input-card" data-tool-input-card={input.id} data-tool-input-state={input.state}>
  <header className="tool-input-heading"><h4>{t('title')}</h4><span className="tool-input-state">{t('state.'+input.state)}</span></header>
  <p className="tool-input-experimental">{t('experimental')}</p><p className="tool-input-help">{t('no_credentials')}</p>
  <ToolInputFacts input={input}/>
  {!editable&&<p className="tool-input-help">{t('historical_readonly')}</p>}
  <div className="tool-input-questions">{questions.map(question=>{
   const answer=editable?draft[question.id]:input.answer?.answers.find(value=>value.question_id===question.id),hasNote=answer?.kind==='option'||answer?.kind==='other',name=group+':'+question.id;
   return <section className="tool-input-question" data-tool-input-question={question.id} key={question.id}><h5>{question.header}</h5><p className="tool-input-question-text">{question.question}</p>
    {editable?<fieldset disabled={locked} className="tool-input-question-controls"><legend>{t('answer_question',{header:question.header})}</legend>
     {question.options.map(option=><label className="tool-input-choice" key={option.label}><input type="radio" name={name} value={option.label} data-tool-input-option={question.id} checked={answer?.kind==='option'&&answer.option===option.label} onChange={()=>select(question,'option',option.label)}/><span><strong>{option.label}</strong>{option.description&&<span>{option.description}</span>}</span></label>)}
     {!!question.options.length&&question.is_other&&<label className="tool-input-choice"><input type="radio" name={name} data-tool-input-other={question.id} checked={answer?.kind==='other'} onChange={()=>select(question,'other')}/><span>{t('other')}</span></label>}
     {(!question.options.length||hasNote)&&<label className="tool-input-text-label">{t(question.options.length?'optional_note':'text_answer')}<textarea data-tool-input-text={question.id} value={answer?.kind==='skip'?'':answer?.text||''} maxLength={4000} rows={3} autoComplete="off" onChange={event=>text(question,event.target.value)}/></label>}
     <label className="tool-input-choice tool-input-skip-choice"><input type="radio" name={name} data-tool-input-question-skip={question.id} checked={answer?.kind==='skip'} onChange={()=>select(question,'skip')}/><span>{t('skip_question')}</span></label>
    </fieldset>:<div className="tool-input-recorded-answer">{answer?<><strong>{t('answer_kind.'+answer.kind)}</strong>{answer.kind==='option'&&<p>{answer.option}</p>}{answer.text&&<p>{answer.text}</p>}</>:<p>{t('no_saved_answer')}</p>}</div>}
   </section>;
  })}</div>
  {editable&&<div className="tool-input-actions"><button type="button" className="primary" data-tool-input-submit disabled={locked||!complete} onClick={submit}>{t('submit')}</button><button type="button" data-tool-input-skip disabled={locked} onClick={skip}>{t('skip_request')}</button></div>}
 </section>;
}

export function ToolInputFacts({input}:{input:ToolInput}){
 useLocale();const observation=input.observation,uncertain=!!observation?.failure_code||!!observation?.reply_started&&!observation.reply_written&&(observation.request_resolved||observation.turn_closed),codex=input.request.codex;
 const source={input_id:input.id,project_id:input.project_id,run_id:input.run_id,node_id:input.node_id,attempt:input.attempt,fence:input.fence,dispatch_id:input.dispatch_id,worker_id:input.worker_id,profile_sha256:input.profile_sha256,version:input.version,created_at:input.created_at,expires_at:input.expires_at,request_id:codex.request_id,thread_id:codex.thread_id,turn_id:codex.turn_id,item_id:codex.item_id,binding_sha256:codex.binding_sha256,params_sha256:codex.params_sha256,answer_sha256:input.answer?.answer_sha256,observation};
 return <div className="tool-input-facts" data-tool-input-facts>
  <p className="tool-input-help">{t(codex.is_blocking?'blocking':'not_blocking')}</p>
  <ul className="tool-input-delivery">
   {input.answer&&<li data-tool-input-status="saved">{t('delivery.saved')} <span className="tool-input-meta">{stamp(input.answer.answered_at)} · {input.answer.actor_id}</span></li>}
   {observation?.answer_observed&&<li data-tool-input-status="observed">{t('delivery.observed')}</li>}
   {observation?.reply_started&&<li data-tool-input-status="started">{t('delivery.started')}</li>}
   {observation?.reply_written&&<li data-tool-input-status="written">{t('delivery.written')}</li>}
   {observation?.request_resolved&&<li data-tool-input-status="resolved">{t('delivery.resolved')}</li>}
   {observation?.turn_closed&&<li data-tool-input-status="turn_closed">{t('delivery.turn_closed')}</li>}
  </ul>
  {!input.answer&&(observation?.request_resolved||observation?.turn_closed||input.state==='closed')&&<p className="tool-input-help">{t('closed_without_answer')}</p>}
  {uncertain&&<p className="tool-input-uncertain" data-tool-input-status="unknown" role="status">{t('delivery.unknown')}</p>}
  <details className="tool-input-source"><summary>{t('source_details')}</summary><a href={'?'+new URLSearchParams({project:input.project_id,view:'development',sub:'runs',run:input.run_id,node:input.node_id})}>{t('source_run')} · {input.run_id}</a><p className="tool-input-meta">{t('source_version',{version:formatNumber(input.version)})} · {t('expires_at')}: <time dateTime={input.expires_at} title={input.expires_at}>{stamp(input.expires_at)}</time></p><pre tabIndex={0}>{JSON.stringify(source,null,2)}</pre></details>
 </div>;
}
