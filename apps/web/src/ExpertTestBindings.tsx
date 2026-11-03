import {useCallback,useEffect,useMemo,useRef,useState} from 'react';
import type {ExpertTeamRole} from '../../../packages/contracts/src';
import {query,type Workflow} from './api';
import {useApi,useApiView} from './api-context';
import {tr,useLocale} from './i18n';
import {ErrorNotice,Field,Loading} from './ui';
import {useToolList,toolError} from './tool-hooks';
import {parseToolManifest,type ToolProfile} from './tool-model';
import {validTerminalCandidateTools} from './terminal-candidate-model';
import {sameToolProfile} from './tool-profile-reuse-model';
import {expertTool} from './expert-team-model';
import {useExpertRead} from './expert-team-hooks';
import {TaskDispatchTestBindings,type TestBindingsChoice} from './TaskDispatchTestBindings';
import {FrozenTestBindingFacts} from './ToolFacts';
import {expertTestCanRetain,expertTestContext,expertTestRoleBasis,expertTestScope,type ExpertTestChoices,type ExpertTestMode,type ExpertTestTask} from './expert-test-bindings-model';
import './ExpertTestBindings.css';

export type {ExpertTestChoices,ExpertTestTask} from './expert-test-bindings-model';
type Props={project:string;tasks:ExpertTestTask[];disabled:boolean;onChange:(value:ExpertTestChoices)=>void};
const t=(key:string)=>tr('expert-test-bindings.'+key);
const started=(task:ExpertTestTask)=>!!task.item?.run_id||!!task.item?.dispatch_plan_id;
const validWorkflows=(value:unknown):value is {items:Workflow[]}=>!!value&&typeof value==='object'&&'items'in value&&Array.isArray(value.items)&&value.items.length<=1000&&value.items.every(flow=>flow&&typeof flow.id==='string'&&Number.isSafeInteger(flow.version)&&flow.version>0&&Array.isArray(flow.nodes)&&Array.isArray(flow.edges)&&flow.nodes.every((node:unknown)=>!!node&&typeof node==='object'&&'id'in node&&typeof node.id==='string'&&'type'in node&&typeof node.type==='string'));

/** Per-item intent over the original dispatcher. Only explicit adjustments
 * mount the shared directory and original exact-profile selector. */
