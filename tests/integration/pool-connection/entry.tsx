import {createRoot} from 'react-dom/client';
import '../../../apps/web/src/style.css';
import '../../../apps/web/src/experience-foundation.css';
import '../../../apps/web/src/execution-pools/PoolConfigPreparation.css';
import {ApiViewProvider} from '../../../apps/web/src/api-context';
import {LanguageProvider} from '../../../apps/web/src/i18n/LanguageSelector';
import {setLocale} from '../../../apps/web/src/i18n';
import {PoolConfigConnectionPanel} from '../../../apps/web/src/execution-pools/PoolConfigConnectionPanel';
import {ToolWorkspace} from '../../../apps/web/src/ToolWorkspace';
import type {Session} from '../../../apps/web/src/api';

type View = {
  session: Session;
  tenant_id: string;
  pool_id: string;
  runner_id: string;
  worker_id: string;
  project_id: string;
  pool_config_file: string;
  scopes: Array<{tenant_id:string;project_id:string}>;
};
const query = new URLSearchParams(location.search);
const locale = query.get('locale') === 'en-US' ? 'en-US' : 'zh-CN';
document.documentElement.lang = locale;
document.documentElement.dataset.theme = query.get('theme') === 'dark' ? 'dark' : 'light';

// Public fixture metadata contains no session cookie, action key or machine
// credential. Production API requests use the actual HttpOnly test session.
void Promise.all([
  setLocale(locale, {persist:false}),
  fetch('/__fixture/view').then(response => response.json() as Promise<View>),
]).then(([,view]) => {
  const tools = query.get('view') === 'settings' && query.get('sub') === 'tools';
  createRoot(document.getElementById('root')!).render(
    <LanguageProvider><ApiViewProvider session={view.session}>
      <main className="pool-connection-fixture">
        <header><strong>suma 1.0.0</strong><h1>{locale === 'zh-CN' ? '资源池机器连接' : 'Connect a Pool machine'}</h1></header>
        {tools ? <ToolWorkspace session={view.session} project={query.get('project') || view.project_id}
          selected="" panel={query.get('panel') || ''} onSelect={() => {}} refresh={0}/>
          : <div className="pool-config-preparation"><PoolConfigConnectionPanel
            ownerTenantID={view.tenant_id} poolID={view.pool_id} runnerID={view.runner_id} workerID={view.worker_id}
            prepared={{shell:'powershell',output:view.pool_config_file,
              scopes:view.scopes.map(scope => ({tenant_id:scope.tenant_id,project_id:scope.project_id}))}}/>
          </div>}
      </main>
    </ApiViewProvider></LanguageProvider>,
  );
});
