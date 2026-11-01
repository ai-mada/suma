// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import type {Session} from './api';
import {ProjectKnowledgePicker} from './ProjectKnowledgePicker';
import {setLocale} from './i18n';
import {knowledgeSelectionKey} from './task-conversation-knowledge-model';
import type {KnowledgeReference} from './knowledge-model';

const pin:KnowledgeReference={id:'knowledge-a',revision:2,sha256:'a'.repeat(64)};
const summary=(reference=pin,project='p')=>({id:reference.id,project_id:project,version:reference.revision+2,kind:'spec',state:'draft',title:'Unpublished draft title',summary:'Private draft summary',latest_revision:reference.revision+1,published_revision:reference.revision,published_sha256:reference.sha256,published_title:'Approved source '+reference.id,published_summary:'Published summary',has_draft:true,published_eligible:true,eligibility:'current',created_at:'2026-10-30T00:00:00Z',updated_at:'2026-10-30T00:00:00Z'});
const revision=(reference=pin)=>({revision:reference.revision,based_on_revision:1,state:'published',kind:'spec',title:'Approved source '+reference.id,summary:'Published summary',content:'Exact published body '+reference.id,sources:[],sha256:reference.sha256,method:'manual',created_by:'u',created_at:'2026-10-30T00:00:00Z'});
const hit=(reference=pin,project='p')=>({reference,project_id:project,kind:'spec',title:'Approved source '+reference.id,summary:'Published summary',snippet:'Exact published body',matched_field:'none',source_count:0,published_at:'2026-10-30T00:00:00Z'});
const page=(items:ReturnType<typeof hit>[]=[])=>({items,next_cursor:'',mode:'keyword',scope:'current_project_publications',scanned:items.length,scan_limit:200,exhausted:true});
let root:Root,host:HTMLDivElement,session:Session,selected:KnowledgeReference[],reads:string[],valid:ReturnType<typeof vi.fn>,denied:ReturnType<typeof vi.fn>;
type Props=Parameters<typeof ProjectKnowledgePicker>[0];
async function settle(){await act(async()=>{for(let i=0;i<24;i++)await Promise.resolve();});}
async function render(props:Partial<Props>={}){await act(async()=>root.render(<ApiViewProvider session={session}><ProjectKnowledgePicker project="p" taskID="task-a" userID={session.user.id} organizationID={session.organization.id} selected={selected} onChange={pins=>{selected=pins;}} disabled={false} accessBlocked={false} refresh={0} onValidityChange={valid} onAccessDenied={denied} {...props}/></ApiViewProvider>));await settle();}
async function click(selector:string){const button=document.querySelector<HTMLButtonElement>(selector);expect(button,selector).not.toBeNull();await act(async()=>button!.click());await settle();}
function transport(reply:(path:string)=>Promise<Response>|Response){vi.stubGlobal('fetch',vi.fn(async(input:string,init:RequestInit)=>{expect(init.method).toBe('GET');expect(init.body).toBeUndefined();const path=String(input).replace(/^\/api\/v1/,'');reads.push(path);return reply(path);}));}
beforeEach(async()=>{await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);root=createRoot(host);session={user:{id:'u',name:'Owner',email:'owner@example.test'},organization:{id:'o',name:'Org'},projects:[]};selected=[];reads=[];valid=vi.fn();denied=vi.fn();});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();});

