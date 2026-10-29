// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {webcrypto} from 'node:crypto';
import {afterEach,expect,it,vi} from 'vitest';
import {IDE} from './IDE';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import type {WorkspaceTaskContext} from '../../../packages/contracts/src';

const taskContextProps={session:{user:{id:'u',name:'Editor',email:'e@example.test'},organization:{id:'o',name:'Org'},projects:[]},selectedTask:'',refresh:0,onTaskSelect:()=>true};
const commands=vi.hoisted(()=>({current:undefined as {run:()=>void|Promise<void>}|undefined}));
vi.mock('@monaco-editor/react',async()=>{
 const {createElement,useEffect,useRef}=await import('react');
 function LocalEditor({value,onChange,onMount}:{value:string;onChange:(value:string)=>void;onMount:(instance:unknown,runtime:unknown)=>void}){
  const mount=useRef(onMount);
  useEffect(()=>{
   let disposed:(()=>void)|undefined;
   const instance={addAction:(action:{run:()=>void|Promise<void>})=>{commands.current=action;return {dispose(){if(commands.current===action)commands.current=undefined;}};},onDidDispose:(callback:()=>void)=>{disposed=callback;return {dispose(){disposed=undefined;}};}};
   mount.current(instance,{KeyMod:{CtrlCmd:2048},KeyCode:{KeyS:49}});return()=>disposed?.();
  },[]);
  return createElement('textarea',{'aria-label':'Mock code editor',value,onChange:(event:{target:{value:string}})=>onChange(event.target.value)});
 }
 return {loader:{config:vi.fn()},default:LocalEditor,DiffEditor:({original,modified}:{original:string;modified:string})=>createElement('section',{'data-git-diff':true},createElement('pre',{'data-before':true},original),createElement('pre',{'data-after':true},modified))};
});
vi.mock('monaco-editor/esm/vs/editor/editor.api.js',()=>({KeyMod:{CtrlCmd:2048},KeyCode:{KeyS:49}}));
vi.mock('monaco-editor/esm/vs/editor/editor.main.js',()=>({}));
vi.mock('monaco-editor/esm/vs/editor/editor.worker?worker',()=>({default:class {}}));
vi.mock('monaco-editor/esm/vs/language/typescript/ts.worker?worker',()=>({default:class {}}));
vi.mock('monaco-editor/esm/vs/language/json/json.worker?worker',()=>({default:class {}}));
vi.mock('monaco-editor/esm/vs/language/css/css.worker?worker',()=>({default:class {}}));
vi.mock('monaco-editor/esm/vs/language/html/html.worker?worker',()=>({default:class {}}));
let root:Root|undefined,host:HTMLDivElement|undefined;
afterEach(async()=>{if(root)await act(async()=>root!.unmount());root=undefined;host?.remove();commands.current=undefined;vi.unstubAllGlobals();vi.restoreAllMocks();});
const digest=async(value:string)=>Array.from(new Uint8Array(await webcrypto.subtle.digest('SHA-256',new TextEncoder().encode(value))),byte=>byte.toString(16).padStart(2,'0')).join('');

