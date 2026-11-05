import {useState} from 'react';
import {createRoot} from 'react-dom/client';
import '../../../apps/web/src/style.css';
import '../../../apps/web/src/experience-foundation.css';
import {ApiViewProvider} from '../../../apps/web/src/api-context';
import {setLocale} from '../../../apps/web/src/i18n';
import {LanguageProvider} from '../../../apps/web/src/i18n/LanguageSelector';
import {ToolWorkspace} from '../../../apps/web/src/ToolWorkspace';
import type {Session} from '../../../apps/web/src/api';

type FixtureView = {project_id: string; session: Session};

const parameters = new URLSearchParams(location.search);
const locale = parameters.get('locale') === 'en-US' ? 'en-US' : 'zh-CN';
const theme = parameters.get('theme') === 'dark' ? 'dark' : 'light';
const title = locale === 'zh-CN' ? '工具管理' : 'Tool management';

function FixtureApp({view}: {view: FixtureView}) {
  const [selected, onSelect] = useState('');
  return (
    <main className="identity-fixture" id="main" data-tool-identity-fixture={view.project_id}>
      <header className="identity-fixture-header">
        <strong>suma</strong>
        <h1>{title}</h1>
      </header>
      <ToolWorkspace
        project={view.project_id}
        session={view.session}
        selected={selected}
        onSelect={onSelect}
        refresh={0}
        panel=""
      />
    </main>
  );
}

document.documentElement.lang = locale;
document.documentElement.dataset.theme = theme;
document.title = title + ' · suma';

async function mount() {
  const [response] = await Promise.all([
    fetch('/__fixture/view'),
    setLocale(locale, {persist: false}),
  ]);
  if (!response.ok) throw new Error('Synthetic project view unavailable');
  const view: FixtureView = await response.json();
  const container = document.getElementById('root');
  if (!container) throw new Error('Missing fixture root');
  createRoot(container).render(
    <LanguageProvider>
      <ApiViewProvider session={view.session}>
        <FixtureApp view={view}/>
      </ApiViewProvider>
    </LanguageProvider>,
  );
}

void mount().catch((error: unknown) => {
  console.error(error);
  const container = document.getElementById('root');
  if (container) container.textContent = 'Fixture initialization failed';
});
