// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,expect,it,vi} from 'vitest';
import {RemoteRunnerSetupGuide} from './RemoteRunnerSetupGuide';
import {remoteRunnerSetupCommands,type RemoteRunnerSetupInput} from './remote-runner-setup-model';
import {setLocale} from './i18n';

const input:RemoteRunnerSetupInput={shell:'powershell',control:'https://suma.example.test/team',project:'project-one',worker:'worker-one',commands:"C:/tools/O'Brien $(whoami)/commands.local.json",state:'D:/project/.cache/runner',localTool:'codex-jsonl',repository:'D:/project/app',baseCommit:'c'.repeat(40)};
let root:Root|undefined,host:HTMLDivElement|undefined,clipboard:PropertyDescriptor|undefined;
afterEach(async()=>{if(root)await act(async()=>root!.unmount());root=undefined;host?.remove();if(clipboard)Object.defineProperty(navigator,'clipboard',clipboard);else Reflect.deleteProperty(navigator,'clipboard');vi.unstubAllGlobals();vi.restoreAllMocks();});
async function change(element:HTMLInputElement|HTMLSelectElement,value:string){const proto=element instanceof HTMLSelectElement?HTMLSelectElement.prototype:HTMLInputElement.prototype;await act(async()=>{Object.getOwnPropertyDescriptor(proto,'value')!.set!.call(element,value);element.dispatchEvent(new Event(element instanceof HTMLSelectElement?'change':'input',{bubbles:true}));});}

it('quotes target-shell literals and keeps preparation independent from credential-bound one-shot reporting',()=>{
 const powershell=remoteRunnerSetupCommands(input);
 expect(powershell.prepare).toBe("suma-runner --prepare-tools 'C:/tools/O''Brien $(whoami)/commands.local.json' --local-tool 'codex-jsonl'");
 expect(powershell.report).toContain('$env:SUMA_RUNNER_TOKEN');expect(powershell.report).toContain("--commands 'C:/tools/O''Brien $(whoami)/commands.local.json' --state 'D:/project/.cache/runner'");expect(powershell.report).toContain("--workspace 'D:/project/app' --repository 'D:/project/app' --base-commit '"+input.baseCommit+"' --allow-exec");expect(powershell.report).toContain('--report-tools');
 const bash=remoteRunnerSetupCommands({...input,shell:'bash',commands:"/srv/O'Brien $(whoami)/commands.local.json",state:'/srv/app/.cache/runner',repository:"/srv/O'Brien/app",baseCommit:'d'.repeat(64),localTool:''});
 expect(bash.prepare).toBe("suma-runner --prepare-tools '/srv/O'\"'\"'Brien $(whoami)/commands.local.json'");expect(bash.report).toContain('${SUMA_RUNNER_TOKEN:');expect(bash.report).not.toContain('--token');expect(bash.prepare).not.toContain('SUMA_RUNNER_TOKEN');
 expect(bash.report).toContain("--workspace '/srv/O'\"'\"'Brien/app' --repository '/srv/O'\"'\"'Brien/app' --base-commit '"+'d'.repeat(64)+"' --allow-exec");
 const prepareOnly=remoteRunnerSetupCommands({...input,control:'',project:'',worker:'',state:'',repository:'',baseCommit:''});expect(prepareOnly.prepare).toBe(powershell.prepare);expect(prepareOnly.report).toBeUndefined();expect(prepareOnly.prepare).not.toContain('--repository');expect(prepareOnly.prepare).not.toContain('--base-commit');
});

