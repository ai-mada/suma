// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {beforeEach,afterEach,expect,it,vi} from 'vitest';
import {ApiViewProvider,bindApiView,useApi} from './api-context';
import {api,ApiError,rawApi,type ApiCaller,type Session} from './api';
import {mutate} from './operations';
import {ToolWriteRecovery} from './tool-write';
import {TokenCreation} from './token-requests';

const session=(id:string):Session=>({user:{id:'user-'+id,name:id,email:id+'@example.test'},organization:{id:'org-'+id,name:id},projects:[]});
let root:Root|undefined,host:HTMLDivElement,current:ApiCaller;
let sent:{path:string;body:any;headers:Headers}[],respond:(path:string,init:RequestInit)=>Promise<Response>;
function Capture(){current=useApi();return null;}
beforeEach(()=>{
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);sent=[];
 respond=async()=>Response.json({ok:true});
 vi.stubGlobal('fetch',vi.fn(async(path:string,init:RequestInit)=>{sent.push({path,body:init.body?JSON.parse(String(init.body)):undefined,headers:new Headers(init.headers)});return respond(path,init);}));
});
afterEach(async()=>{if(root)await act(async()=>root?.unmount());root=undefined;host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();});
async function render(value:Session|null,provided=true){await act(async()=>{root??=createRoot(host);root.render(provided?<ApiViewProvider session={value}><Capture/></ApiViewProvider>:<Capture/>);});}
const ids=(row:typeof sent[number])=>[row.headers.get('X-Suma-Expected-User'),row.headers.get('X-Suma-Expected-Organization')];

