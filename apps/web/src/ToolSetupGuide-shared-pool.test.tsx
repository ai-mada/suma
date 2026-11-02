// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ToolSetupGuide} from './ToolSetupGuide';
import {ApiViewProvider} from './api-context';
import type {Session} from './api';
import {setLocale} from './i18n';

let root:Root,host:HTMLDivElement,clipboard:PropertyDescriptor|undefined;
const session:Session={user:{id:'user',name:'Owner',email:'owner@example.test',role:'owner'},organization:{id:'tenant',name:'Team'},projects:[]};
const choices=[['codex','codex-jsonl'],['codebuddy','codebuddy-cli'],['harness','dsh-multica'],['claude','claude-code-cli'],['qoder','qoder-acp'],['opencode','opencode-sdk'],['cursor','cursor-cli'],['claude-sdk','claude-code-sdk'],['codex-app-server','codex-app-server']] as const;
type Props=Parameters<typeof ToolSetupGuide>[0];
async function draw(props:Props={},active=session){await act(async()=>root.render(<ApiViewProvider session={active}><ToolSetupGuide expanded project="project" expectedWorkerID="pool-machine" {...props}/></ApiViewProvider>));}
async function change(selector:string,value:string){const element=host.querySelector<HTMLInputElement|HTMLSelectElement>(selector)!;expect(element,selector).not.toBeNull();const prototype=element instanceof HTMLSelectElement?HTMLSelectElement.prototype:HTMLInputElement.prototype;await act(async()=>{Object.getOwnPropertyDescriptor(prototype,'value')!.set!.call(element,value);element.dispatchEvent(new Event(element instanceof HTMLSelectElement?'change':'input',{bubbles:true}));});}
async function click(selector:string){const element=host.querySelector<HTMLButtonElement>(selector)!;expect(element,selector).not.toBeNull();expect(element.disabled).toBe(false);await act(async()=>element.click());}
beforeEach(async()=>{await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.stubGlobal('requestAnimationFrame',(run:FrameRequestCallback)=>{run(0);return 1;});vi.stubGlobal('fetch',vi.fn(async()=>Response.json({items:[]})));clipboard=Object.getOwnPropertyDescriptor(navigator,'clipboard');Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:vi.fn(async()=>{})}});host=document.createElement('div');document.body.append(host);root=createRoot(host);});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();if(clipboard)Object.defineProperty(navigator,'clipboard',clipboard);else Reflect.deleteProperty(navigator,'clipboard');vi.unstubAllGlobals();vi.restoreAllMocks();});

it('T219 keeps every shared-node protocol in offline preparation and never exposes an ordinary Runner identity fallback',async()=>{
 const prepare=vi.fn();let nonce=0;
 for(const [preset,localTool] of choices){await draw({presetRequest:{preset,nonce:++nonce},onPrepareSharedPool:prepare});expect(host.querySelector('[data-remote-runner-setup]')).toBeNull();expect(host.querySelector('[name="remote-runner-mode"]')).toBeNull();expect(host.querySelector('[data-shared-pool-guide]')).not.toBeNull();await click('[data-shared-pool-prepare]');expect(prepare.mock.calls.at(-1)?.[0]).toBe(localTool);expect(prepare.mock.calls.at(-1)?.[1]()).toBe(true);}
 await draw({presetRequest:{preset:'codex',nonce:++nonce}});expect(host.querySelector<HTMLButtonElement>('[data-shared-pool-prepare]')?.disabled).toBe(true);expect(host.querySelector('[data-remote-runner-setup]')).toBeNull();
 await change('.tool-cli-setup-select select','junit');expect(host.querySelector<HTMLButtonElement>('[data-shared-pool-prepare]')?.disabled).toBe(true);expect(host.querySelector('[data-remote-runner-setup]')).toBeNull();
});

