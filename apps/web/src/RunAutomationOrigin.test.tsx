// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import type {Session} from './api';
import {setLocale} from './i18n';
import {automationTriggerPanelPrefix,parseWorkbenchRoute} from './navigation';
import {RunAutomationOrigin} from './RunAutomationOrigin';

const session:Session={user:{id:'user-a',name:'Owner',email:'owner@example.test'},organization:{id:'org-a',name:'Team'},projects:[]};
const project='project-a';
const run='run-a';
const occurrence={rule_id:'rule-a',rule_name:'Original automation',occurrence_id:'trigger-a',revision:2,source:'manual',scheduled_at:'2026-11-06T02:00:00Z'};
const exact=(selected=run)=>({project_id:project,run_id:selected,origin:occurrence});
let host:HTMLDivElement,root:Root,requests:{url:URL;init?:RequestInit}[];
let reply:()=>Promise<Response>;

beforeEach(async()=>{
 await setLocale('en-US',{persist:false});
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);
 host=document.createElement('div');document.body.append(host);root=createRoot(host);requests=[];
 reply=async()=>Response.json(exact());
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{
  requests.push({url:new URL(input,location.href),init});
  return reply();
 }));
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();});
async function settle(){for(let i=0;i<3;i++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function render(selected=run){
 await act(async()=>root.render(<ApiViewProvider session={session}><RunAutomationOrigin project={project} run={selected}/></ApiViewProvider>));
 await settle();
}
const get=(key:string)=>host.querySelector<HTMLElement>('[data-run-automation-origin-'+key+']');
async function click(key:string){const node=get(key);expect(node).not.toBeNull();await act(async()=>node!.click());await settle();}
const state=()=>get('state')?.getAttribute('data-run-automation-origin-state');

it('reads only on disclosure, rejects mismatched data, and opens the original trigger after retry',async()=>{
 reply=async()=>Response.json(exact('wrong-run'));
 await render();expect(requests).toHaveLength(0);
 await click('toggle');
 expect(requests[0].url.pathname).toBe('/api/v1/runs/run-a/automation-origin');
 expect(requests[0].url.searchParams.get('project_id')).toBe(project);
 expect(new Headers(requests[0].init?.headers).get('X-Suma-Expected-User')).toBe(session.user.id);
 expect(state()).toBe('error');expect(get('rule')).toBeNull();
 reply=async()=>Response.json(exact());
 await click('retry');
 expect(state()).toBe('found');expect(document.activeElement).toBe(get('toggle'));
 expect(host.textContent).toContain('Manual trigger · Rule revision 2');
 const rule=get('rule') as HTMLAnchorElement;
 expect(parseWorkbenchRoute(new URL(rule.href).search)).toMatchObject({project,view:'development',sub:'automations',record:'rule-a',run:'',panel:automationTriggerPanelPrefix+occurrence.occurrence_id});
 expect(host.querySelector('time')?.dateTime).toBe(occurrence.scheduled_at);
 const guard=vi.fn((event:Event)=>event.preventDefault());
 window.addEventListener('suma:navigate',guard,{once:true});
 await click('rule');expect(guard).toHaveBeenCalledOnce();
 await act(async()=>setLocale('zh-CN',{persist:false}));
 expect(host.textContent).toContain('规则修订 2');expect(host.textContent).toContain('打开原触发');
 expect(requests).toHaveLength(2);
});

it('links to a distinct historical pause and keeps the original trigger when the pause locator is invalid',async()=>{
 const pause={occurrence_id:'pause-trigger',observed_at:'2026-11-06T03:00:00Z',threshold:3};
 const withPause=(failure_pause:typeof pause)=>Response.json({...exact(),origin:{...occurrence,failure_pause}});
 reply=async()=>withPause(pause);
 await render();
 await click('toggle');
 const pauseLink=get('pause') as HTMLAnchorElement;
 expect(pauseLink.textContent).toContain('Open pause evidence');
 expect(parseWorkbenchRoute(new URL(pauseLink.href).search)).toMatchObject({project,record:'rule-a',panel:automationTriggerPanelPrefix+pause.occurrence_id});
 expect(host.textContent).toContain('This run counted toward a pause after 3 consecutive failures.');
 expect(host.querySelectorAll('time')[1]?.dateTime).toBe(pause.observed_at);
 const guard=vi.fn((event:Event)=>event.preventDefault());
 window.addEventListener('suma:navigate',guard,{once:true});
 await click('pause');
 expect(guard).toHaveBeenCalledOnce();
 expect(requests).toHaveLength(1);

 await click('toggle');
 reply=async()=>withPause({...pause,occurrence_id:occurrence.occurrence_id});
 await click('toggle');
 expect(state()).toBe('found');
 expect(get('rule')).not.toBeNull();
 expect(get('pause')).toBeNull();
});

it('aborts obsolete reads and keeps verified absence distinct from denial',async()=>{
 let complete!: (value:Response)=>void;
 reply=()=>new Promise(resolve=>{complete=resolve;});
 await render();await click('toggle');expect(state()).toBe('loading');
 await render('run-b');
 expect(requests[0].init?.signal?.aborted).toBe(true);
 await act(async()=>complete(Response.json(exact())));await settle();
 expect(host.textContent).not.toContain('Original automation');
 reply=async()=>Response.json({project_id:project,run_id:'run-b',origin:null});
 await click('toggle');expect(state()).toBe('none');expect(get('rule')).toBeNull();
 await click('toggle');
 reply=async()=>Response.json({error:{code:'forbidden',message:'Forbidden'}},{status:403});
 await click('toggle');expect(state()).toBe('denied');expect(get('rule')).toBeNull();
});
