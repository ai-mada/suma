import {createRoot} from 'react-dom/client';
import {useState} from 'react';
import '../../../apps/web/src/style.css';
import '../../../apps/web/src/experience-foundation.css';
import {ApiViewProvider} from '../../../apps/web/src/api-context';
import {LanguageProvider} from '../../../apps/web/src/i18n/LanguageSelector';
import {setLocale} from '../../../apps/web/src/i18n';
import {ToolWorkspace} from '../../../apps/web/src/ToolWorkspace';
import type {Session} from '../../../apps/web/src/api';

// Production component, synthetic public fixture. No substituted fetch/IPC,
// real account, Worker, model, command or business mutation is used here.
const parameters=new URLSearchParams(location.search),locale=parameters.get('locale')==='en-US'?'en-US':'zh-CN';
type View={project_id:string;session:Session};
function App({view}:{view:View}){
 const [selected,onSelect]=useState(parameters.get('selected')||'');
 return <main id="main" tabIndex={-1} className="experience-fixture"><header className="experience-fixture-header"><strong>suma</strong><span>{locale==='zh-CN'?'工具管理 · 本地合成界面验证':'Tool management · local synthetic interface verification'}</span></header><ToolWorkspace project={view.project_id} session={view.session} selected={selected} onSelect={onSelect} refresh={0} panel=""/></main>;
}
document.documentElement.dataset.theme=parameters.get('theme')==='dark'?'dark':'light';document.documentElement.lang=locale;
void Promise.all([setLocale(locale,{persist:false}),fetch('/__fixture/view').then(response=>{if(!response.ok)throw Error('Fixture context unavailable');return response.json() as Promise<View>;})]).then(([,view])=>createRoot(document.getElementById('root')!).render(<LanguageProvider><ApiViewProvider session={view.session}><App view={view}/></ApiViewProvider></LanguageProvider>));
