// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ToolSetupGuide} from './ToolSetupGuide';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import type {QoderSetupDraft} from './qoder-setup-guide-model';
import type {Session} from './api';
// Keep real Guide generation, strict Remote request parsing and field adoption.
// The Qoder form is independently tested; capture its strict parent boundary.
const boundary=vi.hoisted(()=>({callbacks:[] as Array<(draft:QoderSetupDraft)=>void>}));
vi.mock('./QoderSetupGuide',()=>({QoderSetupGuide:({onUseConfiguration}:{onUseConfiguration?:(draft:QoderSetupDraft)=>void})=>{if(onUseConfiguration)boundary.callbacks.push(onUseConfiguration);return <button data-test-qoder-use onClick={()=>onUseConfiguration?.({shell:'powershell',commands:'D:/work/qoder.commands.json',localTool:'qoder-acp'})}>Use Qoder output</button>;}}));
vi.mock('./ToolReportFeedback',()=>({ToolReportFeedback:()=>null}));
let root:Root,host:HTMLDivElement,requests:string[]=[];
const session:Session={user:{id:'u',name:'Owner',email:'owner@example.test',role:'owner'},organization:{id:'o',name:'Org'},projects:[]};
async function settle(){await act(async()=>{for(let i=0;i<10;i++)await Promise.resolve();await new Promise(r=>setTimeout(r,20));});}
async function draw({worker='ordinary',user='u',disabled=false}:{worker?:string;user?:string;disabled?:boolean}={}){await act(async()=>root.render(<ApiViewProvider session={{...session,user:{...session.user,id:user}}}><ToolSetupGuide project="p" expanded expectedWorkerID={worker} onPrepareSharedPool={()=>{}} disabled={disabled}/></ApiViewProvider>));await settle();}
async function useCurrent(){await act(async()=>document.querySelector<HTMLButtonElement>('[data-test-qoder-use]')!.click());await settle();}
async function choose(preset:string){await act(async()=>{const select=host.querySelector<HTMLSelectElement>('[data-tool-setup-preset]')!;select.value=preset;select.dispatchEvent(new Event('change',{bubbles:true}));});await settle();}
function commands(){return host.querySelector<HTMLInputElement>('[name="remote-runner-commands"]')?.value;}
beforeEach(async()=>{await setLocale('en-US',{persist:false});boundary.callbacks=[];requests=[];vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.stubGlobal('requestAnimationFrame',(f:FrameRequestCallback)=>setTimeout(()=>f(performance.now()),0));vi.stubGlobal('cancelAnimationFrame',clearTimeout);vi.stubGlobal('fetch',vi.fn(async(input:string)=>{requests.push(String(input));return Response.json({error:{code:'unavailable',message:'No API belongs to offline preparation'}},{status:503});}));host=document.createElement('div');document.body.append(host);root=createRoot(host);});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();});
it('T221 adopts Qoder output into ordinary fields and keeps shared output reference-only',async()=>{
 await draw();await choose('qoder');await useCurrent();
 expect(commands()).toBe('D:/work/qoder.commands.json');expect(host.querySelector<HTMLSelectElement>('[name="remote-runner-shell"]')?.value).toBe('powershell');expect(host.querySelector<HTMLSelectElement>('[name="remote-runner-localTool"]')?.value).toBe('qoder-acp');expect(host.querySelector<HTMLInputElement>('[name="remote-runner-identityPath"]')?.value).toBe('');
 await act(async()=>root.unmount());root=createRoot(host);await draw({worker:'pool-machine'});await choose('qoder');await useCurrent();
 expect(host.querySelector('[data-remote-runner-setup]')).toBeNull();expect(host.querySelector('[data-shared-pool-output]')?.textContent).toContain('D:/work/qoder.commands.json');expect(requests).toEqual([]);
});
it('T221 rejects old Qoder callbacks after tool, lock and actor ABA and refuses extended drafts',async()=>{
 await draw();await choose('qoder');const old=boundary.callbacks.at(-1)!;await choose('codex');await choose('qoder');await act(async()=>old({shell:'powershell',commands:'D:/old.json',localTool:'qoder-acp'}));await settle();expect(commands()).toBe('');
 const beforeLock=boundary.callbacks.at(-1)!;await draw({disabled:true});await draw();await act(async()=>beforeLock({shell:'powershell',commands:'D:/old-lock.json',localTool:'qoder-acp'}));await settle();expect(commands()).toBe('');
 const beforeActor=boundary.callbacks.at(-1)!;await draw({user:'other'});await draw();await choose('qoder');await act(async()=>beforeActor({shell:'powershell',commands:'D:/old-actor.json',localTool:'qoder-acp'}));await settle();expect(commands()).toBe('');
 const current=boundary.callbacks.at(-1)!;await act(async()=>current({...{shell:'powershell',commands:'D:/extra.json',localTool:'qoder-acp'},credential:'forbidden'} as unknown as QoderSetupDraft));await settle();expect(commands()).toBe('');
 await useCurrent();expect(commands()).toBe('D:/work/qoder.commands.json');expect(requests).toEqual([]);
});
