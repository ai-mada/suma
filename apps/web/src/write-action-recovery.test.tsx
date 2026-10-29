// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import {useWriteAction,WriteButton,WriteFields,WriteNotice,type WriteAction} from './write-action';
import {setLocale} from './i18n';
import type {Session} from './api';
const project='project-a',a:Session={user:{id:'a'.repeat(32),name:'A',email:'a@example.test'},organization:{id:'b'.repeat(32),name:'Org A'},projects:[]},b:Session={...a,user:{...a.user,id:'c'.repeat(32)}};
let root:Root|undefined,host:HTMLDivElement,hook:WriteAction,writes:{body:any;headers:Headers}[],mode:'unknown'|'403'|'409'|'success';
const scope=(session:Session)=>session.user.id+':'+session.organization.id+':'+project;
function Harness({session=a,storage=sessionStorage,validateResponse}:{session?:Session;storage?:Pick<Storage,'getItem'|'setItem'|'removeItem'>;validateResponse?:(value:unknown)=>boolean}){
 hook=useWriteAction({scope:scope(session),storage,validateRequest:request=>request.path==='/project-knowledge'&&request.body.project_id===project,validateResponse});
 return <><WriteNotice action={hook}/><WriteFields action={hook}><input aria-label="draft" defaultValue="edited draft"/></WriteFields><WriteButton action={hook}>Write</WriteButton><output>{hook.unknown?'unknown':'known'}:{hook.pending?.idempotency_key}</output></>;
}
beforeEach(async()=>{await setLocale('en-US',{persist:false});sessionStorage.clear();writes=[];mode='unknown';vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);vi.stubGlobal('fetch',vi.fn(async(_input:string,init?:RequestInit)=>{writes.push({body:JSON.parse(String(init?.body)),headers:new Headers(init?.headers)});if(mode==='unknown')throw new TypeError('lost');if(mode==='403'||mode==='409')return Response.json({error:{code:'refused',message:'Current source is not allowed'}},{status:Number(mode)});return Response.json({id:'knowledge-a',version:1});}));});
afterEach(async()=>{if(root)await act(async()=>root?.unmount());root=undefined;host.remove();await setLocale('zh-CN',{persist:false});vi.unstubAllGlobals();vi.restoreAllMocks();});
async function render(session=a,storage:Pick<Storage,'getItem'|'setItem'|'removeItem'>=sessionStorage,validateResponse?:(value:unknown)=>boolean){await act(async()=>{root??=createRoot(host);root.render(<ApiViewProvider session={session}><Harness session={session} storage={storage} validateResponse={validateResponse}/></ApiViewProvider>);});}
async function run(body:Record<string,unknown>={project_id:project,title:'original body',content:'raw {{text}}'} ){let result:unknown;await act(async()=>{result=await hook.run('/project-knowledge',body);});return result;}

it.each(['403','409'] as const)('retains the exact unknown operation and original view after Provider replacement and %s',async(refusal)=>{
 await render();await run();const original=structuredClone(writes[0]);const key=hook.pending!.idempotency_key;await render(b);mode=refusal;await run({project_id:project,title:'changed body',content:'changed'});expect(hook.unknown).toBe(true);expect(hook.conflict).toBe(false);expect(hook.pending?.idempotency_key).toBe(key);expect(writes[1].body).toEqual(original.body);expect(writes[1].headers.get('X-Suma-Expected-User')).toBe(a.user.id);expect(host.querySelector('input')?.matches(':disabled')).toBe(true);expect(host.textContent).not.toContain('Read latest version and keep draft');
 mode='success';await render(a);await run({project_id:project,title:'third body'});expect(writes[2].body).toEqual(original.body);expect(hook.pending).toBeUndefined();
});

it('restores only the exact user/organization/project scope after remount with the same wire key',async()=>{
 await render();await run();const original=structuredClone(writes[0].body),key=hook.pending!.idempotency_key;await act(async()=>root!.unmount());root=undefined;
 await render(b);expect(hook.pending).toBeUndefined();expect(writes).toHaveLength(1);await act(async()=>root!.unmount());root=undefined;
 await render(a);expect(hook.pending?.idempotency_key).toBe(key);mode='success';await run({project_id:project,title:'new input ignored'});expect(writes[1].body).toEqual(original);expect(hook.pending).toBeUndefined();expect(sessionStorage.getItem('suma-write-recovery-v1:'+scope(a))).toBeNull();
});

it('blocks a new write before transport if recovery storage fails',async()=>{
 const storage={getItem:()=>null,setItem:()=>{throw Error('quota');},removeItem:()=>{}};await render(a,storage);await run();expect(writes).toHaveLength(0);expect(hook.pending).toBeUndefined();expect(hook.error).toContain('No new write was sent');
});

