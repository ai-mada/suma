// @vitest-environment jsdom
import {act} from 'react';
import {createRoot, type Root} from 'react-dom/client';
import {afterEach,beforeEach,describe,expect,it,vi} from 'vitest';
import {App} from './App';
import {i18n,tr} from './i18n';
import {managementAreas} from './management-navigation';

vi.mock('./project-views/Overview',()=>({Overview:({onIntelligent}:{onIntelligent?:()=>void})=> <><input aria-label="Workspace draft" defaultValue="Unsaved local draft"/>{onIntelligent&&<button type="button" aria-label="Overview intelligent action" onClick={onIntelligent}>Intelligent action</button>}</>}));
let root:Root|undefined,container:HTMLDivElement;
let narrow=false,mediaChange:(()=>void)|undefined;
beforeEach(async()=>{
  narrow=false;
  mediaChange=undefined;
  localStorage.clear();
  history.replaceState(null,'','/?project=project-a&view=overview&record=selected-record&panel=selected-panel');
  await i18n.changeLanguage('en-US');
  container=document.createElement('div');document.body.append(container);
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);
  vi.stubGlobal('matchMedia',vi.fn(()=>({get matches(){return narrow;},addEventListener:vi.fn((_type:string,listener:()=>void)=>{mediaChange=listener;}),removeEventListener:vi.fn()})));
  vi.stubGlobal('fetch',vi.fn(async(input:string)=>{
    const path=new URL(String(input),location.href).pathname.replace(/^\/api\/v1/,'');
    const value=path==='/auth/session'?{user:{id:'user-a',name:'Owner',email:'owner@example.test',role:'owner'},organization:{id:'organization-a',name:'Organization'},projects:[{id:'project-a',version:1,title:'Current project'}]}:path==='/overview'?{counts:{},activity:[]}:{items:[],total:0};
    return new Response(JSON.stringify(value),{headers:{'Content-Type':'application/json'}});
  }));
});
afterEach(async()=>{if(root)await act(async()=>root?.unmount());root=undefined;container.remove();vi.unstubAllGlobals();vi.restoreAllMocks();});
async function mount(){await act(async()=>{root=createRoot(container);root.render(<App/>);});}
async function click(button:HTMLButtonElement){await act(async()=>button.click());}
function button(label:string){const result=container.querySelector<HTMLButtonElement>(`button[aria-label="${label}"]`);expect(result).not.toBeNull();return result!;}
describe('shell navigation without losing the active workspace',()=>{
  it('uses the overview heading and preserves the original navigation guard for its intelligent action',async()=>{
    await mount();expect(container.querySelector('.page-title')).toBeNull();const originalURL=location.search;const guard=(event:Event)=>event.preventDefault();window.addEventListener('suma:navigate',guard);
    try{await click(button('Overview intelligent action'));expect(location.search).toBe(originalURL);}finally{window.removeEventListener('suma:navigate',guard);}
    await click(button('Overview intelligent action'));expect(new URLSearchParams(location.search).get('panel')).toBe('intent:');
  });
  it('retains the common title and intelligent action in the application catalog',async()=>{
    history.replaceState(null,'','/?project=project-a&view=overview&layout=applications');await mount();expect(container.querySelector('.page-title h1')).not.toBeNull();expect(container.querySelector('.page-title .intent-entry')).not.toBeNull();expect(container.querySelector('button[aria-label="Overview intelligent action"]')).toBeNull();
  });
  it('collapses without navigation or remounting the draft and restores the preference after reload',async()=>{
    await mount();
    const draft=container.querySelector('input[aria-label="Workspace draft"]');
    const originalURL=location.search;
    const guard=vi.fn((event:Event)=>event.preventDefault());window.addEventListener('suma:navigate',guard);
    try {
      await click(button(tr('experience-shell.collapse_navigation')));
      expect(container.querySelector('input[aria-label="Workspace draft"]')).toBe(draft);
      expect(location.search).toBe(originalURL);expect(guard).not.toHaveBeenCalled();
      expect(button(tr('experience-shell.expand_navigation')).getAttribute('aria-expanded')).toBe('false');
      expect(localStorage.getItem('suma-sidebar-collapsed')).toBe('true');
      expect(container.querySelectorAll(`nav[aria-label="${tr('shell.main_navigation')}"] button`)).toHaveLength(6);
      expect(container.querySelectorAll(`nav[aria-label="${tr('management.title')}"] button`)).toHaveLength(6);
      expect(container.querySelector('nav button[aria-current="page"]')?.getAttribute('aria-label')).toBe(tr('nav.overview'));
      await act(async()=>root?.unmount());root=undefined;await mount();
      expect(button(tr('experience-shell.expand_navigation')).getAttribute('aria-expanded')).toBe('false');
      await click(button(tr('nav.requirements')));
      expect(location.search).toBe(originalURL);expect(guard).toHaveBeenCalledTimes(1);
    } finally {window.removeEventListener('suma:navigate',guard);}
  });
  it('keeps workspace destinations usable from the collapsed sidebar',async()=>{
    await mount();await click(button(tr('experience-shell.collapse_navigation')));
    await click(button(tr('nav.requirements')));
    expect(new URLSearchParams(location.search).get('view')).toBe('requirements');
    expect(container.querySelector('nav button[aria-current="page"]')?.getAttribute('aria-label')).toBe(tr('nav.requirements'));
  });
  it('keeps each management destination named and usable when the sidebar is collapsed',async()=>{
    await mount();await click(button(tr('experience-shell.collapse_navigation')));
    const controls=container.querySelectorAll<HTMLButtonElement>(`nav[aria-label="${tr('management.title')}"] button`);
    expect([...controls].map(control=>control.getAttribute('aria-label'))).toEqual(managementAreas.map(area=>tr('management.area.'+area.id)));
    expect([...controls].map(control=>control.title)).toEqual(managementAreas.map(area=>tr('management.area.'+area.id)));
    await click(button(tr('management.area.projects')));
    expect(new URLSearchParams(location.search).get('view')).toBe('settings');
    expect(new URLSearchParams(location.search).get('sub')).toBe('projects');
    expect(new URLSearchParams(location.search).get('project')).toBe('project-a');
    expect(button(tr('management.area.projects')).getAttribute('aria-current')).toBe('page');
  });
  it('closes the narrow drawer with Escape without changing selected context and restores focus',async()=>{
    narrow=true;await mount();
    const trigger=button(tr('shell.open_navigation'));
    const sidebar=container.querySelector('aside.sidebar')!;
    expect(sidebar.hasAttribute('inert')).toBe(true);
    await click(trigger);
    expect(trigger.getAttribute('aria-expanded')).toBe('true');
    expect(sidebar.hasAttribute('inert')).toBe(false);
    expect(container.querySelector('.workspace')?.hasAttribute('inert')).toBe(true);
    expect(document.activeElement).toBe(sidebar.querySelector('.mobile-navigation-close'));
    const originalURL=location.search;
    await act(async()=>{window.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true}));});
    expect(location.search).toBe(originalURL);
    expect(trigger.getAttribute('aria-expanded')).toBe('false');
    expect(sidebar.hasAttribute('inert')).toBe(true);
    expect(document.activeElement).toBe(trigger);
  });
  it('returns focus to a visible desktop control when the open drawer crosses the desktop breakpoint',async()=>{
    narrow=true;await mount();await click(button(tr('shell.open_navigation')));
    await act(async()=>{narrow=false;mediaChange?.();});
    expect(container.querySelector('.workspace')?.hasAttribute('inert')).toBe(false);
    expect(document.activeElement).toBe(button(tr('experience-shell.collapse_navigation')));
  });
});
