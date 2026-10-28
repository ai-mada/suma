// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {webcrypto} from 'node:crypto';
import {afterEach,expect,it,vi} from 'vitest';
import {IDE} from './IDE';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';

const commands=vi.hoisted(()=>({current:undefined as {run:()=>void|Promise<void>}|undefined}));
vi.mock('@monaco-editor/react',async()=>{
 const {createElement,useEffect,useRef}=await import('react');
 function LocalEditor({value,onChange,onMount}:{value:string;onChange:(value:string)=>void;onMount:(instance:unknown,runtime:unknown)=>void}){
  const mount=useRef(onMount);
  useEffect(()=>{let disposed:(()=>void)|undefined;mount.current({addAction:(action:{run:()=>void|Promise<void>})=>{commands.current=action;return {dispose(){if(commands.current===action)commands.current=undefined;}};},onDidDispose:(callback:()=>void)=>{disposed=callback;}},{KeyMod:{CtrlCmd:2048},KeyCode:{KeyS:49}});return()=>disposed?.();},[]);
  return createElement('textarea',{'aria-label':'Mock local code editor',value,onChange:(event:{target:{value:string}})=>onChange(event.target.value)});
 }
 return {loader:{config:vi.fn()},default:LocalEditor};
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

// The transport can commit a write even when its 200 body is unrelated or
// malformed. Recovery must use the original operation and original caller;
// typing during this uncertainty must remain the next explicit draft.
it('hides the original draft in another account and restores its exact unknown operation only for its owner',async()=>{
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.stubGlobal('crypto',webcrypto);
 await setLocale('en-US',{persist:false});
 // The textarea stand-in normalizes DOM newlines; raw CRLF/UTF-8 receipt bytes
 // are covered separately without that test-only browser control behavior.
 const project='ide-save-project',path='src/main.ts',base='before\n',submitted='提交 🧪\n',later='new draft after unknown acknowledgement\n';
 const initial={project_id:project,path,content:base,hash:await digest(base),language:'typescript',workspace_name:'Owned workspace',context_sha256:'a'.repeat(64)};
 const writes:{path:string;body:Record<string,unknown>;headers:Headers}[]=[];
 let provider='A',disk=base;
 const session=()=>({user:{id:'ide-user-'+provider,name:'Editor',email:'editor@example.test'},organization:{id:'ide-org-'+provider,name:'Scope'},projects:[]});
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{
  const url=new URL(String(input),location.href);
  if(init?.body){
   const body=JSON.parse(String(init.body));writes.push({path:url.pathname,body,headers:new Headers(init.headers)});
   if(writes.length===1){disk=submitted;return Response.json({...initial,path:'other.ts',content:submitted,hash:await digest(submitted)});}
   if(writes.length===2)return Response.json({error:{code:'current_refusal',message:'Current permission denied'}},{status:403});
   disk=body.content;return Response.json({...initial,content:body.content,hash:await digest(body.content),context_sha256:body.expected_context_sha256});
  }
  if(url.pathname==='/api/v1/workspace')return Response.json({configured:true,name:'Owned workspace',root:'/owned',files:[{path,name:'main.ts',type:'file'}]});
  if(url.pathname==='/api/v1/workspace/tree')return Response.json({project_id:project,directory:url.searchParams.get('directory')||'',entries:url.searchParams.get('directory')==='src'?[{path,name:'main.ts',kind:'file'}]:[{path:'src',name:'src',kind:'directory'}],limit:100,next_cursor:'',snapshot_sha256:'c'.repeat(64),observed_at:'2026-10-28T00:00:00Z'});
  if(url.pathname==='/api/v1/workspace/file')return Response.json({...initial,content:disk,hash:await digest(disk)});
  throw Error('Unexpected request '+url.pathname);
 }));
 host=document.createElement('div');document.body.append(host);root=createRoot(host);
 const render=()=>root!.render(<ApiViewProvider session={session()}><IDE project={project}/></ApiViewProvider>);
 async function settle(ready:()=>boolean){for(let i=0;i<40;i++){await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});if(ready())return;}expect(ready()).toBe(true);}
 const button=(text:string)=>[...host!.querySelectorAll('button')].find(item=>item.textContent===text);
 const editor=()=>host!.querySelector<HTMLTextAreaElement>('textarea[aria-label="Mock local code editor"]')!;
 async function type(value:string){await act(async()=>{const setter=Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value')!.set!;setter.call(editor(),value);editor().dispatchEvent(new Event('input',{bubbles:true}));});}
 await act(async()=>render());await settle(()=>!!button('src'));await act(async()=>button('src')!.click());await settle(()=>!!button('main.ts'));
 await act(async()=>button('main.ts')!.click());await settle(()=>!!editor());
 await type(submitted);await act(async()=>button('Save')!.click());
 await settle(()=>!!button('Recover previous save'));
 expect(editor().value).toBe(submitted);
 await type(later);provider='B';await act(async()=>render());
 await settle(()=>!!button('src'));expect(editor()).toBeNull();expect(host.textContent).not.toContain(later);expect(button('Recover previous save')).toBeUndefined();expect(button('Save')?.disabled).toBe(true);expect(writes).toHaveLength(1);
 provider='A';await act(async()=>render());await settle(()=>!!editor()&&!!button('Recover previous save'));expect(editor().value).toBe(later);
 await act(async()=>button('Recover previous save')!.click());await settle(()=>writes.length===2&&!!button('Recover previous save')&&!button('Recover previous save')!.disabled);
 expect(editor().value).toBe(later);
 await act(async()=>button('Recover previous save')!.click());await settle(()=>writes.length===3&&!!button('Read current file'));
 expect(editor().value).toBe(later);
 expect(button('Save')?.disabled).toBe(true);
 expect(writes.slice(0,3).map(write=>write.body)).toEqual([writes[0].body,writes[0].body,writes[0].body]);
 for(const write of writes.slice(0,3)){expect(write.path).toBe('/api/v1/workspace/file');expect(write.headers.get('X-Suma-Expected-User')).toBe('ide-user-A');expect(write.headers.get('X-Suma-Expected-Organization')).toBe('ide-org-A');}
 await act(async()=>button('Read current file')!.click());await settle(()=>!!button('Use current workspace and keep draft'));await act(async()=>button('Use current workspace and keep draft')!.click());await act(async()=>button('Save')!.click());await settle(()=>writes.length===4&&!!button('Save')?.disabled);
 expect(writes[3].body).toMatchObject({project_id:project,path,content:later,expected_hash:await digest(submitted)});
 expect(writes[3].body.idempotency_key).not.toBe(writes[0].body.idempotency_key);
 expect(writes[3].headers.get('X-Suma-Expected-User')).toBe('ide-user-A');
});

it('keeps a rebound workspace draft and model until an explicit current read and baseline adoption',async()=>{
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.stubGlobal('crypto',webcrypto);await setLocale('en-US',{persist:false});
 const project='binding-project',path='main.ts',contextA='a'.repeat(64),contextB='b'.repeat(64),draft='draft belongs to A\n';
 const initial={project_id:project,path,content:'same bytes\n',hash:await digest('same bytes\n'),language:'typescript',workspace_name:'Folder A',context_sha256:contextA};
 const writes:Record<string,unknown>[]=[],reads:string[]=[];let metadataReads=0,treeReads=0;
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{
  const url=new URL(String(input),location.href);
  if(url.pathname==='/api/v1/workspace'){metadataReads++;return Response.json({configured:true,name:reads.length>1?'Folder B':'Folder A',binding_sha256:reads.length>1?contextB:contextA});}
  if(url.pathname==='/api/v1/workspace/tree'){treeReads++;return Response.json({project_id:project,directory:'',entries:[{path,name:path,kind:'file'}],limit:100,next_cursor:'',snapshot_sha256:'c'.repeat(64),observed_at:'2026-10-28T00:00:00Z'});}
  if(url.pathname==='/api/v1/workspace/file'){
   if(!init?.body){reads.push(url.pathname);return Response.json({...initial,...(reads.length>1?{workspace_name:'Folder B',context_sha256:contextB}:{})});}
   const body=JSON.parse(String(init.body));writes.push(body);
   if(writes.length===1)return Response.json({error:{code:'workspace_context_changed',message:'Workspace changed',details:{project_id:project,path,operation_id:body.idempotency_key,expected_context_sha256:contextA,not_accepted:true}}},{status:409});
   return Response.json({...initial,workspace_name:'Folder B',context_sha256:contextB,content:body.content,hash:await digest(body.content)});
  }
  throw Error('Unexpected request '+url.pathname);
 }));
 host=document.createElement('div');document.body.append(host);root=createRoot(host);
 await act(async()=>root!.render(<IDE project={project}/>));
 const button=(text:string)=>[...host!.querySelectorAll<HTMLButtonElement>('button')].find(item=>item.textContent?.trim()===text);
 async function settle(ready:()=>boolean){for(let i=0;i<40;i++){await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});if(ready())return;}expect(ready()).toBe(true);}
 await settle(()=>!!button(path));await act(async()=>button(path)!.click());await settle(()=>!!host!.querySelector('textarea'));
 const editor=host.querySelector<HTMLTextAreaElement>('textarea')!;
 await act(async()=>{Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value')!.set!.call(editor,draft);editor.dispatchEvent(new Event('input',{bubbles:true}));});
 await act(async()=>button('Save')!.click());await settle(()=>!!button('Read current file'));
 expect(reads).toHaveLength(1);expect(button('Save')?.disabled).toBe(true);expect(editor.value).toBe(draft);expect(host.querySelector('textarea')).toBe(editor);
 await act(async()=>commands.current!.run());expect(writes).toHaveLength(1);expect(reads).toHaveLength(1);
 expect(host.textContent).toContain('Folder A');expect(host.textContent).not.toContain('Another operation changed this file');
 await act(async()=>{await setLocale('zh-CN',{persist:false});});expect(host.textContent).toContain('工作区已变化');expect(reads).toHaveLength(1);expect(editor.value).toBe(draft);
 await act(async()=>{await setLocale('en-US',{persist:false});});await act(async()=>button('Read current file')!.click());await settle(()=>!!button('Use current workspace and keep draft'));
 expect(host.textContent).toContain('Folder B');expect(button('Save')?.disabled).toBe(true);expect(writes).toHaveLength(1);expect(editor.value).toBe(draft);
 await act(async()=>button('Use current workspace and keep draft')!.click());await settle(()=>metadataReads===2&&treeReads===2);expect(host.querySelector('.tree-heading')?.textContent).toBe('Folder B');expect(button('Save')?.disabled).toBe(false);expect(editor.value).toBe(draft);expect(host.querySelector('textarea')).toBe(editor);
 await act(async()=>button('Save')!.click());await settle(()=>writes.length===2&&button('Save')?.disabled===true);
 expect(writes[0]).toMatchObject({expected_context_sha256:contextA,content:draft});expect(writes[1]).toMatchObject({expected_context_sha256:contextB,content:draft,expected_hash:initial.hash});expect(writes[1].idempotency_key).not.toBe(writes[0].idempotency_key);
});