it('rejects command-line injection, malformed scopes and unsafe control URLs while recognizing loopback-only HTTP',()=>{
 for(const patch of [{shell:''},{shell:'cmd'},{commands:'relative.json'},{commands:'/srv/tools/commands.json\nwhoami'},{commands:'/srv/\u0000commands.json'},{commands:'/srv/commands.txt'},{localTool:'--other'}])expect(remoteRunnerSetupCommands({...input,...patch}).prepare).toBeUndefined();
 for(const patch of [{control:'http://remote.example.test'},{control:'https://user:secret@suma.example.test'},{control:'https://@suma.example.test'},{control:'https://suma.example.test?'},{control:'https://suma.example.test/#x'},{control:'https://suma.example.test\n--other'},{project:'p; whoami'},{worker:'w\n--other'},{state:'relative-state'},{state:'D:/project/\u0000state'},{repository:''},{repository:'relative-repository'},{repository:'D:/project/\nrepo'},{baseCommit:''},{baseCommit:'main'},{baseCommit:'c'.repeat(39)},{baseCommit:'c'.repeat(41)},{baseCommit:'C'.repeat(40)},{baseCommit:'g'.repeat(64)}])expect(remoteRunnerSetupCommands({...input,...patch}).report).toBeUndefined();
 for(const control of ['http://127.0.0.1:4310','http://localhost:4310','http://[::1]:4310']){const result=remoteRunnerSetupCommands({...input,control});expect(result.loopback).toBe(true);expect(result.report).toContain('--report-tools');}
 expect(remoteRunnerSetupCommands({...input,token:'do-not-copy'})).toEqual({loopback:false});
});

it('requires an explicit remote shell and public fields, copies only the shown command and never sends a request',async()=>{
 await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);const fetch=vi.fn(),writeText=vi.fn(async(_text:string)=>{});vi.stubGlobal('fetch',fetch);clipboard=Object.getOwnPropertyDescriptor(navigator,'clipboard');Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText}});
 host=document.createElement('div');document.body.append(host);root=createRoot(host);await act(async()=>root!.render(<RemoteRunnerSetupGuide project="project-one"/>));
 const field=(name:string)=>host!.querySelector<HTMLInputElement|HTMLSelectElement>('[name="remote-runner-'+name+'"]')!;
 expect(field('shell').value).toBe('');expect(field('project').value).toBe('project-one');expect(field('worker').value).toBe('');expect(host.querySelector('[data-remote-runner-command]')).toBeNull();
 await change(field('shell'),'bash');await change(field('commands'),'/srv/tools/commands.local.json');
 const prepare=host.querySelector<HTMLButtonElement>('[data-remote-runner-copy="prepare"]')!;expect(prepare.disabled).toBe(false);expect(host.querySelector('[data-remote-runner-command="prepare"]')!.textContent).not.toContain('SUMA_RUNNER_TOKEN');expect(host.querySelector<HTMLButtonElement>('[data-remote-runner-copy="report"]')!.disabled).toBe(true);
 await change(field('control'),'http://127.0.0.1:4310');await change(field('worker'),'worker-one');await change(field('state'),'/srv/app/.cache/runner');expect(host.querySelector('[data-remote-runner-loopback]')).not.toBeNull();
 expect(host.querySelector<HTMLButtonElement>('[data-remote-runner-copy="report"]')!.disabled).toBe(true);await change(field('repository'),'/srv/app');expect(host.querySelector<HTMLButtonElement>('[data-remote-runner-copy="report"]')!.disabled).toBe(true);await change(field('baseCommit'),'c'.repeat(40));
 const report=host.querySelector<HTMLButtonElement>('[data-remote-runner-copy="report"]')!,shown=host.querySelector('[data-remote-runner-command="report"]')!.textContent!;expect(report.disabled).toBe(false);await act(async()=>report.click());expect(writeText).toHaveBeenCalledExactlyOnceWith(shown);expect(shown).toContain('${SUMA_RUNNER_TOKEN:');expect(shown).toContain("--repository '/srv/app'");expect(fetch).not.toHaveBeenCalled();
 await change(field('worker'),'bad; command');expect(host.querySelector<HTMLButtonElement>('[data-remote-runner-copy="report"]')!.disabled).toBe(true);expect(host.querySelector('[data-remote-runner-command="report"]')).toBeNull();expect(fetch).not.toHaveBeenCalled();
});
