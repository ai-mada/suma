// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {webcrypto} from 'node:crypto';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {WorkspaceDesktopBindingRequest,WorkspaceMetadata} from '../../../packages/contracts/src';
import {ApiViewProvider} from './api-context';
import {DesktopWorkspacePicker} from './DesktopWorkspacePicker';
import {beginDesktopBinding,consumeDesktopBinding,desktopBindingReceipt,desktopWorkspacePath,runDesktopBinding,selectedDesktopDirectory} from './desktop-workspace-model';
import {ApiError,type ApiCaller} from './api';
import {setLocale} from './i18n';

let host:HTMLDivElement,root:Root,sequence=10;
const initial=(project:string):WorkspaceMetadata=>({project_id:project,configured:false,binding_available:false,desktop_binding_available:true,binding_sha256:'',binding_path_sha256:'',binding_operation_id:''});
const scope=(project:string)=>({userID:'u',organizationID:'o',project});
function selected(project:string,path='D:\\projects\\chosen'){
 const suffix=(sequence++).toString(16).padStart(32,'0');
 return {status:'selected' as const,path,grant:{protocol:'suma-desktop-workspace-v1',operation_id:'desktop-workspace-'+suffix,user_id:'u',tenant_id:'o',project_id:project,path,expires_at:new Date(Date.now()+90000).toISOString(),nonce:'2'.repeat(32),signature:'3'.repeat(64)}};
}
const digest=async(value:string)=>Array.from(new Uint8Array(await webcrypto.subtle.digest('SHA-256',new TextEncoder().encode(value))),byte=>byte.toString(16).padStart(2,'0')).join('');
const receipt=async(request:WorkspaceDesktopBindingRequest):Promise<WorkspaceMetadata>=>({...initial(request.project_id),configured:true,name:'chosen',binding_sha256:'a'.repeat(64),binding_path_sha256:await digest(request.grant.path),binding_operation_id:request.grant.operation_id});
const button=(text:string)=>[...host.querySelectorAll<HTMLButtonElement>('button')].find(item=>item.textContent?.trim()===text);
async function settle(ready:()=>boolean){for(let i=0;i<30;i++){await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});if(ready())return;}expect(ready()).toBe(true);}
beforeEach(async()=>{vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.stubGlobal('crypto',webcrypto);await setLocale('en-US',{persist:false});host=document.createElement('div');document.body.append(host);root=createRoot(host);});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();});
function native(choose:()=>Promise<unknown>){vi.stubGlobal('sumaDesktop',{platform:'win32',version:'1.0.0',chooseProjectDirectory:vi.fn(choose)});}
async function render(project:string,options:{user?:string;disabled?:boolean;active?:boolean;metadata?:WorkspaceMetadata;onBound?:(metadata:WorkspaceMetadata)=>void;onRefresh?:()=>Promise<WorkspaceMetadata|undefined>}={}){
 await act(async()=>root.render(<ApiViewProvider session={{user:{id:options.user??'u',name:'User',email:'u@example.test'},organization:{id:'o',name:'Org'}}}><DesktopWorkspacePicker project={project} metadata={options.metadata??initial(project)} active={options.active??true} disabled={options.disabled??false} onBound={options.onBound??(()=>{})} onRefresh={options.onRefresh??(async()=>initial(project))}/></ApiViewProvider>));
}

it('shows native selection only with both capabilities and leaves hidden or dirty workspaces untouched',async()=>{
 const choice=selected('capability'),choose=vi.fn(async()=>choice);native(choose);
 await render('capability',{metadata:{...initial('capability'),desktop_binding_available:false}});expect(button('Choose project folder')).toBeUndefined();
 await render('capability',{disabled:true});expect(button('Choose project folder')?.disabled).toBe(true);button('Choose project folder')!.click();expect(choose).not.toHaveBeenCalled();
 await render('capability',{active:false});expect(button('Choose project folder')?.disabled).toBe(true);expect(choose).not.toHaveBeenCalled();
 vi.stubGlobal('sumaDesktop',undefined);await render('capability');expect(host.textContent).toBe('');
});

