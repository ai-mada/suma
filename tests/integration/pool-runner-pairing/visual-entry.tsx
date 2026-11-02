import {createRoot} from 'react-dom/client';
import '../../../apps/web/src/style.css';
import '../../../apps/web/src/experience-foundation.css';
import {LanguageProvider} from '../../../apps/web/src/i18n/LanguageSelector';
import {setLocale} from '../../../apps/web/src/i18n';
import {PoolRunnerPairingCard} from '../../../apps/web/src/execution-pools/PoolRunnerPairingCard';
import type {PoolRunnerPairing} from '../../../packages/contracts/src';

// Explicitly synthetic, read-only presentation of the original public inputs.
// Approval and every business action remain disabled; no HTTP API is connected.
type View={pairing:PoolRunnerPairing};
const params=new URLSearchParams(location.search),locale=params.get('locale')==='en-US'?'en-US':'zh-CN';
document.documentElement.lang=locale;document.documentElement.dataset.theme=params.get('theme')==='dark'?'dark':'light';
void Promise.all([setLocale(locale,{persist:false}),fetch('/__fixture/view').then(r=>r.json() as Promise<View>)]).then(([,view])=>createRoot(document.getElementById('root')!).render(<LanguageProvider><main id="main" tabIndex={-1} className="pool-fixture"><header><strong>suma 1.0.0</strong><h1>{locale==='zh-CN'?'提案视图 · 只读展示复核':'Proposal view · read-only visual review'}</h1><p>{locale==='zh-CN'?'原公开输入的合成提案状态，禁用审批与业务动作':'Synthetic proposal state from original public inputs; approval and business actions disabled'}</p></header><div className="pool-runner-pairing"><PoolRunnerPairingCard scope={{poolID:view.pairing.pool_id,organizationID:view.pairing.owner_tenant_id,userID:'f'.repeat(32)}} pairing={view.pairing} loading={false} stale={false} disabled refreshDisabled canCreate={false} pending={false} onCreate={()=>{}} onRefresh={()=>{}} onApprove={()=>{}} onReject={()=>{}} onRestore={()=>{}} onContinue={()=>{}}/></div></main></LanguageProvider>));
