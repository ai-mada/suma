// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ClaudeSDKSetupGuide,type ClaudeSDKSetupGuideProps} from './ClaudeSDKSetupGuide';
import {claudeSDKSetupCommands,type ClaudeSDKSetupInput} from './claude-sdk-setup-model';
import {setLocale} from './i18n';

const input:ClaudeSDKSetupInput={shell:'powershell',node:'C:/Program Files/nodejs/node.exe',host:"C:/runtime/O'Brien SDK/suma-claude.bundle.mjs",nativeCLI:'C:/tools/Claude Code/claude.exe',model:'claude-sonnet-5',credentialEnvRef:'ANTHROPIC_API_KEY',commands:'D:/suma configs/claude.commands.json'};
let host:HTMLDivElement,root:Root,copy:ReturnType<typeof vi.fn>,useConfiguration:ReturnType<typeof vi.fn>,fetch:ReturnType<typeof vi.fn>;
beforeEach(async()=>{await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);copy=vi.fn().mockResolvedValue(undefined);useConfiguration=vi.fn();fetch=vi.fn();vi.stubGlobal('navigator',{clipboard:{writeText:copy}});vi.stubGlobal('fetch',fetch);host=document.createElement('div');document.body.append(host);root=createRoot(host);});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();});
async function render(props:ClaudeSDKSetupGuideProps={}){await act(async()=>root.render(<ClaudeSDKSetupGuide onUseConfiguration={useConfiguration} {...props}/>));}
function element<T extends HTMLElement=HTMLButtonElement>(selector:string){const value=host.querySelector<T>(selector);expect(value,selector).not.toBeNull();return value!;}
async function field(name:keyof ClaudeSDKSetupInput,value:string){const control=element<HTMLInputElement|HTMLSelectElement>('[data-claude-sdk-setup-field="'+name+'"]');await act(async()=>{Object.getOwnPropertyDescriptor(name==='shell'?HTMLSelectElement.prototype:HTMLInputElement.prototype,'value')!.set!.call(control,value);control.dispatchEvent(new Event(name==='shell'?'change':'input',{bubbles:true}));});}
async function fill(){for(const[name,value]of Object.entries(input))await field(name as keyof ClaudeSDKSetupInput,value);}
async function click(selector:string){await act(async()=>element(selector).click());}

it('T192 keeps seven public SDK settings explicit and copies only the fixed model-generated command before handing off three draft fields',async()=>{
 await render();expect(host.querySelectorAll('[data-claude-sdk-setup-field]')).toHaveLength(7);for(const control of host.querySelectorAll<HTMLInputElement|HTMLSelectElement>('[data-claude-sdk-setup-field]')){expect(control.value).toBe('');expect(control.closest('label')).not.toBeNull();}expect(element('[data-claude-sdk-setup-copy]').disabled).toBe(true);expect(element('[data-claude-sdk-setup-use]').disabled).toBe(true);expect(host.textContent).toContain('0.3.314');expect(host.textContent).toContain('2.1.314');
 await fill();const result=claudeSDKSetupCommands(input)!;expect(result).toBeDefined();expect(element('[data-claude-sdk-setup-prepare]').textContent).toBe(result.prepare);expect(result.prepare).toContain('--prepare-sdk-stdin');expect(host.querySelector('textarea,[type="password"],[data-sha],[data-argv]')).toBeNull();expect(copy).not.toHaveBeenCalled();expect(useConfiguration).not.toHaveBeenCalled();expect(fetch).not.toHaveBeenCalled();
 await click('[data-claude-sdk-setup-copy]');expect(copy).toHaveBeenCalledExactlyOnceWith(result.prepare);expect(host.querySelector('[data-claude-sdk-setup-copied]')).not.toBeNull();expect(useConfiguration).not.toHaveBeenCalled();
 await click('[data-claude-sdk-setup-use]');expect(useConfiguration).toHaveBeenCalledExactlyOnceWith({shell:input.shell,commands:input.commands,localTool:'claude-code-sdk'});expect(Object.keys(useConfiguration.mock.calls[0][0]).sort()).toEqual(['shell','commands','localTool'].sort());expect(host.textContent).not.toContain('Configuration generated successfully');expect(fetch).not.toHaveBeenCalled();
});

it('T192 preserves failed or disabled SDK drafts and synchronously guards clipboard writes without losing focus or accepting stale success',async()=>{
 await render();await fill();const original=claudeSDKSetupCommands(input)!;copy.mockRejectedValueOnce(Error('Clipboard unavailable'));await click('[data-claude-sdk-setup-copy]');expect(host.querySelector('[data-claude-sdk-setup-copy-failed]')).not.toBeNull();expect(element('[data-claude-sdk-setup-prepare]').textContent).toBe(original.prepare);expect(element('[data-claude-sdk-setup-prepare]').tabIndex).toBe(0);expect(element<HTMLInputElement>('[data-claude-sdk-setup-field="host"]').value).toBe(input.host);
 await field('commands','D:/invalid.txt');expect(element('[data-claude-sdk-setup-copy]').disabled).toBe(true);expect(element('[data-claude-sdk-setup-use]').disabled).toBe(true);expect(element<HTMLInputElement>('[data-claude-sdk-setup-field="model"]').value).toBe(input.model);await field('commands',input.commands);
 await render({disabled:true});expect(Array.from(host.querySelectorAll<HTMLInputElement|HTMLSelectElement>('[data-claude-sdk-setup-field]')).every(control=>control.disabled)).toBe(true);await click('[data-claude-sdk-setup-use]');expect(useConfiguration).not.toHaveBeenCalled();await render();
 const button=element('[data-claude-sdk-setup-copy]');button.focus();let finish!:()=>void;copy.mockImplementationOnce(()=>new Promise<void>(resolve=>{finish=resolve;}));await act(async()=>{button.click();button.click();});expect(copy).toHaveBeenCalledTimes(2);expect(button.disabled).toBe(false);expect(button.getAttribute('aria-disabled')).toBe('true');expect(button.getAttribute('aria-busy')).toBe('true');expect(document.activeElement).toBe(button);await click('[data-claude-sdk-setup-copy]');expect(copy).toHaveBeenCalledTimes(2);
 await field('model','claude-haiku-4');await act(async()=>finish());expect(document.activeElement).toBe(button);expect(button.getAttribute('aria-busy')).toBe('false');expect(host.querySelector('[data-claude-sdk-setup-copied],[data-claude-sdk-setup-copy-failed]')).toBeNull();await click('[data-claude-sdk-setup-copy]');expect(copy).toHaveBeenLastCalledWith(claudeSDKSetupCommands({...input,model:'claude-haiku-4'})!.prepare);expect(host.querySelector('[data-claude-sdk-setup-copied]')).not.toBeNull();expect(useConfiguration).not.toHaveBeenCalled();expect(fetch).not.toHaveBeenCalled();
});
