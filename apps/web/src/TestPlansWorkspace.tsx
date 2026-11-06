import {useCallback,useEffect,useId,useMemo,useRef,useState} from 'react';
import {ArrowRight,ChevronDown,FileText,Plus,RefreshCw,ShieldCheck,Trash2} from 'lucide-react';
import type {QualityTestCase,TestPlanView,TestPlanExecution,TestPlanReport,TestCaseResult} from '../../../packages/contracts/src';
import {ApiError,query,type RecordItem,type Session} from './api';
import {useApi} from './api-context';
import {formatDateTime,formatNumber,tr,UiError,useLocale} from './i18n';
import {Badge,Empty,ErrorNotice,Field,Loading,Modal} from './ui';
import {useLeaveGuard,useWriteAction,WriteFields,WriteNotice} from './write-action';
import {caseStatuses,executionStates,isTestPlanState,newQualityCase,packageEvidenceStates,planDraftErrors,planID,planStates,planText,reportEligibility,taskCriteria,testPlanCatalogPath,testPlanPath,testPlanRecovery,type TestPlanCatalogFilter,type TestPlanState,validPlanPage,validTestPlan,validTestPlanExecution,validTestPlanReport} from './test-plans-model';
import {useTestPlanPage,useTestPlanRead} from './test-plans-hooks';
import './TestPlansWorkspace.css';

export type TestPlansWorkspaceProps={session:Session;project:string;refresh:number;planID?:string;runID?:string;tab?:'cases'|'results';catalogFilter?:TestPlanCatalogFilter;onCatalogFilter?:(filter:TestPlanCatalogFilter)=>void;onSelectPlan:(id:string)=>boolean|void;onSelectExecution:(run:string)=>void;onTab:(tab:'cases'|'results')=>void;onOpenTask:(id:string)=>void;onOpenRun:(run:string,node?:string,attempt?:number)=>void;onOpenIssue:(id:string)=>void;onChanged:()=>void|Promise<void>};
type Draft={id:string;version:number;taskID:string;taskVersion:number;requirementVersion:number;title:string;description:string;cases:QualityTestCase[];canonical:string};
type DefectDraft={planID:string;version:number;runID:string;reason:string}&({kind:'case';key:string}|{kind:'package';nodeID:string;attempt:number;fence:number});
type PackageGate=TestPlanExecution['package_evidence'][number];
type DraftAdoption={previous:Draft;planID:string;complete:()=>void;reject:(error:unknown)=>void};
const emptyCatalogFilter:TestPlanCatalogFilter={};
const t=(key:string,values?:Record<string,unknown>)=>tr('test-plans.'+key,values);
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const content=(value:Pick<Draft,'title'|'description'|'cases'>)=>JSON.stringify({title:value.title,description:value.description,cases:value.cases});
const canLeave=()=>window.dispatchEvent(new Event('suma:navigate',{cancelable:true}));
const fromPlan=(plan:TestPlanView):Draft=>({id:plan.id,version:plan.version,taskID:plan.task_id,taskVersion:plan.latest!.source.task.version,requirementVersion:plan.latest!.source.requirement?.version||0,title:plan.latest!.title,description:plan.latest!.description,cases:structuredClone(plan.latest!.cases),canonical:content(plan.latest!)});
const freshDraft=(taskID=''):Draft=>({id:'',version:0,taskID,taskVersion:0,requirementVersion:0,title:'',description:'',cases:[],canonical:content({title:'',description:'',cases:[]})});
function Status({kind,value}:{kind:'state'|'execution'|'case'|'report'|'package';value:string}){const known=kind==='state'?planStates:kind==='execution'?executionStates:kind==='case'?caseStatuses:kind==='package'?packageEvidenceStates:reportEligibility;return <span className={'tp-status tp-status-'+value}>{known.includes(value as never)?t(kind+'.'+value):t('unknown_fact')}</span>;}
export function TestPlansWorkspace(props:TestPlansWorkspaceProps){return <TestPlanOwner key={JSON.stringify([props.session.user.id,props.session.organization.id,props.project])} {...props}/>;}
function TestPlanOwner(props:TestPlansWorkspaceProps){const [retry,setRetry]=useState(0);return <TestPlanScope key={retry} {...props} retryRead={()=>setRetry(value=>value+1)}/>;}

