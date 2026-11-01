// @vitest-environment jsdom
import {act,type ReactNode} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {KnowledgeSearch} from './project-views/KnowledgeSearch';
import {TaskDispatchDialog} from './TaskDispatch';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import type {Session,RecordItem,Workflow} from './api';

const session:Session={user:{id:'user-a',name:'Owner',email:'owner@example.test',role:'owner'},organization:{id:'org-a',name:'Organization'},projects:[]},project='project-a';
const pin={id:'publication-a',revision:3,sha256:'d'.repeat(64)},content='规范🙂起点😀目标e\u0301结束',characters=Array.from(content),start=characters.indexOf('😀'),end=start+5;
const locator={unit:'unicode_codepoints',start,end},snippet=characters.slice(start,end).join('');
const hit={reference:pin,project_id:project,kind:'spec',title:'Published quality specification',summary:'Reviewed quality instructions',snippet,source_count:0,published_at:'2026-11-02T00:00:00Z',source:{kind:'project_knowledge',id:pin.id,sequence:3,revision:pin.revision,sha256:pin.sha256},locator};
const snapshot={revision:3,based_on_revision:2,state:'published',kind:'spec',title:hit.title,summary:hit.summary,content,sha256:pin.sha256,sources:[],method:'manual',created_by:'user-a',created_at:hit.published_at};
const detail={id:pin.id,project_id:project,version:4,kind:'spec',state:'draft',title:'NEW UNREVIEWED DRAFT',summary:'New draft summary',latest_revision:4,published_revision:3,published_sha256:pin.sha256,published_title:hit.title,published_summary:hit.summary,has_draft:true,published_eligible:true,eligibility:'current',created_at:hit.published_at,updated_at:hit.published_at,latest:{...snapshot,revision:4,based_on_revision:3,state:'draft',content:'UNREVIEWED INDEX DRAFT BODY',title:'NEW UNREVIEWED DRAFT',sha256:'e'.repeat(64)},published:snapshot};
const task={id:'task-a',project_id:project,title:'Develop using reviewed specifications',version:4,description:'Use the exact publication'} as RecordItem;
const flow:Workflow={id:'flow-a',name:'Original workflow',version:3,nodes:[{id:'develop',name:'Develop',type:'agent'},{id:'independent-test',name:'Independent test',type:'test',command:['junit-test'],config:{candidate_from:'develop'}}],edges:[]};
const profile={id:'profile-a',project_id:project,worker_id:'worker-a',name:'Original tool',version:5,state:'enabled',available:true,profile:{command_id:'command-a',adapter:'codex',protocol:'codex-jsonl-v1',profile_sha256:'a'.repeat(64),executable_sha256:'b'.repeat(64),tool_version:'1.0',read_only:false}};
let root:Root|undefined,host:HTMLDivElement,requests:{path:string;method:string;body?:Record<string,unknown>}[],indexAvailable:boolean;
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});sessionStorage.clear();requests=[];indexAvailable=true;vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);
 vi.stubGlobal('fetch',vi.fn(async(input:string,init:RequestInit={})=>{
  const url=new URL(String(input),location.href),path=url.pathname.replace(/^\/api\/v1/,''),method=init.method||'GET',body=init.body?JSON.parse(String(init.body)):undefined;requests.push({path:path+url.search,method,...(body?{body}:{})});
  if(method!=='GET')return Response.json({error:{code:'fixture_preview_boundary',message:'Synthetic consumer verifies the exact original preview request only.'}},{status:409});
  if(path==='/workflows')return Response.json({items:[flow],total:1});
  if(path==='/tool-profiles')return Response.json({items:[profile],total:1});
  if(path==='/project-knowledge/discovery')return Response.json({items:[],next_cursor:'',mode:'keyword',scope:'current_project_publications',scanned:0,scan_limit:200,exhausted:true});
  if(path==='/project-knowledge/index-search')return indexAvailable?Response.json({items:[hit],mode:'lancedb_hybrid',generation:'generation-197',index_watermark:7,candidates_examined:2,discarded:1,scope:'provider_candidates'}):Response.json({error:{code:'index_unavailable',message:'The configured index is unavailable'}},{status:503});
  if(path==='/project-knowledge/index-status')return Response.json({domain:'project_knowledge',as_of:'2026-11-02T00:00:20Z',configured:true,coverage:'outbox_delivery_only',pending:1,indexed:1,failed:0,total:2,current_publications:1,untracked_publications:0,latest_event:null,last_ack:null});
  if(path==='/project-knowledge/'+pin.id)return Response.json(detail);
  if(path==='/project-knowledge/'+pin.id+'/revisions/3')return Response.json({project_id:project,knowledge_id:pin.id,snapshot});
  throw Error('Unexpected index consumer route '+path);
 }));
});
afterEach(async()=>{if(root)await act(async()=>root!.unmount());root=undefined;host.remove();document.body.innerHTML='';vi.restoreAllMocks();vi.unstubAllGlobals();});
async function settle(){await act(async()=>{for(let i=0;i<44;i++)await Promise.resolve();});}
async function render(node:ReactNode){await act(async()=>{root??=createRoot(host);root.render(node);});await settle();}
async function click(selector:string){const button=document.querySelector<HTMLButtonElement>(selector);expect(button,selector).not.toBeNull();await act(async()=>button!.click());await settle();}
async function change(element:HTMLInputElement|HTMLSelectElement,value:string){expect(element).not.toBeNull();await act(async()=>{Object.getOwnPropertyDescriptor(element instanceof HTMLSelectElement?HTMLSelectElement.prototype:HTMLInputElement.prototype,'value')!.set!.call(element,value);element.dispatchEvent(new Event(element instanceof HTMLSelectElement?'change':'input',{bubbles:true}));});await settle();}
async function searchIndex(){await change(document.querySelector<HTMLSelectElement>('[data-knowledge-discovery-mode]')!,'index');await change(document.querySelector<HTMLInputElement>('[data-knowledge-discovery-query]')!,'目标');await click('[data-knowledge-discovery-search]');}

