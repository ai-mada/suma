// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {App} from './App';
import {setLocale,tr} from './i18n';

const user='loaded-user-a',organization='loaded-organization-a',project='loaded-project-a';
let host:HTMLDivElement,root:Root|undefined,writes:{path:string;body:any;headers:Headers}[];
beforeEach(async()=>{
 localStorage.clear();sessionStorage.clear();await setLocale('en-US',{persist:false});
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);writes=[];
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{
  const path=new URL(String(input),location.href).pathname.replace(/^\/api\/v1/,'');
  const body=init?.body?JSON.parse(String(init.body)):undefined;
  if(body){writes.push({path,body,headers:new Headers(init?.headers)});return Response.json({id:'created-object',version:1,...body});}
  if(path==='/auth/session')return Response.json({user:{id:user,name:'Owner',email:'owner@example.test',role:'owner'},organization:{id:organization,name:'Original organization'},projects:[{id:project,version:1,title:'Original project'}]});
  if(path==='/account/preferences')return Response.json({user_id:user,locale:'',version:0,updated_at:''});
  if(path==='/organization/preferences')return Response.json({organization_id:organization,locale:'',version:0,updated_at:''});
  if(path==='/overview')return Response.json({counts:{},activity:[]});
  return Response.json({items:[],total:0});
 }));
});
afterEach(async()=>{if(root)await act(async()=>root?.unmount());root=undefined;host.remove();await setLocale('zh-CN',{persist:false});vi.unstubAllGlobals();vi.restoreAllMocks();});
async function settle(){for(let n=0;n<4;n++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function render(view:string,sub:string){history.replaceState(null,'','/?'+new URLSearchParams({project,view,sub}));await act(async()=>{root=createRoot(host);root.render(<App/>);});await settle();}
async function fill(input:HTMLInputElement,value:string){await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(input,value);input.dispatchEvent(new Event('input',{bubbles:true}));});}
function expectOriginalView(write:typeof writes[number]){expect(write.headers.get('X-Suma-Expected-User')).toBe(user);expect(write.headers.get('X-Suma-Expected-Organization')).toBe(organization);}

it('sends the loaded original view on a real task form write without changing its operation or body',async()=>{
 await render('development','tasks');
 await act(async()=>host.querySelector<HTMLButtonElement>('.records-panel .list-toolbar button.primary')!.click());
 const title=document.querySelector<HTMLInputElement>('input[name="title"]')!;await fill(title,'Original task body');
 await act(async()=>title.closest('form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));await settle();
 const write=writes.find(w=>w.path==='/tasks')!;expect(write).toBeDefined();expectOriginalView(write);
 expect(write.body).toMatchObject({project_id:project,title:'Original task body',idempotency_key:expect.any(String)});
});
it('sends the loaded original view on a real organizational team command',async()=>{
 await render('settings','teams');
 await act(async()=>host.querySelector<HTMLButtonElement>('button[aria-label="'+tr('collaboration.create_team')+'"]')!.click());
 const name=document.querySelector<HTMLInputElement>('input[name="name"]')!;await fill(name,'Original team body');
 await act(async()=>name.closest('form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));await settle();
 const write=writes.find(w=>w.path==='/teams')!;expect(write).toBeDefined();expectOriginalView(write);
 expect(write.body).toMatchObject({name:'Original team body',idempotency_key:expect.any(String)});
});
