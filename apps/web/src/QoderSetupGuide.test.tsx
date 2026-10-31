// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ToolSetupGuide} from './ToolSetupGuide';
let root:Root,host:HTMLDivElement;
beforeEach(()=>{host=document.createElement('div');document.body.append(host);vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);});
afterEach(async()=>{await act(async()=>root?.unmount());host.remove();vi.restoreAllMocks();vi.unstubAllGlobals();});
async function change(name:string,value:string){const input=host.querySelector<HTMLInputElement>('[name="qoder-'+name+'"]')!;await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(input,value);input.dispatchEvent(new Event('input',{bubbles:true}));});}
async function render(){await act(async()=>{root=createRoot(host);root.render(<ToolSetupGuide expanded presetRequest={{preset:'qoder',nonce:1}}/>);});}
it('Qoder setup requires reviewed native executable and fixed model before copying an administrator draft',async()=>{
 await render();const copy=()=>host.querySelector<HTMLButtonElement>('[data-qoder-setup-copy]')!;
 expect(copy()).not.toBeNull();expect(copy().disabled).toBe(true);
 await change('executable','C:/tools/qoder.cmd');await change('version','Qoder reviewed test version');await change('model','explicit-model');expect(copy().disabled).toBe(true);
 await change('executable','C:/tools/qoder.exe');expect(copy().disabled).toBe(false);
 const config=JSON.parse(host.querySelector('[data-qoder-setup-config]')!.textContent!);
 expect(config).toEqual({'qoder-develop':{adapter:'qoder',protocol:'acp-stdio-v1',executable:'C:/tools/qoder.exe',args:['--acp'],input:'acp-stdio',read_only:false,acp:{runtime:'qoder',reviewed_version:'Qoder reviewed test version',model:'explicit-model'}}});
 const write=vi.fn(async(_text:string)=>{});Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:write}});await act(async()=>copy().click());expect(JSON.parse(write.mock.calls[0][0])).toEqual(config);Reflect.deleteProperty(navigator,'clipboard');
});
it('Qoder setup accepts only a separate credential reference and invalidates copy after editing required inputs',async()=>{
 await render();await change('executable','/opt/qoder/qoder');await change('version','reviewed-version');await change('model','explicit-model');
 const copy=()=>host.querySelector<HTMLButtonElement>('[data-qoder-setup-copy]')!;
 for(const value of ['SUMA_AGENT_TOKEN','CODEX_HOME','HOME','PATH','secret=value','LOWER_case']){await change('credential',''+value);expect(copy().disabled).toBe(true);expect(host.querySelector('[data-qoder-setup-config]')).toBeNull();}
 await change('credential','LOCAL_QODER_TOKEN');expect(copy().disabled).toBe(false);expect(JSON.parse(host.querySelector('[data-qoder-setup-config]')!.textContent!)['qoder-develop'].acp.credential_env_ref).toBe('LOCAL_QODER_TOKEN');
 await change('model','');expect(copy().disabled).toBe(true);
});