it('T197 carries the indexed publication location into the exact read-only search preview without adopting provider state or newer drafts',async()=>{
 await render(<ApiViewProvider session={session}><KnowledgeSearch project={project}/></ApiViewProvider>);await click('[data-knowledge-search-mode="published"]');
 await searchIndex();expect(requests.filter(row=>row.path.includes('/index-search'))).toHaveLength(1);expect(requests.some(row=>row.path.startsWith('/project-knowledge/discovery?')&&new URLSearchParams(row.path.split('?')[1]).get('q')==='目标')).toBe(false);
 await click('[data-knowledge-discovery-preview="publication-a"]');expect(document.querySelector('[data-knowledge-preview-match]')?.textContent).toBe(snippet);expect(document.querySelector('[data-knowledge-preview-reference]')?.textContent).toContain(pin.sha256);expect(document.body.textContent).not.toContain(detail.latest.content);expect(document.body.textContent).not.toContain(detail.latest.title);expect(document.querySelector('[data-knowledge-preview-choose]')).toBeNull();
 await click('[data-knowledge-preview-locate]');expect(document.activeElement?.hasAttribute('data-knowledge-preview-match')).toBe(true);
 await click('[data-knowledge-preview-close]');await act(async()=>{await new Promise<void>(resolve=>requestAnimationFrame(()=>resolve()));});expect(document.activeElement?.getAttribute('data-knowledge-discovery-preview')).toBe(pin.id);expect(requests.every(row=>row.method==='GET')).toBe(true);
});

it('T197 keeps an indexed location out of original task references and preserves an explicit selection through provider failure',async()=>{
 await render(<ApiViewProvider session={session}><TaskDispatchDialog project={project} session={session} task={task} planID="new" initialWorkflow={flow} onPlan={()=>{}} onClose={()=>{}} onRun={()=>{}}/></ApiViewProvider>);
 await change(document.querySelector<HTMLSelectElement>('[data-dispatch-manual-tool]')!,profile.id);await click('[data-knowledge-toggle]');await searchIndex();await click('[data-knowledge-discovery-preview="publication-a"]');
 expect(document.querySelector('[data-knowledge-preview-match]')?.textContent).toBe(snippet);expect(document.querySelector('[data-knowledge-selected="publication-a"]')).toBeNull();expect(requests.filter(row=>row.method!=='GET')).toEqual([]);
 await click('[data-knowledge-preview-choose]');expect(document.querySelector('[data-knowledge-selected="publication-a"]')).not.toBeNull();
 indexAvailable=false;const keywordReads=requests.filter(row=>row.path.startsWith('/project-knowledge/discovery?')).length;await click('[data-knowledge-discovery-refresh]');expect(document.querySelector('[data-knowledge-selected="publication-a"]')).not.toBeNull();expect(requests.filter(row=>row.path.startsWith('/project-knowledge/discovery?'))).toHaveLength(keywordReads);
 const submit=document.querySelector<HTMLButtonElement>('.tool-dispatch .dialog-actions button.primary')!;expect(submit.disabled).toBe(false);await act(async()=>document.querySelector<HTMLFormElement>('.tool-dispatch form.tool-form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));await settle();
 const writes=requests.filter(row=>row.method!=='GET');expect(writes).toHaveLength(1);expect(writes[0].path).toBe('/task-dispatch-plans');expect(writes[0].body?.knowledge).toEqual([pin]);expect(Object.keys((writes[0].body!.knowledge as Record<string,unknown>[])[0]).sort()).toEqual(['id','revision','sha256']);expect(writes.some(row=>row.path.endsWith('/start'))).toBe(false);
});
