// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {webcrypto} from 'node:crypto';
import {afterEach,expect,it,vi} from 'vitest';
import {IDE} from './IDE';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';

vi.mock('@monaco-editor/react',async()=>{const {createElement}=await import('react');return {loader:{config:vi.fn()},default:({value,onChange}:{value:string;onChange:(value:string)=>void})=>createElement('textarea',{'aria-label':'Mock local code editor',value,onChange:(event:{target:{value:string}})=>onChange(event.target.value)})};});
vi.mock('monaco-editor/esm/vs/editor/editor.api.js',()=>({}));
vi.mock('monaco-editor/esm/vs/editor/editor.main.js',()=>({}));
vi.mock('monaco-editor/esm/vs/editor/editor.worker?worker',()=>({default:class {}}));
vi.mock('monaco-editor/esm/vs/language/typescript/ts.worker?worker',()=>({default:class {}}));
vi.mock('monaco-editor/esm/vs/language/json/json.worker?worker',()=>({default:class {}}));
vi.mock('monaco-editor/esm/vs/language/css/css.worker?worker',()=>({default:class {}}));
vi.mock('monaco-editor/esm/vs/language/html/html.worker?worker',()=>({default:class {}}));

let root:Root|undefined,host:HTMLDivElement|undefined;
afterEach(async()=>{if(root)await act(async()=>root!.unmount());root=undefined;host?.remove();vi.unstubAllGlobals();vi.restoreAllMocks();});
const digest=async(value:string)=>Array.from(new Uint8Array(await webcrypto.subtle.digest('SHA-256',new TextEncoder().encode(value))),byte=>byte.toString(16).padStart(2,'0')).join('');

// The transport can commit a write even when its 200 body is unrelated or
// malformed. Recovery must use the original operation and original caller;
// typing during this uncertainty must remain the next explicit draft.
it('retains the original file operation across a mismatched200 and later refusal without replacing newer typing',async()=>{
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.stubGlobal('crypto',webcrypto);
 await setLocale('en-US',{persist:false});
 // The textarea stand-in normalizes DOM newlines; raw CRLF/UTF-8 receipt bytes
 // are covered separately without that test-only browser control behavior.
 const project='ide-save-project',path='src/main.ts',base='before\n',submitted='提交 🧪\n',later='new draft after unknown acknowledgement\n';
 const initial={path,content:base,hash:await digest(base),language:'typescript'};
 const writes:{path:string;body:Record<string,unknown>;headers:Headers}[]=[];
 let provider='A';
 const session=()=>({user:{id:'ide-user-'+provider,name:'Editor',email:'editor@example.test'},organization:{id:'ide-org-'+provider,name:'Scope'},projects:[]});
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{
  const url=new URL(String(input),location.href);
  if(init?.body){
   const body=JSON.parse(String(init.body));writes.push({path:url.pathname,body,headers:new Headers(init.headers)});
   if(writes.length===1)return Response.json({...initial,path:'other.ts',content:submitted,hash:await digest(submitted)});
   if(writes.length===2)return Response.json({error:{code:'current_refusal',message:'Current permission denied'}},{status:403});
   return Response.json({path,content:body.content,hash:await digest(body.content),language:'typescript'});
  }
  if(url.pathname==='/api/v1/workspace')return Response.json({configured:true,name:'Owned workspace',root:'/owned',files:[{path,name:'main.ts',type:'file'}]});
  if(url.pathname==='/api/v1/workspace/file')return Response.json(initial);
  throw Error('Unexpected request '+url.pathname);
 }));
 host=document.createElement('div');document.body.append(host);root=createRoot(host);
 const render=()=>root!.render(<ApiViewProvider session={session()}><IDE project={project}/></ApiViewProvider>);
 async function settle(ready:()=>boolean){for(let i=0;i<40;i++){await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});if(ready())return;}expect(ready()).toBe(true);}
 const button=(text:string)=>[...host!.querySelectorAll('button')].find(item=>item.textContent===text);
 const editor=()=>host!.querySelector<HTMLTextAreaElement>('textarea[aria-label="Mock local code editor"]')!;
 async function type(value:string){await act(async()=>{const setter=Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value')!.set!;setter.call(editor(),value);editor().dispatchEvent(new Event('input',{bubbles:true}));});}
 await act(async()=>render());await settle(()=>!!button('main.ts'));
 await act(async()=>button('main.ts')!.click());await settle(()=>!!editor());
 await type(submitted);await act(async()=>button('Save')!.click());
 await settle(()=>!!button('Recover previous save'));
 expect(editor().value).toBe(submitted);
 await type(later);provider='B';await act(async()=>render());
 await act(async()=>button('Recover previous save')!.click());await settle(()=>writes.length===2&&!!button('Recover previous save')&&!button('Recover previous save')!.disabled);
 expect(editor().value).toBe(later);
 await act(async()=>button('Recover previous save')!.click());await settle(()=>writes.length===3&&!!button('Save')&&!button('Save')!.disabled);
 expect(editor().value).toBe(later);
 expect(writes.slice(0,3).map(write=>write.body)).toEqual([writes[0].body,writes[0].body,writes[0].body]);
 for(const write of writes.slice(0,3)){expect(write.path).toBe('/api/v1/workspace/file');expect(write.headers.get('X-Suma-Expected-User')).toBe('ide-user-A');expect(write.headers.get('X-Suma-Expected-Organization')).toBe('ide-org-A');}
 await act(async()=>button('Save')!.click());await settle(()=>writes.length===4&&!!button('Save')?.disabled);
 expect(writes[3].body).toMatchObject({project_id:project,path,content:later,expected_hash:await digest(submitted)});
 expect(writes[3].body.idempotency_key).not.toBe(writes[0].body.idempotency_key);
 expect(writes[3].headers.get('X-Suma-Expected-User')).toBe('ide-user-B');
});
