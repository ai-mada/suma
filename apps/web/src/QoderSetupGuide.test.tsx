// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import {QoderSetupGuide,type QoderSetupGuideProps} from './QoderSetupGuide';
const session={user:{id:'owner',name:'Owner',email:'owner@example.test'},organization:{id:'team',name:'Team'},projects:[]};
let root:Root,host:HTMLDivElement,write:ReturnType<typeof vi.fn>,use:ReturnType<typeof vi.fn>,clipboard:PropertyDescriptor|undefined;
beforeEach(async()=>{await setLocale('en-US',{persist:false});host=document.createElement('div');document.body.append(host);root=createRoot(host);vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.stubGlobal('fetch',vi.fn());write=vi.fn().mockResolvedValue(undefined);use=vi.fn();clipboard=Object.getOwnPropertyDescriptor(navigator,'clipboard');Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:write}});});
afterEach(async()=>{await act(async()=>root?.unmount());host.remove();if(clipboard)Object.defineProperty(navigator,'clipboard',clipboard);else Reflect.deleteProperty(navigator,'clipboard');vi.restoreAllMocks();vi.unstubAllGlobals();});
async function settle(){await act(async()=>{for(let i=0;i<12;i++)await Promise.resolve();});}
async function change(name:string,value:string){const input=host.querySelector<HTMLInputElement|HTMLSelectElement>('[name="qoder-'+name+'"]')!;expect(input,name).not.toBeNull();await act(async()=>{Object.getOwnPropertyDescriptor(input instanceof HTMLSelectElement?HTMLSelectElement.prototype:HTMLInputElement.prototype,'value')!.set!.call(input,value);input.dispatchEvent(new Event(input instanceof HTMLSelectElement?'change':'input',{bubbles:true}));});await settle();}
async function render(props:QoderSetupGuideProps={},active=session){await act(async()=>root.render(<ApiViewProvider session={active}><QoderSetupGuide onUseConfiguration={use} {...props}/></ApiViewProvider>));await settle();}
async function click(selector:string){const button=host.querySelector<HTMLButtonElement>(selector)!;expect(button,selector).not.toBeNull();expect(button.disabled).toBe(false);await act(async()=>button.click());await settle();}
async function fill(){await change('shell','powershell');await change('executable','C:/tools/qoder.exe');await change('version','Qoder reviewed test version');await change('model','explicit-model');await change('commands','D:/new/qoder.json');}
it('Qoder setup requires reviewed native executable and fixed model before copying an administrator draft',async()=>{
 await render();const copy=()=>host.querySelector<HTMLButtonElement>('[data-qoder-setup-copy]')!;
 expect(copy()).not.toBeNull();expect(copy().disabled).toBe(true);
 expect(host.querySelectorAll('input,select')).toHaveLength(6);expect(host.querySelector('input[name=qoder-version]')?.getAttribute('value')).toBe('');
 await change('shell','powershell');await change('commands','D:/new/qoder.json');await change('executable','C:/tools/qoder.cmd');await change('version','Qoder reviewed test version');await change('model','explicit-model');expect(copy().disabled).toBe(true);
 await change('executable','C:/tools/qoder.exe');expect(copy().disabled).toBe(false);
 const config=JSON.parse(host.querySelector('[data-qoder-setup-config]')!.textContent!);
 expect(config).toEqual({'qoder-develop':{adapter:'qoder',protocol:'acp-stdio-v1',executable:'C:/tools/qoder.exe',args:['--acp'],input:'acp-stdio',read_only:false,acp:{runtime:'qoder',reviewed_version:'Qoder reviewed test version',model:'explicit-model'}}});
 expect(JSON.parse(host.querySelector('[data-qoder-setup-request]')!.textContent!)).toEqual({adapter_id:'qoder-acp',executable:'C:/tools/qoder.exe',reviewed_version:'Qoder reviewed test version',model:'explicit-model',credential_env_ref:''});
 await click('[data-qoder-setup-use]');expect(use).toHaveBeenCalledExactlyOnceWith({shell:'powershell',commands:'D:/new/qoder.json',localTool:'qoder-acp'});expect(write).not.toHaveBeenCalled();
 const command=host.querySelector('[data-qoder-setup-preview]')!.textContent!;await click('[data-qoder-setup-copy]');expect(write).toHaveBeenCalledExactlyOnceWith(command);expect(command).toContain("--prepare-tools 'D:/new/qoder.json' --prepare-acp-stdin");expect(host.querySelector('[data-qoder-setup-status=copied]')?.textContent).toContain('No file has been created');expect(fetch).not.toHaveBeenCalled();
});
it('Qoder setup accepts only a separate credential reference and invalidates copy after editing required inputs',async()=>{
 await render();await fill();await change('executable','/opt/qoder/qoder');await change('version','reviewed-version');
 const copy=()=>host.querySelector<HTMLButtonElement>('[data-qoder-setup-copy]')!;
 for(const value of ['SUMA_AGENT_TOKEN','CODEX_HOME','HOME','PATH','secret=value','LOWER_case','NODE_OPTIONS','ANTHROPIC_API_KEY']){await change('credential',''+value);expect(copy().disabled).toBe(true);expect(host.querySelector('[data-qoder-setup-config]')).toBeNull();}
 await change('credential','LOCAL_QODER_TOKEN');expect(copy().disabled).toBe(false);expect(JSON.parse(host.querySelector('[data-qoder-setup-config]')!.textContent!)['qoder-develop'].acp.credential_env_ref).toBe('LOCAL_QODER_TOKEN');
 let release!:()=>void;write.mockImplementationOnce(()=>new Promise<void>(resolve=>{release=resolve;}));await click('[data-qoder-setup-copy]');expect(host.querySelector<HTMLButtonElement>('[data-qoder-setup-use]')?.disabled).toBe(true);
 await change('model','other-model');await change('model','explicit-model');await act(async()=>release());await settle();expect(host.querySelector('[data-qoder-setup-status=copied]')).toBeNull();expect(use).not.toHaveBeenCalled();
 write.mockImplementationOnce(()=>new Promise<void>(resolve=>{release=resolve;}));await click('[data-qoder-setup-copy]');await render({disabled:true});expect(host.querySelector('[data-qoder-setup-preview]')).toBeNull();await render({disabled:false});await act(async()=>release());await settle();expect(host.querySelector('[data-qoder-setup-status=copied]')).toBeNull();
 write.mockImplementationOnce(()=>new Promise<void>(resolve=>{release=resolve;}));await click('[data-qoder-setup-copy]');await render({}, {...session,user:{...session.user,id:'other-owner'}});await render();await act(async()=>release());await settle();expect(host.querySelector<HTMLInputElement>('[data-qoder-setup-output]')?.value).toBe('');expect(host.querySelector('[data-qoder-setup-status=copied]')).toBeNull();expect(host.querySelector<HTMLButtonElement>('[data-qoder-setup-use]')?.disabled).toBe(true);
 await fill();await change('model','');expect(copy().disabled).toBe(true);expect(use).not.toHaveBeenCalled();expect(fetch).not.toHaveBeenCalled();
});