it('keeps the same editor and both drafts while task basis and workspace guidance are toggled',async()=>{
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.stubGlobal('crypto',webcrypto);await setLocale('en-US',{persist:false});sessionStorage.clear();
 const project='compact-workspace',path='main.ts',content='original source',draft='unsaved source',discussion='unsent discussion';
 const counts={total:0,waiting:0,running:0,succeeded:0,failed:0,skipped:0,reconciling:0,other:0,limit:20,complete:true,items:[]};
 const context:WorkspaceTaskContext={project_id:project,observed_at:'2026-10-29T01:00:00Z',task:{id:'task-a',version:2,title:'Current task',description:'Task objective',acceptance_criteria:[],status:'in_review',priority:'medium',owner_id:'',requirement_id:'',run_id:'run-a'},requirement:null,owner:null,design:{state:'none',context:null},delivery:{project_id:project,task_id:'task-a',task_version:2,task_status:'in_review',run_basis_state:'current',formal_acceptance:false,run:{run_id:'run-a',version:3,state:'succeeded',nodes:counts,tests:counts},expert_plans:{scope:'current_run',total:0,limit:20,complete:true,items:[]},issues:{scope:'current_run',total:0,limit:20,complete:true,items:[]}},quality:{plan_id:'',revision:0,plan_sha256:'',run_id:'',run_state:'',state:'none',basis_current:false,total:0,passed:0,failed:0,errors:0,skipped:0,not_run:0}};
 const writes:RequestInit[]=[];
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{
  if(init?.body)writes.push(init);const url=new URL(String(input),location.href);
  if(url.pathname==='/api/v1/workspace/task-context')return Response.json(context);
  if(url.pathname==='/api/v1/task-conversations')return Response.json({project_id:project,task_id:'task-a',task_version:2,can_post:true,items:[],total:0,limit:20,next_before:0});
  if(url.pathname==='/api/v1/workspace')return Response.json({project_id:project,configured:true,binding_available:false,desktop_binding_available:false,binding_sha256:'a'.repeat(64),binding_path_sha256:'b'.repeat(64),binding_operation_id:'',name:'Owned workspace'});
  if(url.pathname==='/api/v1/workspace/tree')return Response.json({project_id:project,directory:'',entries:[{path,name:path,kind:'file'}],limit:100,next_cursor:'',snapshot_sha256:'d'.repeat(64),observed_at:'2026-10-29T01:00:00Z'});
  if(url.pathname==='/api/v1/workspace/file')return Response.json({project_id:project,path,content,hash:await digest(content),language:'typescript',workspace_name:'Owned workspace',context_sha256:'a'.repeat(64)});
  throw Error('Unexpected request '+url.pathname);
 }));
 host=document.createElement('div');document.body.append(host);root=createRoot(host);
 await act(async()=>root!.render(<ApiViewProvider session={taskContextProps.session}><IDE {...taskContextProps} project={project} selectedTask="task-a"/></ApiViewProvider>));
 const button=(name:string)=>[...host!.querySelectorAll<HTMLButtonElement>('button')].find(item=>item.textContent?.trim()===name)!;
 async function settle(ready:()=>boolean){for(let i=0;i<40;i++){if(ready())return;await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}expect(ready()).toBe(true);}
 await settle(()=>!!button(path));await act(async()=>button(path).click());await settle(()=>!!host!.querySelector('[aria-label="Mock code editor"]'));
 const editing=host.querySelector<HTMLTextAreaElement>('[aria-label="Mock code editor"]')!;
 async function type(field:HTMLTextAreaElement,value:string){await act(async()=>{Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value')!.set!.call(field,value);field.dispatchEvent(new Event('input',{bubbles:true}));});}
 await type(editing,draft);
 const basis=host.querySelector<HTMLDetailsElement>('.workbench-task-details')!,help=host.querySelector<HTMLDetailsElement>('.ide-guidance');
 expect(basis.open).toBe(false);expect(help).not.toBeNull();expect(help!.open).toBe(false);
 await act(async()=>host!.querySelector<HTMLButtonElement>('[data-conversation-toggle]')!.click());await settle(()=>!!host!.querySelector('[name=conversation_body]'));
 const compose=host.querySelector<HTMLTextAreaElement>('[name=conversation_body]')!;await type(compose,discussion);
 for(const disclosure of [basis,help!])for(let count=0;count<2;count++)await act(async()=>disclosure.querySelector('summary')!.click());
 expect(host.querySelector('[aria-label="Mock code editor"]')).toBe(editing);expect(editing.value).toBe(draft);
 expect(host.querySelector('[name=conversation_body]')).toBe(compose);expect(compose.value).toBe(discussion);
 await act(async()=>host!.querySelector<HTMLButtonElement>('[data-conversation-toggle]')!.click());
 expect(host.querySelector('[data-conversation-toggle]')?.getAttribute('aria-expanded')).toBe('true');expect(compose.value).toBe(discussion);
 await act(async()=>{expect(window.dispatchEvent(new Event('suma:navigate',{cancelable:true}))).toBe(false);});expect(writes).toHaveLength(0);
 await type(compose,'');await act(async()=>host!.querySelector<HTMLButtonElement>('[data-conversation-toggle]')!.click());
 await act(async()=>host!.querySelector<HTMLButtonElement>('[data-conversation-toggle]')!.click());
 expect(host.querySelector('[aria-label="Mock code editor"]')).toBe(editing);expect(editing.value).toBe(draft);
});

it('requires an explicit desktop bind and enters the acknowledged project workspace',async()=>{
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.stubGlobal('crypto',webcrypto);await setLocale('en-US',{persist:false});
 const path='D:\\projects\\chosen',id='desktop-workspace-'+'1'.repeat(32);
 const grant={protocol:'suma-desktop-workspace-v1',operation_id:id,user_id:'u',tenant_id:'o',project_id:'p',path,expires_at:new Date(Date.now()+90000).toISOString(),nonce:'2'.repeat(32),signature:'3'.repeat(64)};
 const choose=vi.fn(async()=>({status:'selected',path,grant}));vi.stubGlobal('sumaDesktop',{platform:'win32',version:'1.0.0',chooseProjectDirectory:choose});
 const writes:Record<string,unknown>[]=[];let bound=false;
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{
  const url=new URL(String(input),location.href);
  if(url.pathname==='/api/v1/workspace')return Response.json(bound?{project_id:'p',configured:true,binding_available:false,desktop_binding_available:true,name:'current-folder',binding_sha256:'b'.repeat(64),binding_path_sha256:'c'.repeat(64),binding_operation_id:'desktop-workspace-'+'d'.repeat(32)}:{project_id:'p',configured:false,binding_available:false,desktop_binding_available:true,binding_sha256:'',binding_path_sha256:'',binding_operation_id:''});
  if(url.pathname==='/api/v1/workspace/desktop-binding'){bound=true;writes.push(JSON.parse(String(init?.body)));expect(init?.headers).toMatchObject({'X-Suma-Expected-User':'u','X-Suma-Expected-Organization':'o'});return Response.json({project_id:'p',configured:true,binding_available:false,desktop_binding_available:true,name:'chosen',binding_sha256:'a'.repeat(64),binding_path_sha256:await digest(path),binding_operation_id:id});}
  if(url.pathname==='/api/v1/workspace/tree')return Response.json({project_id:'p',directory:'',entries:[],limit:100,next_cursor:'',snapshot_sha256:'d'.repeat(64),observed_at:new Date().toISOString()});
  throw Error('Unexpected request '+url.pathname);
 }));
 host=document.createElement('div');document.body.append(host);root=createRoot(host);
 await act(async()=>root!.render(<ApiViewProvider session={{user:{id:'u',name:'Editor',email:'e@example.test'},organization:{id:'o',name:'Org'}}}><IDE {...taskContextProps} project="p"/></ApiViewProvider>));
 const button=(name:string)=>[...host!.querySelectorAll<HTMLButtonElement>('button')].find(item=>item.textContent?.trim()===name);
 for(let i=0;i<20&&!button('Choose project folder');i++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});
 expect(button('Choose project folder')).toBeDefined();
 await act(async()=>button('Choose project folder')!.click());expect(writes).toHaveLength(0);expect(host.textContent).toContain(path);expect(host.textContent).not.toContain(grant.signature);
 expect(choose).toHaveBeenCalledWith({project_id:'p',user_id:'u',tenant_id:'o',locale:'en-US'});
 await act(async()=>button('Bind folder')!.click());
 for(let i=0;i<20&&!host.querySelector('.editor-panel');i++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});
 expect(writes).toEqual([{project_id:'p',expected_binding_sha256:'',grant,idempotency_key:id}]);expect(host.querySelector('.editor-panel')).not.toBeNull();expect(host.querySelector('.tree-heading')?.textContent).toBe('current-folder');expect(host.textContent).toContain('The workspace now reflects the current project binding');expect(host.textContent).not.toContain(path);
});

