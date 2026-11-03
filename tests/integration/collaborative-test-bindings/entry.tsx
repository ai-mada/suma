import {useState} from 'react';
import {createRoot} from 'react-dom/client';
import '../../../apps/web/src/style.css';
import '../../../apps/web/src/experience-foundation.css';
import {ApiViewProvider} from '../../../apps/web/src/api-context';
import {LanguageProvider} from '../../../apps/web/src/i18n/LanguageSelector';
import {setLocale} from '../../../apps/web/src/i18n';
import {DiscussionDevelopmentPanel} from '../../../apps/web/src/DiscussionDevelopmentPanel';
import {DispatchRecommendation} from '../../../apps/web/src/DispatchRecommendation';
import {TeamTaskDispatchDialog} from '../../../apps/web/src/TeamTaskDispatchDialog';
import {TaskDispatchFacts} from '../../../apps/web/src/ToolFacts';
import {ToolProfileDoctor} from '../../../apps/web/src/ToolProfileDoctor';
import type {Session,RecordItem} from '../../../apps/web/src/api';
import type {TaskConversationPage,TaskConversationReply,TaskDispatchPlan,ToolProfile} from '../../../packages/contracts/src';
import type {RecommendationScope} from '../../../apps/web/src/dispatch-recommendation-model';

type View={session:Session;project:string;reply:TaskConversationReply;page:TaskConversationPage;teamTask:RecordItem;doctorTool:ToolProfile;recommendationScope:RecommendationScope;profiles:ToolProfile[]};
const query=new URLSearchParams(location.search),locale=query.get('locale')==='en-US'?'en-US':'zh-CN',scene=query.get('scene')||'discussion';
document.documentElement.lang=locale;document.documentElement.dataset.theme=query.get('theme')==='dark'?'dark':'light';
function Fixture({view}:{view:View}){
 const [adopted,setAdopted]=useState<TaskDispatchPlan>(),[teamID,setTeamID]=useState(''),[live,setLive]=useState(true);
 const inventory={items:view.profiles,total:view.profiles.length,loading:false,error:'',next_cursor:'',read:async()=>true};
 return <main id="main" tabIndex={-1} className="collaboration-fixture" data-fixture-scene={scene}>
  <header className="fixture-heading"><strong>suma</strong><span>{locale==='zh-CN'?'协作独立测试 · 合成界面验证':'Collaborative independent tests · synthetic interface verification'}</span></header>
  {scene==='discussion'&&<DiscussionDevelopmentPanel session={view.session} project={view.project} taskId={view.reply.task_id} reply={view.reply} page={view.page} disabled={false} accessBlocked={false} onLockChange={()=>{}}/>}
  {scene==='recommendation'&&<><DispatchRecommendation session={view.session} scope={view.recommendationScope} inventory={inventory} disabled={false} creationDisabled
    testBindings={{workflow_id:'flow-a',workflow_version:3,agent_node_id:'develop',ready:true,hasIntent:true,selections:[{node_id:'verify',tool_profile_id:'tester',expected_tool_version:3}]}}
    onPrepared={setAdopted} onOpenPlan={()=>{}} onLockChange={()=>{}} onManual={()=>{}}/>
    {adopted&&<section className="fixture-adopted" data-fixture-adopted><TaskDispatchFacts plan={adopted}/></section>}</>}
  {scene==='team'&&<TeamTaskDispatchDialog project={view.project} session={view.session} tasks={[view.teamTask]} open onClose={()=>{}} onStarted={()=>{}} planID={teamID||undefined} onPlan={setTeamID}/>}
  {scene==='doctor'&&<><label className="fixture-live"><input data-fixture-doctor-live type="checkbox" checked={live} onChange={event=>setLive(event.target.checked)}/>{locale==='zh-CN'?'自动读取当前诊断':'Read current diagnostics automatically'}</label><ToolProfileDoctor project={view.project} tool={view.doctorTool} confirmed autoRefresh={live} onWorker={()=>{}}/></>}
 </main>;
}
void Promise.all([setLocale(locale,{persist:false}),fetch('/__fixture/view').then(response=>response.json() as Promise<View>)]).then(([,view])=>{
 if(scene==='recommendation')sessionStorage.setItem('suma-recommendation-locator-v1:'+JSON.stringify([view.session.user.id,view.session.organization.id,view.project,view.recommendationScope.task_id]),'recommendation-one');
 createRoot(document.getElementById('root')!).render(<LanguageProvider><ApiViewProvider session={view.session}><Fixture view={view}/></ApiViewProvider></LanguageProvider>);
});
