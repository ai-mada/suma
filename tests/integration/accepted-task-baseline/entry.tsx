import {createRoot} from 'react-dom/client';
import {useState} from 'react';
import '../../../apps/web/src/style.css';
import '../../../apps/web/src/experience-foundation.css';
import {TaskCodeBaselinePanel} from '../../../apps/web/src/TaskCodeBaselinePanel';
import {ApiViewProvider} from '../../../apps/web/src/api-context';
import {LanguageProvider} from '../../../apps/web/src/i18n/LanguageSelector';
import {setLocale} from '../../../apps/web/src/i18n';
import type {Session} from '../../../apps/web/src/api';

type View={session:Session;project_id:string;source_task_id:string;source_node_id:string;source_commit:string;target_task_id:string;third_task_id:string;phase:string};
const query=new URLSearchParams(location.search),locale=query.get('locale')==='en-US'?'en-US':'zh-CN';
document.documentElement.lang=locale;document.documentElement.dataset.theme=query.get('theme')==='dark'?'dark':'light';

/** This is the production baseline panel on real isolated HTTP/SQLite. Only
 * the two explicit fixture targets are routable; no browser value grants access. */
function Fixture({view}:{view:View}){
 const [refresh,setRefresh]=useState(0),target=query.get('target')===view.third_task_id?view.third_task_id:view.target_task_id;
 return <main id="main" tabIndex={-1} className="accepted-baseline-fixture" data-fixture-target={target} data-fixture-phase={view.phase}>
  <header><strong>suma</strong><span>{locale==='zh-CN'?'已验收任务代码交接 · 隔离验证':'Accepted task code handoff · isolated verification'}</span></header>
  <TaskCodeBaselinePanel session={view.session} project={view.project_id} taskID={target} initiallyOpen refresh={refresh} onChanged={()=>setRefresh(value=>value+1)}/>
 </main>;
}
void Promise.all([setLocale(locale,{persist:false}),fetch('/__fixture/view').then(response=>{if(!response.ok)throw Error('Owned actual context unavailable');return response.json() as Promise<View>;})]).then(([,view])=>createRoot(document.getElementById('root')!).render(<LanguageProvider><ApiViewProvider session={view.session}><Fixture view={view}/></ApiViewProvider></LanguageProvider>));
