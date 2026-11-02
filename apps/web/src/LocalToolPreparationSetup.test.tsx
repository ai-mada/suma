// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import {LocalToolPreparationSetup,type LocalToolPreparationSetupProps} from './LocalToolPreparationSetup';

const session={user:{id:'owner',name:'Owner',email:'owner@example.test'},organization:{id:'team',name:'Team'},projects:[]};
let root:Root,host:HTMLDivElement,copy:ReturnType<typeof vi.fn>,use:ReturnType<typeof vi.fn>,clipboard:PropertyDescriptor|undefined;
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.stubGlobal('fetch',vi.fn());
 copy=vi.fn().mockResolvedValue(undefined);use=vi.fn();clipboard=Object.getOwnPropertyDescriptor(navigator,'clipboard');Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:copy}});
 host=document.createElement('div');document.body.append(host);root=createRoot(host);
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();if(clipboard)Object.defineProperty(navigator,'clipboard',clipboard);else Reflect.deleteProperty(navigator,'clipboard');vi.unstubAllGlobals();vi.restoreAllMocks();});
async function settle(){await act(async()=>{for(let i=0;i<16;i++)await Promise.resolve();});}
async function draw(props:Partial<LocalToolPreparationSetupProps>={},active=session){await act(async()=>root.render(<ApiViewProvider session={active}><LocalToolPreparationSetup localTool="codebuddy-cli" onUseConfiguration={use} {...props}/></ApiViewProvider>));await settle();}
async function change(selector:string,value:string){const field=host.querySelector<HTMLInputElement|HTMLSelectElement>(selector)!;expect(field,selector).not.toBeNull();await act(async()=>{Object.getOwnPropertyDescriptor(field instanceof HTMLSelectElement?HTMLSelectElement.prototype:HTMLInputElement.prototype,'value')!.set!.call(field,value);field.dispatchEvent(new Event(field instanceof HTMLSelectElement?'change':'input',{bubbles:true}));});await settle();}
async function click(selector:string){const button=host.querySelector<HTMLButtonElement>(selector)!;expect(button,selector).not.toBeNull();expect(button.disabled).toBe(false);await act(async()=>button.click());await settle();}
async function fill(path="D:/new/O'Brien $(unexpected).json"){await change('[data-local-tool-shell]','powershell');await change('[data-local-tool-output]',path);}

it('T220 prepares an exact two-field command and explicitly adopts only its current file reference without copying or running first',async()=>{
 await draw();expect(host.querySelectorAll('input,select')).toHaveLength(2);expect(host.querySelector('[data-local-tool-preview]')).toBeNull();expect(host.querySelector<HTMLButtonElement>('[data-local-tool-use]')?.disabled).toBe(true);
 await fill();const command="suma-runner --prepare-tools 'D:/new/O''Brien $(unexpected).json' --local-tool 'codebuddy-cli'";
 expect(host.querySelector('[data-local-tool-preview]')?.textContent).toBe(command);expect(host.textContent).toContain('CodeBuddy Code');
 await click('[data-local-tool-use]');expect(use).toHaveBeenCalledExactlyOnceWith({shell:'powershell',commands:"D:/new/O'Brien $(unexpected).json",localTool:'codebuddy-cli'});expect(copy).not.toHaveBeenCalled();
 await click('[data-local-tool-copy]');expect(copy).toHaveBeenCalledExactlyOnceWith(command);expect(host.querySelector('[data-local-tool-status=copied]')?.textContent).toContain('No file has been created');
 await change('[data-local-tool-output]','D:/new/not-json.txt');expect(host.querySelector('[data-local-tool-preview]')).toBeNull();expect(host.querySelector<HTMLButtonElement>('[data-local-tool-use]')?.disabled).toBe(true);expect(host.querySelector<HTMLButtonElement>('[data-local-tool-copy]')?.disabled).toBe(true);expect(fetch).not.toHaveBeenCalled();
});

it('T220 invalidates late clipboard acknowledgements after field ABA and disable-reenable while keeping adoption explicit',async()=>{
 let release!:()=>void;copy.mockImplementationOnce(()=>new Promise<void>(resolve=>{release=resolve;}));await draw();await fill('D:/new/a.json');await click('[data-local-tool-copy]');expect(host.querySelector<HTMLButtonElement>('[data-local-tool-use]')?.disabled).toBe(true);
 await change('[data-local-tool-output]','D:/new/b.json');await change('[data-local-tool-output]','D:/new/a.json');await act(async()=>release());await settle();expect(host.querySelector('[data-local-tool-status=copied]')).toBeNull();expect(use).not.toHaveBeenCalled();
 copy.mockImplementationOnce(()=>new Promise<void>(resolve=>{release=resolve;}));await click('[data-local-tool-copy]');await draw({disabled:true});expect(host.querySelector('[data-local-tool-preview]')).toBeNull();await draw({disabled:false});await act(async()=>release());await settle();expect(host.querySelector('[data-local-tool-status=copied]')).toBeNull();
 await click('[data-local-tool-use]');expect(use).toHaveBeenCalledExactlyOnceWith({shell:'powershell',commands:'D:/new/a.json',localTool:'codebuddy-cli'});expect(fetch).not.toHaveBeenCalled();
});

it('T220 erases old tool and account ABA drafts and ignores their late clipboard results without offering a special-protocol fallback',async()=>{
 let release!:()=>void;copy.mockImplementationOnce(()=>new Promise<void>(resolve=>{release=resolve;}));await draw();await fill();await click('[data-local-tool-copy]');
 await draw({localTool:'claude-code-cli'});await draw({localTool:'codebuddy-cli'});await act(async()=>release());await settle();expect(host.querySelector<HTMLInputElement>('[data-local-tool-output]')?.value).toBe('');expect(host.querySelector('[data-local-tool-status=copied]')).toBeNull();
 copy.mockImplementationOnce(()=>new Promise<void>(resolve=>{release=resolve;}));await fill('D:/new/account-a.json');await click('[data-local-tool-copy]');await draw({}, {...session,user:{...session.user,id:'other-owner'}});await draw();await act(async()=>release());await settle();expect(host.querySelector<HTMLInputElement>('[data-local-tool-output]')?.value).toBe('');expect(host.querySelector('[data-local-tool-status=copied]')).toBeNull();
 await draw({localTool:'codex-app-server'});expect(host.querySelector('[data-local-tool-preparation-unavailable]')).not.toBeNull();expect(host.querySelector('[data-local-tool-preview]')).toBeNull();expect(host.querySelector('[data-local-tool-use]')).toBeNull();expect(use).not.toHaveBeenCalled();expect(fetch).not.toHaveBeenCalled();
});
