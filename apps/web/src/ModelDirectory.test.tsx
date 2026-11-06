// @vitest-environment jsdom
import {act,useState} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ModelDirectory} from './ModelDirectory';
import {setLocale} from './i18n';
import type {ModelExecutionOptions,ToolProfile} from '../../../packages/contracts/src';
const profile:ToolProfile={id:'tool',project_id:'p',worker_id:'runner',name:'Native tool',version:2,state:'enabled',enabled_by:'u',created_at:'',updated_at:'',available:true,reason:'',profile:{command_id:'native',adapter:'suma',protocol:'suma-jsonl-v1',profile_sha256:'a'.repeat(64),executable_sha256:'b'.repeat(64),tool_version:'1',read_only:false}};
const options=():ModelExecutionOptions=>({project_id:'p',tool_profile_id:'tool',tool_profile_version:2,tool_profile_sha256:'a'.repeat(64),observed_at:'2026-10-29T05:00:00Z',supported:true,reason:'',codex_supported:false,codex_reason:'',parameter_limits:{max_steps:128,max_output_tokens:131072},items:[{deployment:{deployment_id:'deploy',revision:3,deployment_sha256:'c'.repeat(64),protocol:'openai-chat-completions-v1',model:'Team model',endpoint_origin:'https://provider.example.test',endpoint_config_sha256:'d'.repeat(64),context_window:8192},cost_state:'unpriced',default_parameters:{max_steps:8,max_output_tokens:4096},input_token_reservation:65536,output_token_reservation:4096}]});
let root:Root,host:HTMLDivElement,reads:string[],response:(url:URL)=>Promise<ModelExecutionOptions|Response>;
let detail:(url:URL)=>Promise<ToolProfile|Response>,list:(url:URL)=>{items:ToolProfile[];total:number;next_cursor?:string};
beforeEach(async()=>{
 await setLocale('zh-CN',{persist:false});
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);root=createRoot(host);
 reads=[];response=async()=>options();detail=async()=>profile;list=()=>({items:[profile],total:1});
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{
  expect(init?.method).toBe('GET');reads.push(String(input));
  const url=new URL(input,'http://localhost');
  const data=url.pathname.endsWith('/model-execution/options')?await response(url):url.pathname.includes('/tool-profiles/')?await detail(url):list(url);
  return data instanceof Response?data:new Response(JSON.stringify(data));
 }));
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();});
async function settle(){await act(async()=>{for(let i=0;i<10;i++)await Promise.resolve();});}
function Directory({initial}:{initial:string}){
 const [selected,setSelected]=useState(initial);
 return <ModelDirectory project="p" refresh={0} selected={selected} onSelect={setSelected}/>;
}
async function render(initial=''){await act(async()=>root.render(<Directory initial={initial}/>));await settle();}
async function select(id='tool'){const input=host.querySelector('select')!;await act(async()=>{Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype,'value')!.set!.call(input,id);input.dispatchEvent(new Event('change',{bubbles:true}));});await settle();}
it('loads model options only for an explicitly selected current tool and distinguishes unpriced models',async()=>{await render();expect(reads).toHaveLength(1);await select();expect(reads.at(-1)).toContain('expected_tool_version=2');expect(host.textContent).toContain('Team model');expect(host.textContent).toContain('未定价，不代表免费');expect(host.textContent).toContain('https://provider.example.test');const count=reads.length;await act(async()=>setLocale('en-US',{persist:false}));expect(host.textContent).toContain('Unpriced, not free');expect(reads).toHaveLength(count);});
it('rejects a model catalog for a different tool basis',async()=>{response=async()=>({...options(),tool_profile_version:1});await render();await select();expect(host.textContent).not.toContain('Team model');expect(host.querySelector('[role=alert]')).not.toBeNull();});
it('hides the earlier model catalog during a failed refresh',async()=>{await render();await select();response=async()=>{throw Error('offline');};await act(async()=>[...host.querySelectorAll('button')].find(b=>b.textContent==='重新读取')!.click());await settle();expect(host.textContent).not.toContain('Team model');expect(host.querySelector('[role=alert]')).not.toBeNull();});
it('keeps a real tool model catalog visible when dispatch is blocked by its current budget policy',async()=>{response=async()=>({...options(),supported:false,items:[],codex_supported:false,codex_reason:'budget policy blocks dispatch',codex_catalog:{protocol:'codex-app-server-v1',configuration_sha256:'e'.repeat(64),catalog_sha256:'f'.repeat(64),models:[{model:'codex-team-model',reasoning_efforts:['high'],default_reasoning_effort:'high'}]}});await render();await select();expect(host.textContent).toContain('codex-team-model');expect(host.textContent).toContain('budget policy blocks dispatch');expect(host.textContent).not.toContain('此工具暂无可选模型');});

