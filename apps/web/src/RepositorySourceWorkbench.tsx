import {useEffect,useId,useRef,useState} from 'react';
import {ArrowUpRight,GitCommitHorizontal,Play,RefreshCw,ShieldCheck} from 'lucide-react';
import type {TaskDeliveryRepositoryIntake} from '../../../packages/contracts/src';
import {ApiError,query} from './api';
import {useApi} from './api-context';
import {formatDateTime,formatNumber,tr,useLocale} from './i18n';
import {useReplyRead} from './task-conversation-reply-hooks';
import {validManagedRunIdentity} from './task-dispatch-response';
import {toolSessionStorage} from './tool-hooks';
import {ErrorNotice,Loading} from './ui';
import {useWriteAction,WriteNotice} from './write-action';
import {repositoryPlanLocator,repositoryPlanSource,repositoryWorkbenchQuery,savedWorkbenchPlan,validRepositoryIntake,validRepositoryOptions,validRepositoryPlan,validWorkbenchWrite,
  workbenchScopeKey,type RepositoryWorkbenchOptions,type RepositoryWorkbenchPlan,type RepositoryWorkbenchProfile,type RepositoryWorkbenchScope} from './repository-source-workbench-model';

const t = (key: string, values?: Record<string,unknown>) => tr('task-delivery.repository_source.workbench.' + key, values);
const noop = () => {};
type Props = {
  owner: string; organization: string; project: string; task: string; scope: RepositoryWorkbenchScope; checkedHead?: string; current: boolean; disabled: boolean;
  onClose: () => void; onLockChange: (locked: boolean) => void; onOpenRun?: (run: string) => void;
};

