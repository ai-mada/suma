import {createRoot} from 'react-dom/client';
import {useEffect,useState} from 'react';
import '../../../apps/web/src/style.css';
import '../../../apps/web/src/experience-foundation.css';
import {ApiViewProvider} from '../../../apps/web/src/api-context';
import {LanguageProvider} from '../../../apps/web/src/i18n/LanguageSelector';
import {setLocale} from '../../../apps/web/src/i18n';
import {ToolWorkspace} from '../../../apps/web/src/ToolWorkspace';
import {TaskDispatchDialog} from '../../../apps/web/src/TaskDispatch';
import type {RecordItem,Session} from '../../../apps/web/src/api';

type View={session:Session;project_id:string;tool_id:string;task:RecordItem};
const parameters=new URLSearchParams(location.search),locale=parameters.get('locale')==='en-US'?'en-US':'zh-CN';
document.documentElement.lang=locale;
document.documentElement.dataset.theme=parameters.get('theme')==='dark'?'dark':'light';
function Fixture({view}:{view:View}){
  const [selected,setSelected]=useState(view.tool_id),[refresh,setRefresh]=useState(0),panel=parameters.get('panel')||'';
  useEffect(()=>{const refreshed=()=>setRefresh(value=>value+1);window.addEventListener('suma:fixture-refresh',refreshed);return()=>window.removeEventListener('suma:fixture-refresh',refreshed);},[]);
  return <main className="tool-task-fixture"><header><strong>suma 1.0.0</strong><h1>{locale==='zh-CN'?'工具与任务交接':'Use a tool for a project task'}</h1></header>
    {panel.startsWith('dispatch:')?<TaskDispatchDialog project={view.project_id} session={view.session} task={view.task} planID={panel.slice(9)}
      onPlan={()=>{}} onClose={()=>{}} onRun={()=>{}}/>
      :<ToolWorkspace project={view.project_id} session={view.session} selected={selected} onSelect={setSelected} refresh={refresh} panel=""/>}
  </main>;
}
// Authentication is an HttpOnly cookie installed in the isolated main process.
// The metadata endpoint and the renderer receive no token or fixture action key.
void Promise.all([setLocale(locale,{persist:false}),fetch('/__fixture/view').then(response=>response.json() as Promise<View>)]).then(([,view])=>{
  createRoot(document.getElementById('root')!).render(<LanguageProvider><ApiViewProvider session={view.session}><Fixture view={view}/></ApiViewProvider></LanguageProvider>);
});
