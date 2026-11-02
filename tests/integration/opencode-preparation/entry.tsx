import {createRoot} from 'react-dom/client';
import '../../../apps/web/src/style.css';
import '../../../apps/web/src/experience-foundation.css';
import {ApiViewProvider} from '../../../apps/web/src/api-context';
import {LanguageProvider} from '../../../apps/web/src/i18n/LanguageSelector';
import {setLocale} from '../../../apps/web/src/i18n';
import {ToolSetupGuide} from '../../../apps/web/src/ToolSetupGuide';
import type {Session} from '../../../apps/web/src/api';

// Production setup and original remote/feedback components. The local fixture
// serves public context only; no account, model, Worker or vendor is connected.
type View={project_id:string;worker_id:string;session:Session};
const params=new URLSearchParams(location.search),locale=params.get('locale')==='en-US'?'en-US':'zh-CN';
document.documentElement.lang=locale;document.documentElement.dataset.theme=params.get('theme')==='dark'?'dark':'light';
void Promise.all([setLocale(locale,{persist:false}),fetch('/__fixture/view').then(r=>r.json() as Promise<View>)]).then(([,view])=>createRoot(document.getElementById('root')!).render(<LanguageProvider><ApiViewProvider session={view.session}><main id="main" tabIndex={-1} className="preparation-fixture"><header><strong>suma 1.0.0</strong><h1>{locale==='zh-CN'?'OpenCode 配置准备':'Prepare OpenCode configuration'}</h1><p>{locale==='zh-CN'?'本地隔离验证 · 公开配置草稿，不连接供应方':'Local isolated verification · public configuration draft, no vendor connection'}</p></header><ToolSetupGuide project={view.project_id} expectedWorkerID={view.worker_id} expanded presetRequest={{preset:'opencode',nonce:1}}/></main></ApiViewProvider></LanguageProvider>));
