// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {Capabilities} from '../project-views/Capabilities';
import {setLocale,setServerCatalog} from './index';

// Exact built-in declarations returned by Server.capabilities in
// internal/platform/http.go. Boolean observations remain separate facts.
const declarations=[
 {id:'transactional_store',name:'业务事务库',configured:true,available:true,verified:false,backend:'sqlite',checks:{connection:true,transaction:'not_run',backup_restore:'not_run'},reason:'仅实时探测连接；事务、迁移和恢复验收见交付报告'},
 {id:'dag',name:'持久化 DAG',configured:true,available:true,verified:false,reason:'已探测调度表；完整运行验收见交付证据'},
 {id:'files',name:'本地文件',configured:true,available:true,verified:false,reason:'目录存在性探测，不代表写入、备份或恢复已验证'},
 {id:'semantic_search',name:'多模态语义检索',configured:false,available:false,verified:false,reason:'需要连接检索服务并执行读写与检索验证'},
 {id:'payments',name:'支付结算',configured:false,available:false,verified:false,reason:'尚未配置商户与支付适配器'},
 {id:'collaboration',name:'多人实时协作',configured:false,available:false,verified:false,reason:'协作服务尚未部署'},
];
const english=[
 ['Transactional data store','Only connectivity is probed live; see the delivery report for transaction, migration, and recovery acceptance.'],
 ['Persistent DAG','Scheduling tables were probed; see delivery evidence for full run acceptance.'],
 ['Local files','Directory existence is probed; this does not verify writing, backup, or recovery.'],
 ['Multimodal semantic search','Connect the search service and verify writing, reading, and retrieval.'],
 ['Payments','No merchant account or payment adapter is configured.'],
 ['Real-time collaboration','The collaboration service has not been deployed.'],
];
let root:Root,host:HTMLDivElement;
beforeEach(async()=>{
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);localStorage.clear();
 await setLocale('en-US',{persist:false});
 setServerCatalog({version:'capability-empty',template_format:'named-dot-v1',default_locale:'zh-CN',supported_locales:['zh-CN','en-US'],messages:[]});
 host=document.createElement('div');document.body.append(host);root=createRoot(host);
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();await setLocale('zh-CN',{persist:false});vi.unstubAllGlobals();vi.restoreAllMocks();});
async function settle(){for(let i=0;i<5;i++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function render(project='project-a'){await act(async()=>root.render(<Capabilities project={project} refresh={0}/>));await settle();}
async function locale(value:'zh-CN'|'en-US'){await act(async()=>{await setLocale(value,{persist:false});});await settle();}
function response(items:unknown[]){return new Response(JSON.stringify({items,as_of:'2026-10-24T03:04:05Z'}),{headers:{'Content-Type':'application/json'}});}

it('translates the six actual built-in declarations in place without changing observations or rereading',async()=>{
 const original=JSON.stringify(declarations);
 vi.stubGlobal('fetch',vi.fn(async()=>response(declarations)));await render();
 const cards=[...host.querySelectorAll('.capability-card')];expect(cards).toHaveLength(6);
 english.forEach(([name,reason],index)=>{expect(cards[index].querySelector('h3')?.textContent).toBe(name);expect(cards[index].querySelector('p')?.textContent).toBe(reason);});
 expect(cards.every(card=>card.querySelector('[data-capability-state="verified"] .no'))).toBe(true);
 expect(cards[0].querySelector('[data-capability-state="configured"] .yes')).not.toBeNull();
 expect(cards[3].querySelector('[data-capability-state="configured"] .no')).not.toBeNull();
 await locale('zh-CN');
 declarations.forEach((item,index)=>{expect(cards[index].isConnected).toBe(true);expect(cards[index].querySelector('h3')?.textContent).toBe(item.name);expect(cards[index].querySelector('p')?.textContent).toBe(item.reason);});
 expect(fetch).toHaveBeenCalledTimes(1);const [path,options]=vi.mocked(fetch).mock.calls[0];expect(String(path)).toBe('/api/v1/capabilities?project_id=project-a');expect(options?.body).toBeUndefined();
 expect(JSON.stringify(declarations)).toBe(original);
});

it('preserves custom and unknown declarations even when their text resembles another built-in capability',async()=>{
 const items=[
  {id:'transactional_store',name:'用户名称 <code>{{raw}}</code>',reason:'用户诊断 <strong>{{raw}}</strong>',configured:'configured',available:false,verified:'verified'},
  {id:'future-provider',name:'支付结算',reason:'尚未配置商户与支付适配器',configured:true,available:true,verified:false},
  {id:'dag',name:'本地文件',reason:'协作服务尚未部署',configured:false,available:false,verified:false},
 ];
 vi.stubGlobal('fetch',vi.fn(async()=>response(items)));await render();const cards=[...host.querySelectorAll('.capability-card')];
 const assertRaw=()=>items.forEach((item,index)=>{expect(cards[index].querySelector('h3')?.textContent).toBe(item.name);expect(cards[index].querySelector('p')?.textContent).toBe(item.reason);});
 assertRaw();expect(host.querySelector('code')).toBeNull();expect(host.querySelector('strong')).toBeNull();
 expect(cards[0].querySelector('[data-capability-state="configured"] .unknown')).not.toBeNull();expect(cards[0].querySelector('[data-capability-state="available"] .no')).not.toBeNull();expect(cards[0].querySelector('[data-capability-state="verified"] .unknown')).not.toBeNull();
 await locale('zh-CN');assertRaw();await locale('en-US');assertRaw();expect(fetch).toHaveBeenCalledTimes(1);
});

it('keeps structured capability-read errors translatable with raw parameters and no automatic retry',async()=>{
 setServerCatalog({version:'capability-error',template_format:'named-dot-v1',default_locale:'zh-CN',supported_locales:['zh-CN','en-US'],messages:[{key:'fixture.capability.denied',parameters:[{name:'scope',kind:'text'}],texts:{'en-US':'Cannot read {{.scope}}.','zh-CN':'无法读取{{.scope}}。'}}]});
 vi.stubGlobal('fetch',vi.fn(async()=>new Response(JSON.stringify({error:{code:'scope_denied',message:'Original capability refusal',message_key:'fixture.capability.denied',message_params:{scope:'原始范围 <strong>{{raw}}</strong>'}}}),{status:403})));
 await render();expect(host.querySelector('[role=alert]')?.textContent).toContain('Cannot read 原始范围 <strong>{{raw}}</strong>.');
 await locale('zh-CN');expect(host.querySelector('[role=alert]')?.textContent).toContain('无法读取原始范围 <strong>{{raw}}</strong>。');expect(host.querySelector('strong')).toBeNull();expect(fetch).toHaveBeenCalledTimes(1);
});

it('rejects a late old-project declaration after an in-flight language and project switch',async()=>{
 let resolveOld!:(value:Response)=>void;
 vi.stubGlobal('fetch',vi.fn(async(input:string)=>String(input).includes('project-a')?new Promise<Response>(resolve=>{resolveOld=resolve;}):response([{...declarations[4],name:'当前项目用户名称'}])));
 await render();await locale('zh-CN');await render('project-b');expect(host.textContent).toContain('当前项目用户名称');
 await act(async()=>resolveOld(response([{...declarations[0],name:'旧项目私有名称'}])));await settle();
 expect(host.textContent).not.toContain('旧项目私有名称');expect(host.textContent).toContain('当前项目用户名称');await locale('en-US');expect(host.textContent).toContain('No merchant account or payment adapter is configured.');expect(fetch).toHaveBeenCalledTimes(2);
});