export function RepositorySourceWorkbench({owner,organization,project,task,scope,checkedHead,current,disabled,onClose,onLockChange,onOpenRun}: Props) {
  useLocale();
  const api=useApi(),storage=toolSessionStorage(),field=useId(),heading=useRef<HTMLHeadingElement>(null);
  const scopeKey=workbenchScopeKey(owner,scope),planKey='suma-repository-workbench-plan:'+scopeKey;
  const root='/tasks/'+encodeURIComponent(task)+'/delivery/repository-source',intakePath=root+'/intakes';
  const [savedPlan,setSavedPlan]=useState(()=>{try{return savedWorkbenchPlan(storage?.getItem(planKey));}catch{return undefined;}});
  const planID=savedPlan?.id;
  const [confirmedPlan,setConfirmedPlan]=useState<RepositoryWorkbenchPlan>();
  const [createdIntake,setCreatedIntake]=useState<TaskDeliveryRepositoryIntake>();
  const [intakeID,setIntakeID]=useState(''),[producerID,setProducerID]=useState(''),[testID,setTestID]=useState('');
  const [approveConsent,setApproveConsent]=useState(false),[startConsent,setStartConsent]=useState(false);
  const confirmed=useRef<RepositoryWorkbenchPlan|undefined>(undefined);
  const callbacks=useRef({onLockChange});callbacks.current={onLockChange};
  const validPlan=(value:unknown):value is RepositoryWorkbenchPlan=>validRepositoryPlan(value,project,task,scope)&&
    (!value.execution_route||value.execution_route.tenant_id===organization);
  const matchesSavedPlan=(value:unknown):value is RepositoryWorkbenchPlan=>validPlan(value)&&!!savedPlan&&value.id===savedPlan.id&&
    repositoryPlanLocator(value).identity===savedPlan.identity;
  const action=useWriteAction({scope:scopeKey,storage,allowLeaveWithPending:true,
    validateRequest:request=>validWorkbenchWrite(request,project,intakePath,scope,planID),
    validateResponse:async(value,request)=>{
      const {path,body}=request;
      if(path===intakePath) return validRepositoryIntake(value,project,task,scope) && value.head_commit===body.expected_head && value.base_commit===body.expected_base;
      if(path.endsWith('/dispatch-plans')){
        if(!validPlan(value))return false;
        const manifest=repositoryPlanSource(value),binding=value.test_bindings[0];
        const requestedIntake=path.slice(intakePath.length+1,-'/dispatch-plans'.length);
        const retained=[...(optionsRead.value?.retained_intakes??[]),...(createdIntake?[createdIntake]:[])].find(item=>item.id===requestedIntake);
        if(manifest.intake_id!==requestedIntake||retained&&(manifest.head_commit!==retained.head_commit||manifest.base_commit!==retained.base_commit||manifest.bundle_sha256!==retained.bundle_sha256||manifest.size_bytes!==retained.size_bytes)||
          value.tool_profile_id!==body.producer_profile_id||value.tool_profile_version!==body.expected_producer_profile_version||
          binding.tool_profile_id!==body.test_profile_id||binding.tool_profile_version!==body.expected_test_profile_version)return false;
        // Save the locator before the original operation is acknowledged. A
        // response loss or storage failure must not create a second plan.
        if(!storage)return false;
        storage.setItem(planKey,JSON.stringify(repositoryPlanLocator(value)));
        return true;
      }
      if(!planID||path!=='/task-dispatch-plans/'+planID+'/start')return false;
      const original=await api<unknown>('/task-dispatch-plans/'+encodeURIComponent(planID)+query(project));
      if(!matchesSavedPlan(original)||original.state!=='started'||original.version!==Number(body.expected_version)+1||
        !value||typeof value!=='object'||!('tenant_id' in value)||value.tenant_id!==organization||
        !validManagedRunIdentity(value,project,original.run_id))return false;
      confirmed.current=original;
      return true;
    }});
  const denied=action.errorStatus===401||action.errorStatus===403;
  const optionsRead=useReplyRead(root+'/options'+repositoryWorkbenchQuery(project,scope),current&&!denied,
    (value):value is RepositoryWorkbenchOptions=>validRepositoryOptions(value,project,task,scope),noop);
  const planRead=useReplyRead(planID?'/task-dispatch-plans/'+encodeURIComponent(planID)+query(project):'',!!planID&&!denied,
    matchesSavedPlan,noop);
  const options=optionsRead.error?undefined:optionsRead.value;
  const plan=planRead.error||denied?undefined:confirmedPlan||planRead.value;
  const locked=action.busy||!!action.pending;
  const blocked=disabled||denied||locked||action.persistenceBlocked||action.conflict;
  const intakes=[...new Map([...(options?.retained_intakes??[]),...(createdIntake?[createdIntake]:[])].map(item=>[item.id,item])).values()];
  const intake=intakes.find(item=>item.id===intakeID)??(intakes.length===1?intakes[0]:undefined);
  const producers=(options?.producer_profiles??[]).filter(profile=>options?.test_profiles.some(test=>test.worker_id!==profile.worker_id));
  const producer=producers.find(profile=>profile.id===producerID)??(producers.length===1?producers[0]:undefined);
  const tests=(options?.test_profiles??[]).filter(profile=>producer&&producer.worker_id!==profile.worker_id);
  const tester=tests.find(profile=>profile.id===testID)??(tests.length===1?tests[0]:undefined);
  const live=options?.live_state==='ready';
  const alreadyRetained=!!options&&intakes.some(item=>item.base_commit===options.base_commit&&item.head_commit===options.head_commit);
  const candidateReady=current&&!!options&&!optionsRead.loading&&!options.active_terminal&&options.published_case_count>0;
  const canStart=candidateReady&&!!plan&&plan.state==='ready'&&plan.valid&&!planRead.loading;
  const canRecoverStart=!!plan&&!planRead.loading&&(plan.state==='started'||canStart);
  const pinnedProducer=options?.producer_profiles.find(profile=>profile.id===plan?.tool_profile_id&&profile.version===plan?.tool_profile_version);
  const pinnedTest=options?.test_profiles.find(profile=>profile.id===plan?.test_bindings[0].tool_profile_id&&profile.version===plan?.test_bindings[0].tool_profile_version);
  const busyRead=!!optionsRead.loading||!!planRead.loading;
  const activeStep=plan?2:intake?1:0;
  useEffect(()=>{callbacks.current.onLockChange(locked);return ()=>callbacks.current.onLockChange(false);},[locked]);
  useEffect(()=>{heading.current?.focus();},[]);
  useEffect(()=>{setApproveConsent(false);},[options?.base_commit,options?.head_commit,optionsRead.loading]);
  useEffect(()=>{setStartConsent(false);},[plan?.id,plan?.version,plan?.valid,planRead.loading]);

  function receive(value: unknown,path: string){
    if(path===intakePath&&validRepositoryIntake(value,project,task,scope)){
      setCreatedIntake(value);setIntakeID(value.id);setApproveConsent(false);
    }else if(path.endsWith('/dispatch-plans')&&validPlan(value)){
      setSavedPlan(repositoryPlanLocator(value));setStartConsent(false);
    }else if(confirmed.current){setConfirmedPlan(confirmed.current);setStartConsent(false);}
  }
  async function approve(){
    if(blocked||!current||!options||!live||optionsRead.loading||!approveConsent||alreadyRetained)return;
    const result=await action.run<unknown>(intakePath,{project_id:project,task_version:scope.taskVersion,run_id:scope.runID,target_id:scope.targetID,
      target_version:scope.targetVersion,expected_head:options.head_commit,expected_base:options.base_commit});
    if(result)receive(result,intakePath);
  }
  async function preview(){
    if(blocked||!candidateReady||!intake||!producer||!tester)return;
    const path=intakePath+'/'+encodeURIComponent(intake.id)+'/dispatch-plans';
    const result=await action.run<unknown>(path,{project_id:project,expected_task_version:scope.taskVersion,producer_profile_id:producer.id,
      expected_producer_profile_version:producer.version,test_profile_id:tester.id,expected_test_profile_version:tester.version});
    if(result)receive(result,path);
  }
  async function start(){
    if(blocked||!canStart||!plan||!startConsent)return;
    const path='/task-dispatch-plans/'+encodeURIComponent(plan.id)+'/start';
    const result=await action.run<unknown>(path,{project_id:project,expected_version:plan.version});
    if(result)receive(result,path);
  }
  async function recover(){
    const pending=action.pending;
    if(!pending||disabled||denied||action.busy)return;
    if(pending.path.endsWith('/start')&&!canRecoverStart)return;
    const result=await action.run<unknown>(pending.path,pending.body,pending.method);
    if(result)receive(result,pending.path);
  }
  function forgetPlan(){
    if(locked||!current)return;
    try{storage?.removeItem(planKey);}catch{return;}
    setSavedPlan(undefined);setConfirmedPlan(undefined);confirmed.current=undefined;
  }

  return <section className="repository-workbench" aria-labelledby={field+'-title'} aria-busy={(busyRead||action.busy)||undefined}>
    <header className="repository-workbench-heading">
      <h5 id={field+'-title'} tabIndex={-1} ref={heading}>{t('title')}</h5>
      <button type="button" disabled={locked} onClick={onClose}>{t('close')}</button>
    </header>
    <ol className="repository-workbench-steps" aria-label={t('progress')}>
      {['retain_step','preview_step','start_step'].map((step,index)=><li key={step} aria-current={index===activeStep?'step':undefined}><span aria-hidden="true">{formatNumber(index+1)}</span>{t(step)}</li>)}
    </ol>
    <WriteNotice action={action} onRebase={async()=>{setApproveConsent(false);setStartConsent(false);if(planID)await planRead.read();else await optionsRead.read();}}/>
    {action.pending&&<div className="repository-workbench-callout" role="status"><p>{t('recover_note')}</p>
      <button type="button" data-repository-source-recover disabled={disabled||denied||action.busy||(action.pending.path.endsWith('/start')&&!canRecoverStart)} onClick={()=>void recover()}>{t('recover')}</button></div>}
    {busyRead&&<Loading/>}
    {!!optionsRead.error&&(optionsRead.error instanceof ApiError&&[401,403].includes(optionsRead.error.status)?<p role="status">{t('approver_required')}</p>:
      <ErrorNotice error={optionsRead.error} onRetry={()=>void optionsRead.read()}/>)}
    {!!planRead.error&&<ErrorNotice error={planRead.error} onRetry={()=>void planRead.read()}/>}
    {!denied&&options&&!planID&&<>
      <div className="repository-workbench-card">
        <strong>{t('review_source')}</strong>
        {options.live_state!=='unavailable'&&<CommitFacts head={options.head_commit} base={options.base_commit} baselineRef={options.baseline_ref}/>}
        {options.live_state!=='ready'&&<p className="task-delivery-muted" role="status">{t(options.live_state==='unchanged'?'same_commit':'basis_unavailable')}</p>}
        {live&&checkedHead&&options.head_commit!==checkedHead&&<p className="task-delivery-warning" role="status">{t('check_again')}</p>}
        {live&&!alreadyRetained&&<>
          <label className="repository-workbench-consent"><input type="checkbox" data-repository-source-approve-consent disabled={blocked||busyRead} checked={approveConsent} onChange={event=>setApproveConsent(event.target.checked)}/>{t('approve_consent')}</label>
          <button type="button" data-repository-source-approve disabled={blocked||busyRead||!approveConsent} onClick={()=>void approve()}><ShieldCheck size={14} aria-hidden="true"/>{t('approve')}</button>
        </>}
      </div>
      {intakes.length>0&&<div className="repository-workbench-card">
        <label htmlFor={field+'-intake'}>{t('retained_source')}</label>
        <select id={field+'-intake'} data-repository-source-intake disabled={blocked||busyRead} value={intake?.id??''} onChange={event=>setIntakeID(event.target.value)}>
          <option value="">{t('select_intake')}</option>{intakes.map(item=><option key={item.id} value={item.id}>{t('intake_option',{commit:item.head_commit.slice(0,12),at:formatDateTime(item.approved_at,{dateStyle:'short',timeStyle:'short'})})}</option>)}
        </select>
        {intake&&<><CommitFacts head={intake.head_commit} base={intake.base_commit}/><p className="task-delivery-muted" role="status">{t('retained_only')}</p></>}
      </div>}
      {intake&&<div className="repository-workbench-card">
        <strong>{t('workers')}</strong>
        <p className="task-delivery-muted">{t('workers_note')}</p>
        {options.active_terminal&&<p className="task-delivery-warning" role="status">{t('active_terminal')}</p>}
        {options.published_case_count===0&&<p className="task-delivery-warning" role="status">{t('no_cases')}</p>}
        {options.profiles_truncated&&<p className="task-delivery-muted" role="status">{t('profiles_truncated')}</p>}
        {!producers.length&&<p className="task-delivery-warning" role="status">{t('no_workers')}</p>}
        <div className="repository-workbench-workers">
          <ProfileField field={field+'-producer'} kind="producer" value={producer} profiles={producers} disabled={blocked||busyRead} onChange={value=>{setProducerID(value);setTestID('');}}/>
          <ProfileField field={field+'-tester'} kind="tester" value={tester} profiles={tests} disabled={blocked||busyRead||!producer} onChange={setTestID}/>
        </div>
        <button type="button" className="primary" data-repository-source-preview disabled={blocked||!candidateReady||!producer||!tester} onClick={()=>void preview()}><GitCommitHorizontal size={14} aria-hidden="true"/>{t('preview')}</button>
      </div>}
    </>}
    {!denied&&plan&&<div className="repository-workbench-card" data-repository-source-plan>
      <strong role="status">{t(plan.state==='started'?'started':'preview_ready')}</strong>
      <CommitFacts head={repositoryPlanSource(plan).head_commit} base={repositoryPlanSource(plan).base_commit}/>
      <dl className="repository-workbench-facts">
        <div><dt>{t('producer')}</dt><dd>{pinnedProducer?.name??plan.tool_profile_id} · {t('profile_version',{version:formatNumber(plan.tool_profile_version)})}{pinnedProducer&&<small>{pinnedProducer.worker_id}</small>}</dd></div>
        <div><dt>{t('tester')}</dt><dd>{pinnedTest?.name??plan.test_bindings[0].tool_profile_id} · {t('profile_version',{version:formatNumber(plan.test_bindings[0].tool_profile_version)})}</dd></div>
        <div><dt>{t('test_worker')}</dt><dd><code>{plan.test_bindings[0].worker_id}</code></dd></div>
      </dl>
      {plan.state==='started'?<><p className="task-delivery-muted">{t('started_note')}</p>
        <a className="task-delivery-reference" data-repository-source-open-run href={'?'+new URLSearchParams({project,view:'development',sub:'runs',run:plan.run_id})} onClick={event=>{
          if(event.button!==0||event.ctrlKey||event.metaKey||event.shiftKey||event.altKey)return;
          if(!window.dispatchEvent(new Event('suma:navigate',{cancelable:true}))){event.preventDefault();return;}
          if(onOpenRun){event.preventDefault();onOpenRun(plan.run_id);}
        }}>{t('open_run')}<ArrowUpRight size={13} aria-hidden="true"/></a></>:<>
        <p className="task-delivery-muted">{t('preview_note')}</p>
        {!plan.valid&&<p role="status" className="task-delivery-warning">{t('plan_stale')}</p>}
        <label className="repository-workbench-consent"><input type="checkbox" data-repository-source-start-consent checked={startConsent} disabled={blocked||!canStart} onChange={event=>setStartConsent(event.target.checked)}/>{t('start_consent')}</label>
        <div className="repository-workbench-buttons">
          <button type="button" className="primary" data-repository-source-start disabled={blocked||!canStart||!startConsent} onClick={()=>void start()}><Play size={14} aria-hidden="true"/>{t('start')}</button>
          <button type="button" disabled={locked||!current} onClick={forgetPlan}>{t('new_preview')}</button>
        </div>
      </>}
    </div>}
    {!optionsRead.error&&!planID&&current&&<button type="button" className="repository-workbench-refresh" disabled={locked||busyRead} onClick={()=>void optionsRead.read()}><RefreshCw size={13} aria-hidden="true"/>{t('refresh')}</button>}
  </section>;
}

function CommitFacts({head,base,baselineRef}:{head:string;base:string;baselineRef?:string}){
  return <dl className="repository-workbench-facts">
    <div><dt>{t('source_commit')}</dt><dd><code>{head}</code></dd></div>
    <div><dt>{t('baseline_commit')}</dt><dd><code>{base}</code>{baselineRef&&<small>{baselineRef}</small>}</dd></div>
  </dl>;
}

function ProfileField({field,kind,value,profiles,disabled,onChange}:{field:string;kind:'producer'|'tester';value?:RepositoryWorkbenchProfile;profiles:RepositoryWorkbenchProfile[];disabled:boolean;onChange:(value:string)=>void}){
  return <div>
    <label htmlFor={field}>{t(kind)}</label>
    <select id={field} {...{['data-repository-source-'+kind]:true}} value={value?.id??''} disabled={disabled||!profiles.length} onChange={event=>onChange(event.target.value)}>
      <option value="">{t('choose_profile')}</option>{profiles.map(profile=><option key={profile.id} value={profile.id}>{t('profile_option',{name:profile.name,version:formatNumber(profile.version),worker:profile.worker_id})}</option>)}
    </select>
  </div>;
}
