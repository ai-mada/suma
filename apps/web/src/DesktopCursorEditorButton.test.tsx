// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {WorkspaceMetadata} from '../../../packages/contracts/src';
import {ApiViewProvider} from './api-context';
import {DesktopCursorEditorButton} from './DesktopCursorEditorButton';
import {setLocale} from './i18n';

const project='1'.repeat(32),user='2'.repeat(32),tenant='3'.repeat(32),binding='a'.repeat(64);
const session={user:{id:user,name:'User',email:'user@example.test'},organization:{id:tenant,name:'Team'}};
const metadata:WorkspaceMetadata={project_id:project,configured:true,binding_available:true,desktop_binding_available:true,
 binding_sha256:binding,binding_path_sha256:'b'.repeat(64),binding_operation_id:''};
let root:Root,host:HTMLDivElement;
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);
 host=document.createElement('div');document.body.append(host);root=createRoot(host);
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();});

const bridge=(openCursorEditor:()=>Promise<unknown>,platform='win32')=>
 vi.stubGlobal('sumaDesktop',{platform,version:'1.0.0',openCursorEditor});
async function draw(changes:Partial<WorkspaceMetadata>={},disabled=false){
 await act(async()=>root.render(<ApiViewProvider session={session}>
  <DesktopCursorEditorButton project={project} metadata={{...metadata,...changes}} active disabled={disabled}/>
 </ApiViewProvider>));
}
const button=()=>host.querySelector('button')!;
const status=()=>host.querySelector('[role=status]')?.textContent;

it('requests the exact bound project on explicit action, once, and describes the launch request',async()=>{
 let respond:(value:unknown)=>void=()=>{};
 const open=vi.fn(()=>new Promise(resolve=>{respond=resolve;}));bridge(open);
 await draw();expect(open).not.toHaveBeenCalled();
 await act(async()=>{button().click();button().click();});
 expect(open).toHaveBeenCalledExactlyOnceWith({project_id:project,user_id:user,tenant_id:tenant,expected_binding_sha256:binding,locale:'en-US'});
 expect(button().getAttribute('aria-disabled')).toBe('true');
 await act(async()=>respond({status:'requested'}));
 expect(status()).toBe('Cursor launch requested. Check the Cursor window to continue.');
});

it('clears a prior binding result and does not display raw native errors',async()=>{
 let respond:(value:unknown)=>void=()=>{};
 const open=vi.fn().mockImplementationOnce(()=>new Promise(resolve=>{respond=resolve;}))
  .mockResolvedValueOnce({status:'failed',code:'unexpected code containing a local path'});
 bridge(open);await draw();await act(async()=>button().click());
 await draw({binding_sha256:'c'.repeat(64)});
 await act(async()=>respond({status:'requested'}));expect(status()).toBe('');
 await act(async()=>{await setLocale('zh-CN',{persist:false});});
 await act(async()=>button().click());
 expect(open).toHaveBeenLastCalledWith({project_id:project,user_id:user,tenant_id:tenant,expected_binding_sha256:'c'.repeat(64),locale:'zh-CN'});
 expect(status()).toBe('启动请求未能确认，请先查看 Cursor 再重试。');
});

it('offers the action only for a Windows desktop with verified binding and blocks a disabled action',async()=>{
 const open=vi.fn(async()=>({status:'requested'}));bridge(open,'darwin');
 await draw();expect(button()).toBeNull();
 bridge(open);await draw({configured:false});expect(button()).toBeNull();
 await draw({},true);expect(button().disabled).toBe(true);
 await act(async()=>button().click());expect(open).not.toHaveBeenCalled();
});
