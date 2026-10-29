// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,describe,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import type {Session} from './api';
import {setLocale} from './i18n';
import {CommandPalette,type CommandPaletteProps} from './CommandPalette';

let host:HTMLDivElement,root:Root,props:CommandPaletteProps;
let requests:{body:any;headers:Record<string,string>;signal:AbortSignal;resolve:(response:Response)=>void}[];
const item=(id:string,title:string,project='p')=>({id,type:'tasks',title,snippet:'Task details',revision:1,locator:{project_id:project,type:'tasks',id}});
const reply=(items:unknown[],extra:Record<string,unknown>={})=>new Response(JSON.stringify({items,total:items.length,limit:50,truncated:false,...extra}));
const options=()=>Array.from(document.querySelectorAll<HTMLElement>('[role=option]'));
const input=()=>document.querySelector<HTMLInputElement>('[role=combobox]')!;
async function settle(){await act(async()=>{for(let index=0;index<10;index++)await Promise.resolve();});}
async function render(patch:Partial<CommandPaletteProps>={}){
 props={...props,...patch};
 const session:Session={user:{id:props.userID,name:'Member',email:'member@example.test'},organization:{id:props.organizationID,name:'Org'},projects:[]};
 await act(async()=>root.render(<ApiViewProvider session={session}><CommandPalette {...props}/></ApiViewProvider>));
 await settle();
}
async function type(value:string){await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(input(),value);input().dispatchEvent(new Event('input',{bubbles:true}));});}
async function key(value:string){await act(async()=>input().dispatchEvent(new KeyboardEvent('keydown',{key:value,bubbles:true,cancelable:true})));}
async function debounce(){await act(async()=>vi.advanceTimersByTime(250));await settle();}
async function resolve(index:number,items:unknown[],extra:Record<string,unknown>={}){await act(async()=>requests[index].resolve(reply(items,extra)));await settle();}

beforeEach(async()=>{
 // Keep native response streams and React's scheduler on their real clock;
 // this test controls only the search debounce and request timeout.
 await setLocale('zh-CN',{persist:false});vi.useFakeTimers({toFake:['setTimeout','clearTimeout']});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);
 host=document.createElement('div');document.body.append(host);root=createRoot(host);requests=[];
 props={open:true,onClose:vi.fn(),project:'p',projectName:'Project A',view:'development',sub:'tasks',userID:'u',organizationID:'o',onNavigate:vi.fn(()=>true),onSource:vi.fn(()=>true)};
 vi.stubGlobal('fetch',vi.fn((_url:string,init:RequestInit)=>new Promise<Response>(resolve=>requests.push({body:JSON.parse(String(init.body)),headers:init.headers as Record<string,string>,signal:init.signal as AbortSignal,resolve}))));
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.useRealTimers();vi.unstubAllGlobals();});

