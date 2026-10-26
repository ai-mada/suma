// @vitest-environment jsdom
import {act} from 'react';
import {createRoot} from 'react-dom/client';
import {expect,it,vi} from 'vitest';
import {CandidateCodeInputFacts} from './CandidateCodeInputFacts';
import {setLocale} from './i18n';

it('shows passive exact repair lineage and rejects a result that does not match the workspace',async()=>{
 await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);const host=document.createElement('div');document.body.append(host);const root=createRoot(host),head='a'.repeat(40),base='b'.repeat(40),hash='e'.repeat(64);
 const repair={protocol:'suma.candidate-conflict-repair.v1',plan_id:'repair-plan',plan_version:2,composition_id:'original-composition',original_assignment:{run_id:'failed-run',node_id:'compose',attempt:1,fence:2,worker_id:'old-worker'},original_evidence_sha256:hash,sources_sha256:hash,operation_id:'repair-operation',state:'completed',steps:[{source_index:1,left_commit:head,right_commit:base,seed_tree:head,conflict_sha256:hash,result_commit:head,result_tree:head,parents:[head,base]}],result_commit:head,result_tree:head,parents:[head,base],source_heads:[head,base],merge_commits:[head]};
 try{await act(async()=>root.render(<CandidateCodeInputFacts project="project-a" evidence={{admin_base_commit:base,base_commit:base,head_commit:head,candidate_conflict_repair:repair}}/>));expect(host.textContent).toContain('Recorded conflict repair provenance');expect(host.textContent).toContain('failed-run');expect(host.querySelector('a')?.getAttribute('href')).toContain('run=failed-run');expect(host.querySelector('button')).toBeNull();
  await act(async()=>root.render(<CandidateCodeInputFacts project="project-a" evidence={{admin_base_commit:base,base_commit:base,head_commit:'c'.repeat(40),candidate_conflict_repair:repair}}/>));expect(host.textContent).toContain('The repair facts do not match this workspace');expect(host.textContent).not.toContain('Repair code recorded');
 }finally{await act(async()=>root.unmount());host.remove();await setLocale('zh-CN',{persist:false});vi.unstubAllGlobals();}
});