it('ignores a native dialog after the user or project changes and never automatically posts',async()=>{
 let resolve!:(value:unknown)=>void;const choice=selected('late');native(()=>new Promise(done=>{resolve=done;}));const fetch=vi.fn();vi.stubGlobal('fetch',fetch);
 await render('late');await act(async()=>button('Choose project folder')!.click());
 await render('other',{user:'other'});await act(async()=>resolve(choice));expect(host.textContent).not.toContain(choice.path);expect(button('Bind folder')).toBeUndefined();expect(fetch).not.toHaveBeenCalled();
});

it('keeps an unknown operation across scope changes and explicitly replays its original caller, key and expired grant',async()=>{
 const project='recovery',choice=selected(project);native(async()=>choice);
 const requests:{body:WorkspaceDesktopBindingRequest;headers:RequestInit['headers']}[]=[];
 vi.stubGlobal('fetch',vi.fn(async(_input:string,init:RequestInit)=>{const body=JSON.parse(String(init.body));requests.push({body,headers:init.headers});if(requests.length===1)throw new TypeError('network');return Response.json(await receipt(body));}));
 const current={...initial(project),configured:true,name:'current-folder',binding_sha256:'b'.repeat(64),binding_path_sha256:'c'.repeat(64),binding_operation_id:'desktop-workspace-'+'d'.repeat(32)};
 const refresh=vi.fn(async()=>{if(refresh.mock.calls.length===1)throw new TypeError('read unavailable');return current;});
 const bound=vi.fn();await render(project,{onBound:bound,onRefresh:refresh});await act(async()=>button('Choose project folder')!.click());await act(async()=>button('Bind folder')!.click());
 expect(button('Check original binding')).toBeDefined();await act(async()=>{expect(window.dispatchEvent(new Event('suma:navigate',{cancelable:true}))).toBe(false);});
 await render('different',{user:'another'});expect(host.textContent).not.toContain(choice.path);expect(button('Check original binding')).toBeUndefined();
 expect(window.dispatchEvent(new Event('beforeunload',{cancelable:true}))).toBe(false);
 await act(async()=>root.unmount());root=createRoot(host);
 vi.spyOn(Date,'now').mockReturnValue(Date.parse(choice.grant.expires_at)+1000);
 await render(project,{onBound:bound,onRefresh:refresh});expect(button('Check original binding')).toBeDefined();expect(host.textContent).not.toContain(choice.grant.signature);
 await act(async()=>button('Check original binding')!.click());await settle(()=>!!button('Refresh current binding')&&!button('Refresh current binding')!.disabled);expect(bound).not.toHaveBeenCalled();expect(host.textContent).toContain('The original binding operation is confirmed');expect(requests).toHaveLength(2);
 await act(async()=>{expect(window.dispatchEvent(new Event('suma:navigate',{cancelable:true}))).toBe(false);});
 await act(async()=>button('Refresh current binding')!.click());await settle(()=>bound.mock.calls.length===1);expect(bound).toHaveBeenCalledWith(current);expect(refresh).toHaveBeenCalledTimes(2);
 expect(requests).toHaveLength(2);expect(requests[1]).toEqual(requests[0]);expect(requests[1].headers).toMatchObject({'X-Suma-Expected-User':'u','X-Suma-Expected-Organization':'o'});
 expect(button('Check original binding')).toBeUndefined();expect(window.dispatchEvent(new Event('suma:navigate',{cancelable:true}))).toBe(true);
});

it('keeps a late acknowledgement for its original view without replacing another project',async()=>{
 const project='late-ack',choice=selected(project);native(async()=>choice);
 let resolve!:(value:Response)=>void;let submitted:WorkspaceDesktopBindingRequest|undefined;
 const fetch=vi.fn(async(_input:string,init:RequestInit)=>{submitted=JSON.parse(String(init.body));return new Promise<Response>(done=>{resolve=done;});});vi.stubGlobal('fetch',fetch);
 const bound=vi.fn();await render(project,{onBound:bound});await act(async()=>button('Choose project folder')!.click());await act(async()=>button('Bind folder')!.click());
 await render('late-other',{user:'another',onBound:bound});await act(async()=>resolve(Response.json(await receipt(submitted!))));
 expect(bound).not.toHaveBeenCalled();expect(host.textContent).not.toContain(choice.path);
 let current!:WorkspaceMetadata;await act(async()=>{current={...await receipt(submitted!),configured:false};});
 await render(project,{onBound:bound,onRefresh:async()=>current});expect(bound).not.toHaveBeenCalled();await settle(()=>!!button('Refresh current binding'));await act(async()=>button('Refresh current binding')!.click());expect(bound).toHaveBeenCalledWith(current);expect(host.textContent).not.toContain('Folder bound to this project.');expect(fetch).toHaveBeenCalledOnce();
});

