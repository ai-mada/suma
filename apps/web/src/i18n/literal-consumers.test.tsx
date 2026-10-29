// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {beforeEach,afterEach,expect,it,vi} from 'vitest';
import {setLocale,i18n} from './index';
import {WorkflowView} from '../Workflow';
import {CandidateSource,ArtifactSourceField} from '../CandidateSource';
import {CapacityView} from '../Planning';

let host:HTMLDivElement,root:Root;
beforeEach(()=>{
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.stubGlobal('ResizeObserver',class{observe(){}unobserve(){}disconnect(){}});vi.stubGlobal('DOMMatrixReadOnly',class{m22=1});
 vi.stubGlobal('fetch',vi.fn(async()=>new Response(JSON.stringify({items:[],total:0}),{headers:{'Content-Type':'application/json'}})));
 host=document.createElement('div');document.body.append(host);root=createRoot(host);
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();});

it.each(['en-US','zh-CN'])('preserves parent graph and upstream raw names through real Workflow in %s',async(locale)=>{
 await setLocale('en-US',{persist:false});await act(async()=>root.render(<WorkflowView project="p" refresh={0} onRefresh={()=>{}} onRun={()=>{}} onNode={()=>{}}/>));
 await act(async()=>[...host.querySelectorAll('button')].find(button=>button.textContent==='Add the first node')!.click());
 const raw='{{id}} {{state}} $t(flow.zoom_in) $& 用户';
 const input=host.querySelector<HTMLInputElement>('input[aria-label="Node name"]')!;
 await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(input,raw);input.dispatchEvent(new Event('input',{bubbles:true}));});
 const node=host.querySelector<HTMLElement>('.react-flow__node')!,id=node.dataset.id!;input.focus();const count=vi.mocked(fetch).mock.calls.length;
 await act(async()=>{await setLocale(locale,{persist:false});});
 expect(node.getAttribute('aria-label')).toBe(locale==='en-US'?`Node ${raw} (${id}): Not yet executed`:`节点 ${raw} (${id})：尚未执行`);
 expect(node.querySelector('strong')?.textContent).toBe(raw);expect(input.value).toBe(raw);expect(document.activeElement).toBe(input);expect(vi.mocked(fetch).mock.calls).toHaveLength(count);
 await act(async()=>{await setLocale('en-US',{persist:false});});
 await act(async()=>[...host.querySelectorAll('button')].find(button=>button.textContent==='Add node')!.click());
 const upstream=host.querySelector<HTMLInputElement>('.dependency-options input[type="checkbox"]')!;
 await act(async()=>{await setLocale(locale,{persist:false});});
 expect(upstream.getAttribute('aria-label')).toBe(locale==='en-US'?`Upstream: ${raw} (${id})`:`上游：${raw} (${id})`);
});

it.each(['en-US','zh-CN'])('recovers graph and Handle labels after missing resources without losing raw draft or focus in %s',async(locale)=>{
 await setLocale('en-US',{persist:false});await act(async()=>root.render(<WorkflowView project="p" refresh={0} onRefresh={()=>{}} onRun={()=>{}} onNode={()=>{}}/>));
 await act(async()=>[...host.querySelectorAll('button')].find(button=>button.textContent==='Add the first node')!.click());
 const raw='{{id}} {{state}} raw',input=host.querySelector<HTMLInputElement>('input[aria-label="Node name"]')!;
 await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(input,raw);input.dispatchEvent(new Event('input',{bubbles:true}));});
 const node=host.querySelector<HTMLElement>('.react-flow__node')!,id=node.dataset.id!,handle=node.querySelector<HTMLElement>('.react-flow__handle')!;
 await act(async()=>{await setLocale(locale,{persist:false});});
 const state=locale==='en-US'?'Not yet executed':'尚未执行',generic=locale==='en-US'?'Connection handle':'连线端点';
 const graphTemplate=i18n.getResource(locale,'translation','development.workflow.graph_node_aria'),handleTemplate=i18n.getResource(locale,'translation','flow.handle.input');
 const count=vi.mocked(fetch).mock.calls.length,position=node.style.transform;input.focus();
 const read=i18n.getResource.bind(i18n);
 const missing=vi.spyOn(i18n,'getResource').mockImplementation((language,namespace,key,options)=>language===locale&&key==='development.workflow.graph_node_aria'?undefined:read(language,namespace,key,options));
 try{
  await act(async()=>{i18n.addResource(locale,'translation','flow.handle.input','{{future}}');i18n.emit('loaded',{});});
  expect(node.getAttribute('aria-label')).toBe(`${raw} · ${id} · ${state}`);expect(handle.getAttribute('aria-label')).toBe(`${generic}: ${raw} (${id})`);
  await act(async()=>{missing.mockRestore();i18n.addResource(locale,'translation','flow.handle.input',handleTemplate);i18n.emit('loaded',{});});
  expect(node.getAttribute('aria-label')).toBe(locale==='en-US'?`Node ${raw} (${id}): ${state}`:`节点 ${raw} (${id})：${state}`);expect(handle.getAttribute('aria-label')).toBe(locale==='en-US'?`Input connection (target) for node ${raw} (${id})`:`节点 ${raw}（${id}）的输入连接点（目标）`);
  expect(host.querySelector('.react-flow__node')).toBe(node);expect(node.querySelector('.react-flow__handle')).toBe(handle);expect(document.activeElement).toBe(input);expect(input.value).toBe(raw);expect(node.style.transform).toBe(position);expect(vi.mocked(fetch).mock.calls).toHaveLength(count);
 }finally{await act(async()=>{missing.mockRestore();i18n.addResource(locale,'translation','development.workflow.graph_node_aria',graphTemplate);i18n.addResource(locale,'translation','flow.handle.input',handleTemplate);i18n.emit('loaded',{});});}
});

