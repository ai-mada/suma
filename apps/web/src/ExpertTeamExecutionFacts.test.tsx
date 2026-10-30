// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {beforeEach,afterEach,expect,it,vi} from 'vitest';
import {ExpertTeamExecutionFacts} from './ExpertTeamExecutionFacts';
import {setLocale} from './i18n';
import type {TeamExecutionFact} from './ExpertTeamExecutionFactsModel';
let host:HTMLDivElement,root:Root;
const node=(node_id:string,state:string,attempt=1)=>({node_id,name:node_id,type:'agent',state,reason:'',attempt,worker_id:'runner-a',ready_at:''});
const snapshot:TeamExecutionFact={run_id:'run-a',observed_at:'2026-10-30T00:00:00Z',nodes_total:30,truncated:true,nodes:[node('failure','failed',2),node('review','waiting',0),node('code','running'),node('done','succeeded'),...Array.from({length:20},(_,i)=>node('remaining-'+i,'pending',0))]};
beforeEach(async()=>{await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);root=createRoot(host);});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();});
async function render(execution:unknown=snapshot,runID='run-a'){await act(async()=>root.render(<ExpertTeamExecutionFacts project="project-a" runID={runID} execution={execution}/>));}
it('shows a bounded summary, reveals remaining steps and links exact observed attempts without write controls',async()=>{
 await render();expect(host.textContent).toContain('24 of 30');expect(host.textContent).toContain('Observed');expect(host.querySelectorAll('.team-execution-facts-focus > li')).toHaveLength(3);
 const details=host.querySelector<HTMLDetailsElement>('details[data-execution-more]')!;expect(details.open).toBe(false);await act(async()=>details.querySelector('summary')!.click());expect(details.open).toBe(true);expect(details.textContent).toContain('done');
 const href=host.querySelector<HTMLAnchorElement>('[data-execution-node="failure"] a')!.href;expect(new URL(href).searchParams.get('attempt')).toBe('2');expect(host.querySelector('button')).toBeNull();
});
it('retains unrecognized reason as plain evidence and treats unknown state as unconfirmed',async()=>{
 await render({...snapshot,nodes_total:1,truncated:false,nodes:[{...node('unusual','future_state'),reason:'<script>do not execute</script>'}]});
 expect(host.textContent).toContain('State needs review');expect(host.textContent).toContain('Reason needs review');expect(host.textContent).toContain('<script>do not execute</script>');expect(host.querySelector('script')).toBeNull();expect(host.querySelector('.team-execution-facts-node[data-tone="success"]')).toBeNull();
});
it('does not reuse execution facts from another run or after facts are cleared',async()=>{
 await render();await render(snapshot,'run-b');expect(host.querySelector('[data-execution-node]')).toBeNull();expect(host.textContent).toContain('Step facts are unavailable');await render(undefined,'run-b');expect(host.querySelector('[data-execution-node]')).toBeNull();
});