it('requires explicit refresh and a new selection after binding conflict, retaining the old CAS until then',async()=>{
 const project='conflict',choice=selected(project),writes:WorkspaceDesktopBindingRequest[]=[];native(async()=>choice);
 vi.stubGlobal('fetch',vi.fn(async(_input:string,init:RequestInit)=>{writes.push(JSON.parse(String(init.body)));return Response.json({error:{code:'desktop_workspace_binding_changed',message:'Changed'}},{status:409});}));
 const refresh=vi.fn(async()=>({...initial(project),binding_sha256:'b'.repeat(64)}));
 await render(project,{metadata:{...initial(project),binding_sha256:'c'.repeat(64)},onRefresh:refresh});await act(async()=>button('Choose project folder')!.click());await act(async()=>button('Bind folder')!.click());
 expect(writes[0].expected_binding_sha256).toBe('c'.repeat(64));expect(host.textContent).toContain(choice.path);expect(button('Bind folder')?.disabled).toBe(true);expect(refresh).not.toHaveBeenCalled();
 await act(async()=>button('Refresh binding')!.click());expect(refresh).toHaveBeenCalledOnce();expect(button('Bind folder')).toBeUndefined();expect(host.textContent).not.toContain(choice.path);expect(writes).toHaveLength(1);
});

it('releases a new expired binding but preserves uncertainty after a later authorization refusal',async()=>{
 const project='expired',choice=selected(project);let calls=0;
 const api:ApiCaller=async()=>{calls++;throw new ApiError(410,'desktop_workspace_grant_expired','expired');};
 const first=beginDesktopBinding(scope(project),api,choice.grant,'');expect((await runDesktopBinding(first)).kind).toBe('failed');consumeDesktopBinding(first);
 const again=selected(project);let complete=false;const replay:ApiCaller=async<T,>()=>{calls++;if(complete)return await receipt({project_id:project,expected_binding_sha256:'',grant:again.grant,idempotency_key:again.grant.operation_id}) as T;throw calls===2?new TypeError('network'):new ApiError(403,'forbidden','denied');};
 const second=beginDesktopBinding(scope(project),replay,again.grant,'');expect((await runDesktopBinding(second)).kind).toBe('unknown');expect((await runDesktopBinding(second)).kind).toBe('unknown');expect(second.uncertain).toBe(true);
 complete=true;expect((await runDesktopBinding(second)).kind).toBe('acknowledged');consumeDesktopBinding(second);
});

it('rejects ambiguous native selections and mismatched project, path, operation or name receipts',async()=>{
 const project='receipt',choice=selected(project),request={project_id:project,expected_binding_sha256:'',grant:choice.grant,idempotency_key:choice.grant.operation_id};
 expect(selectedDesktopDirectory({...choice,grant:{...choice.grant,tenant_id:'foreign'}},scope(project))).toBeUndefined();expect(selectedDesktopDirectory({...choice,extra:true},scope(project))).toBeUndefined();
 for(const path of ['\\\\host\\share','\\\\?\\C:\\chosen','D:\\a\\..\\chosen','D:\\chosen.','D:\\chosen\\','/a/../chosen','/a\nchosen'])expect(desktopWorkspacePath(path)).toBe(false);
 expect(desktopWorkspacePath('C:\\')).toBe(true);expect(desktopWorkspacePath('/work/chosen')).toBe(true);
 const ack=await receipt(request);expect(await desktopBindingReceipt(ack,request)).toEqual(ack);
 for(const changed of [{project_id:'foreign'},{binding_path_sha256:'b'.repeat(64)},{binding_operation_id:'desktop-workspace-'+'f'.repeat(32)},{name:'other'}])expect(await desktopBindingReceipt({...ack,...changed},request)).toBeUndefined();
 let complete=false;const original:ApiCaller=async<T,>()=>(complete?ack:{...ack,name:'other'}) as T,slot=beginDesktopBinding(scope(project),original,choice.grant,'');expect((await runDesktopBinding(slot)).kind).toBe('unknown');expect(slot.uncertain).toBe(true);
 complete=true;expect((await runDesktopBinding(slot)).kind).toBe('acknowledged');consumeDesktopBinding(slot);
});
