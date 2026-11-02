import {useEffect, useState} from 'react';
import {createRoot} from 'react-dom/client';
import '../../../apps/web/src/style.css';
import '../../../apps/web/src/experience-foundation.css';
import {ApiViewProvider} from '../../../apps/web/src/api-context';
import type {Session} from '../../../apps/web/src/api';
import {LanguageProvider} from '../../../apps/web/src/i18n/LanguageSelector';
import {setLocale} from '../../../apps/web/src/i18n';
import {ToolWorkspace} from '../../../apps/web/src/ToolWorkspace';
import {PoolWorkspace} from '../../../apps/web/src/execution-pools/PoolWorkspace';
import {buildPoolSetupOwnerHref} from '../../../apps/web/src/execution-pools/pool-tool-setup-model';

type View = {session: Session; project_id: string};
const initial = new URLSearchParams(location.search);
const locale = initial.get('locale') === 'en-US' ? 'en-US' : 'zh-CN';
document.documentElement.dataset.theme = initial.get('theme') === 'dark' ? 'dark' : 'light';
document.documentElement.lang = locale;

/** This small router exercises production component callbacks and strict setup
 * pins. It does not reproduce the full App router or its authorization. */
function Fixture({view}: {view: View}) {
  const [route, setRoute] = useState(() => new URLSearchParams(location.search));
  function navigate(href: string) {
    const url = new URL(href, location.origin);
    if (url.origin !== location.origin) return;
    url.searchParams.set('locale', locale);
    url.searchParams.set('theme', initial.get('theme') ?? 'light');
    history.pushState(null, '', url);
    setRoute(new URLSearchParams(url.search));
  }
  useEffect(() => {
    const click = (event: MouseEvent) => {
      const link = (event.target as Element | null)?.closest<HTMLAnchorElement>('a[href]');
      if (!link || event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || link.target === '_blank') return;
      const url = new URL(link.href);
      if (url.origin === location.origin && url.searchParams.get('view') === 'settings' && ['tools', 'execution-pools'].includes(url.searchParams.get('sub') ?? '')) {
        event.preventDefault(); navigate(url.href);
      }
    };
    document.addEventListener('click', click);
    return () => document.removeEventListener('click', click);
  }, []);
  const pool = route.get('sub') === 'execution-pools';
  return <main id="main" tabIndex={-1} className="shared-setup-fixture" data-shared-setup-view={pool ? 'pool' : 'tools'}>
    <header className="shared-setup-heading"><strong>suma</strong><span>{locale === 'zh-CN' ? '共享工具接入 · 本地合成界面验证' : 'Shared tool setup · local synthetic interface verification'}</span></header>
    {pool ? <PoolWorkspace session={view.session} selected={route.get('record') ?? ''} panel={route.get('panel') ?? ''} refresh={0}
      onSelect={record => {const next = new URLSearchParams(route); next.set('record', record); navigate('?' + next);}}
      onPanel={panel => {const next = new URLSearchParams(route); next.set('panel', panel); navigate('?' + next);}} />
      : <ToolWorkspace session={view.session} project={view.project_id} selected={route.get('record') ?? ''} panel={route.get('panel') ?? ''} refresh={0}
        onSelect={record => {const next = new URLSearchParams(route); next.set('record', record); navigate('?' + next);}}
        onPoolSetup={pin => {const href = buildPoolSetupOwnerHref(pin); if (href) navigate(href);}} />}
  </main>;
}
void Promise.all([setLocale(locale, {persist: false}), fetch('/__fixture/view').then(response => {
  if (!response.ok) throw Error('Synthetic context unavailable');
  return response.json() as Promise<View>;
})]).then(([, view]) => createRoot(document.getElementById('root')!).render(<LanguageProvider><ApiViewProvider session={view.session}><Fixture view={view} /></ApiViewProvider></LanguageProvider>));
