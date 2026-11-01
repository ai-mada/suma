// @vitest-environment jsdom
import {act,useState} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ToolQuickConnect,type ToolConnectDraft,type ToolQuickConnectProps} from './ToolQuickConnect';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import type {IdentityRead,SharedIdentityRead} from './execution-pools/ToolIdentity';
import type {ToolAction} from './tool-hooks';

const adapter={id:'codex-jsonl',product:'Codex',entrypoint_kind:'cli',direction:'suma_to_tool',implementation:'implemented',setup_kind:'runner_profile',adapter:'codex',protocol:'codex-jsonl-v1',capabilities:[],source_refs:[],limitations:[]};
const profile={command_id:'codex-team',adapter:'codex',protocol:'codex-jsonl-v1',profile_sha256:'a'.repeat(64),executable_sha256:'b'.repeat(64),tool_version:'1.0',read_only:false};
const other={...profile,command_id:'codex-other',profile_sha256:'c'.repeat(64)};
const session={user:{id:'u',name:'Owner',email:'u@example.test'},organization:{id:'o',name:'Org'}};
const identity=(ids:string[])=>({loading:false,error:'',data:{items:ids.map(worker_id=>({worker_id,status:'active'}))},read:vi.fn()}) as unknown as IdentityRead;
const inventory=(worker:string,profiles=[profile,other])=>({project_id:'p',worker_id:worker,state:'fresh',as_of:'2026-10-30T08:00:20Z',observed_at:'2026-10-30T08:00:00Z',expires_at:'2026-10-30T08:01:00Z',items:profiles.map(profile=>({profile,available:true,reason:''}))});
let root:Root,host:HTMLDivElement,props:ToolQuickConnectProps,requests:{path:string;body?:Record<string,unknown>}[],readInventory:(worker:string)=>Promise<Response>,readMatches:(body:Record<string,unknown>)=>Promise<Response>;
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);root=createRoot(host);requests=[];
 readInventory=async worker=>Response.json(inventory(worker));readMatches=async body=>Response.json({...body,items:[],total:0});
 vi.stubGlobal('fetch',vi.fn(async(input:string,init:RequestInit={})=>{const path=String(input).replace('/api/v1',''),body=init.body?JSON.parse(String(init.body)):undefined;requests.push({path,body});if(path.startsWith('/adapter-catalog'))return Response.json({project_id:'p',as_of:'2026-10-30T08:00:20Z',items:[adapter]});if(path.startsWith('/tool-discovery'))return readInventory(new URL(path,'https://example.test').searchParams.get('worker_id')!);if(path==='/tool-profiles/matches')return readMatches(body);throw new Error('Unexpected request '+path);}));
 props={project:'p',workers:identity(['one','two']),sharedWorkers:identity([]) as unknown as SharedIdentityRead,action:{run:vi.fn(),clearError:vi.fn(),busy:false,pending:undefined,conflict:false} as unknown as ToolAction,onRegistered:vi.fn(),onEnable:vi.fn(),onOpen:vi.fn(),onAdvanced:vi.fn(),onWorker:vi.fn(),onViewExisting:vi.fn()};
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();});
async function settle(){await act(async()=>{for(let i=0;i<24;i++)await Promise.resolve();});}
async function click(selector:string){const button=host.querySelector<HTMLButtonElement>(selector);expect(button).not.toBeNull();expect(button!.disabled).toBe(false);await act(async()=>button!.click());await settle();}
async function change(selector:string,value:string){const field=host.querySelector<HTMLInputElement|HTMLSelectElement>(selector)!;expect(field).not.toBeNull();await act(async()=>{Object.getOwnPropertyDescriptor(field instanceof HTMLSelectElement?HTMLSelectElement.prototype:HTMLInputElement.prototype,'value')!.set!.call(field,value);field.dispatchEvent(new Event(field instanceof HTMLSelectElement?'change':'input',{bubbles:true}));});await settle();}

