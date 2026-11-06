// @vitest-environment jsdom
import {act} from 'react';
import {createRoot} from 'react-dom/client';
import {expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import type {Session} from './api';
import {setLocale} from './i18n';
import {UsageAttemptFacts} from './UsageAttemptFacts';

const session:Session={user:{id:'user-a',name:'Owner',email:'owner@example.test'},organization:{id:'org-a',name:'Team'},projects:[]};
const project='project-a';
const at='2026-11-06T02:00:00Z';
const digest='a'.repeat(64);
const observation={id:'additional',kind:'result',created_at:at,identity_sha256:digest,original_result:false,usage:{input_tokens:0},usage_state:'partial'};
const page=(attempt:string,items:unknown[],next_cursor='')=>({project_id:project,attempt_id:attempt,items,next_cursor});

it('loads only the expanded attempt, pages its evidence, and clears private facts when access changes',async()=>{
 await setLocale('en-US',{persist:false});
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);
 const host=document.createElement('div');
 document.body.append(host);
 const root=createRoot(host);
 const requests:{url:URL;init?:RequestInit}[]=[];
 let reply=async():Promise<Response>=>Response.json(page('attempt-a',[observation],'additional'));
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{requests.push({url:new URL(input,location.href),init});return reply();}));
 async function settle(){for(let i=0;i<3;i++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
 async function render(attempt:string){
  await act(async()=>root.render(<ApiViewProvider session={session}><UsageAttemptFacts project={project} attempt={attempt}/></ApiViewProvider>));
  await settle();
 }
 async function click(selector:string){
  const node=host.querySelector<HTMLElement>(selector);
  expect(node,selector).not.toBeNull();
  await act(async()=>node!.click());
  await settle();
 }
 try{
  await render('attempt-a');
  expect(requests).toHaveLength(0);
  await click('[data-usage-facts-toggle]');
  expect(requests[0].url.pathname).toBe('/api/v1/usage/attempts/attempt-a/facts');
  expect(Object.fromEntries(requests[0].url.searchParams)).toEqual({project_id:project,limit:'20'});
  expect(requests[0].init?.method).toBe('GET');
  expect(new Headers(requests[0].init?.headers).get('X-Suma-Expected-User')).toBe('user-a');
  expect(host.textContent).toContain('Additional execution receipt');
  expect(host.querySelector('[data-usage-fact="result"] dd')?.textContent).toBe('0');
  expect(host.textContent).toContain('Unknown / not precisely reported');

  reply=async()=>Response.json(page('attempt-a',[{...observation,id:'confirmation',kind:'confirmation',usage_state:'complete',usage:{input_tokens:12,output_tokens:2}}],'confirmation'));
  await click('[data-usage-facts-earlier]');
  expect(requests.at(-1)?.url.searchParams.get('before')).toBe('additional');
  expect(host.textContent).toContain('Manual usage confirmation');
  expect(host.querySelectorAll('[data-usage-fact]')).toHaveLength(2);

  let release!:(response:Response)=>void;
  reply=()=>new Promise(resolve=>{release=resolve;});
  await click('[data-usage-facts-earlier]');
  const original=requests.at(-1)?.init?.signal;
  await render('attempt-b');
  expect(original?.aborted).toBe(true);
  await act(async()=>release(Response.json(page('attempt-a',[{...observation,id:'original',original_result:true}]))));
  await settle();
  expect(host.querySelector('[data-usage-fact]')).toBeNull();

  reply=async()=>Response.json({error:{code:'forbidden',message:'Forbidden'}},{status:403});
  await click('[data-usage-facts-toggle]');
  expect(host.textContent).toContain('Usage evidence is unavailable for this attempt.');
  expect(host.querySelector('[data-usage-fact]')).toBeNull();
  await act(async()=>setLocale('zh-CN',{persist:false}));
  expect(host.textContent).toContain('当前无法访问本次用量证据');
  expect(requests).toHaveLength(4);
 }finally{
  await act(async()=>root.unmount());
  host.remove();vi.unstubAllGlobals();await setLocale('zh-CN',{persist:false});
 }
});
