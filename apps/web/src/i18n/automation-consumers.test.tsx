// @vitest-environment jsdom
import {act, type ReactNode} from 'react';
import {createRoot, type Root} from 'react-dom/client';
import {beforeEach, afterEach, expect, it, vi} from 'vitest';
import {AutomationWorkspace} from '../AutomationWorkspace';
import {AutomationIngressWorkspace, AutomationIngressDeliveryRow, AutomationIngressRevisionFacts} from '../AutomationIngress';
import {setLocale} from './index';
import type {Session} from '../api';
import type {AutomationRule} from '../automation-model';
import type {AutomationIngress, AutomationIngressDelivery} from '../ingress-model';

const project='project-original';
const session={user:{id:'user',name:'Test owner',email:'test@example.test',role:'owner'},organization:{id:'org',name:'Test organization'},projects:[{id:project,version:1,role:'admin'}]} as Session;
const rule:AutomationRule={id:'rule-original',project_id:project,name:'用户规则原名 <code>',workflow_id:'flow-original',workflow_version:2,version:8,revision:3,trigger:'cron',cron:'0 9 * * 1-5',timezone:'Asia/Shanghai',misfire:'catch_up',catch_up_limit:4,overlap:'queue',state:'enabled',activated_by:'user',next_at:'2026-10-24T05:00:00Z',last_error:'',created_at:'2026-10-24T03:04:05Z',updated_at:'2026-10-24T04:05:06Z'};
const entry:AutomationIngress={id:'ingress-original',project_id:project,rule_id:rule.id,rule_revision:3,version:5,revision:2,name:'用户入口原名',provider:'github',events:['push'],repository:'原始仓库/repo',repository_id:'42',ref:'refs/heads/main',conclusions:[],state:'paused',activated_by:'',key_configured:false,key_generation:0,key_fingerprint:'',endpoint_path:'/api/v1/hooks/ingress-original',created_at:rule.created_at,updated_at:rule.updated_at,last_error:''};
const delivery:AutomationIngressDelivery={id:'delivery-original',ingress_id:entry.id,ingress_revision:2,rule_id:rule.id,rule_revision:3,delivery_id:'source-original',provider:'github',body_sha256:'a'.repeat(64),key_generation:1,received_at:rule.created_at,last_received_at:rule.created_at,duplicate_count:1000,state:'blocked',reason:'activation_authority_revoked',occurrence_id:'occ-original',occurrence_state:'blocked',run_id:'',event:{schema_version:1,event:'push',repository:'原始仓库/repo',repository_id:'42',ref:'refs/heads/main',commit:'b'.repeat(40),conclusion:'原始结论 <strong>',external_id:'source-original'}};
let host:HTMLDivElement,root:Root|undefined,writes:{path:string;raw:string;body:Record<string,unknown>}[],reads:string[],unknown:boolean,denied:boolean,empty:boolean;
beforeEach(async()=>{
 localStorage.clear();sessionStorage.clear();writes=[];reads=[];unknown=false;denied=false;empty=false;
 await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);
 host=document.createElement('div');document.body.append(host);
 vi.stubGlobal('fetch',vi.fn(async(input:string,options?:RequestInit)=>{
  const path=new URL(String(input),location.href).pathname.replace(/^\/api\/v1/,'');
  if(options?.body){const raw=String(options.body),body=JSON.parse(raw);writes.push({path,raw,body});if(unknown)throw new TypeError('original transport loss');if(denied)return new Response(JSON.stringify({error:{code:'forbidden',message:'原始服务诊断'}}),{status:403});return new Response(JSON.stringify(path.includes('ingress')?{...entry,key_configured:true,key_generation:1,version:6}:{rule}),{status:200});}
  reads.push(path);if(denied)return new Response(JSON.stringify({error:{code:'forbidden',message:'原始服务诊断'}}),{status:403});
  let value:unknown;
  if(path==='/automations')value={items:empty?[]:[rule],total:empty?0:1};
  else if(path==='/automations/'+rule.id)value=rule;
  else if(path.endsWith('/ingresses'))value={items:empty?[]:[entry],total:empty?0:1};
  else if(path==='/automation-ingresses/'+entry.id)value=entry;
  else if(path.endsWith('/deliveries')||path.endsWith('/occurrences'))value={items:[],total:0};
  else if(path==='/workflows')value={items:[{id:'flow-original',name:'用户流程原名',version:2,nodes:[],edges:[]}],total:1};
  else throw Error('Unexpected automation fixture '+path);
  return new Response(JSON.stringify(value),{status:200});
 }));
});
afterEach(async()=>{if(root)await act(async()=>root?.unmount());root=undefined;host.remove();await setLocale('zh-CN',{persist:false});vi.unstubAllGlobals();vi.restoreAllMocks();});
async function settle(){for(let i=0;i<5;i++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function render(node:ReactNode){await act(async()=>{root=createRoot(host);root.render(node);});await settle();}
async function switchTo(locale:string){await act(async()=>{await setLocale(locale,{persist:false});});await settle();}
async function click(text:string){const item=[...document.querySelectorAll<HTMLButtonElement>('button')].find(button=>button.textContent?.trim()===text);expect(item,'button '+text).toBeDefined();await act(async()=>item!.click());await settle();}
async function change(input:HTMLInputElement,value:string){await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(input,value);input.dispatchEvent(new Event('input',{bubbles:true}));});}
const workspace=()=> <AutomationWorkspace project={project} session={session} selected={rule.id} search="" onSearch={()=>{}} refresh={0} onSelect={()=>{}}/>;
const ingress=()=> <AutomationIngressWorkspace project={project} session={session} rule={rule} refresh={0} panel={'ingress:'+entry.id}/>;

