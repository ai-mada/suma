import {useState} from 'react';
import {createRoot} from 'react-dom/client';
import '../../../apps/web/src/style.css';
import '../../../apps/web/src/experience-foundation.css';
import {ApiViewProvider} from '../../../apps/web/src/api-context';
import type {Session} from '../../../apps/web/src/api';
import {LanguageProvider} from '../../../apps/web/src/i18n/LanguageSelector';
import {setLocale} from '../../../apps/web/src/i18n';
import {ToolWorkspace} from '../../../apps/web/src/ToolWorkspace';
import {ToolWriteRecovery} from '../../../apps/web/src/tool-write';

type FixtureView = {project_id: string; worker_id: string; initial_profile: string; session: Session};
const parameters = new URLSearchParams(location.search);
const locale = parameters.get('locale') === 'en-US' ? 'en-US' : 'zh-CN';

function App({view}: {view: FixtureView}) {
  const [selected, onSelect] = useState(parameters.get('selected') || view.initial_profile);
  return <main id="main" tabIndex={-1} className="recovery-fixture" data-tool-recovery-fixture>
    <header className="recovery-fixture-header"><strong>suma</strong><span>
      {locale === 'zh-CN' ? '工具恢复导航 · 本地合成界面验证' : 'Tool recovery navigation · local synthetic interface verification'}
    </span></header>
    <ToolWorkspace project={view.project_id} session={view.session} selected={selected}
      onSelect={onSelect} refresh={0} panel=""/>
  </main>;
}

document.documentElement.dataset.theme = parameters.get('theme') === 'dark' ? 'dark' : 'light';
document.documentElement.lang = locale;
void Promise.all([
  setLocale(locale, {persist: false}),
  fetch('/__fixture/view').then(response => {
    if (!response.ok) throw Error('Synthetic fixture context unavailable');
    return response.json() as Promise<FixtureView>;
  }),
]).then(async ([, view]) => {
  if (parameters.get('pending') === '1') {
    // Exercise the real recovery journal, with an explicitly synthetic lost
    // acknowledgement. No fetch or server mutation is performed by this sender.
    const scope = `${view.session.organization.id}:${view.session.user.id}:${view.project_id}:profiles`;
    const recovery = new ToolWriteRecovery(scope, sessionStorage, async () => {
      throw Error('Synthetic transport acknowledgement unknown');
    });
    try {
      await recovery.run('/release-workers', {project_id: view.project_id, worker_id: view.worker_id}, 'POST');
    } catch { /* Preserve the original public body/key for the visible recovery check. */ }
  }
  createRoot(document.getElementById('root')!).render(<LanguageProvider>
    <ApiViewProvider session={view.session}><App view={view}/></ApiViewProvider>
  </LanguageProvider>);
});
