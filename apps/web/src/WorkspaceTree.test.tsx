// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import {WorkspaceTree} from './WorkspaceTree';
import {setLocale} from './i18n';
import {appendWorkspaceTreePage,validWorkspaceTreePage,workspaceDirectory,workspaceTreePath} from './workspace-tree-model';
import type {WorkspaceTreeEntry} from '../../../packages/contracts/src';

const stamp='2026-10-28T02:00:00Z';
const page=(directory='',entries:WorkspaceTreeEntry[]=[],next_cursor='',snapshot_sha256='a'.repeat(64),project_id='p')=>({project_id,directory,entries,limit:100,next_cursor,snapshot_sha256,observed_at:stamp});
const file=(path:string):WorkspaceTreeEntry=>({path,name:path.split('/').at(-1)!,kind:'file'});
const folder=(path:string):WorkspaceTreeEntry=>({path,name:path.split('/').at(-1)!,kind:'directory'});
const deferred=<T,>()=>{let resolve!:(value:T)=>void;const promise=new Promise<T>(done=>{resolve=done;});return {promise,resolve};};
let host:HTMLDivElement,root:Root;
beforeEach(async()=>{vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);await setLocale('en-US',{persist:false});host=document.createElement('div');document.body.append(host);root=createRoot(host);});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();await setLocale('zh-CN',{persist:false});});
async function render(props:Partial<Parameters<typeof WorkspaceTree>[0]>={},user='u'){
 await act(async()=>root.render(<ApiViewProvider session={{user:{id:user,name:user,email:user+'@example.test'},organization:{id:'org-'+user,name:'Org'}}}><WorkspaceTree project="p" active onOpen={()=>{}} {...props}/></ApiViewProvider>));
}
async function settle(ready:()=>boolean){for(let attempt=0;attempt<40;attempt++){if(ready())return;await act(async()=>{await new Promise(done=>setTimeout(done,0));});}expect(ready()).toBe(true);}
const button=(name:string)=>[...host.querySelectorAll<HTMLButtonElement>('button')].find(value=>value.textContent?.trim()===name||value.getAttribute('aria-label')===name);
const entry=(path:string)=>[...host.querySelectorAll<HTMLButtonElement>('.workspace-tree-entries button')].find(value=>value.title===path);
async function click(target:HTMLButtonElement|undefined){expect(target).toBeDefined();await act(async()=>target!.click());}
async function filter(value:string){const input=host.querySelector<HTMLInputElement>('input')!;await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(input,value);input.dispatchEvent(new Event('input',{bubbles:true}));});}

it('browses one directory at a time, filters only loaded names, appends a stable page, and keeps file identity and focus',async()=>{
 const paths:string[]=[],opened=vi.fn();
 vi.stubGlobal('fetch',vi.fn(async(input:string)=>{const url=new URL(String(input),location.href);paths.push(url.search);expect(url.pathname).toBe('/api/v1/workspace/tree');expect(url.searchParams.get('limit')).toBe('100');
  return Response.json(url.searchParams.get('directory')==='src'?page('src',[file('src/main.ts')]):url.searchParams.get('before')?page('',[file('last.ts')]):page('',[folder('src'),file('README.md')],'next-root'));
 }));
 await render({selectedPath:'README.md',onOpen:opened});await settle(()=>!!entry('src'));
 expect(entry('src')!.disabled).toBe(false);expect(entry('README.md')!.getAttribute('aria-current')).toBe('page');
 expect(host.textContent).toContain('Loaded: 2');expect(host.textContent).toContain('More entries are available');
 await filter('last');expect(entry('src')).toBeUndefined();expect(host.textContent).toContain('No loaded names match');expect(paths).toHaveLength(1);
 await click(button('Load more'));await settle(()=>!!entry('last.ts'));
 expect(host.textContent).toContain('Matches: 1 · Loaded: 3');expect(host.textContent).toContain('All entries in this directory are loaded');expect(button('Load more')).toBeUndefined();expect(document.activeElement).toBe(entry('last.ts'));
 await filter('');await click(entry('README.md'));expect(opened).toHaveBeenCalledExactlyOnceWith('README.md');
 await click(entry('src'));await settle(()=>!!entry('src/main.ts'));
 expect(host.querySelector('input')!.value).toBe('');expect(entry('README.md')).toBeUndefined();expect(document.activeElement?.getAttribute('aria-current')).toBe('location');expect(document.activeElement?.textContent).toBe('src');
 expect(paths).toHaveLength(3);expect(new URLSearchParams(paths[1]).get('before')).toBe('next-root');
 await click(button('Up one level'));await settle(()=>!!entry('README.md'));expect(paths).toHaveLength(4);
});

