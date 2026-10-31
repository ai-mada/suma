// @vitest-environment jsdom
import {createHash} from 'node:crypto';
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {ExecutionSkillBundle,ExecutionSkillEvidence} from '../../../packages/contracts/src';
import {CodexNativeSkillFacts} from './CodexNativeSkillFacts';
vi.mock('./i18n',()=>({useLocale:()=>{},tr:(key:string,values?:Record<string,unknown>)=>key+(values?' '+JSON.stringify(values):'')}));
const sha=(text:string)=>createHash('sha256').update(text).digest('hex');
function bundle():ExecutionSkillBundle{const name='suma-'+sha('skill-one').slice(0,24),description='Review code carefully',content='PRIVATE INSTRUCTION TEXT';const result={protocol:'suma-codex-skills-v1',documents:[{id:'skill-one',revision:3,sha256:sha('published revision'),title:'Review changes',name,description,content,content_sha256:sha(content),document_sha256:sha('---\nname: '+JSON.stringify(name)+'\ndescription: '+JSON.stringify(description)+'\n---\n\n'+content+'\n')}],bundle_sha256:''};result.bundle_sha256=sha(JSON.stringify(result));return result;}
function evidence():ExecutionSkillEvidence{const b=bundle();return {bundle_sha256:b.bundle_sha256,documents:b.documents.map(({id,revision,sha256,name,document_sha256})=>({id,revision,sha256,name,document_sha256})),materialized:true,catalog_acknowledged:true,invocation_requested:false,turn_acknowledged:false,files_verified:true};}
let root:Root,host:HTMLDivElement;
beforeEach(()=>{vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);root=createRoot(host);});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();});
const render=async(props:Parameters<typeof CodexNativeSkillFacts>[0])=>act(async()=>root.render(<CodexNativeSkillFacts {...props}/>));
it('shows fixed published revisions in plans without presenting runtime acknowledgements or content',async()=>{
 await render({mode:'plan',bundle:bundle(),evidence:evidence()});expect(host.textContent).toContain('codex-native-skills.configured');expect(host.querySelector('[data-skill-facts]')).toBeNull();expect(host.textContent).not.toContain('PRIVATE INSTRUCTION TEXT');expect(host.textContent).toContain('Review changes');expect(host.textContent).toContain('codex-native-skills.revision {"value":3}');expect(host.querySelector('details')!.open).toBe(false);
});
it('renders five independent facts with false unverified and exact receipt identity in details',async()=>{
 await render({mode:'attempt',bundle:bundle(),evidence:evidence()});const facts=host.querySelector('[data-skill-facts]')!;expect(facts.querySelectorAll('dd')).toHaveLength(5);expect(facts.querySelectorAll('[data-verified=true]')).toHaveLength(3);expect(facts.querySelectorAll('[data-verified=false]')).toHaveLength(2);expect(host.textContent).toContain('codex-native-skills.not_verified');expect(host.textContent).toContain('skill-one');expect(host.textContent).toContain(bundle().documents[0].document_sha256);expect(host.textContent).not.toContain('PRIVATE INSTRUCTION TEXT');
});
it('clears positive facts for missing, malformed and mismatched receipts without rendering their values',async()=>{
 await render({mode:'attempt',bundle:bundle()});expect(host.textContent).toContain('codex-native-skills.not_recorded');expect(host.querySelectorAll('[data-verified=true]')).toHaveLength(0);
 await render({mode:'attempt',bundle:bundle(),evidence:{...evidence(),materialized:false}});expect(host.textContent).toContain('codex-native-skills.invalid_evidence');expect(host.querySelectorAll('[data-verified=true]')).toHaveLength(0);
 const wrong=evidence();wrong.documents[0].revision=99;await render({mode:'attempt',bundle:bundle(),evidence:wrong});expect(host.textContent).toContain('codex-native-skills.mismatched_evidence');expect(host.textContent).not.toContain('codex-native-skills.revision {"value":99}');expect(host.querySelectorAll('[data-verified=true]')).toHaveLength(0);
 await render({mode:'attempt',evidence:evidence()});expect(host.textContent).toContain('codex-native-skills.bundle_unavailable');expect(host.querySelectorAll('[data-verified=true]')).toHaveLength(0);
});
it('distinguishes no selected skills, invalid empty bundles and other tools text delivery',async()=>{
 await render({mode:'plan'});expect(host.textContent).toContain('codex-native-skills.none_selected');expect(host.querySelector('details')).toBeNull();
 await render({mode:'plan',bundle:{...bundle(),documents:[]}});expect(host.textContent).toContain('codex-native-skills.invalid_bundle');expect(host.textContent).not.toContain('codex-native-skills.none_selected');
 await render({mode:'text',bundle:bundle(),evidence:evidence()});expect(host.textContent).toContain('codex-native-skills.text_mode');expect(host.querySelector('[data-skill-facts]')).toBeNull();expect(host.textContent).not.toContain('codex-native-skills.confirmed');
});
it('keeps a real failure code folded as data and replaces details when the source changes',async()=>{
 await render({mode:'attempt',bundle:bundle(),evidence:{...evidence(),failure_code:'skill_catalog_mismatch'}});const details=host.querySelector('details')!;expect(details.open).toBe(false);expect(details.textContent).toContain('skill_catalog_mismatch');const failure=host.querySelector('[data-codex-skill-failure]')!;expect(failure).not.toBeNull();expect(failure.closest('details')).toBeNull();expect(failure.textContent).toBe('codex-native-skills.failure_recorded');expect(failure.textContent).not.toContain('skill_catalog_mismatch');await act(async()=>{details.open=true;});await render({mode:'plan'});expect(host.textContent).not.toContain('skill_catalog_mismatch');expect(host.querySelector('details')).toBeNull();expect(host.querySelector('[data-codex-skill-failure]')).toBeNull();
 const mismatch=evidence();mismatch.bundle_sha256='f'.repeat(64);mismatch.failure_code='foreign_failure';await render({mode:'attempt',bundle:bundle(),evidence:mismatch});expect(host.querySelector('[data-codex-skill-failure]')).toBeNull();expect(host.textContent).not.toContain('foreign_failure');
 await render({mode:'attempt',bundle:bundle(),evidence:{...evidence(),materialized:false,failure_code:'invalid_failure'}});expect(host.querySelector('[data-codex-skill-failure]')).toBeNull();expect(host.textContent).not.toContain('invalid_failure');
});