it('captures exact primitive IDs without changing body, idempotency, method or additional headers',async()=>{
 const original=session('A'),call=bindApiView(original),body={project_id:'body-project',user_id:'body-user',organization_id:'body-org',expected_version:7,idempotency_key:'original-key',raw:'用户 {{raw}}'};
 original.user.id='mutated-later';original.organization.id='mutated-later';document.cookie='session=unrelated-cookie';
 await call('/tasks',body,'PATCH',{'X-Test':'original'});
 expect(ids(sent[0])).toEqual(['user-A','org-A']);expect(sent[0].body).toEqual(body);expect(sent[0].headers.get('X-Test')).toBe('original');
 expect(vi.mocked(fetch).mock.calls[0][1]?.method).toBe('PATCH');expect(sent.map(s=>s.path)).toEqual(['/api/v1/tasks']);
});
it('keeps an awaited A closure on A after Provider switches to B; only a new B command carries B',async()=>{
 let release!:()=>void;const gate=new Promise<void>(resolve=>{release=resolve;});
 await render(session('A'));const original=current,oldCommand=async()=>{await gate;return original('/teams',{name:'original A'});};const pending=oldCommand();
 await render(session('B'));respond=async(_path,init)=>new Headers(init.headers).get('X-Suma-Expected-Organization')==='org-B'?Response.json({ok:true}):Response.json({error:{code:'view_scope_changed',message:'original refusal'}},{status:403});
 release();await expect(pending).rejects.toMatchObject({status:403,code:'view_scope_changed'});await current('/teams',{name:'B'});
 expect(sent.map(ids)).toEqual([['user-A','org-A'],['user-B','org-B']]);
});
it('preserves complete explicit original scope, refuses conflicting partial or duplicate headers',async()=>{
 const call=bindApiView(session('B'));
 await call('/organization/preferences',{locale:'en-US'},'PATCH',{'x-suma-expected-user':'user-A','x-suma-expected-organization':'org-A'});
 expect(ids(sent[0])).toEqual(['user-A','org-A']);
 await expect(call('/teams',{name:'wrong'},'POST',{'X-Suma-Expected-User':'user-A'})).rejects.toMatchObject({code:'view_context_conflict',status:403});
 await expect(call('/teams',{},'POST',{'X-Suma-Expected-User':'user-A','x-suma-expected-user':'user-B'})).rejects.toMatchObject({code:'view_context_conflict'});
 expect(sent).toHaveLength(1);
});
it('keeps global account preference scope user-only and never overwrites its explicit original user',async()=>{
 const call=bindApiView(session('B'));
 await call('/account/preferences',{locale:'en-US'},'PATCH',{'X-Suma-Expected-User':'user-A'});
 await call('/account/preferences',{locale:'zh-CN'},'PATCH');
 expect(sent.map(ids)).toEqual([['user-A',null],['user-B',null]]);
 await expect(call('/account/preferences',{},'PATCH',{'X-Suma-Expected-Organization':'org-A'})).rejects.toMatchObject({status:403});
});
it('does not invent view headers for setup, login, invitation acceptance or logout',async()=>{
 const call=bindApiView(session('A'));
 for(const path of ['/auth/setup','/auth/login','/auth/invitations/accept','/auth/logout'])await call(path,{idempotency_key:'original'},'POST',{'X-Suma-Setup-Token':'setup-only'});
 expect(sent.map(ids)).toEqual([[null,null],[null,null],[null,null],[null,null]]);
 expect(sent[0].headers.get('X-Suma-Setup-Token')).toBe('setup-only');
});
it('does not guess a view from body or cookie outside a Provider; required-view refusal stays visible',async()=>{
 await render(null,false);document.cookie='session=cookie-B';respond=async()=>Response.json({error:{code:'view_context_required',message:'required view'}},{status:428});
 await expect(current('/tasks',{user_id:'body-user',organization_id:'body-org'})).rejects.toMatchObject({status:428,code:'view_context_required'});
 expect(ids(sent[0])).toEqual([null,null]);expect(sent).toHaveLength(1);expect(sent[0].path).toBe('/api/v1/tasks');
});
it('preserves aborts, structured errors and raw evidence normalization boundaries',async()=>{
 const call=bindApiView(session('A')),controller=new AbortController();controller.abort();
 await expect(call('/tasks',{},'POST',undefined,controller.signal)).rejects.toMatchObject({name:'AbortError'});expect(sent).toHaveLength(0);
 respond=async()=>Response.json({error:{code:'original_code',message:'原文拒绝',details:{original:'保持'}}},{status:403});
 await expect(call('/tasks',{})).rejects.toMatchObject({status:403,code:'original_code',message:'原文拒绝',details:{original:'保持'}});
 const data={id:'r',version:2,title:'original',data:{raw:'source'}};respond=async()=>Response.json(data);
 expect(await rawApi('/evidence')).toEqual(data);expect(await api('/evidence')).toEqual({...data,raw:'source'});
 expect(ids(sent[1])).toEqual([null,null]);
});
it('retains the first caller and immutable envelope when an unknown write is retried from a later view',async()=>{
 await render(session('A'));const original=current;let calls=0;respond=async()=>{if(++calls===1)throw new TypeError('unknown response');return Response.json({ok:true});};
 const body={project_id:'p',idempotency_key:'view-recovery-key',expected_version:3,nested:{name:'original'}};
 await expect(mutate('/tasks',body,'POST','view-identity-retry',original)).rejects.toBeInstanceOf(ApiError);
 body.nested.name='edited after unknown';await render(session('B'));await mutate('/tasks',body,'POST','view-identity-retry',current);
 expect(sent.map(ids)).toEqual([['user-A','org-A'],['user-A','org-A']]);expect(sent[1].body).toEqual(sent[0].body);
});
it('passes a captured caller explicitly into non-React recovery and preserves its original retry scope',async()=>{
 await render(session('A'));const records=new Map<string,string>(),storage={getItem:(key:string)=>records.get(key)??null,setItem:(key:string,value:string)=>{records.set(key,value);},removeItem:(key:string)=>{records.delete(key);}};
 const recovery=new ToolWriteRecovery('org-A:user-A:p',storage,current);let count=0;respond=async()=>{if(++count===1)throw new TypeError('unknown');return Response.json({ok:true});};
 await expect(recovery.run('/tool-profiles',{project_id:'p',name:'original'})).rejects.toMatchObject({status:0});await render(session('B'));await recovery.restore();
 expect(sent.map(ids)).toEqual([['user-A','org-A'],['user-A','org-A']]);expect(sent[1].body).toEqual(sent[0].body);
});
it('issues a token with one stable key and original caller, retaining it when unknown outcome refuses a new issuance',async()=>{
 await render(session('A'));const creation=new TokenCreation(current);respond=async()=>{throw new TypeError('unknown');};
 await expect(creation.create('p','original label','runner')).rejects.toMatchObject({status:0});const key=creation.operationID;
 await render(session('B'));await expect(creation.create('other','new label','api')).rejects.toMatchObject({code:'token_outcome_unknown'});
 expect(creation.operationID).toBe(key);expect(sent).toHaveLength(1);expect(ids(sent[0])).toEqual(['user-A','org-A']);
 expect(sent[0].body).toEqual({project_id:'p',label:'original label',scope:'runner',idempotency_key:key});
});
