import {createRoot} from 'react-dom/client';
import {useState} from 'react';
import '../../../apps/web/src/style.css';
import '../../../apps/web/src/experience-foundation.css';
import {TaskDispatchDialog} from '../../../apps/web/src/TaskDispatch';
import {ApiViewProvider} from '../../../apps/web/src/api-context';
import {LanguageProvider} from '../../../apps/web/src/i18n/LanguageSelector';
import {setLocale} from '../../../apps/web/src/i18n';
import type {RecordItem,Session} from '../../../apps/web/src/api';

type View={session:Session;project_id:string;task:RecordItem;workflow_id:string;workflow_version:number;developer_profile_id:string;test_profile_id:string};
const parameters=new URLSearchParams(location.search),locale=parameters.get('locale')==='en-US'?'en-US':'zh-CN';
document.documentElement.lang=locale;document.documentElement.dataset.theme=parameters.get('theme')==='dark'?'dark':'light';
function Fixture({view}:{view:View}){
 const [planID,setPlanID]=useState(parameters.get('plan')||'new'),[closed,setClosed]=useState(false),[run,setRun]=useState('');
 return <><output data-fixture-plan-id={planID} data-fixture-run-id={run}/>{!closed&&<TaskDispatchDialog session={view.session} project={view.project_id} task={view.task} planID={planID}
  onClose={()=>setClosed(true)} onRun={setRun} onPlan={id=>setPlanID(id)}/>}</>;
}
void Promise.all([setLocale(locale,{persist:false}),fetch('/__fixture/view').then(response=>response.json() as Promise<View>)]).then(([,view])=>{
 createRoot(document.getElementById('root')!).render(<LanguageProvider><ApiViewProvider session={view.session}><Fixture view={view}/></ApiViewProvider></LanguageProvider>);
});