it('requires an explicit refresh after409 and never blends the old snapshot with new entries',async()=>{
 const beforeValues:(string|null)[]=[];let refreshed=false;
 vi.stubGlobal('fetch',vi.fn(async(input:string)=>{const url=new URL(String(input),location.href);beforeValues.push(url.searchParams.get('before'));
  if(url.searchParams.has('before')){refreshed=true;return Response.json({error:{code:'workspace_tree_changed',message:'Original directory snapshot changed'}},{status:409});}
  return Response.json(refreshed?page('',[file('new.ts')],'','b'.repeat(64)):page('',[file('old.ts')],'next-old'));
 }));
 await render();await settle(()=>!!entry('old.ts'));await click(button('Load more'));await settle(()=>!!button('Refresh and reload'));
 expect(entry('old.ts')).toBeUndefined();expect(button('Load more')).toBeUndefined();expect(host.textContent).toContain('Refresh it to reload from the first page');expect(beforeValues).toEqual([null,'next-old']);
 await click(button('Refresh and reload'));await settle(()=>!!entry('new.ts'));expect(entry('old.ts')).toBeUndefined();expect(beforeValues).toEqual([null,'next-old',null]);
});

it('aborts superseded directory requests and ignores a late response even if the transport disregards cancellation',async()=>{
 const slow=deferred<Response>();let oldSignal:AbortSignal|undefined;
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{const directory=new URL(String(input),location.href).searchParams.get('directory');if(directory==='a'){oldSignal=init?.signal as AbortSignal;return slow.promise;}return Response.json(directory==='b'?page('b',[file('b/current.ts')]):page('',[folder('a'),folder('b')]));}));
 await render();await settle(()=>!!entry('a'));await click(entry('a'));expect(host.textContent).toContain('Reading directory');
 await click(button('Project root'));await settle(()=>!!entry('b'));await click(entry('b'));await settle(()=>!!entry('b/current.ts'));
 expect(oldSignal?.aborted).toBe(true);await act(async()=>slow.resolve(Response.json(page('a',[file('a/late.ts')]))));
 expect(entry('b/current.ts')).toBeDefined();expect(entry('a/late.ts')).toBeUndefined();
});

it('resets API and project identity before old data can render and rejects late private listings',async()=>{
 const old=deferred<Response>();let count=0,oldSignal:AbortSignal|undefined;
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{const project=new URL(String(input),location.href).searchParams.get('project_id')!;count++;if(count===1){oldSignal=init?.signal as AbortSignal;return old.promise;}return Response.json(page('',[file(project==='p'?'new-user.ts':'new-project.ts')],'','b'.repeat(64),project));}));
 await render({},'first');await render({},'second');await settle(()=>!!entry('new-user.ts'));expect(oldSignal?.aborted).toBe(true);
 await act(async()=>old.resolve(Response.json(page('',[file('private-old.ts')]))));expect(entry('private-old.ts')).toBeUndefined();expect(entry('new-user.ts')).toBeDefined();
 await render({project:'p2'},'second');expect(entry('new-user.ts')).toBeUndefined();await settle(()=>!!entry('new-project.ts'));expect(count).toBe(3);
});