function TestPlanScope(props:TestPlansWorkspaceProps&{retryRead:()=>void}){
 useLocale();const {session,project,refresh,planID:requestedPlan='',runID='',tab='cases',catalogFilter=emptyCatalogFilter,onCatalogFilter,retryRead}=props,api=useApi(),callbacks=useRef(props);callbacks.current=props;
 const [accessError,setAccessError]=useState<unknown>(),[draft,setDraft]=useState<Draft>(),[taskID,setTaskID]=useState(''),[basisAck,setBasisAck]=useState(false),[reason,setReason]=useState(''),[notice,setNotice]=useState(''),[caseFilter,setCaseFilter]=useState(''),[activeCase,setActiveCase]=useState(-1),[defect,setDefect]=useState<DefectDraft>();
 const [rebasing,setRebasing]=useState(false),[adoption,setAdoption]=useState<DraftAdoption>();
 const bodyID=useId(),alive=useRef(true),lookup=useRef<AbortController|undefined>(undefined);
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;lookup.current?.abort();};},[]);
 const {state:catalogState,taskID:catalogTask,invalid:catalogInvalid}=catalogFilter;
 const validRecovery=useCallback((request:Parameters<typeof testPlanRecovery>[0])=>testPlanRecovery(request,project),[project]);
 const validResponse=useCallback((value:unknown,request:Parameters<typeof testPlanRecovery>[0])=>request.path.endsWith('/defects')||request.path.endsWith('/package-defects')?object(value)&&planID(value.id)&&value.project_id===project&&Number.isSafeInteger(value.version)&&Number(value.version)>0:validTestPlan(value,project),[project]);
 const action=useWriteAction({scope:JSON.stringify([session.user.id,session.organization.id,project,'test-plans']),storage:sessionStorage,validateRequest:validRecovery,validateResponse:validResponse});
 const pending=action.pending,originalPlan=pending?.path.match(/^\/test-plans\/([A-Za-z0-9_-]+)(?:\/|$)/)?.[1],selectedID=originalPlan||requestedPlan;
 const failed=useCallback((error:unknown)=>{if(error instanceof ApiError&&[401,403].includes(error.status))setAccessError(error);},[]);
 useEffect(()=>{if(action.errorStatus===401||action.errorStatus===403)setAccessError(new UiError('test-plans.access_changed'));},[action.errorStatus]);
 useEffect(()=>{if(accessError)setDefect(undefined);},[accessError]);
 const validPlan=useCallback((value:unknown):value is TestPlanView=>validTestPlan(value,project),[project]);
 const validCatalogPlan=useCallback((value:unknown):value is TestPlanView=>
  validPlan(value)&&(!catalogState||value.state===catalogState)&&(!catalogTask||value.task_id===catalogTask),[validPlan,catalogState,catalogTask]);
 const validPlans=useCallback((value:unknown):value is {items:TestPlanView[];total:number;next_cursor:string}=>validPlanPage(value,validCatalogPlan),[validCatalogPlan]);
 const validRecord=useCallback((value:unknown):value is RecordItem=>object(value)&&planID(value.id)&&Number.isSafeInteger(value.version)&&Number(value.version)>0&&value.project_id===project,[project]);
 const validRecords=useCallback((value:unknown):value is {items:RecordItem[];total:number;next_cursor:string}=>validPlanPage(value,validRecord),[validRecord]);
 const plans=useTestPlanPage(api,!accessError&&!catalogInvalid?testPlanCatalogPath(project,catalogFilter):undefined,refresh,validPlans,failed);
 const detail=useTestPlanRead(api,selectedID&&!accessError?testPlanPath(selectedID)+query(project):undefined,refresh,validPlan,failed),plan=detail.value;
 const tasks=useTestPlanPage(api,!selectedID&&!accessError?'/tasks'+query(project)+'&limit=100':undefined,refresh,validRecords,failed,'cursor');
 const actualTaskID=plan?.task_id||taskID||String(pending?.body.task_id||'');
 const task=useTestPlanRead(api,actualTaskID&&!accessError?'/tasks/'+encodeURIComponent(actualTaskID)+query(project):undefined,refresh,validRecord,failed);
 const requirementID=typeof task.value?.requirement_id==='string'?task.value.requirement_id:'';
 const requirement=useTestPlanRead(api,requirementID&&!accessError?'/requirements/'+encodeURIComponent(requirementID)+query(project):undefined,refresh,validRecord,failed);
 const current=plan?.id===selectedID?plan:undefined,visibleDraft=draft?.id===selectedID?draft:undefined,locked=action.busy||!!pending||!!accessError||rebasing;
 const rebaseScope=useRef({api,actualTaskID,selectedID,accessError,visibleDraft});
 rebaseScope.current={api,actualTaskID,selectedID,accessError,visibleDraft};
 const canCreate=['owner','admin'].includes(session.user.role||'')||['admin','reviewer','developer'].includes(session.projects.find(item=>item.id===project)?.role||'');
 // Existing-plan capabilities come from its current authorized read. Session
 // project grants govern only creation, never override a withheld save action.
 const canWrite=selectedID?current?.allowed_actions.includes('save')===true:canCreate;
 useEffect(()=>{
  setBasisAck(false);setReason('');
  if(current){setTaskID(current.task_id);setDraft(before=>before?.id===current.id&&content(before)!==before.canonical?before:fromPlan(current));}
  else if(!selectedID)setDraft(before=>before?.id===''?before:freshDraft(String(pending?.body.task_id||'')));
 },[current?.id,current?.version,selectedID]);
 const pendingDraft=useMemo(()=>pending&&['POST','PUT'].includes(pending.method)&&Array.isArray(pending.body.cases)&&typeof pending.body.title==='string'?pending:undefined,[pending]);
 useEffect(()=>{if(!pendingDraft)return;setDraft(before=>({...freshDraft(String(pendingDraft.body.task_id||before?.taskID||actualTaskID)),id:originalPlan||'',version:Number(pendingDraft.body.expected_version||0),taskVersion:Number(pendingDraft.body.expected_task_version||0),requirementVersion:Number(pendingDraft.body.expected_requirement_version||0),title:String(pendingDraft.body.title),description:String(pendingDraft.body.description||''),cases:structuredClone(pendingDraft.body.cases as QualityTestCase[]),canonical:before?.canonical||''}));},[pendingDraft?.idempotency_key]);
 const dirty=!!visibleDraft&&(content(visibleDraft)!==visibleDraft.canonical||!selectedID&&!!visibleDraft.taskID),versionChanged=!!current&&!!visibleDraft&&current.version!==visibleDraft.version;
 useLeaveGuard(dirty||!!defect?.reason.trim(),action.setError);
 useEffect(()=>{
  if(!adoption)return;
  // The draft now belongs to the verified destination. After the editor's
  // dirty guard releases, route through the usual guard for any other work.
  const scope=rebaseScope.current;
  if(scope.selectedID||scope.accessError||scope.actualTaskID!==adoption.previous.taskID){
   adoption.reject(new UiError('test-plans.refresh_failed'));
  }else if(callbacks.current.onSelectPlan(adoption.planID)===false){
   setDraft(adoption.previous);
   adoption.reject(new UiError('test-plans.refresh_failed'));
  }else adoption.complete();
  setAdoption(undefined);
 },[adoption]);
 const criteria=taskCriteria(task.value?.acceptance_criteria),requirementVersion=requirementID?requirement.value?.version:0,sourceLoaded=!!task.value&&(!requirementID||!!requirement.value),basisChanged=!!current&&(!current.basis_current||sourceLoaded&&(current.latest!.source.task.title!==(task.value?.title||'')||current.latest!.source.task.description!==(task.value?.description||'')||JSON.stringify(current.latest!.source.task.acceptance_criteria)!==JSON.stringify(criteria)||current.latest!.source.requirement?.version!==(requirementVersion||undefined)));
 const readError=[detail.error,task.error,requirement.error].find(Boolean),sourceReady=sourceLoaded&&!detail.loading&&!task.loading&&!requirement.loading&&!readError;
 // An unread task is not a task with zero criteria. Await the current source
 // before showing coverage errors; saving still requires that complete read.
 const errors=visibleDraft&&sourceReady?planDraftErrors(visibleDraft.title,visibleDraft.description,visibleDraft.cases,criteria):[],canSave=canWrite&&!locked&&!!visibleDraft&&sourceReady&&!errors.length&&!versionChanged&&(!basisChanged||basisAck)&&(!current||current.allowed_actions.includes('save'));
 const validateExecution=useCallback((value:unknown):value is TestPlanExecution=>validTestPlanExecution(value,selectedID,current?.task_id),[selectedID,current?.task_id]);
 const validateExecutions=useCallback((value:unknown):value is {items:TestPlanExecution[];total:number;next_cursor:string}=>validPlanPage(value,validateExecution),[validateExecution]);
 const executions=useTestPlanPage(api,tab==='results'&&current&&!accessError?testPlanPath(current.id)+'/executions'+query(project)+'&limit=20':undefined,refresh,validateExecutions,failed);
 const validateSelectedExecution=useCallback((value:unknown):value is TestPlanExecution=>validateExecution(value)&&value.run_id===runID,[validateExecution,runID]);
 const execution=useTestPlanRead(api,tab==='results'&&current&&runID&&!accessError?testPlanPath(current.id)+'/executions/'+encodeURIComponent(runID)+query(project):undefined,refresh,validateSelectedExecution,failed);
 const currentDefectBasis=!!current?.basis_current&&!!execution.value?.basis_current;
 const canCaseDefect=canWrite&&currentDefectBasis;
 const packageDefect=defect?.kind==='package';
 const defectOpen=!!defect&&!accessError;
 const defectCurrent=!!defect&&currentDefectBasis&&defect.planID===current?.id&&defect.version===current.version&&defect.runID===execution.value?.run_id&&(
  defect.kind==='package'
   ? execution.value.package_evidence.some(gate=>gate.node_id===defect.nodeID&&gate.can_create_issue&&gate.attempt===defect.attempt&&gate.fence===defect.fence)
   : canCaseDefect&&execution.value.cases.some(item=>item.case_key===defect.key&&['failed','error'].includes(item.status)&&!item.issue_id)
 );
 function edit(change:Partial<Pick<Draft,'title'|'description'|'cases'>>){if(locked)return;setDraft(before=>before?{...before,...change}:before);setNotice('');}
 async function changed(){try{await callbacks.current.onChanged();}catch{if(alive.current)action.setError(new UiError('test-plans.refresh_failed'));}}
 async function completed(value:TestPlanView|RecordItem|undefined,packageIssue=false){
  if(!value||!alive.current)return;
  if(!validTestPlan(value,project)){setDefect(undefined);setNotice(packageIssue?'package_defect_saved':'defect_saved');await execution.read();await changed();return;}
  if(validTestPlan(value,project)){
   const confirmedPlan=selectedID,requestedAtConfirmation=callbacks.current.planID||'';
   setDraft(fromPlan(value));setBasisAck(false);setReason('');setNotice('saved');
   // Clear this operation's draft before asking the parent's navigation guard.
   // A same-plan save needs no navigation, and a late list read cannot redirect
   // a different selection or a new owner back to this historical receipt.
   const fresh=await plans.read();if(!alive.current)return;
   if((fresh||catalogInvalid)&&value.id!==confirmedPlan&&(callbacks.current.planID||'')===requestedAtConfirmation)callbacks.current.onSelectPlan(value.id);
   await changed();
  }
 }
 async function save(){if(!canSave||!visibleDraft||!task.value)return;const body={project_id:project,...(current?{expected_version:visibleDraft.version}:{task_id:actualTaskID}),expected_task_version:task.value.version,expected_requirement_version:requirementVersion||0,title:visibleDraft.title,description:visibleDraft.description,cases:visibleDraft.cases};await completed(await action.run<TestPlanView>(testPlanPath(current?.id),body,current?'PUT':'POST'));}
 async function command(name:'publish'|'retire'){if(!current||locked||dirty||versionChanged||!sourceLoaded||readError||!current.allowed_actions.includes(name)||!planText(reason,2000))return;await completed(await action.run<TestPlanView>(testPlanPath(current.id)+'/commands',{project_id:project,expected_version:current.version,action:name,reason}));}
 async function rebase(){
  if(selectedID){
   const latest=await detail.read();
   if(!latest)throw new UiError('test-plans.refresh_failed');
   setDraft(before=>before?{...before,version:latest.version,canonical:content(latest.latest!)}:fromPlan(latest));
   setBasisAck(false);await task.read();if(requirementID)await requirement.read();
   return;
  }
  if(!planID(actualTaskID)||lookup.current)throw new UiError('test-plans.refresh_failed');
  const controller=new AbortController();lookup.current=controller;setRebasing(true);
  const ownScope=()=>{
   const scope=rebaseScope.current;
   return alive.current&&!controller.signal.aborted&&scope.api===api&&!scope.selectedID&&
    !scope.accessError&&scope.actualTaskID===actualTaskID&&scope.visibleDraft?.taskID===actualTaskID;
  };
  try{
   const page=await api<unknown>(testPlanCatalogPath(project,{taskID:actualTaskID},1),undefined,'GET',undefined,controller.signal);
   if(!ownScope())throw new UiError('test-plans.refresh_failed');
   if(!validPlanPage(page,validPlan)||page.total!==1||page.items.length!==1||page.next_cursor||
    page.items[0].task_id!==actualTaskID)throw new UiError('test-plans.existing_plan_hint');
   const existing=page.items[0],previous=rebaseScope.current.visibleDraft!;
   await new Promise<void>((complete,reject)=>{
    setDraft({...fromPlan(existing),title:previous.title,description:previous.description,cases:previous.cases});
    setAdoption({previous,planID:existing.id,complete,reject});
   });
  }catch(error){
   if(ownScope())failed(error);
   throw error;
  }finally{
   if(lookup.current===controller)lookup.current=undefined;
   if(alive.current)setRebasing(false);
  }
 }
 function openDefect(item:TestCaseResult|PackageGate){
  if(locked||!current||!execution.value||!currentDefectBasis)return;
  const basis={planID:current.id,version:current.version,runID:execution.value.run_id,reason:''};
  if('case_key' in item){if(!canCaseDefect||!['failed','error'].includes(item.status)||item.issue_id)return;setDefect({...basis,kind:'case',key:item.case_key});}
  else {if(!item.can_create_issue||!item.attempt||!item.fence||item.issue_id)return;setDefect({...basis,kind:'package',nodeID:item.node_id,attempt:item.attempt,fence:item.fence});}
  action.clearConflict();setNotice('');
 }
 async function registerDefect(){
  if(!defect||!defectCurrent||!planText(defect.reason,2000)||locked||action.conflict)return;
  const body={project_id:project,expected_version:defect.version,reason:defect.reason};
  const path=testPlanPath(defect.planID)+'/executions/'+encodeURIComponent(defect.runID);
  const target=defect.kind==='package'?{path:path+'/package-defects',body:{...body,node_id:defect.nodeID,expected_attempt:defect.attempt,expected_fence:defect.fence}}:{path:path+'/defects',body:{...body,case_key:defect.key}};
  await completed(await action.run<RecordItem>(target.path,target.body),defect.kind==='package');
 }
 async function recheckDefect(){
  // Refresh the selected execution while its plan still owns the route.
  const nextExecution=await execution.read(),nextPlan=await detail.read();
  if(!nextPlan||!nextExecution)throw new UiError('test-plans.refresh_failed');
 }
 const restore=pending&&<div className="tp-callout" role="status"><strong>{t('original_operation')}</strong><p>{t('restore_hint')}</p><button type="button" data-test-plan-restore disabled={action.busy} onClick={()=>void action.run<TestPlanView|RecordItem>(pending.path,pending.body,pending.method).then(value=>completed(value,pending.path.endsWith('/package-defects')))}>{t('restore')}</button></div>;
 return <section className="test-plans-workspace" aria-label={t('title')}>
  <div className="tp-intro"><div><h2>{t('title')}</h2><p>{t('intro')}</p></div>{canCreate&&<button type="button" disabled={locked} onClick={()=>{if(canLeave()){setDraft(freshDraft());setTaskID('');callbacks.current.onSelectPlan('');}}}><Plus size={15}/>{t('new_plan')}</button>}</div>
  {!defectOpen&&<><WriteNotice action={action} onRebase={rebasing?undefined:rebase}/>{rebasing&&<p className="tp-muted" role="status">{t('catalog_recovering')}</p>}{restore}</>}
  {notice&&<p className="tp-success" role="status">{t(notice)}</p>}
  {accessError?<div className="tp-access"><ErrorNotice error={accessError}/><button type="button" data-test-plan-retry onClick={retryRead}>{t('retry_access')}</button></div>:<div className="tp-layout">
   <aside className="tp-list" aria-label={t('plans')}>
    <div className="tp-list-heading"><h3>{t('plans')}</h3><button className="icon-button" type="button" disabled={plans.loading||locked||catalogInvalid} aria-label={t('refresh')} onClick={()=>void plans.read()}><RefreshCw size={15}/></button></div>
    {onCatalogFilter&&<CatalogFilters filter={catalogFilter} onChange={onCatalogFilter} disabled={locked}/>}
    {catalogInvalid&&<p className="tp-warning" role="alert">{t('catalog_invalid_link')}</p>}
    {plans.loading&&<Loading/>}
    {Boolean(plans.error)&&<ErrorNotice error={plans.error} onRetry={()=>void plans.read()}/>}
    {plans.value&&<>
     <p className="tp-muted" role="status" data-test-plan-catalog-count>{t('loaded',{loaded:formatNumber(plans.value.items.length),total:formatNumber(plans.value.total)})}</p>
     <div className="tp-plan-options">{plans.value.items.map(item=><button type="button" key={item.id} data-test-plan-item={item.id} aria-pressed={selectedID===item.id} disabled={locked} onClick={()=>{if(canLeave())callbacks.current.onSelectPlan(item.id);}}><strong>{item.latest?.title||item.id}</strong><small>{item.latest?.source.task.title||item.task_id}</small><Status kind="state" value={item.state}/></button>)}</div>
     {!plans.value.items.length&&<p className="tp-muted">{t(catalogState||catalogTask?'catalog_no_matches':'no_plans')}</p>}
     {plans.value.next_cursor&&<button type="button" data-test-plan-catalog-more disabled={plans.loading||locked} onClick={()=>void plans.read(true)}>{t('more')}</button>}
    </>}
   </aside>
   <div className="tp-main">
    {current&&(catalogState&&current.state!==catalogState||catalogTask&&current.task_id!==catalogTask)&&<p className="tp-muted" role="status">{t('catalog_selected_outside')}</p>}
    {detail.loading&&<Loading/>}{Boolean(readError)&&<ErrorNotice error={readError} onRetry={()=>{void detail.read();void task.read();void requirement.read();}}/>}
    {current&&<><div className="tp-heading"><div><Status kind="state" value={current.state}/><h3>{current.latest?.title}</h3><p className="tp-muted">{t('version',{version:formatNumber(current.version)})} · {t('revision',{revision:formatNumber(current.latest_revision)})}{current.published_revision>0&&<> · {t('published_revision',{revision:formatNumber(current.published_revision)})}</>}{current.has_draft&&<> · {t('has_draft')}</>}</p></div><button type="button" onClick={()=>{if(canLeave())callbacks.current.onOpenTask(current.task_id);}}>{t('open_task')}<ArrowRight size={14}/></button></div><p className={current.basis_current?'tp-muted':'tp-warning'}>{t('eligibility.'+current.eligibility)}</p><div className="tp-tabs" role="tablist" aria-label={t('plan_sections')} onKeyDown={event=>{if(!['ArrowLeft','ArrowRight','Home','End'].includes(event.key))return;const buttons=Array.from(event.currentTarget.querySelectorAll<HTMLButtonElement>('[role=tab]')),index=buttons.indexOf(document.activeElement as HTMLButtonElement),next=event.key==='Home'?0:event.key==='End'?buttons.length-1:(index+(event.key==='ArrowRight'?1:-1)+buttons.length)%buttons.length;event.preventDefault();buttons[next]?.focus();}}>{(['cases','results'] as const).map(value=><button type="button" role="tab" aria-selected={tab===value} tabIndex={tab===value?0:-1} id={bodyID+'-tab-'+value} aria-controls={bodyID+'-panel-'+value} key={value} onClick={()=>{if(canLeave())callbacks.current.onTab(value);}}>{t(value)}</button>)}</div></>}
    {tab==='cases'&&visibleDraft&&<section data-test-plan-editor role="tabpanel" id={bodyID+'-panel-cases'} aria-labelledby={bodyID+'-tab-cases'} className="tp-editor" aria-label={t('cases')}>
     <WriteFields action={action}><div className="tp-form-heading">{!selectedID&&<Field label={t('task')} hint={t('one_plan_per_task')}><select name="test_plan_task" value={taskID} disabled={locked||!!visibleDraft.title||!!visibleDraft.description||!!visibleDraft.cases.length} onChange={event=>{setTaskID(event.target.value);setDraft(freshDraft(event.target.value));}}><option value="">{t('select_task')}</option>{tasks.value?.items.map(item=><option value={item.id} key={item.id}>{item.title||item.id}</option>)}</select>{tasks.loading&&<Loading/>}{Boolean(tasks.error)&&<ErrorNotice error={tasks.error} onRetry={()=>void tasks.read()}/>} {tasks.value?.next_cursor&&<button type="button" onClick={()=>void tasks.read(true)}>{t('more_tasks')}</button>}</Field>}<Field label={t('plan_title')}><input name="test_plan_title" value={visibleDraft.title} maxLength={200} readOnly={!canWrite} onChange={event=>edit({title:event.target.value})}/></Field><Field label={t('description')}><textarea rows={2} value={visibleDraft.description} maxLength={4000} readOnly={!canWrite} onChange={event=>edit({description:event.target.value})}/></Field></div>
      {task.value&&<div className="tp-basis"><strong>{task.value.title||task.value.id}</strong><p>{t('task_version',{version:formatNumber(task.value.version)})}{requirement.value&&<> · {t('requirement_version',{version:formatNumber(requirement.value.version)})}</>}</p><details><summary>{t('task_criteria',{count:formatNumber(criteria.length)})}</summary>{criteria.length?<ol>{criteria.map((criterion,index)=><li key={index}>{criterion}</li>)}</ol>:<p>{t('no_criteria')}</p>}</details></div>}
      {basisChanged&&<div className="tp-warning"><p>{t('basis_changed')}</p><label className="tp-check"><input type="checkbox" checked={basisAck} onChange={event=>setBasisAck(event.target.checked)}/><span>{t('adopt_current_basis')}</span></label></div>}
      <div className="tp-case-heading"><div><h3>{t('cases')}</h3><p className="tp-muted">{t('automatic_only')}</p></div>{canWrite&&<button type="button" disabled={locked||visibleDraft.cases.length>=100} onClick={()=>{edit({cases:[...visibleDraft.cases,newQualityCase()]});setActiveCase(visibleDraft.cases.length);}}><Plus size={14}/>{t('add_case')}</button>}</div>
      <div className="tp-case-list">{visibleDraft.cases.map((item,index)=><CaseEditor key={index} value={item} index={index} open={activeCase===index} onToggle={open=>setActiveCase(current=>open?index:current===index?-1:current)} criteria={criteria} criteriaLoaded={!!task.value&&!task.loading&&!task.error} disabled={locked||!canWrite} onChange={value=>edit({cases:visibleDraft.cases.map((entry,i)=>i===index?value:entry)})} onRemove={()=>edit({cases:visibleDraft.cases.filter((_,i)=>i!==index)})}/>)}</div>{!visibleDraft.cases.length&&<div className="tp-empty-inline"><p>{t('no_cases')}</p><small>{t('draft_can_be_incomplete')}</small></div>}
     </WriteFields>
     {errors.length>0&&<ul className="tp-validation">{errors.map(error=><li key={error}>{t('validation.'+error)}</li>)}</ul>}{versionChanged&&<p className="tp-warning">{t('version_changed')}</p>}
     <div className="tp-actions">{canWrite&&<button className="primary" type="button" data-test-plan-save disabled={!canSave||action.conflict||action.persistenceBlocked} onClick={()=>void save()}>{action.busy?t('saving'):t('save_draft')}</button>}{dirty&&<button type="button" disabled={locked} onClick={()=>{setDraft(current?fromPlan(current):freshDraft());if(!current)setTaskID('');setBasisAck(false);}}>{t('discard_edits')}</button>}{current&&<button type="button" disabled={locked} onClick={()=>void rebase()}>{t('refresh_basis')}</button>}</div>
     {current&&current.allowed_actions.some(value=>value==='publish'||value==='retire')&&<div className="tp-publication"><p>{t('publication_hint')}</p><Field label={t('decision_reason')}><textarea name="test_plan_reason" rows={2} value={reason} maxLength={2000} disabled={locked} onChange={event=>setReason(event.target.value)}/></Field><div className="tp-actions">{current.allowed_actions.includes('publish')&&<button type="button" data-test-plan-publish disabled={locked||dirty||versionChanged||!sourceLoaded||!!readError||!current.basis_current||!planText(reason,2000)} onClick={()=>void command('publish')}><ShieldCheck size={14}/>{t('publish')}</button>}{current.allowed_actions.includes('retire')&&<button type="button" disabled={locked||dirty||versionChanged||!sourceLoaded||!!readError||!planText(reason,2000)} onClick={()=>void command('retire')}>{t('retire')}</button>}</div></div>}
    </section>}
    {tab==='results'&&current&&<section role="tabpanel" id={bodyID+'-panel-results'} aria-labelledby={bodyID+'-tab-results'} className="tp-results" aria-label={t('results')}><p className="tp-muted">{t('results_boundary')}</p>{executions.loading&&<Loading/>}{Boolean(executions.error)&&<ErrorNotice error={executions.error} onRetry={()=>void executions.read()}/>}<Field label={t('execution')}><select value={runID} disabled={locked} onChange={event=>{if(canLeave())callbacks.current.onSelectExecution(event.target.value);}}><option value="">{t('select_execution')}</option>{executions.value?.items.map(item=><option key={item.run_id} value={item.run_id}>{formatDateTime(item.created_at)} · {item.revision?t('revision',{revision:formatNumber(item.revision)}):t('no_bound_revision')} · {t('execution.'+item.state)}</option>)}</select></Field>{executions.value?.next_cursor&&<button type="button" disabled={executions.loading||locked} onClick={()=>void executions.read(true)}>{t('more_executions')}</button>}{executions.value&&!executions.value.items.length&&<Empty title={t('no_executions')} description={t('dispatch_existing_task')} action={<button type="button" onClick={()=>{if(canLeave())callbacks.current.onOpenTask(current.task_id);}}>{t('open_task')}</button>}/>}
     {execution.loading&&<Loading/>}{Boolean(execution.error)&&<ErrorNotice error={execution.error} onRetry={()=>void execution.read()}/>}{execution.value&&<><div className="tp-execution-heading"><Status kind="execution" value={execution.value.state}/><span>{execution.value.revision?t('revision',{revision:formatNumber(execution.value.revision)}):t('no_bound_revision')}</span><button type="button" onClick={()=>{if(canLeave())callbacks.current.onOpenRun(execution.value!.run_id);}}>{t('open_run')}<ArrowRight size={13}/></button></div><p className="tp-muted">{t('actual_run_state')} <Badge state={execution.value.run_state}/></p>{!execution.value.basis_current&&<p className="tp-warning">{t('execution_basis_changed')}</p>}<div className="tp-counts">{(['total','passed','failed','errors','skipped','not_run'] as const).map(key=><span key={key}><strong>{formatNumber(execution.value![key])}</strong>{t('count.'+key)}</span>)}</div>
      {execution.value.package_evidence.length>0&&<section className="tp-reports" aria-labelledby={bodyID+'-package-checks'}>
       <h3 id={bodyID+'-package-checks'}>{t('package_checks')}</h3>
       <p className="tp-muted">{t('package_checks_help')}</p>
        {execution.value.package_evidence.map(evidence=><PackageEvidence key={evidence.node_id} evidence={evidence} project={project} locked={locked} canDefect={currentDefectBasis&&evidence.can_create_issue} onRun={()=>{if(canLeave())callbacks.current.onOpenRun(runID,evidence.node_id,evidence.attempt);}} onIssue={id=>{if(canLeave())callbacks.current.onOpenIssue(id);}} onDefect={()=>openDefect(evidence)}/>)}
      </section>}
       <Field label={t('filter_cases')}><input value={caseFilter} onChange={event=>setCaseFilter(event.target.value)}/></Field><div className="tp-result-cases">{execution.value.cases.filter(item=>!caseFilter||(item.title+' '+item.case_key).toLocaleLowerCase().includes(caseFilter.toLocaleLowerCase())).map(item=><ResultCase key={item.case_key} item={item} locked={locked} canDefect={canCaseDefect} onRun={(node,attempt)=>{if(canLeave())callbacks.current.onOpenRun(runID,node,attempt);}} onIssue={id=>{if(canLeave())callbacks.current.onOpenIssue(id);}} onDefect={()=>openDefect(item)}/>)}</div>
      <section className="tp-reports"><h3>{t('reports')}</h3>{execution.value.reports.length?execution.value.reports.map(report=><Report key={report.id} project={project} planID={current.id} runID={runID} report={report} failed={failed}/>):<p className="tp-warning">{t('no_reports')}</p>}</section>
     </>}</section>}
   </div>
  </div>}
   <Modal open={defectOpen} onClose={()=>{if(!locked){setDefect(undefined);action.clearConflict();}}} title={t(packageDefect?'package_register_defect':'register_defect')} description={t(packageDefect?'package_defect_hint':'defect_hint')}>
    <WriteNotice action={action} onRebase={recheckDefect}/>{restore}
    {defect?.kind==='package'&&<p className="tp-muted">{t('package_defect_target',{node:defect.nodeID,attempt:formatNumber(defect.attempt)})}</p>}
    {!defectCurrent&&!locked&&!execution.loading&&!detail.loading&&<p className="tp-warning" role="status">{t('defect_target_changed')}</p>}
    <WriteFields action={action}><Field label={t('defect_reason')}><textarea name="test_plan_defect_reason" value={defect?.reason||''} maxLength={2000} rows={3} onChange={event=>setDefect(before=>before?{...before,reason:event.target.value}:before)}/></Field></WriteFields>
    {!pending&&<button className="primary" type="button" data-test-plan-defect disabled={locked||!defectCurrent||action.conflict||!planText(defect?.reason,2000)} onClick={()=>void registerDefect()}>{t('register_defect')}</button>}
   </Modal>
 </section>;
}

