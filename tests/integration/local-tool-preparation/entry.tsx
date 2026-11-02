import {createRoot} from 'react-dom/client';
import '../../../apps/web/src/style.css';
import '../../../apps/web/src/experience-foundation.css';
import {ApiViewProvider} from '../../../apps/web/src/api-context';
import type {Session} from '../../../apps/web/src/api';
import {LanguageProvider} from '../../../apps/web/src/i18n/LanguageSelector';
import {setLocale} from '../../../apps/web/src/i18n';
import {ToolSetupGuide} from '../../../apps/web/src/ToolSetupGuide';

type PublicView = {session: Session; project_id: string};
const query = new URLSearchParams(location.search), locale = query.get('locale') === 'en-US' ? 'en-US' : 'zh-CN';
document.documentElement.lang = locale;
document.documentElement.dataset.theme = query.get('theme') === 'dark' ? 'dark' : 'light';

/** The real Guide owns draft adoption. The fixture supplies only current
 * public actor/project metadata and a clearly explicit ordinary/shared target. */
function Fixture({view}: {view: PublicView}) {
  const shared = query.get('mode') === 'shared';
  return <main id="main" tabIndex={-1} className="local-preparation-fixture" data-local-preparation-mode={shared ? 'shared' : 'ordinary'}>
    <header><strong>suma</strong><span>{locale === 'zh-CN' ? '本机配置准备 · 隔离验证' : 'Local configuration preparation · isolated verification'}</span></header>
    <ToolSetupGuide project={view.project_id} expanded expectedWorkerID={shared ? 'pool-' + 'c'.repeat(32) : 'ordinary-worker-220'} presetRequest={{preset: 'codex', nonce: 1}} />
  </main>;
}
void Promise.all([setLocale(locale, {persist: false}), fetch('/__fixture/view').then(response => {
  if (!response.ok) throw Error('Synthetic public view unavailable');
  return response.json() as Promise<PublicView>;
})]).then(([, view]) => createRoot(document.getElementById('root')!).render(<LanguageProvider><ApiViewProvider session={view.session}><Fixture view={view} /></ApiViewProvider></LanguageProvider>));
