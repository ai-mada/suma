// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import {ToolQuickConnect,type ToolQuickConnectProps} from './ToolQuickConnect';
import type {AdapterDescriptor} from './AdapterConnections';
import type {LocalToolsScanPanelProps} from './LocalToolsScanPanel';
import type {IdentityRead,SharedIdentityRead} from './execution-pools/ToolIdentity';
import type {ToolAction} from './tool-hooks';
import {setLocale} from './i18n';

const scan=vi.hoisted(()=>({latest:undefined as LocalToolsScanPanelProps|undefined}));
vi.mock('./LocalToolsScanPanel',()=>({LocalToolsScanPanel:(props:LocalToolsScanPanelProps)=>{scan.latest=props;return <div>{props.adapters.map(adapter=><button key={adapter.id} type="button" data-scan-choose={adapter.id} disabled={props.disabled} onClick={()=>props.onSelect(adapter)}>{adapter.product}</button>)}</div>;}}));
const adapter:AdapterDescriptor={id:'codex-jsonl',product:'Codex',entrypoint_kind:'cli',direction:'suma_to_tool',implementation:'implemented',setup_kind:'runner_profile',adapter:'codex',protocol:'codex-jsonl-v1',capabilities:[],source_refs:[],limitations:[]};
const buddy:AdapterDescriptor={...adapter,id:'codebuddy-cli',product:'CodeBuddy Code',adapter:'codebuddy',protocol:'codebuddy-jsonl-v1'};
const session={user:{id:'2'.repeat(32),name:'Owner',email:'owner@example.test'},organization:{id:'3'.repeat(32),name:'Team'}};
const project='1'.repeat(32),worker='worker-one';
const identity={loading:false,error:'',data:{items:[{worker_id:worker,status:'active'}]},read:vi.fn()} as unknown as IdentityRead;
let root:Root,host:HTMLDivElement,props:ToolQuickConnectProps;
async function settle(){await act(async()=>{for(let i=0;i<20;i++)await Promise.resolve();});}
async function render(actor=session){await act(async()=>root.render(<ApiViewProvider session={actor}><ToolQuickConnect {...props}/></ApiViewProvider>));await settle();}
async function choose(id:string){const button=host.querySelector<HTMLButtonElement>('[data-scan-choose="'+id+'"]');expect(button).not.toBeNull();await act(async()=>button!.click());await settle();}
async function name(value:string){const input=host.querySelector<HTMLInputElement>('[data-quick-name]')!;await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(input,value);input.dispatchEvent(new Event('input',{bubbles:true}));});await settle();}
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);root=createRoot(host);
 props={project,workers:identity,sharedWorkers:{...identity,data:{items:[]}} as unknown as SharedIdentityRead,action:{run:vi.fn(),busy:false,conflict:false} as unknown as ToolAction,onRegistered:vi.fn(),onEnable:vi.fn(),onAdvanced:vi.fn(),onWorker:vi.fn()};
 vi.stubGlobal('fetch',vi.fn(async(input:string)=>Response.json(String(input).includes('/adapter-catalog')?{project_id:project,as_of:'2026-10-31T08:00:00Z',items:[adapter,buddy]}:{project_id:project,worker_id:worker,state:'fresh',as_of:'2026-10-31T08:00:00Z',observed_at:'2026-10-31T08:00:00Z',expires_at:'2026-10-31T08:01:00Z',items:[adapter,buddy].map(item=>({profile:{command_id:item.adapter+'-team',adapter:item.adapter,protocol:item.protocol,profile_sha256:'a'.repeat(64),executable_sha256:'b'.repeat(64),tool_version:'1.0',read_only:false},available:true,reason:''}))})));
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();});

it('routes a scan selection through existing configuration while retaining per-tool drafts and the original pending-operation lock',async()=>{
 await render();expect(host.querySelector<HTMLSelectElement>('[data-quick-tool]')!.value).toBe('');
 await choose('codex-jsonl');expect(document.activeElement).toBe(host.querySelector('[data-quick-tool]'));await name('Team Codex');
 await choose('codebuddy-cli');await name('Team Buddy');
 await choose('codex-jsonl');expect(host.querySelector<HTMLInputElement>('[data-quick-name]')!.value).toBe('Team Codex');
 const original={path:'/tool-profiles',method:'POST',idempotency_key:'original',body:{project_id:project}};
 props.action={...props.action,pending:original} as unknown as ToolAction;await render();
 expect(host.querySelector<HTMLSelectElement>('[data-quick-tool]')!.disabled).toBe(true);expect(host.querySelector<HTMLButtonElement>('[data-scan-choose="codebuddy-cli"]')!.disabled).toBe(true);
 await act(async()=>scan.latest!.onSelect(buddy));await settle();expect(host.querySelector<HTMLSelectElement>('[data-quick-tool]')!.value).toBe('codex-jsonl');
 expect(props.action.pending).toBe(original);expect(props.action.run).not.toHaveBeenCalled();expect(props.onEnable).not.toHaveBeenCalled();
 props.action={...props.action,pending:undefined};await render({...session,user:{...session.user,id:'4'.repeat(32)}});
 expect(host.querySelector<HTMLInputElement>('[data-quick-name]')).toBeNull();expect(host.querySelector<HTMLSelectElement>('[data-quick-tool]')!.value).toBe('');
});
