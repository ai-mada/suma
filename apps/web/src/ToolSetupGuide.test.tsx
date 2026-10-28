// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {setLocale} from './i18n';
import {ToolSetupGuide} from './ToolSetupGuide';

let root:Root|undefined,host:HTMLDivElement,clipboard:PropertyDescriptor|undefined;
beforeEach(async()=>{await setLocale('zh-CN',{persist:false});host=document.createElement('div');document.body.append(host);vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);clipboard=Object.getOwnPropertyDescriptor(navigator,'clipboard');});
afterEach(async()=>{if(root)await act(async()=>root?.unmount());root=undefined;host.remove();if(clipboard)Object.defineProperty(navigator,'clipboard',clipboard);else Reflect.deleteProperty(navigator,'clipboard');vi.unstubAllGlobals();vi.restoreAllMocks();});
async function change(element:HTMLInputElement|HTMLSelectElement,value:string){const proto=element instanceof HTMLSelectElement?HTMLSelectElement.prototype:HTMLInputElement.prototype;await act(async()=>{Object.getOwnPropertyDescriptor(proto,'value')!.set!.call(element,value);element.dispatchEvent(new Event(element instanceof HTMLSelectElement?'change':'input',{bubbles:true}));});}

it.each([['extractor','知识提炼模型','knowledge_extraction'],['diagnosis','失败分诊模型','failure_diagnosis'],['operations','智能操作规划模型','intelligent_operations']] as const)('configures the %s model with explicit valid inputs and no planning or control credentials',async(_preset,title,capability)=>{
 const writeText=vi.fn(async(_text:string)=>{});Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText}});
 await act(async()=>{root=createRoot(host);root.render(<ToolSetupGuide/>);});
 const details=host.querySelector('details')!;await act(async()=>{details.open=true;details.dispatchEvent(new Event('toggle'));});
 const option=[...host.querySelectorAll('option')].find(value=>value.textContent===title);expect(option).toBeDefined();await change(option!.parentElement as HTMLSelectElement,option!.value);
 const copy=()=>host.querySelector<HTMLButtonElement>('.tool-cli-setup-actions button')!,entry=host.querySelector<HTMLInputElement>('.tool-cli-setup-entry input')!;
 expect(copy().disabled).toBe(true);await change(entry,'relative.mjs');expect(copy().disabled).toBe(true);await change(entry,'C:/tools/suma/suma-agent.mjs');expect(copy().disabled).toBe(true);
 const model=host.querySelector<HTMLInputElement>('.form-grid input:not([type=url])')!,endpoint=host.querySelector<HTMLInputElement>('.form-grid input[type=url]')!;
 await change(model,'configured-extraction-model');await change(endpoint,'https://provider.example.test/v1?token=do-not-use');expect(copy().disabled).toBe(true);await change(endpoint,'https://provider.example.test/v1');expect(copy().disabled).toBe(false);
 const literal=host.querySelector('pre')!.textContent!,configuration=JSON.parse(literal),command=Object.values(configuration)[0] as Record<string,unknown>;
 expect(command).toMatchObject({adapter:'suma',input:'agent-jsonl',read_only:true,[capability]:true,executable:'node',args:['C:/tools/suma/suma-agent.mjs'],env:{SUMA_MODEL:'configured-extraction-model',SUMA_MODEL_BASE_URL:'https://provider.example.test/v1'},env_from:['SUMA_MODEL_API_KEY']});expect(command).not.toHaveProperty('planning');expect(literal).not.toContain('SUMA_CONTROL_URL');expect(literal).not.toContain('SUMA_AGENT_TOKEN');expect(literal).not.toContain('SUMA_PROJECT_ID');
 await act(async()=>copy().click());expect(writeText).toHaveBeenCalledExactlyOnceWith(literal);
});

