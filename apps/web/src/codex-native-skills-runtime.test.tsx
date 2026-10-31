// @vitest-environment jsdom
import {createHash} from 'node:crypto';
import {createElement} from 'react';
import {renderToStaticMarkup} from 'react-dom/server';
import {beforeAll,expect,it} from 'vitest';
import type {ExecutionAttempt,ExecutionSkillBundle,ExecutionSkillEvidence,ToolEvidence} from '../../../packages/contracts/src';
import {ToolExecutionFacts} from './ToolFacts';
import {AttemptFacts} from './SchedulingFacts';
import {setLocale} from './i18n';

const sha=(text:string)=>createHash('sha256').update(text).digest('hex');
function bundle():ExecutionSkillBundle{
 const name='suma-'+sha('skill-runtime').slice(0,24),description='Review the current code',content='PRIVATE PUBLISHED SKILL CONTENT';
 const document={id:'skill-runtime',revision:3,sha256:sha('publication'),title:'Reviewed code practice',name,description,content,content_sha256:sha(content),document_sha256:sha('---\nname: '+JSON.stringify(name)+'\ndescription: '+JSON.stringify(description)+'\n---\n\n'+content+'\n')};
 const fixed={protocol:'suma-codex-skills-v1',documents:[document],bundle_sha256:''};fixed.bundle_sha256=sha(JSON.stringify(fixed));return fixed;
}
function receipt():ExecutionSkillEvidence{const fixed=bundle();return {bundle_sha256:fixed.bundle_sha256,documents:fixed.documents.map(({id,revision,sha256,name,document_sha256})=>({id,revision,sha256,name,document_sha256})),materialized:true,catalog_acknowledged:true,invocation_requested:false,turn_acknowledged:false,files_verified:true};}
function tool():ToolEvidence{return {profile:{command_id:'codex',adapter:'codex',protocol:'codex-app-server-v1',tool_version:'controlled',profile_sha256:sha('profile'),executable_sha256:sha('executable'),read_only:false},terminal:'completed',event_count:1,usage_state:'unreported',codex_execution:{binding_sha256:sha('binding'),requested_model:'model-a',requested_reasoning_effort:'low',configuration_acknowledged:true,interrupt_requested:false,interrupt_acknowledged:false,token_notice_exceeded:false,skills:receipt()}};}
function dom(html:string){const host=document.createElement('div');host.innerHTML=html;return host;}
beforeAll(async()=>{await setLocale('en-US',{persist:false});});

it('mounts exact native skill receipts in current tool facts while clearing missing and mismatched confirmations',()=>{
 const render=(evidence:ToolEvidence|undefined,executionSkills=bundle())=>dom(renderToStaticMarkup(createElement(ToolExecutionFacts,{state:'running',evidence,executionSkills})));
 let host=render(tool());expect(host.querySelectorAll('[data-codex-native-skills]')).toHaveLength(1);expect(host.querySelectorAll('[data-verified=true]')).toHaveLength(3);expect(host.textContent).not.toContain('PRIVATE PUBLISHED SKILL CONTENT');
 host=render(undefined);expect(host.querySelectorAll('[data-codex-native-skills]')).toHaveLength(1);expect(host.querySelectorAll('[data-verified=true]')).toHaveLength(0);
 const wrong=tool();wrong.codex_execution!.skills!.documents[0].revision=4;host=render(wrong);expect(host.querySelectorAll('[data-verified=true]')).toHaveLength(0);
});

it('retains native skill facts on an exact historical attempt without treating a missing original package as confirmation',()=>{
 const attempt:ExecutionAttempt={run_id:'run-a',node_id:'code',attempt:2,fence:4,worker_id:'worker-a',state:'succeeded',started_at:'2026-10-31T00:00:00Z',completed_at:'2026-10-31T00:01:00Z',result_source:'runner_result',result:{run_id:'run-a',node_id:'code',attempt:2,fence:4,worker_id:'worker-a',operation_id:'result-a',exit_code:0,output:'',error:'',stopped:true,outcome_unknown:false,tool_evidence:tool()}};
 let host=dom(renderToStaticMarkup(createElement(AttemptFacts,{attempt,executionSkills:bundle()})));expect(host.querySelectorAll('[data-codex-native-skills]')).toHaveLength(1);expect(host.querySelectorAll('[data-verified=true]')).toHaveLength(3);
 host=dom(renderToStaticMarkup(createElement(AttemptFacts,{attempt})));expect(host.querySelectorAll('[data-codex-native-skills]')).toHaveLength(1);expect(host.querySelectorAll('[data-verified=true]')).toHaveLength(0);
});
