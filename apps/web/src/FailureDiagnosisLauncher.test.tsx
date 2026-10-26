// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import type {Session} from './api';
import {setLocale} from './i18n';
import {FailureDiagnosisLauncher} from './FailureDiagnosisLauncher';
vi.mock('./FailureDiagnosisPanel',()=>({FailureDiagnosisPanel:(props:{source:unknown;initiallyOpen?:boolean})=><div data-launch-diagnosis data-source={JSON.stringify(props.source)} data-open={String(props.initiallyOpen)}/> }));
const session:Session={user:{id:'user-a',name:'Owner',email:'owner@example.test'},organization:{id:'org-a',name:'Org'},projects:[{id:'project-a',version:1}]};
const run=()=>({id:'run-a',project_id:'project-a',version:3,state:'failed',nodes:[{id:'failed.node',name:'Actual failed node',state:'failed',attempt:3,fence:8},{id:'done',name:'Successful node',state:'succeeded',attempt:2,fence:7}],events:[{node_id:'old-event-node',attempt:99}]});
let host:HTMLDivElement,root:Root|undefined,reads:{path:string;signal?:AbortSignal|null}[],writes:number,reply:(path:string)=>Promise<Response>;
beforeEach(async()=>{await setLocale('en-US',{persist:false});host=document.createElement('div');document.body.append(host);reads=[];writes=0;vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);reply=async()=>Response.json(run());vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{if(init?.body)writes++;const path=String(input).replace(/^\/api\/v1/,'');reads.push({path,signal:init?.signal});return reply(path);}));});
afterEach(async()=>{if(root)await act(async()=>root?.unmount());root=undefined;host.remove();await setLocale('zh-CN',{persist:false});vi.restoreAllMocks();vi.unstubAllGlobals();});
async function settle(){for(let n=0;n<3;n++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function render(identity=session,project='project-a',runID='run-a'){await act(async()=>{root??=createRoot(host);root.render(<ApiViewProvider session={identity}><FailureDiagnosisLauncher session={identity} project={project} runID={runID} refresh={0}/></ApiViewProvider>);});await settle();}
async function click(selector:string){const node=host.querySelector<HTMLElement>(selector);expect(node,selector).not.toBeNull();await act(async()=>node!.click());await settle();}
async function select(value:string){const node=host.querySelector<HTMLSelectElement>('[name=fd_current_node]')!;expect(node).not.toBeNull();await act(async()=>{Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype,'value')!.set!.call(node,value);node.dispatchEvent(new Event('change',{bubbles:true}));});await settle();}

it('reads only after expansion selects an exact current failed attempt and only links unknown runs for reconciliation',async()=>{
 await render();expect(reads).toHaveLength(0);await click('[data-fd-launch-toggle]');expect(reads[0].path).toBe('/runs/run-a?project_id=project-a');const options=host.querySelector('[name=fd_current_node]')?.textContent;expect(options).toContain('Actual failed node');expect(options).not.toContain('Successful node');expect(options).not.toContain('old-event-node');await select('failed.node');expect(JSON.parse(host.querySelector('[data-launch-diagnosis]')!.getAttribute('data-source')!)).toEqual({run_id:'run-a',node_id:'failed.node',attempt:3});expect(host.querySelector('[data-launch-diagnosis]')?.getAttribute('data-open')).toBe('true');
 reply=async()=>Response.json({...run(),state:'running',nodes:[...run().nodes,{id:'unknown',name:'Unknown current node',state:'reconciling',attempt:4,fence:9}]});await click('[data-fd-launch-refresh]');await select('unknown');expect(host.querySelector('[data-launch-diagnosis]')).toBeNull();expect(host.textContent).toContain('reconcile the original run');expect(host.querySelector('[data-fd-launch-run]')?.getAttribute('href')).toContain('node=unknown');expect(writes).toBe(0);
});

it('aborts and clears old scope reads then explains forbidden and empty current runs without guessing a source',async()=>{
 const pending:{resolve:(value:Response)=>void}[]=[];reply=async()=>new Promise<Response>(resolve=>pending.push({resolve}));await render();await click('[data-fd-launch-toggle]');const previous=reads[0];await render({...session,user:{...session.user,id:'other'},organization:{id:'other-org',name:'Other'}},'project-b','run-b');expect(previous.signal?.aborted).toBe(true);await act(async()=>pending.forEach(value=>value.resolve(Response.json(run()))));await settle();expect(host.querySelector('[name=fd_current_node]')).toBeNull();expect(host.querySelector('[data-launch-diagnosis]')).toBeNull();
 reply=async()=>Response.json({error:{code:'forbidden',message:'Access denied'}},{status:403});await click('[data-fd-launch-toggle]');expect(host.textContent).toContain('Current access');expect(host.querySelector('[name=fd_current_node]')).toBeNull();reply=async()=>Response.json({id:'run-b',project_id:'project-b',version:1,state:'succeeded',nodes:[{id:'node',name:'Done',state:'succeeded',attempt:1}]});await click('[data-fd-launch-refresh]');expect(host.textContent).toContain('No current failed');expect(host.querySelector('[data-launch-diagnosis]')).toBeNull();expect(writes).toBe(0);
});
