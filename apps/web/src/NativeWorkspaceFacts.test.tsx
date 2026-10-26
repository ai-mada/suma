// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {WorkspaceEvidence} from './WorkspaceEvidence';
import {setLocale} from './i18n';
import type {WorkspaceEvidence as Evidence} from './api';
import type {NativeWorkspaceEvidence} from '../../../packages/contracts/src';

const policy='a'.repeat(64),base='b'.repeat(40),head='c'.repeat(40),fileSHA='d'.repeat(64),tree='e'.repeat(40),rawOutput='<script>raw {{.value}} 用户原文</script>';
const native=():NativeWorkspaceEvidence=>({schema_version:1,policy_sha256:policy,generation:2,state:'closed',files:[{path:'src/原始<name>.ts',operation_id:'file-operation',previous_sha256:'f'.repeat(64),sha256:fileSHA,removed:false,generation:2}],checks:[{operation_id:'check-operation',check_id:'check-fixed',profile_sha256:'1'.repeat(64),generation:2,workspace_sha256:'2'.repeat(64),exit_code:0,stopped:false,outcome_unknown:false,output:rawOutput,output_truncated:false}],seal:{schema_version:1,operation_id:'seal-operation',policy_sha256:policy,state:'sealed',previous_commit:base,commit:head,tree,changed_paths:1}});
const evidence=():Evidence=>({base_commit:base,head_commit:head,evidence_sha256:'3'.repeat(64),native_workspace:native()});
let host:HTMLDivElement,root:Root|undefined;
beforeEach(async()=>{await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.stubGlobal('fetch',vi.fn());host=document.createElement('div');document.body.append(host);});
afterEach(async()=>{if(root)await act(async()=>root?.unmount());root=undefined;host.remove();await setLocale('zh-CN',{persist:false});vi.unstubAllGlobals();});
async function render(value:Evidence){await act(async()=>{root??=createRoot(host);root.render(<WorkspaceEvidence evidence={value}/>);});}
const facts=()=>host.querySelector('.native-workspace-facts');
async function open(details:HTMLDetailsElement){await act(async()=>{details.open=true;details.dispatchEvent(new Event('toggle'));});}

it('shows compact development snapshots separately from independent testing and acceptance',async()=>{
 await render(evidence());expect(facts()?.textContent).toContain('Native development record');expect(facts()?.textContent).toContain('Candidate sealed locally');expect(facts()?.textContent).toContain('Files affected: 1');expect(facts()?.textContent).toContain('Current known successes: 1 of 1 reported development checks');expect(facts()?.textContent).toContain('Development checks are not independent DAG tests');expect(facts()?.querySelector('details')?.open).toBe(false);expect(facts()?.querySelector('pre')).toBeNull();expect(fetch).not.toHaveBeenCalled();
});
it('keeps raw output, identities, focus and disclosures across language changes without HTTP',async()=>{
 await render(evidence());const disclosure=facts()!.querySelector<HTMLDetailsElement>('details')!;await open(disclosure);const output=facts()!.querySelector<HTMLDetailsElement>('details.native-workspace-output')!;await open(output);const summary=output.querySelector('summary')!;summary.focus();expect(output.querySelector('pre')?.textContent).toBe(rawOutput);const raw=[...facts()!.querySelectorAll('code')].map(node=>node.textContent);await act(async()=>setLocale('zh-CN',{persist:false}));expect(facts()?.textContent).toContain('原生开发记录');expect(disclosure.isConnected).toBe(true);expect(disclosure.open).toBe(true);expect(output.open).toBe(true);expect(document.activeElement).toBe(summary);expect(output.querySelector('pre')?.textContent).toBe(rawOutput);expect([...facts()!.querySelectorAll('code')].map(node=>node.textContent)).toEqual(raw);expect(host.querySelector('script,name')).toBeNull();expect(fetch).not.toHaveBeenCalled();
});
it('does not invent native records for old workspaces and resets disclosures for another receipt',async()=>{
 await render({base_commit:base,head_commit:head});expect(facts()).toBeNull();await render(evidence());const disclosure=facts()!.querySelector<HTMLDetailsElement>('details')!;await open(disclosure);const next=evidence();(next.native_workspace as NativeWorkspaceEvidence).policy_sha256='9'.repeat(64);(next.native_workspace as NativeWorkspaceEvidence).seal=undefined;await render(next);expect(disclosure.isConnected).toBe(false);expect(facts()?.querySelector('details')?.open).toBe(false);await render({head_commit:head});expect(facts()).toBeNull();
});
it.each([
 ['schema',{schema_version:2}],['generation',{generation:1.5}],['state',{state:'verified'}],['unknown property',{secret:'must-not-render-secret'}],['checks missing',{checks:undefined}],['bad file',{files:[{path:{secret:'must-not-render-secret'}}]}],['bad boolean',{checks:[{...native().checks[0],stopped:'false'}]}],['oversized output',{checks:[{...native().checks[0],output:'s'.repeat(32769)}]}],['foreign policy',{seal:{...native().seal,policy_sha256:'9'.repeat(64)}}],['seal state',{seal:{...native().seal,state:'accepted'}}],
])('reports incomplete evidence for invalid %s without rendering arbitrary values',async(_name,change)=>{
 const value=evidence();value.native_workspace={...native(),...change};await render(value);expect(facts()?.textContent).toContain('Native development evidence is incomplete or unsupported');expect(facts()?.textContent).not.toContain('Candidate sealed locally');expect(facts()?.textContent).not.toContain(fileSHA);expect(host.textContent).not.toContain('must-not-render-secret');expect(facts()?.querySelector('pre')).toBeNull();
});
it.each([undefined,'f'.repeat(40)])('requires the exact outer candidate commit before presenting a seal: %s',async(outerHead)=>{
 const value=evidence();value.head_commit=outerHead;await render(value);expect(facts()?.textContent).toContain('Candidate basis incomplete');expect(facts()?.textContent).not.toContain('Candidate sealed locally');
});
it('prioritizes unknown effects over positive check and seal claims',async()=>{
 const value=evidence();(value.native_workspace as NativeWorkspaceEvidence).state='unknown';await render(value);expect(facts()?.textContent).toContain('Effects require reconciliation');expect(facts()?.textContent).not.toContain('Candidate sealed locally');expect(facts()?.textContent).not.toContain('Current known successes: 1');expect(facts()?.textContent).not.toContain('Known successful snapshot for this content generation');
});
it.each([{generation:1},{stopped:true},{outcome_unknown:true},{exit_code:1}])('does not promote an unsuccessful or stale check snapshot %j',async(change)=>{
 const value=evidence(),record=value.native_workspace as NativeWorkspaceEvidence;record.checks=[{...record.checks[0],...change}];record.seal=undefined;await render(value);expect(facts()?.textContent).not.toContain('Current known successes: 1');expect(facts()?.textContent).toContain('outcome_unknown' in change&&change.outcome_unknown?'Effects require reconciliation':'Not sealed');
});
it('distinguishes no recorded checks, no seal and an actual no-change receipt',async()=>{
 const value=evidence(),record=value.native_workspace as NativeWorkspaceEvidence;record.checks=[];record.seal=undefined;await render(value);expect(facts()?.textContent).toContain('No development checks recorded');expect(facts()?.textContent).toContain('Not sealed');record.seal={...native().seal!,state:'no_change',changed_paths:0,commit:base};value.head_commit=base;await render(value);expect(facts()?.textContent).toContain('No net source change');expect(facts()?.textContent).not.toContain('Candidate sealed locally');
});
