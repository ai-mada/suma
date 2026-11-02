// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {OpenCodeSetupGuide} from './OpenCodeSetupGuide';
import {openCodeSetupCommands,type OpenCodeSetupInput} from './opencode-setup-guide-model';
import {setLocale} from './i18n';

const input:OpenCodeSetupInput={shell:'powershell',node:'C:/Program Files/nodejs/node.exe',manifest:"D:/suma/O'Brien/suma-opencode.manifest.json",executable:'D:/tools/OpenCode/opencode.exe',provider:'approved',model:'approved-model',endpoint:'https://models.example/v1',credential:'MODEL_PROVIDER_API_KEY',commands:'D:/suma configs/opencode.commands.json'};
let root:Root,host:HTMLDivElement,copy:ReturnType<typeof vi.fn>,onUse:ReturnType<typeof vi.fn>;
beforeEach(async()=>{await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);copy=vi.fn().mockResolvedValue(undefined);onUse=vi.fn();vi.stubGlobal('navigator',{clipboard:{writeText:copy}});vi.stubGlobal('fetch',vi.fn());host=document.createElement('div');document.body.append(host);root=createRoot(host);});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();});
async function draw(disabled=false){await act(async()=>root.render(<OpenCodeSetupGuide disabled={disabled} onUseConfiguration={onUse}/>));}
async function change(name:string,value:string){const field=host.querySelector<HTMLInputElement|HTMLSelectElement>('[data-opencode-setup-field="'+name+'"]')!;expect(field,name).not.toBeNull();await act(async()=>{Object.getOwnPropertyDescriptor(field instanceof HTMLSelectElement?HTMLSelectElement.prototype:HTMLInputElement.prototype,'value')!.set!.call(field,value);field.dispatchEvent(new Event(field instanceof HTMLSelectElement?'change':'input',{bubbles:true}));});}
async function fill(){for(const [name,value] of Object.entries(input))await change(name,value);}
async function click(hook:string){const button=host.querySelector<HTMLButtonElement>('[data-opencode-setup-'+hook+']')!;expect(button).not.toBeNull();await act(async()=>button.click());}

it('T207 presents model connection fields first, keeps machine paths optional to reveal and explicitly hands off only the public new-file draft',async()=>{
 await draw();const machine=host.querySelector<HTMLDetailsElement>('[data-opencode-setup-machine]')!;expect(machine.open).toBe(false);expect(host.querySelector('[data-opencode-setup-field=provider]')?.closest('details')).toBeNull();expect(host.querySelector('[data-opencode-setup-field=manifest]')?.closest('details')).toBe(machine);expect(host.querySelector<HTMLInputElement>('[data-opencode-setup-field=node]')?.value).toBe('');expect(host.querySelector('[name=opencode-bundle]')).toBeNull();expect(host.querySelector('[name=opencode-checks]')).toBeNull();expect(host.querySelector<HTMLButtonElement>('[data-opencode-setup-use]')?.disabled).toBe(true);await fill();const result=openCodeSetupCommands(input)!;expect(host.querySelector('[data-opencode-setup-prepare]')?.textContent).toBe(result.prepare);expect(JSON.parse(host.querySelector('[data-opencode-setup-configuration]')!.textContent!)).toEqual(result.request);await click('copy');expect(copy).toHaveBeenCalledExactlyOnceWith(result.prepare);expect(onUse).not.toHaveBeenCalled();await click('use');expect(onUse).toHaveBeenCalledExactlyOnceWith(result.draft);expect(fetch).not.toHaveBeenCalled();
 await act(async()=>setLocale('zh-CN',{persist:false}));expect(host.textContent).toContain('使用此配置继续');expect(host.textContent).not.toContain('opencode-setup-guide.');await draw(true);expect(host.querySelector<HTMLButtonElement>('[data-opencode-setup-use]')?.disabled).toBe(true);expect(host.querySelector<HTMLButtonElement>('[data-opencode-setup-copy]')?.disabled).toBe(true);await click('use');expect(onUse).toHaveBeenCalledOnce();
});

it('T207 refuses invalid edited drafts and never applies a late clipboard response to the new configuration',async()=>{
 await draw();await fill();let finish!:()=>void;copy.mockImplementationOnce(()=>new Promise<void>(resolve=>{finish=resolve;}));await click('copy');expect(host.querySelector<HTMLButtonElement>('[data-opencode-setup-copy]')?.disabled).toBe(true);await change('model','next-model');await act(async()=>finish());expect(host.querySelector('[data-opencode-setup-copy]')?.textContent).not.toContain('Copied');await change('credential','NODE_OPTIONS');expect(host.querySelector('[data-opencode-setup-prepare]')).toBeNull();expect(host.querySelector<HTMLButtonElement>('[data-opencode-setup-use]')?.disabled).toBe(true);await click('use');expect(onUse).not.toHaveBeenCalled();expect(fetch).not.toHaveBeenCalled();
});