const fixedBasis={configuration_sha256:'a'.repeat(64),budget_mode:'observe_only',identity_evidence:'configuration_ack_only'};
const fixedModels:[Partial<ModelExecutionOptions>,string][]=[
 [{fixed_opencode:{...fixedBasis,model:'opencode-team',provider_id:'provider',sdk_version:'1.19.4',server_version:'1.19.4',endpoint_origin:'https://provider.example.test'}},'opencode-team'],
 [{fixed_claude:{...fixedBasis,model:'claude-team-5',model_source:'administrator_cli_argument',auth_strategy:'existing_cli_login'}},'claude-team-5'],
 [{fixed_claude_sdk:{...fixedBasis,model:'claude-sdk-team-5',sdk_version:'0.3.314',cli_version:'2.1.314',model_source:'administrator_sdk_option',identity_evidence:'sdk_init_model_ack_only',auth_strategy:'api_key_env_ref'}},'claude-sdk-team-5'],
 [{fixed_acp:{...fixedBasis,model:'cursor-team',runtime:'cursor',sdk_version:'0.13.5',protocol_version:1,model_source:'administrator_session_config',auth_strategy:'explicit_api_key_env',permission_mode:'human_once'}},'cursor-team'],
];
it.each(fixedModels)('displays the fixed model %j',async(fields,model)=>{
 response=async()=>({...options(),parameter_limits:undefined,supported:false,reason:'fixed_by_tool',items:[],...fields});
 await render();await select();
 expect(host.querySelector('article h3')?.textContent??'').toContain(model);
 expect(host.textContent).not.toContain('此工具暂无可选模型');
});
it('rechecks the exact selected source before reading its catalog on refresh',async()=>{
 await render();await select();
 const before=reads.length;
 let resolveDetail!:(value:ToolProfile)=>void;
 detail=()=>new Promise(resolve=>{resolveDetail=resolve;});
 response=async()=>({...options(),tool_profile_version:3});
 await act(async()=>[...host.querySelectorAll('button')].find(button=>button.textContent==='重新读取')!.click());
 expect(host.textContent).not.toContain('Team model');
 expect(reads.slice(before).filter(path=>path.includes('/model-execution/'))).toEqual([]);
 await act(async()=>resolveDetail({...profile,version:3}));await settle();
 expect(reads.at(-1)).toContain('expected_tool_version=3');
 expect(host.textContent).toContain('Team model');
});
it('rejects an exact source response belonging to a different project',async()=>{
 detail=async()=>({...profile,project_id:'elsewhere'});
 await render();await select();
 expect(reads.some(path=>path.includes('/model-execution/'))).toBe(false);
 expect(host.querySelector('[role=alert]')).not.toBeNull();
});
it('clears model facts when a selected source is revoked',async()=>{
 await render();await select();
 const before=reads.length;
 detail=async()=>({...profile,state:'revoked'});list=()=>({items:[],total:0});
 await act(async()=>[...host.querySelectorAll('button')].find(button=>button.textContent==='重新读取')!.click());await settle();
 expect(reads.slice(before).some(path=>path.includes('/model-execution/'))).toBe(false);
 expect(host.textContent).not.toContain('Team model');
 expect(host.querySelector('[role=alert]')).not.toBeNull();
});

