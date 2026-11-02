import {createRoot} from 'react-dom/client';
import '../../../apps/web/src/style.css';
import '../../../apps/web/src/experience-foundation.css';
import {ApiViewProvider} from '../../../apps/web/src/api-context';
import {LanguageProvider} from '../../../apps/web/src/i18n/LanguageSelector';
import {setLocale} from '../../../apps/web/src/i18n';
import {PoolConfigPreparationPanel} from '../../../apps/web/src/execution-pools/PoolConfigPreparationPanel';
import type {Session} from '../../../apps/web/src/api';
import type {ExecutionPoolGrant,ExecutionPoolRunner} from '../../../packages/contracts/src';

// Only public scope metadata enters this fixture. The production Panel produces
// private stdin locally; neither a platform API nor a machine is connected.
type View={owner_tenant_id:string;pool_id:string;runner:Pick<ExecutionPoolRunner,'id'|'pool_id'|'worker_id'|'max_slots'|'status'>;grants:Pick<ExecutionPoolGrant,'id'|'pool_id'|'owner_tenant_id'|'tenant_id'|'project_id'|'status'|'shared_project_name'>[];session:Session};
const params=new URLSearchParams(location.search),locale=params.get('locale')==='en-US'?'en-US':'zh-CN';
document.documentElement.lang=locale;document.documentElement.dataset.theme=params.get('theme')==='dark'?'dark':'light';
void Promise.all([setLocale(locale,{persist:false}),fetch('/__fixture/view').then(r=>r.json() as Promise<View>)]).then(([,view])=>createRoot(document.getElementById('root')!).render(<LanguageProvider><ApiViewProvider session={view.session}><main id="main" tabIndex={-1} className="preparation-fixture"><header><strong>suma 1.0.0</strong><h1>{locale==='zh-CN'?'资源池本机配置准备':'Prepare local Pool configuration'}</h1><p>{locale==='zh-CN'?'隔离验证 · 静态新文件，不连接控制面或执行任务':'Isolated verification · new local file, no control connection or task execution'}</p></header><PoolConfigPreparationPanel ownerTenantID={view.owner_tenant_id} poolID={view.pool_id} runner={view.runner} grants={view.grants}/></main></ApiViewProvider></LanguageProvider>));
