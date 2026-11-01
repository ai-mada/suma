// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {CursorSetupGuide,type CursorSetupGuideProps} from './CursorSetupGuide';
import {cursorSetupCommands,type CursorSetupInput} from './cursor-setup-guide-model';
import {setLocale} from './i18n';

const input:CursorSetupInput={shell:'powershell',packageRoot:"C:\\tools\\Cursor 包 O'Brien $(whoami)",reviewedVersion:'2026.10.22-27dcf5f',model:'synthetic-selected-model',credentialEnvRef:'CURSOR_API_KEY',commands:'C:\\tools\\commands.local.json'};
let host:HTMLDivElement,root:Root,copy:ReturnType<typeof vi.fn>,useConfiguration:ReturnType<typeof vi.fn>,fetch:ReturnType<typeof vi.fn>;
beforeEach(async()=>{await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);copy=vi.fn().mockResolvedValue(undefined);useConfiguration=vi.fn();fetch=vi.fn();vi.stubGlobal('navigator',{clipboard:{writeText:copy}});vi.stubGlobal('fetch',fetch);host=document.createElement('div');document.body.append(host);root=createRoot(host);});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();});
async function render(props:CursorSetupGuideProps={}){await act(async()=>root.render(<CursorSetupGuide onUseConfiguration={useConfiguration} {...props}/>));}
function element<T extends HTMLElement=HTMLButtonElement>(selector:string){const value=host.querySelector<T>(selector);expect(value,selector).not.toBeNull();return value!;}
async function field(name:keyof CursorSetupInput,value:string){const control=element<HTMLInputElement|HTMLSelectElement>('[data-cursor-setup-field="'+name+'"]');await act(async()=>{Object.getOwnPropertyDescriptor(name==='shell'?HTMLSelectElement.prototype:HTMLInputElement.prototype,'value')!.set!.call(control,value);control.dispatchEvent(new Event(name==='shell'?'change':'input',{bubbles:true}));});}
async function fill(){for(const [name,value]of Object.entries(input))await field(name as keyof CursorSetupInput,value);}
async function click(selector:string){await act(async()=>element(selector).click());}

it('keeps all six fields explicit and copies only the model-generated command before an explicit configuration-path handoff',async()=>{
 await render();expect(host.querySelectorAll('[data-cursor-setup-field]')).toHaveLength(6);expect(element<HTMLSelectElement>('[data-cursor-setup-field="shell"]').value).toBe('');expect(element<HTMLInputElement>('[data-cursor-setup-field="model"]').value).toBe('');expect(element('[data-cursor-setup-copy]').disabled).toBe(true);expect(element('[data-cursor-setup-use]').disabled).toBe(true);
 await fill();const expected=cursorSetupCommands(input)!;expect(expected).toBeDefined();expect(element('[data-cursor-setup-prepare]').textContent).toBe(expected.prepare);expect(host.textContent).toContain(expected.request.executable);expect(host.querySelector('[type="password"],textarea,[data-package-sha],[data-argv]')).toBeNull();expect(copy).not.toHaveBeenCalled();expect(useConfiguration).not.toHaveBeenCalled();expect(fetch).not.toHaveBeenCalled();
 await click('[data-cursor-setup-copy]');expect(copy).toHaveBeenCalledExactlyOnceWith(expected.prepare);expect(host.querySelector('[data-cursor-setup-copied]')).not.toBeNull();expect(useConfiguration).not.toHaveBeenCalled();
 await click('[data-cursor-setup-use]');expect(useConfiguration).toHaveBeenCalledExactlyOnceWith(expected.draft);expect(host.textContent).not.toContain('Configuration generated successfully');expect(fetch).not.toHaveBeenCalled();
 await field('model','another-synthetic-model');expect(host.querySelector('[data-cursor-setup-copied]')).toBeNull();expect(element('[data-cursor-setup-prepare]').textContent).toBe(cursorSetupCommands({...input,model:'another-synthetic-model'})!.prepare);
});

it('preserves failed or disabled drafts and leaves the exact command selectable without claiming stale clipboard results or running anything',async()=>{
 await render();await fill();const expected=cursorSetupCommands(input)!;copy.mockRejectedValueOnce(Error('Clipboard unavailable'));await click('[data-cursor-setup-copy]');expect(host.querySelector('[data-cursor-setup-copy-failed]')).not.toBeNull();expect(element('[data-cursor-setup-prepare]').textContent).toBe(expected.prepare);expect(element('[data-cursor-setup-prepare]').tabIndex).toBe(0);expect(element<HTMLInputElement>('[data-cursor-setup-field="packageRoot"]').value).toBe(input.packageRoot);
 await field('commands','C:\\tools\\invalid.txt');expect(element('[data-cursor-setup-copy]').disabled).toBe(true);expect(element('[data-cursor-setup-use]').disabled).toBe(true);expect(element<HTMLInputElement>('[data-cursor-setup-field="model"]').value).toBe(input.model);await field('commands',input.commands);
 await render({disabled:true});expect(Array.from(host.querySelectorAll<HTMLInputElement|HTMLSelectElement>('[data-cursor-setup-field]')).every(control=>control.disabled)).toBe(true);await click('[data-cursor-setup-use]');expect(useConfiguration).not.toHaveBeenCalled();expect(element<HTMLInputElement>('[data-cursor-setup-field="packageRoot"]').value).toBe(input.packageRoot);await render();
 const button=element('[data-cursor-setup-copy]');button.focus();let finish!:()=>void;copy.mockImplementationOnce(()=>new Promise<void>(resolve=>{finish=resolve;}));await act(async()=>{button.click();button.click();});expect(button.disabled).toBe(false);expect(button.getAttribute('aria-disabled')).toBe('true');expect(button.getAttribute('aria-busy')).toBe('true');expect(document.activeElement).toBe(button);expect(copy).toHaveBeenCalledTimes(2);await click('[data-cursor-setup-copy]');expect(copy).toHaveBeenCalledTimes(2);
 await field('model','new-current-model');await act(async()=>finish());expect(document.activeElement).toBe(button);expect(button.getAttribute('aria-busy')).toBe('false');expect(button.getAttribute('aria-disabled')).toBe('false');expect(host.querySelector('[data-cursor-setup-copied]')).toBeNull();expect(host.querySelector('[data-cursor-setup-copy-failed]')).toBeNull();expect(copy).toHaveBeenCalledTimes(2);expect(useConfiguration).not.toHaveBeenCalled();expect(fetch).not.toHaveBeenCalled();
});
