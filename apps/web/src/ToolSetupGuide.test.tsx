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

it('configures the extraction-only model with explicit valid inputs and no planning or control credentials',async()=>{
 const writeText=vi.fn(async(_text:string)=>{});Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText}});
 await act(async()=>{root=createRoot(host);root.render(<ToolSetupGuide/>);});
 const details=host.querySelector('details')!;await act(async()=>{details.open=true;details.dispatchEvent(new Event('toggle'));});
 const option=[...host.querySelectorAll('option')].find(value=>value.textContent==='知识提炼模型');expect(option).toBeDefined();await change(option!.parentElement as HTMLSelectElement,option!.value);
 const copy=()=>host.querySelector<HTMLButtonElement>('.tool-cli-setup-actions button')!,entry=host.querySelector<HTMLInputElement>('.tool-cli-setup-entry input')!;
 expect(copy().disabled).toBe(true);await change(entry,'relative.mjs');expect(copy().disabled).toBe(true);await change(entry,'C:/tools/suma/suma-agent.mjs');expect(copy().disabled).toBe(true);
 const model=host.querySelector<HTMLInputElement>('.form-grid input:not([type=url])')!,endpoint=host.querySelector<HTMLInputElement>('.form-grid input[type=url]')!;
 await change(model,'configured-extraction-model');await change(endpoint,'https://provider.example.test/v1?token=do-not-use');expect(copy().disabled).toBe(true);await change(endpoint,'https://provider.example.test/v1');expect(copy().disabled).toBe(false);
 const literal=host.querySelector('pre')!.textContent!,configuration=JSON.parse(literal),command=Object.values(configuration)[0] as Record<string,unknown>;
 expect(command).toMatchObject({adapter:'suma',input:'agent-jsonl',read_only:true,knowledge_extraction:true,executable:'node',args:['C:/tools/suma/suma-agent.mjs'],env:{SUMA_MODEL:'configured-extraction-model',SUMA_MODEL_BASE_URL:'https://provider.example.test/v1'},env_from:['SUMA_MODEL_API_KEY']});expect(command).not.toHaveProperty('planning');expect(literal).not.toContain('SUMA_CONTROL_URL');expect(literal).not.toContain('SUMA_AGENT_TOKEN');expect(literal).not.toContain('SUMA_PROJECT_ID');
 await act(async()=>copy().click());expect(writeText).toHaveBeenCalledExactlyOnceWith(literal);
});
