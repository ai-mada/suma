import {createRoot} from 'react-dom/client';
import {useState} from 'react';
import '../../../apps/web/src/style.css';
import '../../../apps/web/src/experience-foundation.css';
import {ApiViewProvider} from '../../../apps/web/src/api-context';
import {LanguageProvider} from '../../../apps/web/src/i18n/LanguageSelector';
import {setLocale} from '../../../apps/web/src/i18n';
import {RemoteRunnerSetupGuide} from '../../../apps/web/src/RemoteRunnerSetupGuide';
import {ToolQuickConnect} from '../../../apps/web/src/ToolQuickConnect';
import type {ToolReportFeedbackSelection} from '../../../apps/web/src/tool-report-feedback-model';
import type {Session} from '../../../apps/web/src/api';

// This entry renders production components against the real isolated API.
// Only public context enters the renderer. The driver alone owns its temporary
// cookie and fixture action key; no fetch/API/IPC implementation is substituted.
type View={project_id:string;worker_id:string;local_tool:'codex-jsonl';command_id:string;session:Session};
const params=new URLSearchParams(location.search),locale=params.get('locale')==='en-US'?'en-US':'zh-CN';
const selections:ToolReportFeedbackSelection[]=[];
function App({view}:{view:View}){
 const [selection,setSelection]=useState<ToolReportFeedbackSelection>();
 const idleRead={loading:false,error:'',read:async()=>{}};
 const workers={...idleRead,data:{items:[{worker_id:view.worker_id,status:'active'}]}};
 const shared={...idleRead,data:{items:[]}};
 const action={busy:false,pending:undefined,conflict:false,clearError(){},run:async()=>{throw Error('This read-only journey must not mutate a tool profile');}};
 return <main className="feedback-fixture" data-tool-report-fixture>
  <header><small>suma 1.0.0</small><h1>{locale==='zh-CN'?'工具接入反馈':'Tool connection feedback'}</h1><p>{locale==='zh-CN'?'隔离环境 · 真实服务端清单':'Isolated environment · actual server inventory'}</p></header>
  <section className="feedback-fixture-card"><RemoteRunnerSetupGuide project={view.project_id} expectedWorkerID={view.worker_id} selectionRequest={{localTool:view.local_tool,nonce:1}} onReportSelection={value=>{selections.push(value);setSelection(value);}}/></section>
  {selection&&<section className="feedback-fixture-card" data-tool-report-return><ToolQuickConnect project={view.project_id} draft={{adapterID:selection.adapterID,workerID:selection.workerID,commandID:selection.commandID,name:'',nameEdited:false}} preferredWorkerID={selection.workerID} workers={workers as any} sharedWorkers={shared as any} action={action as any} onRegistered={()=>{throw Error('Unexpected registration');}} onEnable={()=>{throw Error('Unexpected enable');}} onAdvanced={()=>{throw Error('Unexpected guide');}} onWorker={()=>{throw Error('Unexpected pairing');}}/></section>}
 </main>;
}
Object.assign(window,{__toolReportFeedback204:{snapshot:()=>({selections:structuredClone(selections)})}});
document.documentElement.dataset.theme=params.get('theme')==='dark'?'dark':'light';
document.documentElement.lang=locale;
void Promise.all([setLocale(locale,{persist:false}),fetch('/__fixture/view').then(response=>{if(!response.ok)throw Error('Fixture context unavailable');return response.json() as Promise<View>;})]).then(([,view])=>createRoot(document.getElementById('root')!).render(<LanguageProvider><ApiViewProvider session={view.session}><App view={view}/></ApiViewProvider></LanguageProvider>));
