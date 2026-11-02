// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,expect,it,vi} from 'vitest';
import {ToolSetupGuide} from './ToolSetupGuide';
import {setLocale} from './i18n';

let root:Root|undefined,host:HTMLDivElement|undefined;
afterEach(async()=>{if(root)await act(async()=>root!.unmount());host?.remove();vi.unstubAllGlobals();vi.restoreAllMocks();});
async function change(element:HTMLInputElement|HTMLSelectElement,value:string){const prototype=element instanceof HTMLSelectElement?HTMLSelectElement.prototype:HTMLInputElement.prototype;await act(async()=>{Object.getOwnPropertyDescriptor(prototype,'value')!.set!.call(element,value);element.dispatchEvent(new Event(element instanceof HTMLSelectElement?'change':'input',{bubbles:true}));});}

it('T203 copies the displayed Codex preparation and explicitly hands its public file draft into original-node report and start without registration',async()=>{
 await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);const fetch=vi.fn(async()=>Response.json({project_id:'project-one',as_of:'2026-11-02T00:00:00Z',items:[]}));vi.stubGlobal('fetch',fetch);const writeText=vi.fn(async(_text:string)=>{});vi.stubGlobal('navigator',{...navigator,clipboard:{writeText}});
 host=document.createElement('div');document.body.append(host);root=createRoot(host);await act(async()=>root!.render(<ToolSetupGuide expanded project="project-one" expectedWorkerID="original-node" presetRequest={{preset:'codex-app-server',nonce:203}}/>));
 const field=(name:string)=>host!.querySelector<HTMLInputElement|HTMLSelectElement>('[data-codex-app-server-setup-field="'+name+'"]')!,remote=(name:string)=>host!.querySelector<HTMLInputElement|HTMLSelectElement>('[name="remote-runner-'+name+'"]')!;
 expect(host.querySelector('[data-codex-app-server-setup-configuration]')).toBeNull();expect(host.querySelector<HTMLButtonElement>('[data-codex-app-server-setup-use]')!.disabled).toBe(true);
 const values={shell:'bash',executable:'/opt/tools/codex',providerID:'team',baseURL:'https://provider.example.test/v1',credentialEnvRef:'CODEX_TEAM_KEY',windowsSandbox:'',commands:'/srv/new.commands.json'};for(const [key,value] of Object.entries(values))await change(field(key),value);
 expect((field('enableUserInput') as HTMLInputElement).checked).toBe(false);const request=JSON.parse(host.querySelector('[data-codex-app-server-setup-configuration]')!.textContent!);expect(Object.keys(request)).toHaveLength(7);expect(request.enable_user_input).toBe(false);expect(request.api_key_env_ref).toBe('CODEX_TEAM_KEY');
 await act(async()=>host!.querySelector<HTMLButtonElement>('[data-codex-app-server-setup-copy]')!.click());expect(writeText).toHaveBeenCalledExactlyOnceWith(host.querySelector('[data-codex-app-server-setup-prepare]')!.textContent);expect(writeText.mock.calls[0][0]).toContain('--prepare-codex-stdin');expect(remote('commands').value).toBe('');
 await act(async()=>host!.querySelector<HTMLButtonElement>('[data-codex-app-server-setup-use]')!.click());expect(remote('commands').value).toBe(values.commands);expect(remote('localTool').value).toBe('codex-app-server');expect(remote('worker').value).toBe('original-node');expect(host.querySelector('[data-remote-runner-command="prepare"]')).toBeNull();
 for(const [key,value] of Object.entries({control:'https://control.example.test',identityPath:'/srv/original.identity.json',repository:'/srv/repository',baseCommit:'a'.repeat(40)}))await change(remote(key),value);
 for(const kind of ['report','start']){const shown=host.querySelector('[data-remote-runner-command="'+kind+'"]')!.textContent!;expect(shown).toContain("--worker 'original-node'");expect(shown).toContain("--commands '/srv/new.commands.json'");expect(shown.includes('--report-tools')).toBe(kind==='report');expect(shown).not.toContain('--prepare-tools');}
 await change(remote('commands'),'/srv/manual.json');await act(async()=>root!.render(<ToolSetupGuide expanded project="project-one" expectedWorkerID="original-node" presetRequest={{preset:'codex-app-server',nonce:203}}/>));expect(remote('commands').value).toBe('/srv/manual.json');expect(fetch.mock.calls.every((call:any[])=>!call[1]?.method||call[1].method==='GET')).toBe(true);
});
