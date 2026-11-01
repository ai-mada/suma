// @vitest-environment jsdom
import {act} from 'react';
import {createRoot, type Root} from 'react-dom/client';
import {afterEach, beforeEach, expect, it, vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import type {Session} from './api';
import {TaskConversationKnowledge, RecordedConversationKnowledge} from './TaskConversationKnowledge';
import {setLocale} from './i18n';

const pin={id:'knowledge-a',revision:2,sha256:'a'.repeat(64)};
const entry={...pin,project_id:'p',version:4,kind:'spec',state:'published',title:'New draft title',summary:'',latest_revision:3,published_revision:2,published_sha256:pin.sha256,published_title:'Approved source',published_summary:'',has_draft:true,published_eligible:true,eligibility:'current',created_at:'2026-10-30T00:00:00Z',updated_at:'2026-10-30T00:00:00Z'};
const snapshot={revision:2,based_on_revision:1,state:'published',kind:'spec',title:'Approved source',summary:'',content:'Exact historical body',sha256:pin.sha256,sources:[],method:'manual',created_by:'u',created_at:'2026-10-30T00:00:00Z'};
let host:HTMLDivElement,root:Root,current:Session,selected:typeof pin[],reads:string[],denied:ReturnType<typeof vi.fn>,valid:ReturnType<typeof vi.fn>;
beforeEach(async()=>{vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);await setLocale('en-US',{persist:false});host=document.createElement('div');document.body.append(host);root=createRoot(host);selected=[];reads=[];denied=vi.fn();valid=vi.fn();current={user:{id:'u',name:'User',email:'u@example.test'},organization:{id:'o',name:'Org'},projects:[]};});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();await setLocale('zh-CN',{persist:false});});
async function settle(){for(let i=0;i<7;i++)await act(async()=>{await Promise.resolve();await Promise.resolve();});}
async function render(refresh=0){await act(async()=>root.render(<ApiViewProvider session={current}><TaskConversationKnowledge project="p" taskID="t" userID={current.user.id} organizationID="o" selected={selected} onChange={pins=>{selected=pins;}} disabled={false} accessBlocked={false} refresh={refresh} onValidityChange={valid} onAccessDenied={denied}/></ApiViewProvider>));await settle();}
async function click(selector:string){const node=document.querySelector<HTMLElement>(selector);expect(node,selector).not.toBeNull();await act(async()=>node!.click());await settle();}
function transport(reply?:(path:string)=>Response|Promise<Response>){vi.stubGlobal('fetch',vi.fn(async(input:string)=>{const path=String(input).replace(/^\/api\/v1/,'');reads.push(path);return reply?reply(path):Response.json(path.startsWith('/project-knowledge?')?{items:[entry],total:1,next_cursor:''}:entry);}));}

it('reads only on expansion, chooses the publication rather than a newer draft, and keeps an unavailable pin',async()=>{
  let eligible=true;transport(path=>Response.json(path.startsWith('/project-knowledge?')?{items:[entry],total:1,next_cursor:''}:{...entry,published_eligible:eligible,eligibility:eligible?'current':'retired'}));
  await render();expect(reads).toEqual([]);await click('[data-knowledge-toggle]');await click('[data-knowledge-option=knowledge-a]');await render();
  expect(selected).toEqual([pin]);expect(host.textContent).toContain('Approved source');expect(host.textContent).not.toContain('New draft title');
  eligible=false;await render(1);expect(selected).toEqual([pin]);expect(host.querySelector('[data-knowledge-selected]')?.textContent).toContain('unavailable');
  expect(valid.mock.calls.at(-1)?.[1]).toBe(false);await click('[data-knowledge-remove]');expect(selected).toEqual([]);
});
it('does not silently replace a selected revision and clears current private names on a current API denial',async()=>{
  selected=[{...pin,revision:1,sha256:'b'.repeat(64)}];let refused=false;
  transport(path=>refused?Response.json({error:'Forbidden'},{status:403}):Response.json(path.startsWith('/project-knowledge?')?{items:[entry],total:1,next_cursor:''}:entry));
  await render();await click('[data-knowledge-toggle]');await click('[data-knowledge-option=knowledge-a]');
  expect(selected[0].revision).toBe(1);expect(host.textContent).toContain('Remove the selected revision');
  refused=true;await render(1);expect(denied).toHaveBeenCalled();expect(host.textContent).not.toContain('Approved source');expect(selected[0].revision).toBe(1);
});
it('opens the exact historical revision on demand and refuses a mismatched hash',async()=>{
  let corrupt=false;transport(()=>Response.json({project_id:'p',knowledge_id:pin.id,snapshot:{...snapshot,sha256:corrupt?'b'.repeat(64):pin.sha256}}));
  const renderFacts=async()=>{await act(async()=>root.render(<ApiViewProvider session={current}><RecordedConversationKnowledge project="p" items={[{...pin,title_at_post:'Original title',kind_at_post:'spec'}]} refresh={0} onAccessDenied={denied}/></ApiViewProvider>));await settle();};
  await renderFacts();expect(reads).toEqual([]);await click('[data-knowledge-history]');expect(reads[0]).toContain('/knowledge-a/revisions/2?project_id=p');expect(document.body.textContent).toContain(snapshot.content);
  expect(document.querySelectorAll('[role=dialog] .knowledge-hash')).toHaveLength(1);
  expect(document.querySelector('[role=dialog]')?.textContent).toContain(pin.id);
  await click('[role=dialog] .dialog-heading .icon-button');corrupt=true;await click('[data-knowledge-history]');expect(document.body.textContent).not.toContain(snapshot.content);expect(document.body.textContent).toContain('could not be confirmed');
  expect(document.querySelector('[role=dialog]')?.textContent).toContain(pin.id);
  expect(document.querySelector('[role=dialog] .knowledge-hash')?.textContent).toBe(pin.sha256);
});
it('ignores an old identity denial and hides the exact-history content on a current denial',async()=>{
  let finish:(value:Response)=>void=()=>{};
  selected=[pin];transport(()=>new Promise<Response>(resolve=>{finish=resolve;}));
  await render();const old=finish;
  current={...current,user:{...current.user,id:'other'}};selected=[];await render();
  await act(async()=>old(Response.json({error:'Forbidden'},{status:403})));await settle();expect(denied).not.toHaveBeenCalled();
  transport(()=>Response.json({error:'Forbidden'},{status:403}));
  await act(async()=>root.render(<ApiViewProvider session={current}><RecordedConversationKnowledge project="p" items={[{...pin,title_at_post:'Protected original title',kind_at_post:'spec'}]} refresh={0} onAccessDenied={denied}/></ApiViewProvider>));await settle();
  await click('[data-knowledge-history]');expect(denied).toHaveBeenCalledTimes(1);expect(document.body.textContent).not.toContain('Protected original title');expect(document.querySelector('[role=dialog]')).toBeNull();
});
