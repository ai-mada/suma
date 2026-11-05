// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {App} from './App';
import {setLocale} from './i18n';
let root:Root,host:HTMLDivElement,writes:number;
const projects=[{id:'p',name:'Project A',version:1},{id:'p2',name:'Project B',version:2}];
const portfolioItem=(project:(typeof projects)[number])=>({project_id:project.id,title:project.name,description:'',status:'active',
 task_total:0,accepted_tasks:0,blocked_tasks:0,active_risks:0});
const portfolio=projects.map(portfolioItem);
let visibleProjects:typeof projects, portfolioItems:typeof portfolio;
const asOf='2026-10-29T05:00:00Z';
beforeEach(async()=>{
 await setLocale('zh-CN',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);history.replaceState(null,'','/?project=p&view=settings&sub=tools');host=document.createElement('div');document.body.append(host);root=createRoot(host);writes=0;visibleProjects=projects;portfolioItems=portfolio;localStorage.clear();sessionStorage.clear();
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{
  const url=new URL(String(input),location.origin),path=url.pathname.replace('/api/v1','');
  if(init?.method&&init.method!=='GET')writes++;
  switch(path){
   case '/auth/session': return Response.json({user:{id:'u',name:'Owner',email:'owner@example.test',role:'owner'},organization:{id:'o',name:'Org'},projects:visibleProjects});
   case '/account/preferences': return Response.json({user_id:'u',version:0,locale:'',updated_at:''});
   case '/organization/preferences': return Response.json({organization_id:'o',version:0,locale:'',updated_at:''});
   case '/project-portfolio': return Response.json({items:portfolioItems.filter(item=>item.title.includes(url.searchParams.get('q')||'')),as_of:asOf});
   case '/overview': return Response.json({project:visibleProjects.find(item=>item.id===url.searchParams.get('project_id')),counts:{},activity:[],as_of:asOf});
   default: return Response.json({items:[],total:0,as_of:asOf});
  }
 }));
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
async function openProject(id:string, destination:'overview'|'settings'){
 const button=host.querySelector<HTMLButtonElement>('[data-portfolio-project="'+id+'"] [data-portfolio-'+destination+']');
 expect(button).not.toBeNull();
 await act(async()=>button!.click());await settle();
}
it('opens the exact selected project settings from the organization portfolio',async()=>{
 await render();await nav('项目管理');await openProject('p2','settings');
 const url=new URLSearchParams(location.search);
 expect(url.get('project')).toBe('p2');expect(url.get('sub')).toBe('project');
 expect(host.querySelector('.settings-panel')?.textContent).toContain('Project B');expect(writes).toBe(0);
});
it('restores the URL search and opens the exact filtered project overview',async()=>{
 history.replaceState(null,'','/?project=p&view=settings&sub=projects&q=Project+B');await render();
 expect(host.querySelector<HTMLInputElement>('[data-portfolio-search]')?.value).toBe('Project B');
 expect(host.querySelectorAll('[data-portfolio-project]')).toHaveLength(1);
 await openProject('p2','overview');
 const url=new URLSearchParams(location.search);
 expect(url.get('project')).toBe('p2');expect(url.get('view')).toBe('overview');
 expect(host.querySelector('.project-overview .po-header')?.textContent).toContain('Project B');expect(writes).toBe(0);
});
it('rechecks the current session before opening a newly granted project outside the old session list',async()=>{
 const late={id:'p3',name:'Newly granted project',version:1};
 portfolioItems=[...portfolio,portfolioItem(late)];
 await render();await nav('项目管理');
 expect(host.querySelector('[data-portfolio-project=p3]')).not.toBeNull();
 visibleProjects=[...projects,late];
 await openProject('p3','settings');
 expect(new URLSearchParams(location.search).get('project')).toBe('p3');
 expect(host.querySelector('.settings-panel')?.textContent).toContain('Newly granted project');
 const reads=vi.mocked(fetch).mock.calls.map(([url])=>String(url)).filter(url=>url==='/api/v1/auth/session');
 expect(reads).toHaveLength(2);expect(writes).toBe(0);
});
