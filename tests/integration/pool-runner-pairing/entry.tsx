import {createRoot} from 'react-dom/client';
import {useState} from 'react';
import '../../../apps/web/src/style.css';
import '../../../apps/web/src/experience-foundation.css';
import {ApiViewProvider} from '../../../apps/web/src/api-context';
import {LanguageProvider} from '../../../apps/web/src/i18n/LanguageSelector';
import {setLocale} from '../../../apps/web/src/i18n';
import {PoolWorkspace} from '../../../apps/web/src/execution-pools/PoolWorkspace';
import type {Session} from '../../../apps/web/src/api';

// Actual isolated HTTP service and production Pool workspace. No fetch or IPC
// substitute; test authentication stays in the native driver's private input.
type View={pool_id:string;session:Session};
const params=new URLSearchParams(location.search),locale=params.get('locale')==='en-US'?'en-US':'zh-CN';
function App({view}:{view:View}){const[selected,onSelect]=useState(view.pool_id),[panel,onPanel]=useState('runners');return <main id="main" tabIndex={-1} className="pool-fixture"><header><strong>suma 1.0.0</strong><h1>{locale==='zh-CN'?'共享执行节点配对':'Pair a shared execution node'}</h1><p>{locale==='zh-CN'?'隔离测试组织 · 原HTTP服务与系统凭据库，不连接供应方':'Isolated test organization · original HTTP service and system credential store, no vendor connection'}</p></header><PoolWorkspace session={view.session} selected={selected} onSelect={onSelect} panel={panel} onPanel={onPanel} refresh={0}/></main>;}
document.documentElement.lang=locale;document.documentElement.dataset.theme=params.get('theme')==='dark'?'dark':'light';
void Promise.all([setLocale(locale,{persist:false}),fetch('/__fixture/view').then(response=>{if(!response.ok)throw Error('Fixture view unavailable');return response.json() as Promise<View>;})]).then(([,view])=>createRoot(document.getElementById('root')!).render(<LanguageProvider><ApiViewProvider session={view.session}><App view={view}/></ApiViewProvider></LanguageProvider>));
