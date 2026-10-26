// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {WorkspaceEvidence} from './WorkspaceEvidence';
import type {WorkspaceEvidence as Evidence} from './api';
import {setLocale} from './i18n';

const base='a'.repeat(40),head='b'.repeat(40),sourceDigest='c'.repeat(64),bundleDigest='d'.repeat(64);
const binding={schema_version:1,transfer_id:'transfer-018',project_id:'project-018',state:'sealed',source:{run_id:'run-018',node_id:'code-018',attempt:2,fence:7,worker_id:'source<worker>'},base_commit:base,head_commit:head,evidence_sha256:sourceDigest,bundle_sha256:bundleDigest,size_bytes:32768};
const producer=():Evidence=>({base_commit:base,head_commit:head,evidence_sha256:sourceDigest,diff_sha256:'original-diff',changed_paths:['src/original.ts'],candidate_bundle:structuredClone(binding)});
const consumer=():Evidence=>({base_commit:head,head_commit:head,evidence_sha256:'e'.repeat(64),diff_sha256:'consumer-diff',candidate_from:'code-018',candidate_attempt:2,candidate_fence:7,candidate_evidence_sha256:sourceDigest,candidate_import:{binding:structuredClone(binding),imported:true,target_worker_id:'target<worker>'}});
let host:HTMLDivElement,root:Root|undefined;
beforeEach(async()=>{await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.stubGlobal('fetch',vi.fn());host=document.createElement('div');document.body.append(host);});
afterEach(async()=>{if(root)await act(async()=>root?.unmount());root=undefined;host.remove();await setLocale('zh-CN',{persist:false});vi.unstubAllGlobals();});
async function render(evidence:Evidence){await act(async()=>{root??=createRoot(host);root.render(<WorkspaceEvidence evidence={evidence}/>);});}
function transfer(){return host.querySelector('.candidate-transfer-facts');}

it('separates the administrator bundle anchor from selected development input and verifies inherited import literally',async()=>{
 const selected='e'.repeat(40),inherited={...structuredClone(binding),base_commit:selected,bundle_base_commit:base};const evidence:Evidence={base_commit:selected,admin_base_commit:base,head_commit:head,evidence_sha256:sourceDigest,candidate_bundle:inherited};await render(evidence);expect(transfer()?.textContent).toContain('Selected development input');expect(transfer()?.textContent).toContain('Administrator anchor');expect(transfer()?.textContent).not.toContain('could not be verified');
 const baseline={protocol:'suma.code-baseline.v1',id:'baseline-031',project_id:binding.project_id,task_id:'downstream-031',task_version:4,composition_id:'composition-031',composition_version:5,accepted_basis_sha256:sourceDigest,source:inherited,binding_sha256:bundleDigest};const imported:Evidence={base_commit:head,admin_base_commit:base,head_commit:head,code_baseline:baseline,candidate_import:{binding:inherited,imported:true,target_worker_id:'target<worker>'}};await render(imported);expect(transfer()?.textContent).toContain('Candidate imported for this attempt');expect(host.querySelector('worker')).toBeNull();await render({...imported,admin_base_commit:'f'.repeat(40)});expect(transfer()?.textContent).toContain('could not be verified');expect(transfer()?.textContent).not.toContain('Candidate imported for this attempt');expect(fetch).not.toHaveBeenCalled();
});

it('shows sealing as a source fact without claiming delivery, testing or review',async()=>{
 await render(producer());expect(transfer()?.textContent||'').toContain('Sealed candidate bundle');expect(transfer()?.textContent||'').toContain('source<worker>');expect(transfer()?.textContent||'').toContain(bundleDigest);expect(transfer()?.textContent||'').toContain('32,768 bytes');expect(transfer()?.textContent||'').toContain('No import receipt is recorded here');expect(transfer()?.textContent||'').toContain('Transfer metadata does not prove tests, human acceptance, merging or release.');expect(host.textContent).toContain('original-diff');expect(host.textContent).toContain('src/original.ts');expect(host.querySelector('a,button')).toBeNull();expect(fetch).not.toHaveBeenCalled();
});

