import {useState} from 'react';
import {createRoot} from 'react-dom/client';
import '../../../apps/web/src/style.css';
import '../../../apps/web/src/experience-foundation.css';
import {ApiViewProvider} from '../../../apps/web/src/api-context';
import {LanguageProvider} from '../../../apps/web/src/i18n/LanguageSelector';
import {setLocale} from '../../../apps/web/src/i18n';
import {RemoteToolScanPanel} from '../../../apps/web/src/RemoteToolScanPanel';
import {LocalToolsScanPanel} from '../../../apps/web/src/LocalToolsScanPanel';
import type {LocalToolsScanRequest} from '../../../apps/web/src/local-tools-scan-model';
import type {AdapterDescriptor} from '../../../apps/web/src/AdapterConnections';
import type {Session} from '../../../apps/web/src/api';

type View={project:string;worker:string;session:Session;adapters:AdapterDescriptor[];local_initial:unknown;local_stopped:unknown};
const query=new URLSearchParams(location.search),locale=query.get('locale')==='en-US'?'en-US':'zh-CN',local=query.get('scene')==='local';
document.documentElement.lang=locale;document.documentElement.dataset.theme=query.get('theme')==='dark'?'dark':'light';
const observations:{localCalls:LocalToolsScanRequest[];guides:{adapterID:string;workerID:string}[];selections:string[]}={localCalls:[],guides:[],selections:[]};
Object.defineProperty(window,'__sumaProbeFixture',{value:observations});

/** Only the callback capture and desktop observation bridge are synthetic.
 * Rows, parsing, actions, source admission and stop gates are production UI. */
function Fixture({view}:{view:View}){
 const [guide,setGuide]=useState<{adapterID:string;workerID:string}>();
 function opened(adapter:AdapterDescriptor,workerID=''){const target={adapterID:adapter.id,workerID};observations.guides.push(target);setGuide(target);}
 return <main className="probe-onboarding-fixture" data-probe-onboarding-fixture>
  <header><strong>suma</strong><h1>{locale==='zh-CN'?'工具安装检测':'Tool installation check'}</h1></header>
  <p className="fixture-boundary">{locale==='zh-CN'?'合成公共HTTP与桌面观察桥；不运行真实工具、节点或模型。指南回调由夹具记录，不替代完整应用路由。':'Synthetic public HTTP and desktop observation bridge; no real tool, node or model. The fixture records the guide callback, rather than full App routing.'}</p>
  {local?<LocalToolsScanPanel project={view.project} adapters={view.adapters} selectedAdapterID="claude-code-sdk" onSelect={adapter=>observations.selections.push(adapter.id)} onAdvanced={adapter=>opened(adapter)}/>:<RemoteToolScanPanel project={view.project} workerID={view.worker} selectedAdapterID="claude-code-sdk" onPrepare={(adapter,workerID)=>opened(adapter,workerID)}/>}
  {guide&&<p role="status" data-fixture-guide-target data-adapter-id={guide.adapterID} data-worker-id={guide.workerID}>{locale==='zh-CN'?'已交接准确指南目标':'Exact guide callback recorded'}: <code>{guide.adapterID}</code> · <code>{guide.workerID}</code></p>}
 </main>;
}
void Promise.all([setLocale(locale,{persist:false}),fetch('/__fixture/view').then(response=>{if(!response.ok)throw Error('Fixture view unavailable');return response.json() as Promise<View>;})]).then(([,view])=>{
 if(local)Object.defineProperty(window,'sumaDesktop',{value:{platform:'win32',version:'1.0.0',discoverInstalledTools:async(request:LocalToolsScanRequest)=>{observations.localCalls.push(structuredClone(request));return structuredClone(observations.localCalls.length===1?view.local_initial:view.local_stopped);}}});
 createRoot(document.getElementById('root')!).render(<LanguageProvider><ApiViewProvider session={view.session}><Fixture view={view}/></ApiViewProvider></LanguageProvider>);
});