it('switches real automation facts and its dirty policy dialog without reading or writing again',async()=>{
 await render(workspace());expect(host.textContent).toContain('Automation');expect(host.textContent).toContain('Bounded catch-up');
 await click('Edit policy');const input=document.querySelector<HTMLInputElement>('[role=dialog] input')!;await change(input,'用户草稿 <b>');
 const count=reads.length;await switchTo('zh-CN');expect(input.isConnected).toBe(true);expect(input.value).toBe('用户草稿 <b>');expect(document.body.textContent).toContain('编辑规则策略');expect(document.body.textContent).toContain('用户规则原名 <code>');expect(reads).toHaveLength(count);expect(writes).toHaveLength(0);
 await click('保存并暂停规则');expect(writes[0].body).toMatchObject({project_id:project,action:'revise',expected_version:8,spec:{name:'用户草稿 <b>',workflow_id:'flow-original',workflow_version:2,cron:'0 9 * * 1-5',timezone:'Asia/Shanghai',misfire:'catch_up',catch_up_limit:4,overlap:'queue'}});
});
it('keeps the exact unknown automation operation and relocalizes its later refusal without another automatic send',async()=>{
 await render(workspace());unknown=true;await click('Trigger once');expect(writes).toHaveLength(1);const raw=writes[0].raw,count=reads.length;
 await switchTo('zh-CN');expect(host.textContent).toContain('自动化操作结果尚未确认');expect(writes).toHaveLength(1);expect(reads).toHaveLength(count);
 unknown=false;denied=true;await click('恢复原操作回执');expect(writes[1].raw).toBe(raw);expect(host.textContent).toContain('当前账号没有访问或执行此操作的权限');
 await switchTo('en-US');expect(host.textContent).toContain('Your account does not have permission');expect(host.textContent).toContain('The original operation still needs reconciliation');expect(writes).toHaveLength(2);
});
it('keeps the original signing text in the rotation dialog and replays the same unknown signature body',async()=>{
 await render(ingress());await click('Set signing key');await click('Securely generate a new key');const secret=document.querySelector<HTMLTextAreaElement>('[role=dialog] textarea')!;const original=secret.value;expect(original).toMatch(/^[a-f0-9]{64}$/);
 const count=reads.length;await switchTo('zh-CN');expect(secret.isConnected).toBe(true);expect(secret.value).toBe(original);expect(reads).toHaveLength(count);
 const check=document.querySelector<HTMLInputElement>('[role=dialog] input[type=checkbox]')!;await act(async()=>check.click());unknown=true;await act(async()=>document.querySelector<HTMLFormElement>('[role=dialog] form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));await settle();expect(writes).toHaveLength(1);expect(writes[0].body.secret).toBe(original);
 expect([...Array.from({length:sessionStorage.length},(_,i)=>sessionStorage.getItem(sessionStorage.key(i)!))].join('')).not.toContain(original);
 const raw=writes[0].raw;await switchTo('en-US');expect(writes).toHaveLength(1);expect(secret.value).toBe(original);unknown=false;await click('Recover original receipt');expect(writes[1].raw).toBe(raw);expect(document.body.textContent).toContain('New key saved; ingress remains paused');
});
it('updates permission and unread failures without turning them into confirmed empty data or refetching',async()=>{
 denied=true;await render(workspace());expect(host.textContent).toContain('Your account does not have permission');expect(host.textContent).not.toContain('No automation rules yet');const count=reads.length;await switchTo('zh-CN');expect(host.textContent).toContain('当前账号没有访问或执行此操作的权限');expect(reads).toHaveLength(count);expect(writes).toHaveLength(0);
});
it('translates real empty and read-only ingress states in place',async()=>{
 empty=true;const viewer={...session,user:{...session.user,role:'member'},projects:[{id:project,version:1,role:'viewer'}]} as Session;
 await render(<AutomationIngressWorkspace project={project} session={viewer} rule={rule} refresh={0}/>);expect(host.textContent).toContain('No event ingresses yet');expect(host.textContent).toContain('Project write and approval permissions');const count=reads.length;await switchTo('zh-CN');expect(host.textContent).toContain('尚无事件入口');expect(host.textContent).toContain('入口写入与启用均需要项目写入及审批权限');expect(reads).toHaveLength(count);expect(writes).toHaveLength(0);
});
it('relocalizes delivery decisions and counts while preserving signed source evidence and identifiers',async()=>{
 await render(<AutomationIngressDeliveryRow delivery={delivery} project={project} timezone="Asia/Shanghai"/>);expect(host.textContent).toContain('1,000');expect(host.textContent).toContain('The ingress or rule activator');expect(host.textContent).toContain('原始结论 <strong>');await switchTo('zh-CN');expect(host.textContent).toContain('入口或规则启用人');expect(host.textContent).toContain('source-original');expect(host.textContent).toContain('原始结论 <strong>');expect(host.querySelector('code')?.textContent).toBe('activation_authority_revoked');expect(writes).toHaveLength(0);expect(reads).toHaveLength(0);
});

it('keeps an ingress filter draft and its machine event selection while translating event choices',async()=>{
 await render(ingress());await click('Edit source');
 expect(document.querySelector('[role=dialog]')?.textContent).toContain('Repository push');
 expect(document.querySelector('[role=dialog]')?.textContent).not.toContain('{{id}}');
 const input=document.querySelector<HTMLInputElement>('[role=dialog] input')!;await change(input,'用户过滤草稿 <b>');
 const checked=document.querySelector<HTMLInputElement>('[role=dialog] input[type=checkbox]')!;expect(checked.checked).toBe(true);
 const count=reads.length;await switchTo('zh-CN');expect(document.querySelector('[role=dialog]')?.textContent).toContain('仓库推送');expect(input.value).toBe('用户过滤草稿 <b>');expect(checked.isConnected).toBe(true);expect(checked.checked).toBe(true);expect(reads).toHaveLength(count);
 await click('保存并暂停入口');expect(writes[0].body).toMatchObject({project_id:project,action:'revise',expected_version:5,expected_rule_version:8,expected_rule_revision:3,spec:{name:'用户过滤草稿 <b>',provider:'github',events:['push'],repository:'原始仓库/repo',repository_id:'42',ref:'refs/heads/main',conclusions:[]}});
});

it('relocalizes a displayed local validation error while retaining the invalid draft and avoiding requests',async()=>{
 await render(workspace());await click('Edit policy');
 const zone=document.querySelector<HTMLInputElement>('[role=dialog] input[list=automation-timezones]')!;await change(zone,'original/invalid-zone');
 const count=reads.length;await click('Save and pause rule');expect(document.body.textContent).toContain('Enter a valid IANA time zone');
 await switchTo('zh-CN');expect(document.body.textContent).toContain('填写有效的 IANA 时区');expect(zone.isConnected).toBe(true);expect(zone.value).toBe('original/invalid-zone');expect(reads).toHaveLength(count);expect(writes).toHaveLength(0);
});

it('retains only a fingerprint after reload and never resends a key whose original text has left memory',async()=>{
 sessionStorage.setItem('suma-ingress-recovery-v1:org:user:'+project+':'+rule.id,JSON.stringify({path:'/automation-ingresses/ingress-original/commands',body:{project_id:project,action:'rotate_key',expected_version:5},idempotency_key:'original-operation',fingerprint:'a'.repeat(64)}));
 await render(ingress());expect(host.textContent).toContain('The signing key has left memory');expect(host.textContent).toContain('original-operation');const count=reads.length;
 await switchTo('zh-CN');expect(host.textContent).toContain('当次签名密钥已离开内存');expect(host.textContent).toContain('original-operation');expect(writes).toHaveLength(0);expect(reads).toHaveLength(count);
 await click('读取最新元数据核对');expect(host.textContent).toContain('尚不能确认原密钥已生效');expect(writes).toHaveLength(0);
});

it('keeps rule creation and update times distinguishable to the second on the same day across locale changes',async()=>{
 await render(workspace());
 const audit=host.querySelector('.automation-detail .automation-technical')!;
 const created=audit.querySelectorAll('dd')[2],updated=audit.querySelectorAll('dd')[3];
 // The rule audit uses the device zone. Derive only the clock fields from the
 // fixed RFC3339 evidence, independently of the production formatter.
 const clock=(raw:string)=>{const date=new Date(raw);return [date.getHours(),date.getMinutes(),date.getSeconds()].map(part=>String(part).padStart(2,'0')).join(':');};
 expect(created.textContent).toContain(clock('2026-10-24T03:04:05Z'));
 expect(updated.textContent).toContain(clock('2026-10-24T04:05:06Z'));
 expect(created.textContent).not.toBe(updated.textContent);
 const count=reads.length;await switchTo('zh-CN');
 expect(created.isConnected).toBe(true);expect(updated.isConnected).toBe(true);
 expect(created.textContent).toContain(clock('2026-10-24T03:04:05Z'));
 expect(updated.textContent).toContain(clock('2026-10-24T04:05:06Z'));
 expect(reads).toHaveLength(count);expect(writes).toHaveLength(0);
});

it('shows source revision clocks in UTC and retains exact RFC3339 evidence without refetching on a locale switch',async()=>{
 const original='2026-10-24T03:04:05Z',later='2026-10-24T04:05:06Z';
 await render(<>{workspace()}<div className="revision-clock-evidence"><AutomationIngressRevisionFacts revision={{revision:1,rule_revision:3,spec:entry,created_at:original}}/><AutomationIngressRevisionFacts revision={{revision:2,rule_revision:3,spec:entry,created_at:later}}/></div></>);
 const times=[...host.querySelectorAll<HTMLTimeElement>('.revision-clock-evidence time')];
 expect(times[0].textContent).toContain('03:04:05');expect(times[1].textContent).toContain('04:05:06');
 expect(times.map(time=>time.dateTime)).toEqual([original,later]);expect(times.map(time=>time.title)).toEqual([original,later]);
 expect(times.every(time=>time.textContent?.endsWith('UTC'))).toBe(true);
 const count=reads.length;await switchTo('zh-CN');
 expect(times.every(time=>time.isConnected)).toBe(true);
 expect(times[0].textContent).toContain('03:04:05');expect(times[1].textContent).toContain('04:05:06');
 expect(times.map(time=>time.dateTime)).toEqual([original,later]);expect(times.map(time=>time.title)).toEqual([original,later]);
 expect(reads).toHaveLength(count);expect(writes).toHaveLength(0);
});