export function ExpertTestBindings(props:Props){
 const api=useApi(),view=useApiView(),scope=expertTestScope(props.project,props.tasks),basis=JSON.stringify([view?.userID,view?.organizationID,expertTestContext(props.project,props.tasks)]);
 const owner=useRef({api,basis,epoch:0});if(owner.current.api!==api||owner.current.basis!==basis)owner.current={api,basis,epoch:owner.current.epoch+1};
 const current=owner.current;
 return <TestScope key={current.epoch} {...props} scope={scope} disabled={props.disabled||!view?.userID||!view.organizationID} owns={()=>owner.current===current}/>;
}
function TestScope({project,tasks,scope,disabled,onChange,owns}:Props&{scope:string;owns:()=>boolean}){
 useLocale();const [modes,setModes]=useState<Record<string,ExpertTestMode>>({}),[choices,setChoices]=useState<Record<string,TestBindingsChoice>>({}),alive=useRef(true),callbacks=useRef(onChange);callbacks.current=onChange;
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;};},[]);
 useEffect(()=>{
  if(!alive.current||!owns())return;
  const selections:ExpertTestChoices['selections']={};
  const ready=tasks.length>0&&tasks.every(task=>{
   if(started(task)){selections[task.taskID]=undefined;return true;}
   const mode=modes[task.taskID]||'retain';
   if(mode==='retain'){selections[task.taskID]=undefined;return expertTestCanRetain(task);}
   if(mode==='definition'){selections[task.taskID]=[];return true;}
   const choice=choices[task.taskID];selections[task.taskID]=choice?.selections;
   return !disabled&&!!choice?.ready&&choice.hasIntent&&choice.selections.length>0;
  });
  callbacks.current({scope,ready,selections});
 },[scope,modes,choices,disabled]);
 const editing=tasks.filter(task=>!started(task)&&modes[task.taskID]==='adjust');
 const update=useCallback((taskID:string,value:TestBindingsChoice)=>{if(alive.current&&owns())setChoices(previous=>({...previous,[taskID]:value}));},[scope]);
 function mode(taskID:string,value:ExpertTestMode){if(disabled||!owns())return;setModes(previous=>({...previous,[taskID]:value}));setChoices(previous=>{const next={...previous};delete next[taskID];return next;});}
 return <section className="expert-test-bindings" aria-label={t('title')} data-expert-test-bindings><h4>{t('title')}</h4><p className="team-dispatch-muted">{t('intro')}</p>
  {tasks.map(task=><details className="expert-test-task" data-expert-test-task={task.taskID} key={task.taskID}><summary>{task.title}</summary>
   <FrozenTestBindingFacts bindings={task.item?.plan.test_bindings}/>
   {started(task)?<p className="team-dispatch-muted" data-expert-test-started>{t('started')}</p>:<>
    <Field label={t('mode')}><select data-expert-test-mode={task.taskID} value={modes[task.taskID]||'retain'} disabled={disabled} onChange={event=>mode(task.taskID,event.target.value as ExpertTestMode)}><option value="retain">{t('retain')}</option><option value="adjust">{t('adjust')}</option><option value="definition">{t('definition')}</option></select></Field>
    {(modes[task.taskID]||'retain')==='retain'&&<p className={expertTestCanRetain(task)?'team-dispatch-muted':'warning'} data-expert-test-retain={expertTestCanRetain(task)?'current':'changed'}>{t(expertTestCanRetain(task)?'retain_help':'source_changed')}</p>}
    {modes[task.taskID]==='definition'&&<p className="team-dispatch-muted">{t('definition_help')}</p>}
    {modes[task.taskID]==='adjust'&&<p className="team-dispatch-muted">{t('adjust_help')}</p>}
   </>}
  </details>)}
  {!!editing.length&&<AdjustmentDirectory project={project} tasks={editing} scope={expertTestContext(project,tasks)} disabled={disabled} onChange={update} owns={owns}/>}
 </section>;
}
function AdjustmentDirectory({project,tasks,scope,disabled,onChange,owns}:{project:string;tasks:ExpertTestTask[];scope:string;disabled:boolean;onChange:(task:string,value:TestBindingsChoice)=>void;owns:()=>boolean}){
 useLocale();const api=useApi(),inventory=useToolList<ToolProfile>(project,'tool-profiles'),failed=useCallback(()=>{},[]),workflows=useExpertRead<{items:Workflow[]}>(api,'/workflows'+query(project),true,0,validWorkflows,failed);
 const groups=useMemo(()=>{const map=new Map<string,ExpertTestTask[]>();for(const task of tasks){const key=expertTestRoleBasis(task.role);map.set(key,[...(map.get(key)||[]),task]);}return [...map.entries()];},[JSON.stringify(tasks.map(task=>[task.taskID,expertTestRoleBasis(task.role)]))]);
 return <div className="expert-test-adjustments">{workflows.loading&&<Loading/>}{Boolean(workflows.error)&&<ErrorNotice error={toolError(workflows.error)} onRetry={()=>void workflows.read()}/>}
  {groups.map(([basis,group])=><RoleAdjustments key={basis} project={project} tasks={group} role={group[0].role} scope={scope} workflow={workflows.value?.items.find(flow=>flow.id===group[0].role.workflow_id&&flow.version===group[0].role.workflow_version)} loading={workflows.loading} inventory={inventory} disabled={disabled||!!workflows.error} onChange={onChange} owns={owns}/>)}
 </div>;
}
function RoleAdjustments({project,tasks,role,scope,workflow,loading,inventory,disabled,onChange,owns}:{project:string;tasks:ExpertTestTask[];role:ExpertTeamRole;scope:string;workflow?:Workflow;loading:boolean;inventory:ReturnType<typeof useToolList<ToolProfile>>;disabled:boolean;onChange:(task:string,value:TestBindingsChoice)=>void;owns:()=>boolean}){
 useLocale();const api=useApi(),failed=useCallback(()=>{},[]),basis=expertTestRoleBasis(role);
 const valid=useCallback((value:unknown):value is ToolProfile=>{
  if(!validTerminalCandidateTools({items:[value],total:1},project))return false;const tool=value as ToolProfile;
  if(tool.id!==role.tool_profile_id||tool.version!==role.tool_profile_version||tool.profile.profile_sha256!==role.tool_profile_sha256||!expertTool(tool))return false;
  try{const [observation]=parseToolManifest(JSON.stringify([tool.observation]));return !!observation?.available&&sameToolProfile(observation.profile,tool.profile);}catch{return false;}
 },[project,basis]);
 const developer=useExpertRead<ToolProfile>(api,'/tool-profiles/'+encodeURIComponent(role.tool_profile_id)+query(project),true,0,valid,failed);
 const ready=!disabled&&!loading&&!!workflow&&workflow.nodes.some(node=>node.id===role.agent_node_id&&node.type==='agent')&&!!developer.value&&!developer.loading&&!developer.error;
 useEffect(()=>{if(!ready&&owns())for(const task of tasks)onChange(task.taskID,{ready:false,hasIntent:true,selections:[]});},[ready,scope,basis]);
 return <section className="expert-test-role" data-expert-test-role={role.key}>
  {(loading||developer.loading)&&<Loading/>}{Boolean(developer.error)&&<ErrorNotice error={toolError(developer.error)} onRetry={()=>void developer.read()}/>}
  {!loading&&!developer.loading&&!ready&&!developer.error&&<p className="warning">{t('inputs_changed')}</p>}
  {ready&&workflow&&developer.value&&tasks.map(task=><div data-expert-test-adjust={task.taskID} key={task.taskID}><strong>{task.title}</strong><TaskDispatchTestBindings project={project} workflow={workflow} agentID={role.agent_node_id} developer={developer.value!} inventory={inventory} scopeKey={JSON.stringify([scope,task.taskID,basis,developer.value!.worker_id])} disabled={disabled} onChange={value=>{if(owns())onChange(task.taskID,value);}}/></div>)}
 </section>;
}
