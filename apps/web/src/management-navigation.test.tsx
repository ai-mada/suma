// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {App} from './App';
import {setLocale} from './i18n';
let root:Root,host:HTMLDivElement,writes:number;
beforeEach(async()=>{
 await setLocale('zh-CN',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);history.replaceState(null,'','/?project=p&view=settings&sub=tools');host=document.createElement('div');document.body.append(host);root=createRoot(host);writes=0;localStorage.clear();sessionStorage.clear();
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{const path=String(input).replace('/api/v1','').split('?')[0];if(init?.method&&init.method!=='GET')writes++;
 const value=path==='/auth/session'?{user:{id:'u',name:'Owner',email:'owner@example.test',role:'owner'},organization:{id:'o',name:'Org'},projects:[{id:'p',name:'Project A',version:1},{id:'p2',name:'Project B',version:2}]}:path==='/account/preferences'?{user_id:'u',version:0,locale:'',updated_at:''}:path==='/organization/preferences'?{organization_id:'o',version:0,locale:'',updated_at:''}:{items:[],total:0,as_of:'2026-10-29T05:00:00Z'};return new Response(JSON.stringify(value));}));
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();});
async function settle(){await act(async()=>{for(let i=0;i<8;i++)await new Promise(resolve=>setTimeout(resolve,0));});}
async function render(){await act(async()=>root.render(<App/>));await settle();}
async function nav(name:string){const button=[...host.querySelectorAll<HTMLButtonElement>('nav[aria-label="管理中心"] button')].find(item=>item.textContent?.trim()===name);expect(button).toBeDefined();await act(async()=>button!.click());await settle();}
it('provides separate management domains and only their own secondary pages',async()=>{
 await render();expect(host.querySelector('nav[aria-label="管理中心"]')).not.toBeNull();expect([...host.querySelectorAll('.page-tabs button')].map(e=>e.textContent)).toEqual(['执行工具','执行资源','执行调度','共享资源池']);
 await nav('项目管理');expect(host.querySelector('.page-title h1')?.textContent).toContain('项目管理');expect(host.textContent).toContain('Project B');expect(host.querySelector('.page-tabs')?.textContent).not.toContain('接入令牌');expect(new URLSearchParams(location.search).get('sub')).toBe('projects');
 await nav('模型管理');expect(host.querySelector('.page-title h1')?.textContent).toContain('模型管理');expect(new URLSearchParams(location.search).get('sub')).toBe('models');expect(writes).toBe(0);
});
it('keeps domain navigation behind the existing leave guard',async()=>{
 await render();const guard=(event:Event)=>event.preventDefault();window.addEventListener('suma:navigate',guard);await nav('项目管理');expect(new URLSearchParams(location.search).get('sub')).toBe('tools');window.removeEventListener('suma:navigate',guard);
});
it('opens a real project from its directory and keeps its exact scope after refresh',async()=>{
 await render();await nav('项目管理');const project=[...host.querySelectorAll<HTMLButtonElement>('[data-project-directory] button')].find(b=>b.textContent?.includes('Project B'));expect(project).toBeDefined();await act(async()=>project!.click());await settle();expect(new URLSearchParams(location.search).get('project')).toBe('p2');expect(new URLSearchParams(location.search).get('sub')).toBe('project');expect(host.querySelector('.settings-panel')?.textContent).toContain('Project B');expect(writes).toBe(0);
});