it('T196 validates selected pins independently of discovery pages, preserves them through refusal and ignores late old-scope results',async()=>{
 selected=[pin];let status=200,eligible=true;
 transport(path=>Response.json(status!==200?{error:{code:'forbidden',message:'Denied'}}:path.startsWith('/project-knowledge/discovery?')?page():{...summary(),published_eligible:eligible,eligibility:eligible?'current':'source_changed'},{status}));
 await render();expect(reads).toEqual(['/project-knowledge/knowledge-a?project_id=p']);expect(valid).toHaveBeenLastCalledWith(knowledgeSelectionKey(selected,0),true);expect(host.textContent).toContain('Approved source');expect(host.textContent).not.toContain('Unpublished draft');
 await click('[data-knowledge-toggle]');expect(reads.some(path=>path.startsWith('/project-knowledge/discovery?'))).toBe(true);expect(host.querySelector('[data-knowledge-selected="knowledge-a"]')).not.toBeNull();expect(valid).toHaveBeenLastCalledWith(knowledgeSelectionKey(selected,0),true);
 eligible=false;await render({refresh:1});expect(valid).toHaveBeenLastCalledWith(knowledgeSelectionKey(selected,1),false);expect(selected).toEqual([pin]);expect(host.textContent).toContain('unavailable');
 status=403;await render({refresh:2});expect(denied).toHaveBeenCalledOnce();expect(host.textContent).not.toContain('Approved source');expect(selected).toEqual([pin]);
 status=200;eligible=true;await render({refresh:3});expect(valid).toHaveBeenLastCalledWith(knowledgeSelectionKey(selected,3),true);expect(host.textContent).toContain('Approved source');
 let oldResolve!:(value:Response)=>void;transport(()=>new Promise<Response>(resolve=>{oldResolve=resolve;}));await render({refresh:4});expect(valid).toHaveBeenLastCalledWith(knowledgeSelectionKey(selected,4),false);
 const old=oldResolve;session={...session,user:{...session.user,id:'other'}};selected=[];await render({project:'q',taskID:'task-q',refresh:5});expect(valid).toHaveBeenLastCalledWith(knowledgeSelectionKey([],5),true);
 await act(async()=>old(Response.json({error:{code:'forbidden',message:'Old account denied'}},{status:403})));await settle();expect(denied).toHaveBeenCalledOnce();expect(host.textContent).not.toContain('Approved source');
});

it('T196 previews an exact publication before explicit selection and never upgrades a conflicting pin or exceeds sixteen references',async()=>{
 let candidate=pin;
 transport(path=>{
  if(path.startsWith('/project-knowledge/discovery?'))return Response.json(page([hit(candidate)]));
  const match=path.match(/^\/project-knowledge\/([^/?]+)(?:\/revisions\/(\d+))?\?/);if(!match)throw Error('Unexpected read '+path);
  const ref=match[1]===candidate.id?candidate:{id:match[1],revision:2,sha256:'a'.repeat(64)};
  return Response.json(match[2]?{project_id:'p',knowledge_id:ref.id,snapshot:revision(ref)}:summary(ref));
 });
 await render();expect(reads).toEqual([]);expect(valid).toHaveBeenLastCalledWith(knowledgeSelectionKey([],0),true);
 await click('[data-knowledge-toggle]');await click('[data-knowledge-discovery-preview="knowledge-a"]');expect(selected).toEqual([]);expect(host.textContent).toContain('Exact published body');expect(host.textContent).not.toContain('Unpublished draft');
 await click('[data-knowledge-preview-choose]');expect(selected).toEqual([pin]);await render();expect(valid).toHaveBeenLastCalledWith(knowledgeSelectionKey(selected,0),true);expect(Object.keys(selected[0]).sort()).toEqual(['id','revision','sha256']);
 candidate={...pin,revision:4,sha256:'b'.repeat(64)};await render({refresh:1});await click('[data-knowledge-discovery-preview="knowledge-a"]');await click('[data-knowledge-preview-choose]');expect(selected).toEqual([pin]);expect(host.textContent).toContain('Remove the selected revision');
 selected=Array.from({length:16},(_,index)=>({id:'selected-'+index,revision:2,sha256:'a'.repeat(64)}));candidate={id:'knowledge-extra',revision:2,sha256:'a'.repeat(64)};await render({refresh:2});await click('[data-knowledge-discovery-preview="knowledge-extra"]');
 const choose=host.querySelector<HTMLButtonElement>('[data-knowledge-preview-choose]');expect(!choose||choose.disabled).toBe(true);expect(selected).toHaveLength(16);expect(host.textContent).toContain('16');
});