it('keeps a late old-account acknowledgement from clearing its hidden operation or replacing the restored draft',async()=>{
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.stubGlobal('crypto',webcrypto);await setLocale('en-US',{persist:false});
 const project='late-owner-project',path='main.ts',submitted='original submission',later='newer typing while saving';
 const initial={project_id:project,path,content:'base',hash:await digest('base'),language:'typescript',workspace_name:'Owner A folder',context_sha256:'a'.repeat(64)};
 const writes:{body:Record<string,unknown>;headers:Headers}[]=[];
 let provider='A',finish:((value:Response)=>void)|undefined;
 const session=()=>({user:{id:'late-user-'+provider,name:'Editor',email:'owner@example.test'},organization:{id:'late-org-'+provider,name:'Scope'}});
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{
  const url=new URL(String(input),location.href);
  if(url.pathname==='/api/v1/workspace')return Response.json({configured:true,name:'Current owner folder'});
  if(url.pathname==='/api/v1/workspace/tree')return Response.json({project_id:project,directory:'',entries:[{path,name:path,kind:'file'}],limit:100,next_cursor:'',snapshot_sha256:'c'.repeat(64),observed_at:'2026-10-28T00:00:00Z'});
  if(url.pathname==='/api/v1/workspace/file'){
   if(!init?.body)return Response.json(initial);
   const body=JSON.parse(String(init.body));writes.push({body,headers:new Headers(init.headers)});
   if(writes.length===1)return new Promise<Response>(resolve=>{finish=resolve;});
   return Response.json({...initial,content:body.content,hash:await digest(body.content)});
  }
  throw Error('Unexpected request '+url.pathname);
 }));
 host=document.createElement('div');document.body.append(host);root=createRoot(host);
 const render=()=>root!.render(<ApiViewProvider session={session()}><IDE project={project}/></ApiViewProvider>);
 const button=(text:string)=>[...host!.querySelectorAll<HTMLButtonElement>('button')].find(item=>item.textContent?.trim()===text);
 async function settle(ready:()=>boolean){for(let i=0;i<40;i++){await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});if(ready())return;}expect(ready()).toBe(true);}
 await act(async()=>render());await settle(()=>!!button(path));await act(async()=>button(path)!.click());await settle(()=>!!host!.querySelector('textarea'));
 const editor=()=>host!.querySelector<HTMLTextAreaElement>('textarea')!;
 async function type(content:string){await act(async()=>{Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value')!.set!.call(editor(),content);editor().dispatchEvent(new Event('input',{bubbles:true}));});}
 await type(submitted);await act(async()=>button('Save')!.click());await settle(()=>!!finish);await type(later);
 provider='B';await act(async()=>render());await settle(()=>!!button(path));expect(editor()).toBeNull();expect(host.textContent).not.toContain(later);
 const receipt={...initial,content:submitted,hash:await digest(submitted)};await act(async()=>{finish!(Response.json(receipt));});
 expect(editor()).toBeNull();expect(writes).toHaveLength(1);
 provider='A';await act(async()=>render());await settle(()=>!!editor()&&!!button('Recover previous save'));expect(editor().value).toBe(later);
 await act(async()=>button('Recover previous save')!.click());await settle(()=>writes.length===2&&!!button('Read current file'));
 expect(writes[1].body).toEqual(writes[0].body);expect(writes[1].headers.get('X-Suma-Expected-User')).toBe('late-user-A');expect(editor().value).toBe(later);expect(button('Save')?.disabled).toBe(true);
});

it('releases an unknown operation only on its exact not-accepted proof and keeps the draft blocked for current-context review',async()=>{
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.stubGlobal('crypto',webcrypto);await setLocale('en-US',{persist:false});
 const project='proof-project',path='main.ts',draft='keep this draft';
 const initial={project_id:project,path,content:'base',hash:await digest('base'),language:'typescript',workspace_name:'Original folder',context_sha256:'a'.repeat(64)};
 const writes:Record<string,unknown>[]=[],reads:string[]=[];
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{
  const url=new URL(String(input),location.href);
  if(url.pathname==='/api/v1/workspace')return Response.json({configured:true,name:'Original folder'});
  if(url.pathname==='/api/v1/workspace/tree')return Response.json({project_id:project,directory:'',entries:[{path,name:path,kind:'file'}],limit:100,next_cursor:'',snapshot_sha256:'c'.repeat(64),observed_at:'2026-10-28T00:00:00Z'});
  if(url.pathname==='/api/v1/workspace/file'){
   if(!init?.body){reads.push(path);return Response.json(initial);}
   const body=JSON.parse(String(init.body));writes.push(body);
   if(writes.length===1)throw new TypeError('Lost response');
   return Response.json({error:{code:'workspace_context_changed',message:'Original request was not accepted',details:{project_id:project,path,operation_id:body.idempotency_key,expected_context_sha256:body.expected_context_sha256,not_accepted:true}}},{status:409});
  }
  throw Error('Unexpected request '+url.pathname);
 }));
 host=document.createElement('div');document.body.append(host);root=createRoot(host);
 const button=(text:string)=>[...host!.querySelectorAll<HTMLButtonElement>('button')].find(item=>item.textContent?.trim()===text);
 async function settle(ready:()=>boolean){for(let i=0;i<40;i++){await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});if(ready())return;}expect(ready()).toBe(true);}
 await act(async()=>root!.render(<IDE project={project}/>));await settle(()=>!!button(path));await act(async()=>button(path)!.click());await settle(()=>!!host!.querySelector('textarea'));
 const editor=host.querySelector<HTMLTextAreaElement>('textarea')!;await act(async()=>{Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value')!.set!.call(editor,draft);editor.dispatchEvent(new Event('input',{bubbles:true}));});
 await act(async()=>button('Save')!.click());await settle(()=>!!button('Recover previous save'));await act(async()=>button('Recover previous save')!.click());await settle(()=>!!button('Read current file'));
 expect(writes).toEqual([writes[0],writes[0]]);expect(reads).toHaveLength(1);expect(editor.value).toBe(draft);expect(button('Recover previous save')).toBeUndefined();expect(button('Save')?.disabled).toBe(true);expect(button('Read current file')?.disabled).toBe(false);
});
