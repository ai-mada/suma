import {createRoot} from 'react-dom/client';
import {useState} from 'react';
import '../../../apps/web/src/style.css';
import '../../../apps/web/src/experience-foundation.css';
import {TaskDeliveryPanel} from '../../../apps/web/src/TaskDeliveryPanel';
import {TaskAcceptanceLocator} from '../../../apps/web/src/TaskAcceptanceLocator';
import {ApiViewProvider} from '../../../apps/web/src/api-context';
import {LanguageProvider} from '../../../apps/web/src/i18n/LanguageSelector';
import {setLocale} from '../../../apps/web/src/i18n';
import type {Session} from '../../../apps/web/src/api';

const parameters=new URLSearchParams(location.search),locale=parameters.get('locale')==='en-US'?'en-US':'zh-CN';
document.documentElement.lang=locale;document.documentElement.dataset.theme=parameters.get('theme')==='dark'?'dark':'light';
function Fixture({session}:{session:Session}){
  const [opened,setOpened]=useState(0),panel=parameters.get('panel')||'';
  return <main className="delivery-navigation-fixture"><header><strong>suma 1.0.0</strong><h1>{locale==='zh-CN'?'交付与质量':'Delivery and quality'}</h1></header>
    {panel.startsWith('task-acceptance')?<>
      <TaskAcceptanceLocator session={session} project="project-a" taskID="task-a" taskVersion={3} runID="run-a" panel={panel} onOpenReview={()=>{setOpened(value=>value+1);return true;}}/>
      <p data-fixture-opened={opened}>{locale==='zh-CN'?'只读夹具记录验收入口回调；原验收表单另由App检查覆盖。':'Read-only fixture observes the entry callback; the original Review form is covered separately by the App check.'}</p>
    </>:parameters.get('view')?<p data-fixture-destination>{parameters.get('view')} / {parameters.get('sub')} / {parameters.get('record')||panel}</p>
      :<TaskDeliveryPanel session={session} project="project-a" taskId="task-a" taskVersion={3} refresh={0} onOpenRun={()=>{}}/>}
  </main>;
}
void Promise.all([setLocale(locale,{persist:false}),fetch('/__fixture/session').then(response=>response.json() as Promise<Session>)]).then(([,session])=>{
  createRoot(document.getElementById('root')!).render(<LanguageProvider><ApiViewProvider session={session}><Fixture session={session}/></ApiViewProvider></LanguageProvider>);
});
