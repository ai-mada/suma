import {useMemo,useState} from 'react';
import {createRoot} from 'react-dom/client';
import '../../../apps/web/src/style.css';
import '../../../apps/web/src/experience-foundation.css';
import {ApiViewProvider,useApi} from '../../../apps/web/src/api-context';
import {LanguageProvider} from '../../../apps/web/src/i18n/LanguageSelector';
import {setLocale} from '../../../apps/web/src/i18n';
import {RemoteToolScanPanel} from '../../../apps/web/src/RemoteToolScanPanel';
import {ToolQuickConnect,type ToolConnectDraft} from '../../../apps/web/src/ToolQuickConnect';
import {ToolWriteRecovery} from '../../../apps/web/src/tool-write';
import {useToolAction} from '../../../apps/web/src/tool-hooks';
import type {IdentityRead,SharedIdentityRead} from '../../../apps/web/src/execution-pools/ToolIdentity';
import type {Session} from '../../../apps/web/src/api';
import type {AdapterDescriptor} from '../../../apps/web/src/AdapterConnections';
import type {ToolProfile} from '../../../packages/contracts/src';

type View={project:string;worker:string;session:Session;adapter:AdapterDescriptor;draft:ToolConnectDraft;identity:IdentityRead['data']};
const query=new URLSearchParams(location.search),locale=query.get('locale')==='en-US'?'en-US':'zh-CN',scene=query.get('scene')==='drift'?'drift':'prepare';
document.documentElement.lang=locale;document.documentElement.dataset.theme=query.get('theme')==='dark'?'dark':'light';
/** This transparent fixture bridge replaces only the host modal transition.
 * Both scan-source ownership and Quick's current preparation/inventory gates
 * are the production components; this is not full App routing or a server. */
function Fixture({view}:{view:View}){
 const api=useApi(),[draft,setDraft]=useState<ToolConnectDraft|undefined>(scene==='drift'?view.draft:undefined),[saved,setSaved]=useState<ToolProfile>();
 const recovery=useMemo(()=>new ToolWriteRecovery(view.session.organization.id+':'+view.session.user.id+':'+view.project+':profiles',sessionStorage,api),[api,view]),action=useToolAction(recovery);
 const workers={loading:false,error:'',data:view.identity,read:async()=>true} as IdentityRead,sharedWorkers={loading:false,error:'',data:{items:[],total:0},read:async()=>true} as SharedIdentityRead;
 return <main id="main" tabIndex={-1} className="managed-preparation-fixture" data-managed-preparation-fixture data-fixture-scene={scene}>
  <header className="fixture-heading"><strong>suma</strong><span>{locale==='zh-CN'?'远程配置接续 · 合成界面验证':'Remote preparation handoff · synthetic interface verification'}</span></header>
  <p className="fixture-boundary">{locale==='zh-CN'?'隔离公共 HTTP 夹具；不连接真实节点，不安装、登录或调用模型。':'Isolated public HTTP fixture; no real node, installation, login or model.'}</p>
  {!draft?<RemoteToolScanPanel project={view.project} workerID={view.worker} selectedAdapterID={view.adapter.id} onPrepare={()=>{}} onPreparedConfiguration={(selection,preparationID)=>setDraft({adapterID:selection.adapterID,workerID:selection.workerID,commandID:selection.commandID,name:'',nameEdited:false,observationPin:{profileSHA256:selection.profileSHA256,observedAt:selection.observedAt,preparationID}})}/>:<ToolQuickConnect project={view.project} adapter={view.adapter} draft={draft} preferredWorkerID={view.worker} workers={workers} sharedWorkers={sharedWorkers} action={action} onDraftChange={setDraft} onRegistered={setSaved} onEnable={()=>{throw Error('Fixture must never enable a tool');}} onAdvanced={()=>{throw Error('Fixture must not invoke a preparation guide');}} onWorker={()=>{throw Error('Fixture must not register a node');}}/>}
  {saved&&<p role="status" data-fixture-paused-saved={saved.id}>{locale==='zh-CN'?'已保存暂停配置，未启用。':'Paused configuration saved; not enabled.'}</p>}
 </main>;
}
void Promise.all([setLocale(locale,{persist:false}),fetch('/__fixture/view').then(response=>{if(!response.ok)throw Error('Fixture view unavailable');return response.json() as Promise<View>;})]).then(([,view])=>createRoot(document.getElementById('root')!).render(<LanguageProvider><ApiViewProvider session={view.session}><Fixture view={view}/></ApiViewProvider></LanguageProvider>));