const sourceName='{{id}} {{type}} {{command}} $t(flow.zoom_in) $& 用户';
const workflow={id:'w',name:'w',version:1,nodes:[{id:'source',name:sourceName,type:'command',command:['build-command']},{id:'target',name:'Target',type:'test',command:['test-command']}],edges:[{from:'source',to:'target'}]};
const workers=[{project_id:'p',status:'active',expires_at:'2036-01-01T00:00:00Z',profiles:[{id:'build-command',kind:'build',sha256:'a'.repeat(64)}]}];
it.each(['en-US','zh-CN'])('preserves real candidate and artifact option raw values and selection in %s',async(locale)=>{
 await setLocale(locale,{persist:false});const candidate=vi.fn(),artifact=vi.fn();
 await act(async()=>root.render(<><CandidateSource workflow={workflow} nodeID="target" onChange={candidate}/><ArtifactSourceField workflow={workflow} nodeID="target" project="p" workers={workers} loading={false} error="" onRetry={()=>{}} onChange={artifact}/></>));
 const options=[...host.querySelectorAll<HTMLOptionElement>('option[value="source"]')];expect(options).toHaveLength(2);
 expect(options.map(option=>option.textContent)).toEqual([
  `${sourceName} · source (${locale==='en-US'?'Command':'命令'})`,
  `${sourceName} · source · ${locale==='en-US'?'Registered build':'已登记构建'} · build-command`,
 ]);
 for(const option of options)await act(async()=>{option.parentElement!.querySelector('option[value="source"]')!.setAttribute('selected','');(option.parentElement as HTMLSelectElement).value='source';option.parentElement!.dispatchEvent(new Event('change',{bubbles:true}));});
 expect(candidate.mock.calls[0][0]).toMatchObject({candidate_from:'source'});expect(artifact.mock.calls[0][0]).toMatchObject({artifact_from:'source'});expect(fetch).not.toHaveBeenCalled();
});

it.each(['en-US','zh-CN'])('preserves raw resource name in actual capacity modal and keeps its draft in %s',async(locale)=>{
 const raw='{{week}} $t(flow.zoom_in) $& 用户';await setLocale('en-US',{persist:false});
 vi.stubGlobal('fetch',vi.fn(async()=>Response.json({project_id:'p',member_id:'member',version:1,can_edit:true,updated_at:'',schedule:{weekdays:[8,8,8,8,8,0,0],exceptions:[]}})));
 await act(async()=>root.render(<CapacityView project="p" week="2026-10-26" onWeek={()=>true} rows={[{member_id:'member',name:raw,kind:'human',week_start:'2026-10-26',calendar_version:1,days:[],available_hours:40,allocated_hours:0,unestimated_tasks:0,unscheduled_tasks:0,distribution_unknown_tasks:0,distribution_unknown_hours:0}]} ready={true} pending={false} routeError="" onRefresh={()=>{}}/>));expect(fetch).not.toHaveBeenCalled();
 await act(async()=>[...host.querySelectorAll('button')].find(button=>button.textContent==='Edit work calendar')!.click());
 const dialog=document.querySelector<HTMLElement>('[role="dialog"]')!,hours=dialog.querySelector<HTMLInputElement>('input[name="weekday-0"]')!;
 // Calendar fields keep a controlled draft. Dispatch the same native input
 // event as a user edit instead of changing only the DOM's displayed value.
 await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(hours,'12.5');hours.dispatchEvent(new Event('input',{bubbles:true}));});hours.focus();
 await act(async()=>{await setLocale(locale,{persist:false});});
 const paragraph=[...dialog.querySelectorAll('p')].find(p=>p.textContent?.includes('$t(flow.zoom_in)'))!;
 expect(paragraph.textContent).toBe(locale==='en-US'?`${raw} · Week starting Oct 26, 2026`:`${raw} · 2026年10月26日 开始的一周`);
 expect(document.querySelector('[role="dialog"]')).toBe(dialog);expect(document.activeElement).toBe(hours);expect(hours.value).toBe('12.5');expect(fetch).toHaveBeenCalledTimes(1);
 let allowed=true;await act(async()=>{allowed=window.dispatchEvent(new Event('suma:navigate',{cancelable:true}));});expect(allowed).toBe(false);
});
