// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {RemoteRunnerSetupGuide} from './RemoteRunnerSetupGuide';
import {ToolSetupGuide} from './ToolSetupGuide';
import {setLocale} from './i18n';
import * as ApiContext from './api-context';
import {api} from './api';

// The real OpenCode form is tested by its owner. This narrow shared-Guide
// fixture exercises the exact callback contract without duplicating its fields.
const capture=vi.hoisted(()=>({props:undefined as undefined|{disabled?:boolean;onUseConfiguration?:(value:unknown)=>void}}));
vi.mock('./OpenCodeSetupGuide',()=>({OpenCodeSetupGuide:(props:typeof capture.props)=>{capture.props=props;return <div data-owned-opencode-form/>;}}));
const draft={shell:'bash' as const,commands:'/srv/commands.opencode.json',localTool:'opencode-sdk' as const};
let root:Root,host:HTMLDivElement,fetch:ReturnType<typeof vi.fn>;
beforeEach(async()=>{await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);fetch=vi.fn();vi.stubGlobal('fetch',fetch);capture.props=undefined;host=document.createElement('div');document.body.append(host);root=createRoot(host);});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();});
const field=(name:string)=>host.querySelector<HTMLInputElement|HTMLSelectElement>('[name="remote-runner-'+name+'"]')!;
async function change(element:HTMLInputElement|HTMLSelectElement,value:string){const proto=element instanceof HTMLSelectElement?HTMLSelectElement.prototype:HTMLInputElement.prototype;await act(async()=>{Object.getOwnPropertyDescriptor(proto,'value')!.set!.call(element,value);element.dispatchEvent(new Event(element instanceof HTMLSelectElement?'change':'input',{bubbles:true}));});}

it('T207 adopts only the explicit OpenCode configuration into the same remote worker and leaves report and startup independent',async()=>{
 const configure=vi.fn();
 await act(async()=>root.render(<RemoteRunnerSetupGuide project="project-one" expectedWorkerID="original-worker" selectionRequest={{localTool:'opencode-sdk',nonce:1}} configurationRequest={{...draft,nonce:1,selectionNonce:1}} onConfigureTool={configure}/>));
 expect(field('commands').value).toBe(draft.commands);expect(field('shell').value).toBe(draft.shell);expect(field('localTool').value).toBe('opencode-sdk');expect(field('worker').value).toBe('original-worker');expect(field('project').value).toBe('project-one');
 for(const [name,value] of Object.entries({control:'https://control.example.test',identityPath:'/srv/original.identity.json',repository:'/srv/original-repository',baseCommit:'a'.repeat(40)}))await change(field(name),value);
 for(const kind of ['report','start']){const command=host.querySelector('[data-remote-runner-command="'+kind+'"]')!.textContent!;expect(command).toContain("--commands '/srv/commands.opencode.json'");expect(command).toContain("--worker 'original-worker'");expect(command).toContain("--identity '/srv/original.identity.json'");expect(command).toContain("--repository '/srv/original-repository'");expect(command).not.toContain('--prepare-tools');expect(command.includes('--report-tools')).toBe(kind==='report');}
 expect(host.querySelector('[data-remote-runner-command="prepare"]')).toBeNull();expect(host.querySelector<HTMLInputElement>('[data-remote-runner-allow-installed-discovery]')?.checked).toBe(false);
 await act(async()=>host.querySelector<HTMLButtonElement>('[data-remote-runner-configure-tool="opencode-sdk"]')!.click());expect(configure).toHaveBeenCalledExactlyOnceWith('opencode-sdk');expect(fetch).not.toHaveBeenCalled();
});

