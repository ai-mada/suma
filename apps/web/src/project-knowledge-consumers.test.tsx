// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import {KnowledgeWorkspace} from './project-views/KnowledgeWorkspace';
import {KnowledgeSearch} from './project-views/KnowledgeSearch';
import {setLocale,tr} from './i18n';
import type {KnowledgeDetail,KnowledgeRevision} from './knowledge-model';
import type {Session} from './api';

const project='project-a',session:Session={user:{id:'a'.repeat(32),name:'Owner',email:'a@example.test'},organization:{id:'b'.repeat(32),name:'Team'},projects:[{id:project,version:1,title:'Project'}]};
const raw='Original <script>never execute</script> {{raw}} 用户原文';
const revision=(number:number,content:string,state:'draft'|'published'='draft'):KnowledgeRevision=>({revision:number,based_on_revision:0,state,kind:'memory',title:'Revision '+number,summary:'summary',content,sha256:'c'.repeat(64),sources:[{kind:'documents',id:'doc-a',version:2,title:'Source 用户标题',quote:' Exact source '+raw,sha256:'d'.repeat(64)}],method:'extractive',created_by:session.user.id,created_at:'2026-10-25T00:00:00Z'});
let root:Root|undefined,host:HTMLDivElement,current:KnowledgeDetail,reads:string[],writes:{path:string;body:any;headers:Headers}[],writeReply:(path:string,body:any)=>Promise<Response>;
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});localStorage.clear();sessionStorage.clear();vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);reads=[];writes=[];
 current={id:'knowledge-a',project_id:project,version:4,kind:'memory',state:'published',title:'New draft title',summary:'New draft summary',latest_revision:4,published_revision:3,published_sha256:'c'.repeat(64),published_title:'Published original title',published_summary:'Published summary',has_draft:true,published_eligible:true,eligibility:'current',created_at:'',updated_at:'',latest:revision(4,raw),published:revision(3,'APPROVED '+raw,'published')};
 writeReply=async(_path,body)=>{current={...current,version:5,latest:{...current.latest,content:body.content??current.latest.content}};return Response.json({...current,version:1});};
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{
  const url=new URL(String(input),location.href),path=url.pathname.replace(/^\/api\/v1/,'');
  if(init?.body){const body=JSON.parse(String(init.body));if(path==='/search')return Response.json({items:[{id:'issue-a',type:'issues',title:'Issue source',snippet:raw,revision:2,locator:{project_id:project,type:'issues',id:'issue-a'}}],mode:'keyword'});writes.push({path,body,headers:new Headers(init.headers)});return writeReply(path,body);}
  reads.push(url.pathname+url.search);
  if(path==='/project-knowledge')return Response.json({items:[current],total:1,next_cursor:''});
  if(path==='/project-knowledge/knowledge-a/revisions')return Response.json({items:[revision(1,'HISTORY '+raw)],total:1,next_cursor:''});
  if(path==='/project-knowledge/knowledge-a')return Response.json(current);
  if(path==='/documents')return Response.json({items:[{id:'doc-a',version:2,title:'Source 用户标题',description:raw}],total:1,next_cursor:''});
  return Response.json({items:[],total:0});
 }));
});
afterEach(async()=>{if(root)await act(async()=>root?.unmount());root=undefined;host.remove();await setLocale('zh-CN',{persist:false});vi.unstubAllGlobals();vi.restoreAllMocks();});
async function settle(){for(let n=0;n<4;n++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function render(selected='knowledge-a',identity=session){await act(async()=>{root??=createRoot(host);root.render(<ApiViewProvider session={identity}><KnowledgeWorkspace project={project} session={identity} selected={selected} filters={{kind:'all',state:'all',q:''}} onFilter={()=>{}} onSelect={()=>{}} refresh={0} onRefresh={()=>{}}/></ApiViewProvider>);});await settle();}
async function click(text:string,scope:ParentNode=document){const button=[...scope.querySelectorAll<HTMLButtonElement>('button')].find(item=>item.textContent?.trim()===text);expect(button,'button '+text).toBeDefined();await act(async()=>button!.click());await settle();}
async function fill(field:HTMLInputElement|HTMLTextAreaElement|HTMLSelectElement,value:string){const prototype=field instanceof HTMLSelectElement?HTMLSelectElement.prototype:field instanceof HTMLTextAreaElement?HTMLTextAreaElement.prototype:HTMLInputElement.prototype;await act(async()=>{Object.getOwnPropertyDescriptor(prototype,'value')!.set!.call(field,value);field.dispatchEvent(new Event(field instanceof HTMLSelectElement?'change':'input',{bubbles:true}));});}
async function submit(){await act(async()=>document.querySelector('form.knowledge-editor')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));await settle();}

it('separates newer draft from published text and loads immutable history only on demand',async()=>{
 await render();expect(host.querySelector('.knowledge-content')?.textContent).toBe(raw);expect(reads.some(path=>path.includes('/revisions'))).toBe(false);
 await fill(host.querySelector<HTMLSelectElement>('.knowledge-detail select')!,'published');expect(host.querySelector('.knowledge-detail h2')?.textContent).toBe(current.published!.title);expect(host.querySelector('.knowledge-list-items .knowledge-entry strong')?.textContent).toBe(current.title);expect(host.querySelector('.knowledge-detail')?.textContent).toContain(tr('knowledge.version',{version:4}));expect(host.querySelector('.knowledge-content')?.textContent).toBe('APPROVED '+raw);expect(host.querySelector('script')).toBeNull();
 const history=[...host.querySelectorAll('details')].find(item=>item.querySelector('summary')?.textContent===tr('knowledge.history'))!;
 await act(async()=>{history.open=true;history.dispatchEvent(new Event('toggle'));});await settle();expect(reads.filter(path=>path.includes('/revisions'))).toHaveLength(1);
 const oldRevision=[...history.querySelectorAll<HTMLButtonElement>('button')].find(button=>button.textContent?.includes('Revision 1 · Revision 1'))!;expect(oldRevision).toBeDefined();await act(async()=>oldRevision.click());await settle();expect(history.textContent).toContain('HISTORY '+raw);
 const count=reads.length;await act(async()=>{await setLocale('zh-CN',{persist:false});});expect(reads).toHaveLength(count);expect(history.textContent).toContain(raw);expect(host.querySelector('.knowledge-source a')?.getAttribute('href')).toContain('sub=documents');
});

it('keeps an unknown edit body and operation through locale changes and reads current CAS after recovery',async()=>{
 let first=true;const original=writeReply;writeReply=async(path,body)=>{if(first){first=false;throw new TypeError('lost response');}return original(path,body);};
 await render();await click(tr('knowledge.edit'));const field=document.querySelector<HTMLTextAreaElement>('textarea[name=content]')!;await fill(field,raw+' original submitted');await submit();expect(writes).toHaveLength(1);const originalBody=structuredClone(writes[0].body),count=reads.length;
 await act(async()=>{await setLocale('zh-CN',{persist:false});});expect(reads).toHaveLength(count);expect(field.value).toBe(raw+' original submitted');expect(field.matches(':disabled')).toBe(true);
 await submit();expect(writes).toHaveLength(2);expect(writes[1].body).toEqual(originalBody);expect(writes[1].headers.get('X-Suma-Expected-User')).toBe(session.user.id);expect(originalBody.expected_version).toBe(4);expect(originalBody.idempotency_key).toBeTruthy();expect(host.querySelector('.knowledge-detail')?.textContent).toContain('记录版本 5');
});

it('extracts an explicit current source and exact quote into a draft without claiming model execution',async()=>{
 await render('');await click(tr('knowledge.extract'));await fill(document.querySelector<HTMLSelectElement>('select[name=source]')!,'doc-a');const quote=' Exact {{quoted}} 用户原文 ';await fill(document.querySelector<HTMLTextAreaElement>('textarea[name=quote]')!,quote);await submit();
 expect(writes[0].path).toBe('/project-knowledge/extractions');expect(writes[0].body).toMatchObject({project_id:project,kind:'memory',sources:[{kind:'documents',id:'doc-a',version:2,quote}],idempotency_key:expect.any(String)});expect(writes[0].body).not.toHaveProperty('content');
});

it('opens an authorized source result in the correct quality route',async()=>{
 const open=vi.fn();await act(async()=>{root=createRoot(host);root.render(<KnowledgeSearch project={project} onOpenSource={open}/>);});await fill(host.querySelector('input')!,'source');await act(async()=>host.querySelector('form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));await settle();
 const link=host.querySelector<HTMLAnchorElement>('.knowledge-result-actions a')!;expect(link.getAttribute('href')).toContain('view=quality');expect(link.getAttribute('href')).toContain('sub=issues');await act(async()=>link.click());expect(open).toHaveBeenCalledWith(expect.objectContaining({id:'issue-a'}));
});

it('recovers an unknown create after reload even before any detail record is selected',async()=>{
 let first=true;const original=writeReply;writeReply=async(path,body)=>{if(first){first=false;throw new TypeError('lost create receipt');}return original(path,body);};
 await render('');await click(tr('knowledge.create'));await fill(document.querySelector<HTMLInputElement>('input[name=title]')!,'Original created knowledge');await fill(document.querySelector<HTMLTextAreaElement>('textarea[name=content]')!,raw);await submit();const submitted=structuredClone(writes[0].body);await act(async()=>root!.unmount());root=undefined;
 await render('');expect(writes).toHaveLength(1);expect(host.querySelector('.knowledge-recovery')?.textContent).toContain('Original created knowledge');await click(tr('knowledge.restore'),host);expect(writes).toHaveLength(2);expect(writes[1].body).toEqual(submitted);expect(host.querySelector('.knowledge-recovery')).toBeNull();
});

it('rebases a first definite CAS conflict explicitly while retaining the edited content and sources',async()=>{
 let first=true;const original=writeReply;writeReply=async(path,body)=>{if(first){first=false;current={...current,version:7,latest_revision:7,latest:revision(7,'Concurrent server content')};return Response.json({error:{code:'version_conflict',message:'Version changed'}},{status:409});}return original(path,body);};
 await render();await click(tr('knowledge.edit'));const content=document.querySelector<HTMLTextAreaElement>('textarea[name=content]')!;await fill(content,raw+' retained draft');await submit();const firstBody=structuredClone(writes[0].body);expect(writes).toHaveLength(1);expect(content.matches(':disabled')).toBe(false);
 await click(tr('preferences.write.rebase'));expect(content.value).toBe(raw+' retained draft');expect(document.querySelector('.knowledge-editor')?.textContent).toContain('Concurrent server content');await submit();expect(writes[1].body).toMatchObject({content:raw+' retained draft',expected_version:7,sources:firstBody.sources});expect(writes[1].body.idempotency_key).not.toBe(firstBody.idempotency_key);
});