it('shows an actual import separately and keeps exact provenance and open disclosure across languages',async()=>{
 await render(consumer());expect(transfer()?.textContent||'').toContain('Candidate imported for this attempt');expect(transfer()?.textContent||'').toContain('target<worker>');expect(transfer()?.textContent||'').not.toContain('Sealed candidate bundle');const disclosure=transfer()!.querySelector('details')!;disclosure.open=true;const raw=[...transfer()!.querySelectorAll('code')].map(item=>item.textContent);await act(async()=>{await setLocale('zh-CN',{persist:false});});expect(transfer()?.textContent||'').toContain('本次尝试已导入候选');expect(disclosure.isConnected).toBe(true);expect(disclosure.open).toBe(true);expect([...transfer()!.querySelectorAll('code')].map(item=>item.textContent)).toEqual(raw);expect(host.querySelector('source,worker,script,a,button')).toBeNull();expect(fetch).not.toHaveBeenCalled();
});

it('does not carry transfer or target-worker facts into another workspace receipt',async()=>{
 await render(consumer());expect(transfer()?.textContent||'').toContain('target<worker>');await render({base_commit:base,head_commit:base,diff_sha256:'other-original-diff'});expect(transfer()).toBeNull();expect(host.textContent).not.toContain(bundleDigest);expect(host.textContent).not.toContain('target<worker>');expect(host.textContent).toContain('other-original-diff');
});

it('shows the complete reported pool source identity as literal facts and resets disclosure for another transfer',async()=>{
 const evidence=producer(),bundle=evidence.candidate_bundle as typeof binding;const poolSource={...bundle.source,tenant_id:'tenant-018',project_id:'project-018',pool_id:'pool-018',pool_runner_id:'pool-runner-018',pool_slot_id:'slot-018',pool_grant_id:'grant-018',pool_grant_version:3,pool_credential_id:'credential-018',pool_credential_version:4,dispatch_id:'dispatch-018',dispatch_state:'running',lease_until:'2026-10-26T10:00:00Z'};bundle.source=poolSource;await render(evidence);expect(transfer()?.textContent||'').toContain('Sealed candidate bundle');const disclosure=transfer()!.querySelector('details')!;disclosure.open=true;expect(JSON.parse(disclosure.querySelector('pre')!.textContent!)).toEqual(poolSource);const next=producer();(next.candidate_bundle as typeof binding).transfer_id='other-transfer';await render(next);expect(disclosure.isConnected).toBe(false);expect(transfer()!.querySelector('details')!.open).toBe(false);expect(transfer()?.textContent||'').not.toContain('pool-runner-018');
});

it.each([
 ['version',{schema_version:2}],['unsealed state',{state:'uploading'}],['unknown field',{download_url:'https://example.invalid/bundle'}],['digest',{bundle_sha256:'not-a-digest'}],['size type',{size_bytes:'32768'}],['size limit',{size_bytes:64*1024*1024+1}],['source identity',{source:{...binding.source,attempt:0}}],['source extension',{source:{...binding.source,url:'https://example.invalid'}}],['source evidence',{evidence_sha256:'f'.repeat(64)}],['head',{head_commit:'f'.repeat(40)}],
])('does not promote invalid %s metadata into verified transfer facts',async(_name,change)=>{
 const evidence=producer();evidence.candidate_bundle={...binding,...change};await render(evidence);expect(transfer()?.textContent||'').toContain('Candidate transfer metadata could not be verified.');expect(transfer()?.textContent||'').not.toContain('Sealed candidate bundle');expect(transfer()?.textContent||'').not.toContain(bundleDigest);expect(host.textContent).toContain('original-diff');expect(host.textContent).toContain(sourceDigest);expect(fetch).not.toHaveBeenCalled();
});

it('requires the exact fixed candidate and an affirmative import receipt before showing actual import',async()=>{
 const evidence=consumer();evidence.candidate_fence=8;await render(evidence);expect(transfer()?.textContent||'').toContain('Candidate transfer metadata could not be verified.');expect(transfer()?.textContent||'').not.toContain('Candidate imported for this attempt');const negative=consumer();(negative.candidate_import as Record<string,unknown>).imported=false;await render(negative);expect(transfer()?.textContent||'').not.toContain('Candidate imported for this attempt');expect(transfer()?.textContent||'').not.toContain('target<worker>');expect(host.textContent).toContain('consumer-diff');
});