describe('context-aware command palette',()=>{
 it.each([['zh-CN','editor','工作空间'],['en-US','编辑器','Workspace']] as const)('matches translated page aliases in %s',async(locale,alias,label)=>{
  await setLocale(locale,{persist:false});await render();await type(alias);
  expect(options().some(option=>option.textContent?.includes(label))).toBe(true);await key('Enter');
  expect(props.onNavigate).toHaveBeenCalledWith({view:'development',sub:'ide'});expect(requests).toHaveLength(0);
 });
 it('offers current pages without searching and supports keyboard navigation through the leave guard',async()=>{
  await render();expect(requests).toHaveLength(0);expect(document.body.textContent).toContain('Project A');expect(input().getAttribute('aria-expanded')).toBe('true');
  await key('End');const last=options().at(-1)!;expect(input().getAttribute('aria-activedescendant')).toBe(last.id);
  await key('Home');await key('ArrowDown');const chosen=options()[1];expect(chosen.getAttribute('aria-selected')).toBe('true');
  await key('Enter');expect(props.onNavigate).toHaveBeenCalledOnce();expect(props.onClose).toHaveBeenCalledOnce();
 });
 it('leaves the dialog open if navigation is blocked and restores focus to the input after a pointer selection',async()=>{
  await render({onNavigate:vi.fn(()=>false)});await act(async()=>options()[1].click());
  expect(props.onNavigate).toHaveBeenCalledOnce();expect(props.onClose).not.toHaveBeenCalled();expect(document.activeElement).toBe(input());
 });
 it('debounces current-project reads and sends the loaded actor and organization',async()=>{
  await render();await type('first');await type('needle');expect(requests).toHaveLength(0);await debounce();
  expect(requests).toHaveLength(1);expect(requests[0].body).toEqual({project_id:'p',query:'needle',types:[]});
  expect(requests[0].headers['X-Suma-Expected-User']).toBe('u');expect(requests[0].headers['X-Suma-Expected-Organization']).toBe('o');
  await resolve(0,[item('t1','Needle result')]);expect(document.body.textContent).toContain('Needle result');await key('End');await key('Enter');
  expect(props.onSource).toHaveBeenCalledWith(expect.objectContaining({id:'t1'}));
 });
 it('hides old-query results immediately and ignores a late response after the query changes',async()=>{
  await render();await type('alpha');await debounce();await type('beta');expect(requests[0].signal.aborted).toBe(true);
  await resolve(0,[item('old','Private alpha')]);expect(document.body.textContent).not.toContain('Private alpha');await key('Enter');expect(props.onSource).not.toHaveBeenCalled();
  await debounce();await resolve(1,[item('new','Current beta')]);expect(document.body.textContent).toContain('Current beta');
  await type('gamma');expect(document.body.textContent).not.toContain('Current beta');await key('Enter');expect(props.onSource).not.toHaveBeenCalled();
 });
 it.each(['project','userID','organizationID'] as const)('clears pending reads, query and private rows when %s changes',async field=>{
  await render();await type('private');await debounce();const changed={[field]:'next'};await render(changed);
  expect(requests[0].signal.aborted).toBe(true);expect(input().value).toBe('');await resolve(0,[item('old','Previous private title')]);
  expect(document.body.textContent).not.toContain('Previous private title');expect(props.onSource).not.toHaveBeenCalled();
 });
 it('rejects foreign and malformed locators and shows bounded-result coverage',async()=>{
  await render();await type('proof');await debounce();await resolve(0,[item('foreign','Foreign secret','another'),{...item('unsafe','Unsafe'),locator:{project_id:'p',type:'tasks',id:'https://bad.test'}},item('safe','Visible proof')],{total:100,limit:50,truncated:true});
  expect(document.body.textContent).not.toContain('Foreign secret');expect(document.body.textContent).not.toContain('Unsafe');expect(document.body.textContent).toContain('Visible proof');expect(document.body.textContent).toContain('最多 50');
 });
 it('distinguishes an authorization failure from no matches and retries the current query explicitly',async()=>{
  await render();await type('needle');await debounce();await act(async()=>requests[0].resolve(new Response(JSON.stringify({error:{code:'forbidden',message:'Current access denied'}}),{status:403})));await settle();
  expect(document.querySelector('[role=alert]')?.textContent).toContain('Current access denied');expect(document.body.textContent).not.toContain('没有匹配结果');expect(props.onSource).not.toHaveBeenCalled();
  const retry=document.querySelector<HTMLButtonElement>('[role=alert] button');expect(retry?.textContent).toBe('重新读取');await act(async()=>retry!.click());await debounce();expect(requests).toHaveLength(2);expect(requests[1].body).toEqual(requests[0].body);
 });
 it('clears a closed search and offers only safe recent page destinations on reopening',async()=>{
  await render();await key('End');await key('Enter');await render({open:false});await render({open:true});
  expect(document.body.textContent).toContain('最近访问');expect(input().value).toBe('');expect(requests).toHaveLength(0);
 });
 it('uses English copy and does not search before a project is selected',async()=>{
  await setLocale('en-US',{persist:false});await render({project:'',projectName:''});await type('task');await debounce();
  expect(document.body.textContent).toContain('Choose a project');expect(input().getAttribute('placeholder')).toContain('Search pages');expect(requests).toHaveLength(0);
 });
 it('blocks an oversized UTF-8 query before a request',async()=>{
  await render();await type('测'.repeat(200));await debounce();expect(requests).toHaveLength(0);expect(document.querySelector('[role=alert]')).not.toBeNull();
 });
});