function CatalogFilters({filter,onChange,disabled}:{filter:TestPlanCatalogFilter;onChange:(filter:TestPlanCatalogFilter)=>void;disabled:boolean}){
 const [state,setState]=useState<TestPlanState|''>(filter.state||''),[taskID,setTaskID]=useState(filter.taskID||'');
 const validTask=!taskID||planID(taskID),errorID=useId();
 useEffect(()=>{setState(filter.state||'');setTaskID(filter.taskID||'');},[filter.state,filter.taskID,filter.invalid]);
 return <form className="tp-catalog-filters" aria-label={t('catalog_filters')} onSubmit={event=>{
  event.preventDefault();
  if(!disabled&&validTask)onChange({state:state||undefined,taskID:taskID||undefined});
 }}>
  <label>
   <span>{t('catalog_state')}</span>
   <select data-test-plan-filter-state value={state} disabled={disabled} onChange={event=>setState(isTestPlanState(event.target.value)?event.target.value:'')}>
    <option value="">{t('catalog_all_states')}</option>
    {planStates.map(item=><option key={item} value={item}>{t('state.'+item)}</option>)}
   </select>
  </label>
  <label>
   <span>{t('catalog_task_id')}</span>
   <input type="text" data-test-plan-filter-task value={taskID} disabled={disabled} maxLength={64} autoComplete="off" spellCheck={false}
    aria-invalid={!validTask||undefined} aria-describedby={!validTask?errorID:undefined} onChange={event=>setTaskID(event.target.value)}/>
  </label>
  {!validTask&&<p id={errorID} className="tp-muted" role="status">{t('catalog_invalid_task')}</p>}
  <div className="tp-catalog-filter-actions">
   <button type="submit" data-test-plan-filter-apply disabled={disabled||!validTask}>{t('catalog_apply')}</button>
   <button type="button" data-test-plan-filter-clear disabled={disabled||!state&&!taskID&&!filter.state&&!filter.taskID&&!filter.invalid}
    onClick={()=>{setState('');setTaskID('');onChange({});}}>{t('catalog_clear')}</button>
  </div>
 </form>;
}