it('makes no inactive request, aborts an active read on hide, and resumes only the current directory',async()=>{
 const first=deferred<Response>();const calls:RequestInit[]=[],directories:string[]=[];let directoryReads=0;
 vi.stubGlobal('fetch',vi.fn(async(input:string,init:RequestInit)=>{calls.push(init);const directory=new URL(String(input),location.href).searchParams.get('directory')||'';directories.push(directory);if(calls.length===1)return first.promise;if(directory==='src'){directoryReads++;return Response.json(page('src',[file(directoryReads===1?'src/current.ts':'src/revalidated.ts')],'',directoryReads===1?'a'.repeat(64):'b'.repeat(64)));}return Response.json(page('',[folder('src'),file('current.ts')]));}));
 await render({active:false});expect(calls).toHaveLength(0);await render({active:true});expect(calls).toHaveLength(1);
 await render({active:false});expect(calls[0].signal?.aborted).toBe(true);await act(async()=>first.resolve(Response.json(page('',[file('late.ts')]))));expect(entry('late.ts')).toBeUndefined();expect(calls).toHaveLength(1);
 await render({active:true});await settle(()=>!!entry('current.ts'));expect(calls).toHaveLength(2);
 await click(entry('src'));await settle(()=>!!entry('src/current.ts'));await render({active:false});expect(calls).toHaveLength(3);
 await render({active:true});await settle(()=>!!entry('src/revalidated.ts'));expect(entry('src/current.ts')).toBeUndefined();expect(directories).toEqual(['','','src','src']);
});

it('preserves loaded pages after a transient next-page error, retries the same cursor, and clears them on authorization loss',async()=>{
 const cursors:(string|null)[]=[];
 vi.stubGlobal('fetch',vi.fn(async(input:string)=>{const before=new URL(String(input),location.href).searchParams.get('before');cursors.push(before);if(!before)return Response.json(page('',[file('kept.ts')],'next'));return Response.json({error:{code:cursors.length===2?'temporary':'forbidden',message:cursors.length===2?'Temporary read failure':'Access revoked'}},{status:cursors.length===2?500:403});}));
 await render();await settle(()=>!!entry('kept.ts'));await click(button('Load more'));await settle(()=>!!button('Reload'));expect(entry('kept.ts')).toBeDefined();
 await click(button('Reload'));await settle(()=>host.textContent!.includes('Access revoked'));expect(entry('kept.ts')).toBeUndefined();expect(cursors).toEqual([null,'next','next']);
});

it('shows empty and scoped directory copy in both languages without another request',async()=>{
 const fetch=vi.fn(async()=>Response.json(page()));vi.stubGlobal('fetch',fetch);await render();await settle(()=>host.textContent!.includes('This directory has no displayable entries'));
 expect(host.textContent).toContain('Loaded: 0');expect(host.textContent).toContain('does not search the whole project');
 await act(async()=>setLocale('zh-CN',{persist:false}));expect(host.textContent).toContain('此目录没有可显示的项目');expect(host.textContent).toContain('已加载 0 项');expect(host.textContent).toContain('不会搜索整个项目');expect(fetch).toHaveBeenCalledTimes(1);
});

it('rejects malformed or cross-directory pages, unsafe paths, duplicate entries and snapshot drift',()=>{
 const current=page('',[file('one.ts')],'next');expect(validWorkspaceTreePage(current,'p','')).toBe(true);
 for(const bad of [{...current,project_id:'other'},{...current,directory:'src'},{...current,limit:99},{...current,total:500},{...current,next_cursor:'x'.repeat(1025)},{...current,entries:[file('src/deep.ts')]},{...current,entries:[{...file('one.ts'),name:'other.ts'}]},{...current,entries:[file('one.ts'),file('one.ts')]}])expect(validWorkspaceTreePage(bad,'p','')).toBe(false);
 for(const path of ['../private','./file','C:/absolute','src\\file','/root','src//file','src/../file','src\nfile'])expect(workspaceDirectory(path)).toBe(false);
 expect(workspaceTreePath('p','源代码','next')).toContain('directory=%E6%BA%90%E4%BB%A3%E7%A0%81');
 expect(()=>appendWorkspaceTreePage(current,page('',[file('two.ts')],'','b'.repeat(64)),'next')).toThrow();expect(()=>appendWorkspaceTreePage(current,page('',[file('one.ts')]),'next')).toThrow();
 expect(current.entries).toHaveLength(1);
});
