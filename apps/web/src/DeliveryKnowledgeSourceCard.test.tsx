// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {ProjectKnowledgeSource} from '../../../packages/contracts/src';
import {DeliveryKnowledgeSourceCard} from './DeliveryKnowledgeSourceCard';

vi.mock('./i18n',()=>({useLocale:()=>{},tr:(key:string,values?:Record<string,unknown>)=>key+(values?' '+JSON.stringify(values):'')}));
const source=():ProjectKnowledgeSource=>({kind:'accepted_delivery',id:'task-original',version:4,title:'Accepted fix',quote:'Original source <script>not executable</script>',sha256:'a'.repeat(64),delivery:{protocol:'suma-accepted-delivery-v1',task:{id:'task-original',version:4,run_id:'run-original',review_id:'review-original',sha256:'b'.repeat(64)},run_version:7,sha256:'c'.repeat(64),tests:[{run_id:'run-test',node_id:'node-test',attempt:2,fence:9,worker_id:'worker-test',result_sha256:'d'.repeat(64),report_id:'report-original',report_sha256:'e'.repeat(64)}],candidates:[]}});
let root:Root,host:HTMLDivElement;
beforeEach(()=>{vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);root=createRoot(host);});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.restoreAllMocks();vi.unstubAllGlobals();});
const render=async(value:ProjectKnowledgeSource,disabled=false,onRefresh?:()=>void)=>act(async()=>root.render(<DeliveryKnowledgeSourceCard project="project-current" source={value} disabled={disabled} onRefresh={onRefresh}/>));

it('shows zero candidates honestly and keeps exact test and review facts in collapsed evidence',async()=>{
 await render(source());
 expect(host.textContent).toContain('delivery-knowledge.no_candidates');
 expect(host.textContent).toContain('review-original');expect(host.textContent).toContain('report-original');expect(host.textContent).toContain('worker-test');
 expect(host.querySelectorAll('details')).toHaveLength(2);for(const d of host.querySelectorAll('details'))expect(d.open).toBe(false);
 expect(host.querySelector('[data-delivery-candidates]')).toBeNull();expect(host.querySelector('script')).toBeNull();
 expect(host.querySelector('[data-delivery-quote]')?.textContent).toBe(source().quote);
});
it('preserves actual candidate commit and optional evidence without inventing missing source commit',async()=>{
 const value=source();value.delivery!.candidates=[{kind:'workspace_revision',run_id:'run-original',node_id:'node-code',attempt:1,fence:3,worker_id:'worker-code',head_commit:'f'.repeat(40),evidence_sha256:'1'.repeat(64)}];await render(value);
 expect(host.textContent).toContain('delivery-knowledge.candidate_kind.workspace_revision');expect(host.textContent).not.toContain('delivery-knowledge.candidate_kind.sealed_bundle');expect(host.textContent).not.toContain('delivery-knowledge.transfer_id');
 value.delivery!.candidates[0]={...value.delivery!.candidates[0],kind:'sealed_bundle',transfer_id:'transfer-original',bundle_sha256:'2'.repeat(64)};await render(value);expect(host.textContent).toContain('delivery-knowledge.candidate_kind.sealed_bundle');
 expect(host.textContent).not.toContain('delivery-knowledge.no_candidates');expect(host.querySelector('[data-delivery-candidates]')?.textContent).toContain('f'.repeat(40));expect(host.textContent).toContain('transfer-original');expect(host.textContent).not.toContain('delivery-knowledge.source_commit');
});
it('binds links to current project and exact original IDs while respecting navigation and disabled guards',async()=>{
 const refresh=vi.fn();await render(source(),false,refresh);const a=host.querySelector<HTMLAnchorElement>('[data-delivery-run]')!;
 const query=new URL(a.href).searchParams;expect(query.get('project')).toBe('project-current');expect(query.get('run')).toBe('run-original');expect(query.get('sub')).toBe('runs');
 expect(new URL(host.querySelector<HTMLAnchorElement>('[data-delivery-task]')!.href).searchParams.get('record')).toBe('task-original');
 const block=(e:Event)=>e.preventDefault();window.addEventListener('suma:navigate',block);try{expect(a.dispatchEvent(new MouseEvent('click',{bubbles:true,cancelable:true}))).toBe(false);}finally{window.removeEventListener('suma:navigate',block);}
 await act(async()=>host.querySelector<HTMLButtonElement>('[data-delivery-refresh]')!.click());expect(refresh).toHaveBeenCalledTimes(1);
 await render(source(),true,refresh);expect(a.hasAttribute('href')).toBe(false);expect(a.getAttribute('aria-disabled')).toBe('true');expect(host.querySelector<HTMLButtonElement>('[data-delivery-refresh]')!.disabled).toBe(true);
});
it('does not label incomplete or mismatched source metadata as accepted delivery',async()=>{
 const value=source();delete value.delivery;await render(value);expect(host.textContent).toContain('delivery-knowledge.source_unavailable');expect(host.querySelector('[data-delivery-run]')).toBeNull();
 const mismatched=source();mismatched.delivery!.task.id='other-task';await render(mismatched);expect(host.textContent).not.toContain('review-original');
});
