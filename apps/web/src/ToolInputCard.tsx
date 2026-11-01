import {useId,useState} from 'react';
import type {CodexUserInputQuestion,CodexUserInputRequest,CodexUserInputSelection,CursorInputRequest,ToolInput} from '../../../packages/contracts/src';
import {formatDateTime,formatNumber,tr,useLocale,UiError} from './i18n';
import {ErrorNotice} from './ui';
import {canAnswerToolInput,toolInputSource,validToolInputAnswers,validToolInputSubmission,type ToolInputSubmission} from './tool-input-model';
import './ToolInputCard.css';

export type ToolInputCardProps={input:ToolInput;disabled?:boolean;readOnly?:boolean;onAnswer:(submission:ToolInputSubmission)=>void};
const t=(key:string,values?:Record<string,unknown>)=>tr('tool-inputs.'+key,values);
const stamp=(value:string)=>formatDateTime(value,{dateStyle:'short',timeStyle:'medium',hour12:false});
const specification=(value:unknown)=>JSON.stringify(value,(_key,item:unknown)=>item&&typeof item==='object'&&!Array.isArray(item)?Object.fromEntries(Object.keys(item).sort().map(key=>[key,(item as Record<string,unknown>)[key]])):item);

export function ToolInputCard(props:ToolInputCardProps){
 const value=props.input,source=toolInputSource(value.request),spec=specification([value.id,value.project_id,value.run_id,value.node_id,value.attempt,value.fence,value.dispatch_id,value.worker_id,value.profile_sha256,value.request]);
 if(!source)return <ErrorNotice error={new UiError('tool-inputs.invalid_read')}/>;
 return source.kind==='codex'?<ToolInputDraft key={spec} {...props} request={source.request}/>:<CursorInputDraft key={spec} {...props} kind={source.kind} request={source.request}/>;
}
function ToolInputDraft({input,request,disabled=false,readOnly=false,onAnswer}:ToolInputCardProps&{request:CodexUserInputRequest}){
 useLocale();const group=useId(),[draft,setDraft]=useState<Record<string,CodexUserInputSelection|undefined>>({}),questions=request.questions,editable=!readOnly&&canAnswerToolInput(input),locked=disabled||!editable;
 if(questions.some(question=>question.is_secret))return <ErrorNotice error={new UiError('tool-inputs.secret_unsupported')}/>;
 const answers=questions.flatMap(question=>draft[question.id]?[draft[question.id]!]:[]),complete=validToolInputAnswers(input.request,answers);
 function select(question:CodexUserInputQuestion,kind:'option'|'other'|'skip',option?:string){if(locked)return;setDraft(current=>({...current,[question.id]:{question_id:question.id,kind,...(kind==='option'?{option}:{}),...(kind!=='skip'&&current[question.id]?.text?{text:current[question.id]!.text}:{})}}));}
 function text(question:CodexUserInputQuestion,value:string){if(locked)return;setDraft(current=>{const old=current[question.id],kind=question.options.length?old?.kind:'text';if(kind!=='option'&&kind!=='other'&&kind!=='text')return current;return {...current,[question.id]:{question_id:question.id,kind,...(kind==='option'?{option:old?.option}:{}),...(value!==''?{text:value}:{})}};});}
 function submit(){if(!locked&&complete)onAnswer({answers});}
 function skip(){if(locked)return;const skipped=questions.map(question=>({question_id:question.id,kind:'skip'}));if(!validToolInputAnswers(input.request,skipped))return;setDraft(Object.fromEntries(skipped.map(answer=>[answer.question_id,answer])));onAnswer({answers:skipped});}
 return <section className="tool-input-card" data-tool-input-card={input.id} data-tool-input-state={input.state}>
  <header className="tool-input-heading"><h4>{t('title')}</h4><span className="tool-input-state">{t('state.'+input.state)}</span></header>
  <p className="tool-input-experimental">{t('experimental')}</p><p className="tool-input-help">{t('no_credentials')}</p>
  <ToolInputFacts input={input}/>
  {!editable&&<p className="tool-input-help">{t('historical_readonly')}</p>}
  <div className="tool-input-questions">{questions.map(question=>{
   const answer=editable?draft[question.id]:input.answer?.answers?.find(value=>value.question_id===question.id),hasNote=answer?.kind==='option'||answer?.kind==='other',name=group+':'+question.id;
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

type CursorOutcome='answered'|'skipped'|'accepted'|'rejected'|'cancelled';
function CursorInputDraft({input,kind,request,disabled=false,readOnly=false,onAnswer}:ToolInputCardProps&{kind:'cursor-questions'|'cursor-plan';request:CursorInputRequest}){
 useLocale();const group=useId(),[selections,setSelections]=useState<Record<string,string[]>>({}),[outcome,setOutcome]=useState<CursorOutcome>(),[reason,setReason]=useState('');
 const editable=!readOnly&&canAnswerToolInput(input),locked=disabled||!editable,questions=kind==='cursor-questions'?request.questions?.questions||[]:[],plan=kind==='cursor-plan'?request.plan:undefined,recorded=input.answer?.cursor;
 const outcomes:CursorOutcome[]=plan?['accepted','rejected','cancelled']:['answered','skipped','cancelled'];
 const submission:ToolInputSubmission|undefined=outcome?{cursor:{outcome,...(outcome==='answered'?{answers:questions.map(question=>({question_id:question.id,option_ids:selections[question.id]||[]}))}:{}),...((outcome==='skipped'||outcome==='rejected')&&reason!==''?{reason}:{})}}:undefined;
 const complete=!!submission&&validToolInputSubmission(input.request,submission);
 function choose(question:typeof questions[number],optionID:string,checked:boolean){
  if(locked)return;setSelections(current=>{const old=current[question.id]||[],next=question.allow_multiple?checked?[...old.filter(id=>id!==optionID),optionID]:old.filter(id=>id!==optionID):[optionID];return {...current,[question.id]:question.options.filter(option=>next.includes(option.id)).map(option=>option.id)};});setOutcome('answered');setReason('');
 }
 function decide(value:CursorOutcome){if(locked)return;setOutcome(value);if(value!=='skipped'&&value!=='rejected')setReason('');}
 function submit(){if(!locked&&complete&&submission)onAnswer(submission);}
 return <section className="tool-input-card" data-tool-input-card={input.id} data-tool-input-state={input.state}>
  <header className="tool-input-heading"><h4>{t(plan?'cursor_plan_title':'cursor_questions_title')}</h4><span className="tool-input-state">{t('state.'+input.state)}</span></header>
  <p className="tool-input-help">{t('no_credentials')}</p><ToolInputFacts input={input}/>
  {!editable&&<p className="tool-input-help">{t('historical_readonly')}</p>}
  {request.questions?.title&&<h5 className="tool-input-request-title">{request.questions.title}</h5>}
  {!!questions.length&&<div className="tool-input-questions">{questions.map(question=>{
   const ids=editable?selections[question.id]||[]:recorded?.answers?.find(answer=>answer.question_id===question.id)?.option_ids||[];
   return <section className="tool-input-question" data-tool-input-question={question.id} key={question.id}><h5>{question.prompt}</h5><p className="tool-input-help">{t(question.allow_multiple?'multiple_choice':'single_choice')}</p>
    {editable?<fieldset className="tool-input-question-controls" disabled={locked}><legend>{t('choose_original_options')}</legend>{question.options.map(option=><label className="tool-input-choice" key={option.id}><input type={question.allow_multiple?'checkbox':'radio'} name={group+':'+question.id} value={option.id} data-tool-input-cursor-option={question.id} checked={ids.includes(option.id)} onChange={event=>choose(question,option.id,event.target.checked)}/><span>{option.label}</span></label>)}</fieldset>:<div className="tool-input-recorded-answer">{ids.length?<ul>{question.options.filter(option=>ids.includes(option.id)).map(option=><li key={option.id}>{option.label}</li>)}</ul>:<p>{t('no_saved_selection')}</p>}</div>}
   </section>;
  })}</div>}
  {plan&&<div className="tool-input-plan">
   {plan.name&&<h5 className="tool-input-request-title">{plan.name}</h5>}{plan.overview&&<p className="tool-input-question-text">{plan.overview}</p>}
   <p className="tool-input-help">{t('plan_boundary')}</p><pre className="tool-input-plan-text" data-tool-input-plan tabIndex={0}>{plan.plan}</pre>
   {(!!plan.todos.length||!!plan.phases?.length)&&<section className="tool-input-plan-items"><h5>{t('plan_items')}</h5><p className="tool-input-help">{t('plan_items_boundary')}</p><NativePlanTodos values={plan.todos}/>{plan.phases?.map((phase,index)=><section className="tool-input-plan-phase" key={index}><h6>{phase.name}</h6><NativePlanTodos values={phase.todos}/></section>)}</section>}
  </div>}
  {editable?<>
   <fieldset disabled={locked} className="tool-input-question-controls tool-input-decisions"><legend>{t('decision_prompt')}</legend>{outcomes.map(value=><label className="tool-input-choice" key={value}><input type="radio" name={group+':decision'} data-tool-input-decision={value} checked={outcome===value} onChange={()=>decide(value)}/><span>{t('decision.'+value)}</span></label>)}</fieldset>
   {(outcome==='skipped'||outcome==='rejected')&&<label className="tool-input-text-label">{t('decision_reason')}<textarea data-tool-input-reason value={reason} disabled={locked} maxLength={4000} rows={3} autoComplete="off" onChange={event=>{if(!locked)setReason(event.target.value);}}/></label>}
   <div className="tool-input-actions"><button type="button" className="primary" data-tool-input-submit disabled={locked||!complete} onClick={submit}>{t(plan?'submit_decision':'submit')}</button></div>
  </>:recorded&&<div className="tool-input-recorded-answer" data-tool-input-recorded-decision><strong>{t('saved_decision')}: {t('decision.'+recorded.outcome)}</strong>{recorded.reason&&<p>{recorded.reason}</p>}</div>}
 </section>;
}

function NativePlanTodos({values}:{values:readonly {id:string;content:string;status:string}[]}){
 return values.length?<ul className="tool-input-plan-todos">{values.map(todo=><li key={todo.id}><span>{todo.content}</span><span className="tool-input-native-status">{t('native_todo.'+todo.status)}</span></li>)}</ul>:null;
}

export function ToolInputFacts({input}:{input:ToolInput}){
 useLocale();const observation=input.observation,uncertain=!!observation?.failure_code||!!observation?.reply_started&&!observation.reply_written&&(observation.request_resolved||observation.execution_closed),native=toolInputSource(input.request);
 if(!native)return <ErrorNotice error={new UiError('tool-inputs.invalid_read')}/>;
 const identity=native.kind==='codex'?{request_id:native.request.request_id,thread_id:native.request.thread_id,turn_id:native.request.turn_id,item_id:native.request.item_id,binding_sha256:native.request.binding_sha256}:{request_id:native.request.request_id,session_id:native.request.session_id,prompt_id:native.request.prompt_id,method:native.request.method,configuration_sha256:native.request.configuration_sha256,tool_call_id:native.request.questions?.tool_call_id||native.request.plan?.tool_call_id,is_project:native.request.plan?.is_project};
 const source={input_id:input.id,project_id:input.project_id,run_id:input.run_id,node_id:input.node_id,attempt:input.attempt,fence:input.fence,dispatch_id:input.dispatch_id,worker_id:input.worker_id,profile_sha256:input.profile_sha256,version:input.version,created_at:input.created_at,expires_at:input.expires_at,protocol:native.protocol,kind:native.kind,...identity,params_sha256:native.paramsSHA256,answer_sha256:input.answer?.answer_sha256,observation};
 return <div className="tool-input-facts" data-tool-input-facts>
  <p className="tool-input-help">{native.kind==='codex'?t(native.request.is_blocking?'blocking':'not_blocking'):t('cursor_boundary')}</p>
  <ul className="tool-input-delivery">
   {input.answer&&<li data-tool-input-status="saved">{t('delivery.saved')} <span className="tool-input-meta">{stamp(input.answer.answered_at)} · {input.answer.actor_id}</span></li>}
   {observation?.answer_observed&&<li data-tool-input-status="observed">{t('delivery.observed')}</li>}
   {observation?.reply_started&&<li data-tool-input-status="started">{t('delivery.started')}</li>}
   {observation?.reply_written&&<li data-tool-input-status="written">{t('delivery.written')}</li>}
   {observation?.request_resolved&&<li data-tool-input-status="resolved">{t('delivery.resolved')}</li>}
   {observation?.execution_closed&&<li data-tool-input-status="execution_closed">{t('delivery.execution_closed')}</li>}
  </ul>
  {!input.answer&&(observation?.request_resolved||observation?.execution_closed||input.state==='closed')&&<p className="tool-input-help">{t('closed_without_answer')}</p>}
  {uncertain&&<p className="tool-input-uncertain" data-tool-input-status="unknown" role="status">{t('delivery.unknown')}</p>}
  <details className="tool-input-source"><summary>{t('source_details')}</summary><a href={'?'+new URLSearchParams({project:input.project_id,view:'development',sub:'runs',run:input.run_id,node:input.node_id})}>{t('source_run')} · {input.run_id}</a><p className="tool-input-meta">{t('source_version',{version:formatNumber(input.version)})} · {t('expires_at')}: <time dateTime={input.expires_at} title={input.expires_at}>{stamp(input.expires_at)}</time></p><pre tabIndex={0}>{JSON.stringify(source,null,2)}</pre></details>
 </div>;
}
