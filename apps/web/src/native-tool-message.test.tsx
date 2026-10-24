// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {NativeToolMessage} from './NativeToolMessage';
import {setLocale,setServerCatalog} from './i18n';
import type {ToolEvent} from './tool-model';

let host:HTMLDivElement,root:Root;
beforeEach(async()=>{vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);await setLocale('en-US',{persist:false});setServerCatalog({version:'native-fixture',template_format:'named-dot-v1',default_locale:'zh-CN',supported_locales:['en-US','zh-CN'],messages:[{key:'native.endpoint.rejected',parameters:[{name:'status',kind:'integer'}],texts:{'en-US':'Endpoint rejected request (HTTP {{.status}})','zh-CN':'端点拒绝请求（HTTP {{.status}}）'}}]});host=document.createElement('div');document.body.append(host);root=createRoot(host);});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();});
const event:ToolEvent={schema_version:1,command_id:'native',profile_sha256:'a'.repeat(64),protocol:'suma-jsonl-v1',kind:'error',status:'upstream_rejected',message:'Endpoint rejected request (HTTP 429)',diagnostic:{schema_version:1,message_key:'native.endpoint.rejected',message_params:{status:429}}};

it('switches native presentation while keeping the original error and execution facts',async()=>{
 const before=JSON.stringify(event);await act(async()=>root.render(<NativeToolMessage event={event}/>));
 expect(host.querySelector('p')?.textContent).toBe(event.message);expect(host.querySelector('details pre')?.textContent).toBe(event.message);
 await act(async()=>{await setLocale('zh-CN',{persist:false});});
 expect(host.querySelector('p')?.textContent).toBe('端点拒绝请求（HTTP 429）');expect(host.querySelector('details pre')?.textContent).toBe(event.message);
 expect(JSON.stringify(event)).toBe(before);
});
it('keeps an open focused original disclosure bound to the same raw event when locale changes',async()=>{
 const before=JSON.stringify(event);await act(async()=>root.render(<NativeToolMessage event={event}/>));
 const details=host.querySelector('details')!,summary=details.querySelector('summary')!;
 details.open=true;summary.focus();
 await act(async()=>{await setLocale('zh-CN',{persist:false});});
 expect(host.querySelector('details')).toBe(details);expect(details.open).toBe(true);expect(document.activeElement).toBe(summary);
 expect(details.querySelector('pre')?.textContent).toBe(event.message);expect(JSON.stringify(event)).toBe(before);
});
it.each([
 ['foreign protocol',{protocol:'codex-jsonl-v1'}],['model text',{kind:'message'}],['unknown schema',{diagnostic:{...event.diagnostic,schema_version:2}}],
 ['wrong raw source',{message:'External original output <script>'}],['unknown key',{diagnostic:{...event.diagnostic,message_key:'native.future'}}],
 ['missing params',{diagnostic:{schema_version:1,message_key:'native.endpoint.rejected'}}],['extra params',{diagnostic:{...event.diagnostic,message_params:{status:429,secret:'private'}}}],
 ['fractional params',{diagnostic:{...event.diagnostic,message_params:{status:429.1}}}],
])('preserves raw evidence for %s',async(_name,patch)=>{
 const value={...event,...patch} as ToolEvent;
 await act(async()=>root.render(<NativeToolMessage event={value}/>));await act(async()=>{await setLocale('zh-CN',{persist:false});});
 expect(host.textContent).toBe(value.message);expect(host.querySelector('details')).toBeNull();expect(host.querySelector('script')).toBeNull();
});

it('keeps raw evidence while the catalog loads and re-renders only presentation when it arrives',async()=>{
 const messages=[{key:'native.endpoint.rejected',parameters:[{name:'status',kind:'integer'}],texts:{'en-US':'Endpoint rejected request (HTTP {{.status}})','zh-CN':'端点拒绝请求（HTTP {{.status}}）'}}];
 const catalog={version:'native-late',template_format:'named-dot-v1',default_locale:'zh-CN',supported_locales:['en-US','zh-CN'],messages};
 setServerCatalog({...catalog,messages:[]});await setLocale('zh-CN',{persist:false});
 await act(async()=>root.render(<NativeToolMessage event={event}/>));
 expect(host.textContent).toBe(event.message);expect(host.querySelector('details')).toBeNull();
 await act(async()=>{setServerCatalog(catalog);});
 expect(host.querySelector('p')?.textContent).toBe('端点拒绝请求（HTTP 429）');
 expect(host.querySelector('pre')?.textContent).toBe(event.message);
});
