// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import type {Session} from './api';
import {setLocale,tr} from './i18n';
import type {KnowledgeDetail,KnowledgeSource} from './knowledge-model';
import {KnowledgeWorkspace} from './project-views/KnowledgeWorkspace';

vi.mock('./KnowledgeIntelligence',()=>({KnowledgeIntelligence:()=>null}));

const project='project-a',taskID='task-a',knowledgeID='knowledge-a',stamp='2026-10-31T00:00:00Z';
const session:Session={user:{id:'a'.repeat(32),name:'Owner',email:'owner@example.test'},organization:{id:'b'.repeat(32),name:'Team'},projects:[]};
const oldBasis='c'.repeat(64),newBasis='d'.repeat(64),sourceHash='e'.repeat(64),lesson='Keep business acceptance separate from execution success.';
function acceptedSource(version:number,basis:string,quote:string):KnowledgeSource{return {kind:'accepted_delivery',id:taskID,version,title:'Accepted delivery lesson',quote,sha256:sourceHash,delivery:{protocol:'suma-accepted-delivery-v1',task:{id:taskID,version,run_id:'delivery-run',review_id:'review-'+version,sha256:'f'.repeat(64)},run_version:8,tests:[{run_id:'delivery-run',node_id:'verify',attempt:1,fence:2,worker_id:'test-worker',result_sha256:'f'.repeat(64)}],candidates:[],sha256:basis}};}
let root:Root|undefined,host:HTMLDivElement,current:KnowledgeDetail,reads:string[],writes:{path:string;method:string;body:Record<string,unknown>}[];
const fresh=acceptedSource(5,newBasis,'  Current accepted delivery\nKeep this exact source text.  ');
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});sessionStorage.clear();vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);reads=[];writes=[];
 current={id:knowledgeID,project_id:project,version:7,kind:'memory',state:'draft',title:'Accepted lesson',summary:'Reviewed proposal',latest_revision:1,published_revision:0,published_sha256:'',published_title:'',published_summary:'',has_draft:true,published_eligible:false,eligibility:'current',created_at:stamp,updated_at:stamp,latest:{revision:1,based_on_revision:0,state:'draft',kind:'memory',title:'Accepted lesson',summary:'Reviewed proposal',content:lesson,sha256:sourceHash,sources:[acceptedSource(4,oldBasis,'Original accepted delivery')],method:'model_assisted',created_by:session.user.id,created_at:stamp},model_adoption:{analysis_id:'analysis-a',analysis_version:3,run_id:'analysis-run',tool_profile_sha256:sourceHash,source_sha256:sourceHash,original_output_sha256:sourceHash,source_indices:[0],project_id:project,knowledge_id:knowledgeID,revision:1,sha256:sourceHash,adopted_by:session.user.id,adopted_at:stamp,reason:'Reviewed model proposal'}};
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{
  const url=new URL(String(input),location.href),path=url.pathname.replace(/^\/api\/v1/,'');
  if(init?.body){writes.push({path,method:init.method||'POST',body:JSON.parse(String(init.body))});current={...current,version:8,latest:{...current.latest,revision:2,sources:[fresh]}};return Response.json(current);}
  reads.push(path+url.search);
  if(path==='/project-knowledge')return Response.json({items:[current],total:1,next_cursor:''});
  if(path==='/project-knowledge/'+knowledgeID)return Response.json(current);
  if(path==='/documents')return Response.json({items:[],total:0,next_cursor:''});
  if(path==='/tasks/'+taskID+'/delivery-knowledge-source')return Response.json({project_id:project,source:fresh});
  return Response.json({error:{code:'not_found',message:'No record route for accepted delivery'}},{status:404});
 }));
});
afterEach(async()=>{if(root)await act(async()=>root!.unmount());root=undefined;host.remove();await setLocale('zh-CN',{persist:false});vi.unstubAllGlobals();vi.restoreAllMocks();});
async function settle(){for(let count=0;count<4;count++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function click(text:string,scope:ParentNode=document){const button=Array.from(scope.querySelectorAll<HTMLButtonElement>('button')).find(item=>item.textContent?.trim()===text);expect(button,text).toBeDefined();expect(button!.disabled).toBe(false);await act(async()=>button!.click());await settle();}

it('rechecks an accepted delivery draft through the formal source and saves only the explicitly adopted basis',async()=>{
 await act(async()=>{root=createRoot(host);root.render(<ApiViewProvider session={session}><KnowledgeWorkspace project={project} session={session} selected={knowledgeID} filters={{kind:'all',state:'all',q:''}} onFilter={()=>{}} onSelect={()=>{}} refresh={0} onRefresh={()=>{}}/></ApiViewProvider>);});await settle();
 await click(tr('knowledge.edit'));const form=document.querySelector<HTMLFormElement>('form.knowledge-editor')!;expect(form).not.toBeNull();const sourceList=form.querySelector('details')!;await act(async()=>{sourceList.open=true;sourceList.dispatchEvent(new Event('toggle'));});
 await click(tr('knowledge.source_read_current'),form);expect(reads).toContain('/tasks/'+taskID+'/delivery-knowledge-source?project_id='+project);expect(reads.some(path=>path.startsWith('/accepted_delivery/'))).toBe(false);
 expect(sourceList.textContent).toContain(tr('knowledge.source_version',{version:'4'}));expect(sourceList.textContent).toContain(fresh.quote);expect(writes).toHaveLength(0);
 const content=form.querySelector<HTMLTextAreaElement>('textarea[name=content]')!;expect(content.value).toBe(lesson);
 await click(tr('knowledge.source_adopt',{version:'5'}),form);expect(sourceList.textContent).toContain(tr('knowledge.source_version',{version:'5'}));expect(content.value).toBe(lesson);expect(writes).toHaveLength(0);
 await act(async()=>form.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));await settle();
 expect(writes).toHaveLength(1);expect(writes[0]).toMatchObject({path:'/project-knowledge/'+knowledgeID,method:'PATCH',body:{project_id:project,expected_version:7,kind:'memory',content:lesson,sources:[{kind:'accepted_delivery',id:taskID,version:5,quote:fresh.quote,basis_sha256:newBasis}]}});
 expect(writes[0].body.sources).toEqual([{kind:'accepted_delivery',id:taskID,version:5,quote:fresh.quote,basis_sha256:newBasis}]);expect(writes[0].body.idempotency_key).toEqual(expect.any(String));expect(writes.some(write=>write.path.endsWith('/commands'))).toBe(false);expect(current.state).toBe('draft');expect(current.published_revision).toBe(0);
});