it('T219 invalidates prior shared preparation callbacks across selection, pending and actor ABA without adopting private fields',async()=>{
 const prepare=vi.fn();await draw({presetRequest:{preset:'codex',nonce:1},onPrepareSharedPool:prepare});await click('[data-shared-pool-prepare]');const original=prepare.mock.calls.at(-1)![1] as ()=>boolean;
 expect(original()).toBe(true);await change('.tool-cli-setup-select select','codebuddy');await change('.tool-cli-setup-select select','codex');expect(original()).toBe(false);
 await click('[data-shared-pool-prepare]');const unlocked=prepare.mock.calls.at(-1)![1] as ()=>boolean;
 await draw({onPrepareSharedPool:prepare,disabled:true});expect(unlocked()).toBe(false);await draw({onPrepareSharedPool:prepare,disabled:false});expect(unlocked()).toBe(false);
 await click('[data-shared-pool-prepare]');const actor=prepare.mock.calls.at(-1)![1] as ()=>boolean;
 await draw({onPrepareSharedPool:prepare},{...session,user:{...session.user,id:'another-user'}});await draw({onPrepareSharedPool:prepare});expect(actor()).toBe(false);
 await click('[data-shared-pool-prepare]');const mounted=prepare.mock.calls.at(-1)![1] as ()=>boolean;await act(async()=>root.render(<div/>));expect(mounted()).toBe(false);
 expect(vi.mocked(fetch).mock.calls.every(([,options])=>!options?.method||options.method==='GET')).toBe(true);
});

it('T219 keeps SDK and composed command outputs local, copies only the selected reference and never fills ordinary or Pool identity fields',async()=>{
 const prepare=vi.fn();await draw({presetRequest:{preset:'claude-sdk',nonce:1},onPrepareSharedPool:prepare});
 const values={shell:'powershell',node:'C:/Program Files/nodejs/node.exe',host:'D:/sdk/suma-claude.bundle.mjs',nativeCLI:'D:/tools/claude.exe',model:'claude-sonnet-5',credentialEnvRef:'ANTHROPIC_API_KEY',commands:'D:/runner/commands.sdk.json'};
 for(const [name,value]of Object.entries(values))await change('[data-claude-sdk-setup-field="'+name+'"]',value);
 await click('[data-claude-sdk-setup-use]');expect(host.querySelector('[data-shared-pool-output]')?.textContent).toContain(values.commands);await click('[data-shared-pool-copy-output]');expect(navigator.clipboard.writeText).toHaveBeenLastCalledWith(values.commands);
 await change('[data-command-composition-shell]','powershell');await change('[data-command-composition-output]','D:/runner/combined.json');const sources=host.querySelectorAll<HTMLInputElement>('[data-command-composition-source]');
 for(const [index,value]of ['D:/runner/development.json','D:/runner/tests.json'].entries()){sources[index].setAttribute('data-shared-composition-source',String(index));await change('[data-shared-composition-source="'+index+'"]',value);}
 await click('[data-command-composition-copy]');await click('[data-command-composition-use]');expect(host.querySelector('[data-shared-pool-output]')?.textContent).toContain('D:/runner/combined.json');await click('[data-shared-pool-copy-output]');expect(navigator.clipboard.writeText).toHaveBeenLastCalledWith('D:/runner/combined.json');
 expect(host.querySelector('[data-remote-runner-setup]')).toBeNull();expect(host.querySelector('[data-pool-config-preparation]')).toBeNull();expect(prepare).not.toHaveBeenCalled();expect(vi.mocked(fetch).mock.calls.every(([,options])=>!options?.method||options.method==='GET')).toBe(true);
 await change('.tool-cli-setup-select select','codebuddy');expect(host.querySelector('[data-shared-pool-output]')).toBeNull();
 await act(async()=>{await setLocale('zh-CN',{persist:false});});expect(host.querySelector('[data-shared-pool-prepare]')?.textContent).toBe('准备共享节点连接');
});