it('does not offer workspace binding when deployment disabled it, in either language, while project navigation stays available',async()=>{
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);await setLocale('en-US',{persist:false});
 const requests:RequestInit[]=[];
 vi.stubGlobal('fetch',vi.fn(async(input:string,init:RequestInit)=>{
  requests.push(init);expect(new URL(String(input),location.href).pathname).toBe('/api/v1/workspace');
  return Response.json({configured:false,binding_available:false,reason:'Forbidden workspace detail'});
 }));
 host=document.createElement('div');document.body.append(host);root=createRoot(host);let navigated=0;
 await act(async()=>root!.render(<ApiViewProvider session={{user:{id:'u',name:'Editor',email:'e@example.test'},organization:{id:'o',name:'Org'}}}><nav aria-label="Project"><button type="button" onClick={()=>{if(window.dispatchEvent(new Event('suma:navigate',{cancelable:true})))navigated++;}}>Overview</button></nav><IDE {...taskContextProps} project="p"/></ApiViewProvider>));
 for(let i=0;i<30&&!host.querySelector('.settings-panel');i++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});
 expect(host.textContent).toContain('Local workspace is not enabled');
 expect(host.textContent).toContain('Ask the deployment administrator to configure an allowed workspace directory before binding this project');
 expect(host.textContent).not.toContain('Forbidden workspace detail');
 expect(host.querySelector('form')).toBeNull();expect(host.querySelector('input')).toBeNull();expect(host.querySelector('[type=submit]')).toBeNull();
 await act(async()=>[...host!.querySelectorAll('nav button')][0].dispatchEvent(new MouseEvent('click',{bubbles:true})));expect(navigated).toBe(1);
 await act(async()=>{await setLocale('zh-CN',{persist:false});});
 expect(host.textContent).toContain('本地工作空间尚未启用');
 expect(host.textContent).toContain('请由部署管理员配置允许的工作目录后，再绑定项目');
 expect(host.textContent).not.toContain('Forbidden workspace detail');expect(host.querySelector('form')).toBeNull();expect(host.querySelector('input')).toBeNull();
 await act(async()=>[...host!.querySelectorAll('nav button')][0].dispatchEvent(new MouseEvent('click',{bubbles:true})));expect(navigated).toBe(2);
 expect(requests).toHaveLength(1);expect(requests.every(request=>request.method==='GET'&&!request.body)).toBe(true);
});