it('T193 restores only the public Quick draft after Advanced and rechecks fresh inventory and exact reuse without callback loops',async()=>{
 const seed:ToolConnectDraft={adapterID:'codex-jsonl',workerID:'two',commandID:'codex-other',name:'Retained team tool',nameEdited:true},updates:ToolConnectDraft[]=[];
 function Coordinator(){const [draft,setDraft]=useState(seed),[quick,setQuick]=useState(true);return <ApiViewProvider session={session}>{quick?<ToolQuickConnect {...props} draft={draft} onDraftChange={value=>{updates.push(value);setDraft(value);}} onAdvanced={item=>{props.onAdvanced(item);setQuick(false);}}/>:<button data-return onClick={()=>setQuick(true)}>Return</button>}</ApiViewProvider>;}
 await act(async()=>root.render(<Coordinator/>));await settle();
 expect(host.querySelector<HTMLSelectElement>('[data-quick-tool]')?.value).toBe(adapter.id);expect(host.querySelector<HTMLSelectElement>('[data-quick-worker]')?.value).toBe('two');expect(host.querySelector<HTMLSelectElement>('[data-quick-configuration]')?.value).toBe(other.command_id);expect(host.querySelector<HTMLInputElement>('[data-quick-name]')?.value).toBe(seed.name);
 expect(requests.filter(item=>item.path==='/tool-profiles/matches').at(-1)?.body).toEqual({project_id:'p',worker_id:'two',profile:other});
 await click('[data-quick-advanced]');expect(props.onAdvanced).toHaveBeenCalledExactlyOnceWith(adapter);
 let finish!:(value:Response)=>void;readInventory=()=>new Promise(resolve=>{finish=resolve;});const freshOther={...other,profile_sha256:'d'.repeat(64)},existing={id:'existing',project_id:'p',worker_id:'two',name:seed.name,version:4,state:'paused',profile:freshOther,available:false,reason:'Needs explicit enable.',enabled_by:'',created_at:'2026-10-30T08:00:00Z',updated_at:'2026-10-30T08:00:00Z'};
 readMatches=async body=>Response.json({...body,items:[existing],total:1});await click('[data-return]');expect(requests.filter(item=>item.path.startsWith('/tool-discovery'))).toHaveLength(2);expect(requests.filter(item=>item.path==='/tool-profiles/matches')).toHaveLength(1);expect(host.querySelector('[data-existing-tools]')).toBeNull();expect(props.action.run).not.toHaveBeenCalled();
 await act(async()=>finish(Response.json(inventory('two',[profile,freshOther]))));await settle();expect(requests.filter(item=>item.path==='/tool-profiles/matches').at(-1)?.body).toEqual({project_id:'p',worker_id:'two',profile:freshOther});expect(host.querySelector('[data-quick-register]')).toBeNull();expect(host.querySelector('[data-existing-tools]')).not.toBeNull();expect(updates.at(-1)).toEqual(seed);expect(updates.length).toBeLessThan(8);
 await click('[data-existing-tool-enable]');expect(props.onEnable).toHaveBeenCalledExactlyOnceWith(existing);expect(props.action.run).not.toHaveBeenCalled();
});

it('T193 never substitutes a sole different node or command for a restored target and offers an explicit existing-connections route',async()=>{
 props.workers=identity(['one']);props.draft={adapterID:adapter.id,workerID:'missing-node',commandID:'missing-command',name:'Keep my name',nameEdited:true};props.onDraftChange=vi.fn();
 async function render(key:string){await act(async()=>root.render(<ApiViewProvider session={session}><ToolQuickConnect key={key} {...props}/></ApiViewProvider>));await settle();}
 await render('missing-node');expect(requests.filter(item=>item.path.startsWith('/tool-discovery'))).toHaveLength(0);expect(host.querySelector<HTMLSelectElement>('[data-quick-worker]')?.value).toBe('');expect(host.querySelector('[data-quick-register]')).toBeNull();
 props.draft={...props.draft,workerID:'one'};readInventory=async worker=>Response.json(inventory(worker,[profile]));await render('missing-command');expect(host.querySelector<HTMLSelectElement>('[data-quick-configuration]')?.value).toBe('');expect(requests.filter(item=>item.path==='/tool-profiles/matches')).toHaveLength(0);expect(host.querySelector<HTMLButtonElement>('[data-quick-register]')?.disabled).toBe(true);
 await click('[data-quick-view-existing]');expect(props.onViewExisting).toHaveBeenCalledExactlyOnceWith(adapter);expect(props.action.run).not.toHaveBeenCalled();
 await change('[data-quick-configuration]',profile.command_id);expect(host.querySelector<HTMLInputElement>('[data-quick-name]')?.value).toBe('Keep my name');expect(requests.filter(item=>item.path==='/tool-profiles/matches').at(-1)?.body).toEqual({project_id:'p',worker_id:'one',profile});
 props.preferredWorkerID='two';props.workers=identity(['one','two']);await render('preferred');expect(host.querySelector<HTMLSelectElement>('[data-quick-worker]')?.value).toBe('two');expect(requests.filter(item=>item.path.startsWith('/tool-discovery')).at(-1)?.path).toContain('worker_id=two');expect(props.action.run).not.toHaveBeenCalled();
});
