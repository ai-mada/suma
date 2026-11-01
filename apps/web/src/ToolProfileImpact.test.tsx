// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,expect,it,vi} from 'vitest';
import type {ToolProfileImpact as Impact} from '../../../packages/contracts/src';
import {ToolProfileImpact} from './ToolProfileImpact';
import {setLocale} from './i18n';
const item={run_id:'run-a',node_id:'develop-a',run_state:'running',node_state:'running',worker_id:'worker-a',attempt:2,fence:7};
const snapshot=():Impact=>({scope:'active_project_runs',observed_at:'2026-11-01T10:00:00Z',coverage:'complete',runs_examined:2,nodes_examined:8,observed_reference_count:1,items:[{...item}],items_truncated:false,reason:''});
let host:HTMLDivElement,root:Root|undefined;
afterEach(async()=>{if(root)await act(async()=>root!.unmount());root=undefined;host?.remove();vi.unstubAllGlobals();vi.restoreAllMocks();});
async function mount(){await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);root=createRoot(host);}
it('T193 shows only the bounded complete project observation and explicitly opens its exact run and node',async()=>{
 await mount();const onRun=vi.fn(),fetch=vi.fn();vi.stubGlobal('fetch',fetch);const value=snapshot();await act(async()=>root!.render(<ToolProfileImpact impact={value} onRun={onRun}/>));
 expect(host.querySelector('[data-tool-profile-impact]')?.getAttribute('data-coverage')).toBe('complete');expect(host.querySelector('[data-impact-summary]')?.textContent).toContain('1 reference');expect(host.querySelector('[data-impact-summary]')?.textContent).toContain('active runs in this project');const details=host.querySelector('details')!;expect(details.open).toBe(false);expect(host.querySelector('time')?.dateTime).toBe(value.observed_at);
 const open=host.querySelector<HTMLButtonElement>('[data-impact-run="run-a"][data-impact-node="develop-a"]')!;expect(open).not.toBeNull();expect(onRun).not.toHaveBeenCalled();await act(async()=>open.click());expect(onRun).toHaveBeenCalledExactlyOnceWith('run-a','develop-a');expect(fetch).not.toHaveBeenCalled();
 await act(async()=>root!.render(<ToolProfileImpact impact={value}/>));expect(host.querySelector('[data-impact-run]')).toBeNull();expect(host.textContent).toContain('run-a');expect(host.textContent).toContain('develop-a');
 const empty={...value,runs_examined:0,nodes_examined:0,observed_reference_count:0,items:[]};await act(async()=>root!.render(<ToolProfileImpact impact={empty}/>));expect(host.querySelector('[data-impact-summary]')?.textContent).toBe('No references observed in active runs in this project.');expect(host.textContent).toContain('drafts');expect(host.textContent).toContain('external processes');
});
it('T193 keeps partial, unknown and malformed impact neutral without claiming zero dependencies or fabricating a run action',async()=>{
 await mount();const onRun=vi.fn();const value=snapshot();
 for(const impact of [undefined,{...value,coverage:'unknown',reason:'source_unavailable',observed_reference_count:0,items:[]},{...value,coverage:'partial',reason:'run_limit',runs_examined:200,observed_reference_count:0,items:[]},{...value,scope:'all_history'},{...value,observed_reference_count:0},{...value,items:[{...item}, {...item}]},{...value,items:[{...item,node_state:'succeeded'}]},{...value,observed_at:'bad'},{...value,runs_examined:201},{...value,nodes_examined:2001},{...value,token:'private'}]){
  await act(async()=>root!.render(<ToolProfileImpact impact={impact as Impact} onRun={onRun}/>));const summary=host.querySelector('[data-impact-summary]')?.textContent;expect(summary).toBeTruthy();expect(summary).not.toMatch(/No references|\b0\b|No impact/);expect(host.querySelector('[data-impact-run]')).toBeNull();
 }
 await act(async()=>root!.render(<ToolProfileImpact impact={{...value,coverage:'partial',reason:'node_limit',nodes_examined:2000}} onRun={onRun}/>));expect(host.querySelector('[data-impact-summary]')?.textContent).toContain('1 reference');expect(host.querySelector('[data-impact-summary]')?.textContent).toContain('not fully checked');expect(host.textContent).toContain('node limit');
 await act(async()=>setLocale('zh-CN',{persist:false}));expect(host.querySelector('[data-impact-summary]')?.textContent).toContain('尚未核对');expect(onRun).not.toHaveBeenCalled();
});