it('T207 rejects stale, cross-tool and caller ABA OpenCode drafts while preserving manually edited remote fields',async()=>{
 let caller=api;vi.spyOn(ApiContext,'useApi').mockImplementation(()=>caller);
 const render=async(request:unknown,nonce=1,disabled=false)=>act(async()=>root.render(<RemoteRunnerSetupGuide project="project-one" expectedWorkerID="original-worker" disabled={disabled} selectionRequest={{localTool:'opencode-sdk',nonce}} configurationRequest={request as never}/>));
 await render({...draft,nonce:1,selectionNonce:1});expect(field('commands').value).toBe(draft.commands);await change(field('commands'),'/srv/manually.edited.json');await change(field('repository'),'/srv/manual-repository');
 await render({...draft,nonce:1,selectionNonce:1});expect(field('commands').value).toBe('/srv/manually.edited.json');
 await render({...draft,commands:'/srv/wrong-tool.json',localTool:'claude-code-sdk',nonce:2,selectionNonce:1});expect(field('commands').value).toBe('/srv/manually.edited.json');
 await render({...draft,commands:'/srv/invalid.json',secret:'unexpected',nonce:3,selectionNonce:1});expect(field('commands').value).toBe('/srv/manually.edited.json');
 await render({...draft,commands:'/srv/wrong-selection.json',nonce:4,selectionNonce:2});expect(field('commands').value).toBe('/srv/manually.edited.json');
 await render({...draft,commands:'/srv/next.json',nonce:5,selectionNonce:1},1,true);expect(field('commands').value).toBe('/srv/manually.edited.json');
 await render({...draft,commands:'/srv/next.json',nonce:5,selectionNonce:1});expect(field('commands').value).toBe('/srv/next.json');expect(field('repository').value).toBe('/srv/manual-repository');
 caller=ApiContext.bindApiView({user:{id:'another-user',name:'Another',email:'another@example.test'},organization:{id:'another-team',name:'Another'}},api);await render({...draft,nonce:5,selectionNonce:1});expect(field('commands').value).toBe('');caller=api;await render({...draft,nonce:5,selectionNonce:1});expect(field('commands').value).toBe('');expect(field('localTool').value).toBe('');expect(fetch).not.toHaveBeenCalled();
});

it('T207 passes only the current OpenCode public draft through the original shared Guide selection and source epochs',async()=>{
 let caller=api;vi.spyOn(ApiContext,'useApi').mockImplementation(()=>caller);
 const render=async(preset:'opencode'|'codex-app-server'='opencode',nonce=1,worker='original-worker',disabled=false)=>act(async()=>root.render(<ToolSetupGuide expanded expectedWorkerID={worker} disabled={disabled} presetRequest={{preset,nonce}}/>));
 await render();const initial=capture.props;expect(initial?.onUseConfiguration).toBeTypeOf('function');expect(initial?.disabled).toBe(false);await act(async()=>initial?.onUseConfiguration?.(draft));expect(field('commands').value).toBe(draft.commands);expect(field('worker').value).toBe('original-worker');expect(field('localTool').value).toBe('opencode-sdk');
 await change(field('commands'),'/srv/manual.json');await render('opencode',1,'original-worker',true);const disabled=capture.props;expect(disabled?.disabled).toBe(true);await act(async()=>disabled?.onUseConfiguration?.({...draft,commands:'/srv/disabled.json'}));expect(field('commands').value).toBe('/srv/manual.json');
 await render('opencode',1);const enabled=capture.props;await act(async()=>enabled?.onUseConfiguration?.({...draft,commands:'/srv/not-closed.json',secret:'unexpected'}));expect(field('commands').value).toBe('/srv/manual.json');
 await render('codex-app-server',2);await act(async()=>initial?.onUseConfiguration?.({...draft,commands:'/srv/late.json'}));expect(field('commands').value).toBe('/srv/manual.json');
 await render('opencode',3,'next-worker');await act(async()=>enabled?.onUseConfiguration?.({...draft,commands:'/srv/old-source.json'}));expect(field('commands').value).toBe('');expect(field('worker').value).toBe('next-worker');
 caller=ApiContext.bindApiView({user:{id:'different-user',name:'Another',email:'other@example.test'},organization:{id:'different-org',name:'Other'}},api);await render('opencode',4,'next-worker');await act(async()=>initial?.onUseConfiguration?.(draft));expect(field('commands').value).toBe('');expect(fetch).not.toHaveBeenCalled();
});
