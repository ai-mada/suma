import {createRoot} from 'react-dom/client';
import {useState} from 'react';
import '../../../apps/web/src/style.css';
import '../../../apps/web/src/experience-foundation.css';
import {ApiViewProvider} from '../../../apps/web/src/api-context';
import {LanguageProvider} from '../../../apps/web/src/i18n/LanguageSelector';
import {setLocale} from '../../../apps/web/src/i18n';
import {RemoteToolScanPanel} from '../../../apps/web/src/RemoteToolScanPanel';
import {ToolSetupGuide} from '../../../apps/web/src/ToolSetupGuide';
import {toolPreparationTarget} from '../../../apps/web/src/tool-preparation-target';
import type {AdapterDescriptor} from '../../../apps/web/src/AdapterConnections';
import type {Session} from '../../../apps/web/src/api';

// No fetch/IPC substitution. Public boot context only; the native driver owns
// temporary authentication and private fixture actions outside this renderer.
type View={project_id:string;worker_id:string;local_tool:string;session:Session};
const params=new URLSearchParams(location.search),locale=params.get('locale')==='en-US'?'en-US':'zh-CN',replay=params.get('replay')==='1';
const prepares:{adapterID:string;workerID:string}[]=[];
function App({view}:{view:View}){
 const [guide,setGuide]=useState<{preset:any;workerID:string;nonce:number}>();
 function prepare(adapter:AdapterDescriptor,workerID:string){const target=toolPreparationTarget(adapter);if(!target)throw Error('Unexpected adapter');prepares.push({adapterID:adapter.id,workerID});setGuide(previous=>({preset:target.preset,workerID,nonce:(previous?.nonce||0)+1}));}
 return <main className="scan-fixture" data-remote-scan-fixture><header><small>suma 1.0.0</small><h1>{locale==='zh-CN'?'在执行节点检测开发工具':'Discover tools on the execution node'}</h1><p>{replay?(locale==='zh-CN'?'只读界面复核 · 原扫描公开数据':'Read-only visual review · original public scan data'):(locale==='zh-CN'?'隔离服务 · 真实 Runner 与受控原生检测':'Isolated service · actual Runner and owned native probes')}</p></header>
 <section className="scan-fixture-card"><RemoteToolScanPanel project={view.project_id} workerID={view.worker_id} selectedAdapterID={view.local_tool} onPrepare={prepare}/></section>
 {guide&&<section className="scan-fixture-card" data-remote-scan-guide><ToolSetupGuide project={view.project_id} expectedWorkerID={guide.workerID} expanded presetRequest={{preset:guide.preset,nonce:guide.nonce}}/></section>}
 </main>;
}
Object.assign(window,{__remoteToolScan205:{snapshot:()=>({prepares:structuredClone(prepares)})}});
document.documentElement.dataset.theme=params.get('theme')==='dark'?'dark':'light';document.documentElement.lang=locale;
void Promise.all([setLocale(locale,{persist:false}),fetch('/__fixture/view').then(response=>{if(!response.ok)throw Error('Fixture context unavailable');return response.json() as Promise<View>;})]).then(([,view])=>createRoot(document.getElementById('root')!).render(<LanguageProvider><ApiViewProvider session={view.session}><App view={view}/></ApiViewProvider></LanguageProvider>));