it('keeps the editing model and draft through Git navigation, then saves once through the registered Monaco action',async()=>{
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.stubGlobal('crypto',webcrypto);await setLocale('en-US',{persist:false});
 const path='src/main.ts',head='a'.repeat(40),observed='2026-10-27T01:00:00Z',initial='disk before\n',draft='unsaved draft 🧪\n';
 const replacement='D:\\projects\\replacement',replacementID='desktop-workspace-'+'4'.repeat(32);
 const choose=vi.fn(async()=>({status:'selected',path:replacement,grant:{protocol:'suma-desktop-workspace-v1',operation_id:replacementID,user_id:'u',tenant_id:'o',project_id:'p',path:replacement,expires_at:new Date(Date.now()+90000).toISOString(),nonce:'5'.repeat(32),signature:'6'.repeat(64)}}));vi.stubGlobal('sumaDesktop',{platform:'win32',version:'1.0.0',chooseProjectDirectory:choose});
 let disk=initial,metadataReads=0;const writes:Record<string,unknown>[]=[];
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{
  const url=new URL(String(input),location.href);
  if(url.pathname==='/api/v1/workspace'){metadataReads++;return Response.json({project_id:'p',configured:true,binding_available:true,desktop_binding_available:true,binding_sha256:(metadataReads===1?'a':'e').repeat(64),binding_path_sha256:'b'.repeat(64),binding_operation_id:'',name:'Owned workspace'});}
  if(url.pathname==='/api/v1/workspace/desktop-binding')return Response.json({error:{code:'desktop_workspace_binding_changed',message:'Changed'}},{status:409});
  if(url.pathname==='/api/v1/workspace/tree')return Response.json({project_id:'p',directory:url.searchParams.get('directory')||'',entries:url.searchParams.get('directory')==='src'?[{path,name:'main.ts',kind:'file'}]:[{path:'src',name:'src',kind:'directory'}],limit:100,next_cursor:'',snapshot_sha256:'d'.repeat(64),observed_at:observed});
  if(url.pathname==='/api/v1/workspace/file'){
   if(init?.body){const body=JSON.parse(String(init.body));writes.push(body);disk=body.content;}
   return Response.json({project_id:'p',path,content:disk,hash:await digest(disk),language:'typescript',workspace_name:'Owned workspace',context_sha256:'a'.repeat(64)});
  }
  if(url.pathname==='/api/v1/workspace/git')return Response.json({project_id:'p',available:true,reason_code:'ready',branch:'main',head,unborn:false,detached:false,files:[{path,previous_path:'',index_status:' ',worktree_status:'M',untracked:false,conflicted:false}],total:1,limit:200,truncated:false,restricted_files:0,observed_at:observed});
  if(url.pathname==='/api/v1/workspace/git/diff')return Response.json({project_id:'p',path,previous_path:'',source:'worktree',head,original:initial,modified:disk,original_sha256:await digest(initial),modified_sha256:await digest(disk),binary:false,observed_at:observed});
  throw Error('Unexpected request '+url.pathname);
 }));
 host=document.createElement('div');document.body.append(host);root=createRoot(host);
 await act(async()=>root!.render(<ApiViewProvider session={{user:{id:'u',name:'Editor',email:'e@example.test'},organization:{id:'o',name:'Org'}}}><IDE {...taskContextProps} project="p"/></ApiViewProvider>));
 const button=(name:string)=>[...host!.querySelectorAll('button')].find(item=>item.textContent?.trim()===name||item.getAttribute('aria-label')===name)!;
 async function settle(ready:()=>boolean){for(let i=0;i<40;i++){await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});if(ready())return;}expect(ready()).toBe(true);}
 await settle(()=>!!button('src'));await act(async()=>button('src').click());await settle(()=>!!button('main.ts'));await act(async()=>button('main.ts').click());await settle(()=>!!host!.querySelector('textarea'));
 const editing=host.querySelector<HTMLTextAreaElement>('textarea')!;
 await act(async()=>{Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value')!.set!.call(editing,draft);editing.dispatchEvent(new Event('input',{bubbles:true}));});
 expect(button('Choose project folder').disabled).toBe(true);expect(choose).not.toHaveBeenCalled();
 await act(async()=>button('Up one level').click());await settle(()=>!!button('src'));await act(async()=>button('Refresh directory').click());await settle(()=>!!button('src'));await act(async()=>button('src').click());await settle(()=>!!button('main.ts'));
 expect(host.querySelector('textarea')).toBe(editing);expect(editing.value).toBe(draft);expect(writes).toHaveLength(0);
 await act(async()=>button('Changes').click());await settle(()=>!!button(path));await act(async()=>button(path).click());await settle(()=>!!host!.querySelector('[data-git-diff]'));
 expect(host.querySelector('[data-after]')?.textContent).toBe(initial);expect(writes).toHaveLength(0);
 expect(host.querySelector('textarea')).toBe(editing);expect(editing.value).toBe(draft);
 await act(async()=>button('Files').click());expect(editing.closest('[hidden]')).toBeNull();expect(editing.value).toBe(draft);
 await act(async()=>commands.current!.run());await settle(()=>!!button('Save')?.disabled);
 expect(writes).toHaveLength(1);expect(writes[0]).toMatchObject({project_id:'p',path,content:draft,expected_hash:await digest(initial),expected_context_sha256:'a'.repeat(64)});
 expect(button('Choose project folder').disabled).toBe(false);
 await act(async()=>commands.current!.run());expect(writes).toHaveLength(1);
 await act(async()=>button('Changes').click());await settle(()=>host!.querySelector('[data-after]')?.textContent===draft);
 expect(host.querySelector('textarea')).toBe(editing);expect(writes).toHaveLength(1);
 await act(async()=>button('Files').click());await act(async()=>button('Choose project folder').click());expect(host.querySelector('textarea')).toBe(editing);expect(editing.value).toBe(draft);
 await act(async()=>button('Bind folder').click());expect(host.querySelector('textarea')).toBe(editing);expect(host.textContent).toContain('The project binding changed');
 await act(async()=>button('Refresh binding').click());expect(metadataReads).toBe(2);expect(host.querySelector('textarea')).toBeNull();expect(writes).toHaveLength(1);expect(button('Bind folder')).toBeUndefined();
});