it('requires an explicit local native repair opt-in and valid administrator runtime inputs without changing existing presets',async()=>{
 await act(async()=>{root=createRoot(host);root.render(<ToolSetupGuide/>);});
 const tool=host.querySelector<HTMLSelectElement>('.tool-cli-setup-select select')!;expect(JSON.parse(host.querySelector('pre')!.textContent!)['codex-develop']).not.toHaveProperty('native_workspace');
 const option=[...tool.options].find(value=>value.value==='native-coder');expect(option).toBeDefined();await change(tool,'native-coder');
 const copy=()=>host.querySelector<HTMLButtonElement>('.tool-cli-setup-actions button')!;expect(copy().disabled).toBe(true);
 await change(host.querySelector<HTMLInputElement>('.tool-cli-setup-entry input')!,'D:/tools/suma/suma-agent.bundle.mjs');await change(host.querySelector<HTMLInputElement>('[name=native_runtime_manifest]')!,'D:/tools/suma/suma-agent.manifest.json');await change(host.querySelector<HTMLInputElement>('[name=native_check_ids]')!,'formal-check');expect(copy().disabled).toBe(false);
 const command=()=>Object.values(JSON.parse(host.querySelector('pre')!.textContent!))[0] as any;
 expect(command().native_workspace).toEqual({checks:['formal-check'],runtime_manifest:'D:/tools/suma/suma-agent.manifest.json'});const optIn=host.querySelector<HTMLInputElement>('[name=native_conflict_repair]')!;expect(optIn.checked).toBe(false);await act(async()=>optIn.click());expect(command().native_workspace.conflict_repair).toBe(true);expect(host.textContent).toContain('浏览器不会修改 Runner 策略');await change(tool,'codex');expect(JSON.parse(host.querySelector('pre')!.textContent!)['codex-develop']).not.toHaveProperty('native_workspace');
});

it('configures real Node JUnit reporting with a runner-assigned path and no model credentials',async()=>{
 await act(async()=>{root=createRoot(host);root.render(<ToolSetupGuide/>);});await change(host.querySelector<HTMLSelectElement>('.tool-cli-setup-select select')!,'junit');
 const configuration=JSON.parse(host.querySelector('pre')!.textContent!),command=configuration['junit-tests'];expect(command).toMatchObject({adapter:'command',protocol:'text-v1',executable:'node',input:'none',read_only:false,test_evidence:true,test_report:true,args:['--test','--test-reporter=junit','--test-reporter-destination={{test_report_path}}']});expect(command).not.toHaveProperty('env_from');expect(command).not.toHaveProperty('env');expect(host.querySelector('.tool-cli-setup-entry')).toBeNull();expect(host.querySelector<HTMLButtonElement>('.tool-cli-setup-actions button')?.disabled).toBe(false);
});
it('requires an explicit Python runtime and fixed Octop Harness inputs before copying a local ACP configuration',async()=>{
 await act(async()=>{root=createRoot(host);root.render(<ToolSetupGuide/>);});await change(host.querySelector<HTMLSelectElement>('.tool-cli-setup-select select')!,'octop');
 const copy=()=>host.querySelector<HTMLButtonElement>('[data-octop-setup-copy]')!;expect(copy()).not.toBeNull();expect(copy().disabled).toBe(true);
 const inputs={python:'C:/Python313/python.exe',bundle:'D:/tools/suma/octop-runtime.pyz',manifest:'D:/tools/suma/octop-runtime.manifest.json',checks:'formal-check',provider:'openai',model:'configured-model',endpoint:'https://provider.example.test/v1'};
 for(const [name,text] of Object.entries(inputs))await change(host.querySelector<HTMLInputElement>('[name="octop-'+name+'"]')!,text);
 expect(copy().disabled).toBe(false);const config=JSON.parse(host.querySelector('[data-octop-setup-config]')!.textContent!),command=config['octop-harness-develop'];expect(command.octop.api_key_env_ref).toBe('OCTOP_MODEL_API_KEY');expect(command).not.toHaveProperty('env_from');expect(command).not.toHaveProperty('env');expect(command.args).toEqual([inputs.bundle]);expect(host.textContent).toContain('预算仅观测');
 await change(host.querySelector<HTMLInputElement>('[name="octop-maxSteps"]')!,'33');expect(copy().disabled).toBe(true);await change(host.querySelector<HTMLInputElement>('[name="octop-maxSteps"]')!,'8');expect(copy().disabled).toBe(false);await change(host.querySelector<HTMLInputElement>('[name="octop-bundle"]')!,'relative.pyz');expect(copy().disabled).toBe(true);
});