function CaseEditor({value,index,criteria,criteriaLoaded,disabled,open,onToggle,onChange,onRemove}:{value:QualityTestCase;index:number;criteria:string[];criteriaLoaded:boolean;disabled:boolean;open:boolean;onToggle:(open:boolean)=>void;onChange:(value:QualityTestCase)=>void;onRemove:()=>void}){
 return <details className="tp-case" open={open} onToggle={event=>onToggle(event.currentTarget.open)}><summary><span className="tp-case-number">{formatNumber(index+1)}</span><span><strong>{value.title||t('untitled_case')}</strong><small>{value.selector.class_name?value.selector.class_name+' · ':''}{value.selector.name||t('selector_incomplete')}</small></span><ChevronDown size={15}/></summary>{open&&<fieldset className="form-fields tp-case-fields" disabled={disabled}><div className="tp-two-columns"><Field label={t('case_title')}><input value={value.title} maxLength={200} onChange={event=>onChange({...value,title:event.target.value})}/></Field><Field label={t('priority')}><select value={value.priority} onChange={event=>onChange({...value,priority:event.target.value})}>{['low','medium','high','urgent'].map(priority=><option value={priority} key={priority}>{tr('common.priority.'+priority)}</option>)}</select></Field></div><Field label={t('description')}><textarea rows={2} value={value.description} maxLength={4000} onChange={event=>onChange({...value,description:event.target.value})}/></Field><Field label={t('preconditions')}><textarea rows={2} value={value.preconditions} maxLength={4000} onChange={event=>onChange({...value,preconditions:event.target.value})}/></Field><Field label={t('steps')} hint={t('steps_hint')}><textarea rows={3} value={value.steps.join('\n')} onChange={event=>onChange({...value,steps:event.target.value?event.target.value.split('\n'):[]})}/></Field><Field label={t('expected_result')}><textarea rows={2} value={value.expected_result} maxLength={4000} onChange={event=>onChange({...value,expected_result:event.target.value})}/></Field><div className="tp-two-columns"><Field label={t('class_name')}><input value={value.selector.class_name} maxLength={500} spellCheck={false} onChange={event=>onChange({...value,selector:{...value.selector,class_name:event.target.value}})}/></Field><Field label={t('test_name')} hint={t('selector_hint')}><input value={value.selector.name} maxLength={500} spellCheck={false} onChange={event=>onChange({...value,selector:{...value.selector,name:event.target.value}})}/></Field></div><fieldset className="tp-criteria" disabled={!criteriaLoaded}><legend>{t('covers_task')}</legend>{!criteriaLoaded?<Loading/>:criteria.length?criteria.map((criterion,index)=><label className="tp-check" key={index}><input type="checkbox" checked={value.covers.includes(index+1)} onChange={event=>onChange({...value,covers:event.target.checked?[...value.covers,index+1].sort((a,b)=>a-b):value.covers.filter(item=>item!==index+1)})}/><span>{formatNumber(index+1)}. {criterion}</span></label>):<p className="tp-muted">{t('no_criteria')}</p>}</fieldset><details className="tp-key"><summary>{t('stable_key')}</summary><Field label={t('case_key')}><input value={value.key} maxLength={64} spellCheck={false} onChange={event=>onChange({...value,key:event.target.value})}/></Field></details><button type="button" className="tp-remove" onClick={onRemove}><Trash2 size={13}/>{t('remove_case')}</button></fieldset>}</details>;
}
function ResultCase({item,locked,canDefect,onRun,onIssue,onDefect}:{item:TestCaseResult;locked:boolean;canDefect:boolean;onRun:(node:string,attempt:number)=>void;onIssue:(id:string)=>void;onDefect:()=>void}){
 return <article className="tp-result-case" data-test-case-result={item.case_key}><div><strong>{item.title}</strong><Status kind="case" value={item.status}/></div><small className="tp-muted"><code>{item.case_key}</code>{item.covers.length>0&&<> · {t('covers_indices',{indices:item.covers.map(index=>formatNumber(index)).join(', ')})}</>}</small>{item.evidence.length?<details><summary>{t('case_evidence',{count:formatNumber(item.evidence.length)})}</summary><ul>{item.evidence.map((evidence,index)=><li key={evidence.report_id+'-'+index}><Status kind="case" value={evidence.status}/><button type="button" disabled={locked} onClick={()=>onRun(evidence.node_id,evidence.attempt)}>{evidence.node_id} · {t('attempt',{attempt:formatNumber(evidence.attempt)})}<ArrowRight size={12}/></button><code>{evidence.source_commit||t('not_recorded')}</code><small>{t('duration',{duration:formatNumber(evidence.duration_ms)})}</small>{evidence.message&&<pre>{evidence.message}</pre>}</li>)}</ul></details>:<p className="tp-muted">{t('case_no_evidence')}</p>}{item.issue_id?<button type="button" disabled={locked} onClick={()=>onIssue(item.issue_id)}>{t('open_issue')}<ArrowRight size={12}/></button>:['failed','error'].includes(item.status)&&canDefect&&<button type="button" disabled={locked} onClick={onDefect}>{t('register_defect')}</button>}</article>;
}
function PackageEvidence({evidence,project,locked,canDefect,onRun,onIssue,onDefect}:{evidence:PackageGate;project:string;locked:boolean;canDefect:boolean;onRun:()=>void;onIssue:(id:string)=>void;onDefect:()=>void}){
 const verified=evidence.state==='verified';
 const issueID=evidence.issue_id;
 const artifact=verified&&evidence.artifact_id?'?'+new URLSearchParams({project,view:'operations',sub:'artifacts',record:evidence.artifact_id}):'';
 return <article className="tp-report tp-package" data-test-package-evidence={evidence.node_id}>
  <div><ShieldCheck aria-hidden="true" size={16}/><strong>{evidence.node_id}</strong><Status kind="package" value={evidence.state}/></div>
  <dl className="tp-package-facts">
   <dt>{t('package_source_node')}</dt><dd><code>{evidence.source_node_id}</code></dd>
   {evidence.worker_id&&<><dt>{t('package_worker')}</dt><dd><code>{evidence.worker_id}</code></dd></>}
   {evidence.source_worker_id&&<><dt>{t('package_source_worker')}</dt><dd><code>{evidence.source_worker_id}</code></dd></>}
   {verified&&<><dt>{t('package_manifest')}</dt><dd><code>{evidence.manifest_sha256}</code></dd><dt>{t('package_commit')}</dt><dd><code>{evidence.source_commit}</code></dd></>}
  </dl>
  <div className="tp-actions tp-package-actions">
   <button type="button" disabled={locked} onClick={onRun}>{evidence.attempt?t('package_open_attempt',{attempt:formatNumber(evidence.attempt)}):t('package_open_node')}<ArrowRight aria-hidden="true" size={13}/></button>
   {artifact&&!locked&&<a href={artifact} onClick={event=>{if(!canLeave())event.preventDefault();}}>{t('package_open_artifact')}<ArrowRight aria-hidden="true" size={13}/></a>}
   {issueID?<button type="button" data-test-package-open-issue disabled={locked} onClick={()=>onIssue(issueID)}>{t('open_issue')}<ArrowRight aria-hidden="true" size={13}/></button>:canDefect&&<button type="button" data-test-package-defect disabled={locked} onClick={onDefect}>{t('register_defect')}</button>}
  </div>
 </article>;
}
function Report({project,planID,runID,report,failed}:{project:string;planID:string;runID:string;report:TestPlanExecution['reports'][number];failed:(error:unknown)=>void}){
 const api=useApi(),[open,setOpen]=useState(false),valid=useCallback((value:unknown):value is TestPlanReport=>validTestPlanReport(value,runID,report.id),[runID,report.id]);
 const read=useTestPlanRead(api,open?testPlanPath(planID)+'/reports/'+encodeURIComponent(report.id)+query(project):undefined,0,valid,failed);
 return <article className="tp-report"><div><FileText size={16}/><strong>{report.node_id}</strong><Status kind="report" value={report.eligibility}/></div><small className="tp-muted">{t('attempt',{attempt:formatNumber(report.attempt)})} · {t('unmapped',{count:formatNumber(report.unmapped)})}</small>{['current','source_changed'].includes(report.eligibility)?<p className="tp-muted">{t('report_counts',{total:formatNumber(report.total),passed:formatNumber(report.passed),failed:formatNumber(report.failed+report.errors),skipped:formatNumber(report.skipped)})}</p>:<p className="tp-muted">{t('report_unverified')}</p>}<small className="tp-muted">{report.outcome_unknown?t('report_unknown'):t('process_exit',{code:formatNumber(report.process_exit_code)})}</small><code>{report.source_commit||t('not_recorded')}</code><code>{report.sha256||t('not_recorded')}</code><button type="button" onClick={()=>setOpen(value=>!value)}>{t(open?'close_original':'open_original')}</button>{open&&<div>{read.loading&&<Loading/>}{Boolean(read.error)&&<ErrorNotice error={read.error} onRetry={()=>void read.read()}/>} {read.value&&<pre className="tp-original" tabIndex={0}>{read.value.xml||t('empty_original')}</pre>}</div>}</article>;
}