it('cleans an acknowledged local record without repeating its confirmed HTTP write',async()=>{
 let fail=true;const storage={getItem:(key:string)=>sessionStorage.getItem(key),setItem:(key:string,value:string)=>sessionStorage.setItem(key,value),removeItem:(key:string)=>{if(fail)throw Error('cleanup');sessionStorage.removeItem(key);}};
 await render(a,storage);mode='success';expect(await run()).toBeUndefined();expect(writes).toHaveLength(1);expect(hook.unknown).toBe(false);expect(hook.pending).toBeDefined();fail=false;expect(await run()).toMatchObject({id:'knowledge-a'});expect(writes).toHaveLength(1);expect(hook.pending).toBeUndefined();
});

it('keeps scoped recovery when a late success arrives after view change and unmount',async()=>{
 let finish!:(response:Response)=>void;const sent:unknown[]=[];
 vi.stubGlobal('fetch',vi.fn(async(_input:string,init?:RequestInit)=>{sent.push(JSON.parse(String(init?.body)));if(sent.length===1)return new Promise<Response>(resolve=>{finish=resolve;});return Response.json({id:'a-result',version:1});}));
 await render(a);let request!:Promise<unknown>;await act(async()=>{request=hook.run('/project-knowledge',{project_id:project,title:'A accepted operation'});await new Promise(resolve=>setTimeout(resolve,0));});
 const original=structuredClone(sent[0]);await render(b);await act(async()=>root!.unmount());root=undefined;
 await act(async()=>{finish(Response.json({id:'a-result',version:1}));await request;});expect(await request).toBeUndefined();expect(sessionStorage.getItem('suma-write-recovery-v1:'+scope(a))).not.toBeNull();
 await render(a);expect(hook.pending).toBeDefined();expect(await run({project_id:project,title:'new text must not replace recovery'})).toMatchObject({id:'a-result'});expect(sent[1]).toEqual(original);expect(hook.pending).toBeUndefined();expect(sessionStorage.getItem('suma-write-recovery-v1:'+scope(a))).toBeNull();
});

it('retains malformed success as the original uncertain operation and validates the recovered response before clearing it',async()=>{
 const valid=(value:unknown)=>!!value&&typeof value==='object'&&(value as {id?:unknown}).id==='knowledge-a';
 mode='success';await render(a,sessionStorage,valid);vi.mocked(fetch).mockImplementationOnce(async(_input,init)=>{writes.push({body:JSON.parse(String(init?.body)),headers:new Headers(init?.headers)});return Response.json({unrelated:'not the created record'});});
 expect(await run()).toBeUndefined();expect(hook.unknown).toBe(true);expect(hook.pending).toBeDefined();expect(host.querySelector('input')?.matches(':disabled')).toBe(true);
 const original=structuredClone(writes[0].body);expect(sessionStorage.getItem('suma-write-recovery-v1:'+scope(a))).not.toBeNull();
 await act(async()=>root!.unmount());root=undefined;await render(a,sessionStorage,valid);
 expect(await run({project_id:project,title:'must not replace original request'})).toMatchObject({id:'knowledge-a'});expect(writes).toHaveLength(2);expect(writes[1].body).toEqual(original);expect(hook.pending).toBeUndefined();
});

it('keeps the original response validator with an unknown operation across provider and validator changes',async()=>{
 const original=vi.fn((value:unknown)=>!!value&&typeof value==='object'&&(value as {id?:unknown}).id==='knowledge-a'),replacement=vi.fn(()=>false);
 await render(a,sessionStorage,original);await run();const request=structuredClone(writes[0].body);
 await render(b,sessionStorage,replacement);mode='success';expect(await run()).toBeUndefined();expect(hook.pending).toBeDefined();expect(original).not.toHaveBeenCalled();expect(replacement).not.toHaveBeenCalled();
 // A response observed in another view cannot acknowledge the original owner's
 // operation. That owner rereads its exact idempotent request before validation.
 await render(a,sessionStorage,replacement);expect(await run()).toMatchObject({id:'knowledge-a'});expect(writes).toHaveLength(3);expect(hook.pending).toBeUndefined();
 expect(original).toHaveBeenCalledTimes(1);expect(replacement).not.toHaveBeenCalled();for(const write of writes){expect(write.body).toEqual(request);expect(write.headers.get('X-Suma-Expected-User')).toBe(a.user.id);expect(write.headers.get('X-Suma-Expected-Organization')).toBe(a.organization.id);}
});