it('opens a linked source independently of the loaded source page',async()=>{
 list=()=>({items:[{...profile,id:'first',name:'First tool'}],total:2,next_cursor:'next'});
 await render('tool');
 expect(host.querySelector('select')?.value).toBe('tool');
 expect(host.textContent).toContain('Team model');
 expect(host.querySelector('a[href*="record=tool"]')?.getAttribute('href')).toContain('project=p&view=settings&sub=tools&record=tool');
});

it('ignores the previous catalog when switching sources during a read',async()=>{
 let resolveEarlier!:(value:ModelExecutionOptions)=>void;
 const second={...profile,id:'second',name:'Second tool'};
 list=()=>({items:[profile,second],total:2});
 detail=async url=>url.pathname.endsWith('/second')?second:profile;
 response=url=>url.searchParams.get('tool_profile_id')==='tool'?
  new Promise(resolve=>{resolveEarlier=resolve;}):
  Promise.resolve({...options(),tool_profile_id:'second',items:[],supported:false,parameter_limits:undefined});
 await render();await select();await select('second');
 await act(async()=>resolveEarlier(options()));await settle();
 expect(host.querySelector('select')?.value).toBe('second');
 expect(host.textContent).toContain('此工具未公开模型目录');
 expect(host.textContent).not.toContain('Team model');
});

it('does not restore an old catalog when quickly returning to its source',async()=>{
 const second={...profile,id:'second',name:'Second tool'};
 list=()=>({items:[profile,second],total:2});
 await render();await select();
 expect(host.textContent).toContain('Team model');
 let resolveSecond!:(value:ToolProfile)=>void,resolveAgain!:(value:ToolProfile)=>void;
 detail=url=>url.pathname.endsWith('/second')?
  new Promise(resolve=>{resolveSecond=resolve;}):
  new Promise(resolve=>{resolveAgain=resolve;});
 await select('second');await select();
 expect(host.textContent).not.toContain('Team model');
 await act(async()=>resolveSecond(second));
 expect(host.textContent).not.toContain('Team model');
 await act(async()=>resolveAgain(profile));await settle();
 expect(host.textContent).toContain('Team model');
});

it('clears the catalog and source options when current project access is refused',async()=>{
 await render();await select();
 detail=async()=>new Response(JSON.stringify({error:{code:'forbidden',message:'Access was revoked'}}),{status:403});
 await act(async()=>[...host.querySelectorAll('button')].find(button=>button.textContent==='重新读取')!.click());await settle();
 expect(host.textContent).not.toContain('Team model');
 expect(host.textContent).not.toContain('Native tool');
 expect(host.querySelector('[role=alert]')).not.toBeNull();
});

const pricedOptions=():ModelExecutionOptions=>{
 const page=options(),item=page.items[0],d=item.deployment;
 item.cost_state='priced';
 item.rate={id:'rate-2',version:2,project_id:'p',tool_profile_id:'tool',profile_sha256:profile.profile.profile_sha256,model:d.model,endpoint_origin:d.endpoint_origin,
  model_deployment_id:d.deployment_id,model_deployment_revision:d.revision,model_deployment_sha256:d.deployment_sha256,
  currency:'USD',input_rate_micros_per_million:1_125_000,output_rate_micros_per_million:5_000_001,
  input_token_reservation:1,output_token_reservation:4096,reason:'rate update',created_at:page.observed_at,created_by:'u'};
 return page;
};
it('shows only rates that match the exact deployment and preserves the micros precision',async()=>{
 response=async()=>pricedOptions();await render();await select();
 expect(host.textContent).toContain('1.125 USD / 百万 Token');
 expect(host.textContent).toContain('5.000001 USD / 百万 Token');
 response=async()=>{const page=pricedOptions();page.items[0].rate!.model_deployment_revision=2;return page;};
 await act(async()=>[...host.querySelectorAll('button')].find(button=>button.textContent==='重新读取')!.click());await settle();
 expect(host.textContent).not.toContain('5.000001 USD');
 expect(host.querySelector('[role=alert]')).not.toBeNull();
});
