// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,expect,it,vi} from 'vitest';
import type {ToolInput} from '../../../packages/contracts/src';
import {ToolInputCard} from './ToolInputCard';
import {toolInputAnswer,toolInputRecord,toolInputSelections} from './tool-input.test-support';
import {setLocale} from './i18n';

let root:Root|undefined,host:HTMLDivElement|undefined;
afterEach(async()=>{if(root)await act(async()=>root!.unmount());host?.remove();vi.unstubAllGlobals();vi.restoreAllMocks();});
async function mount(onAnswer:ReturnType<typeof vi.fn>){await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);root=createRoot(host);const render=async(input=toolInputRecord,props:{disabled?:boolean;readOnly?:boolean}={})=>{await act(async()=>root!.render(<ToolInputCard input={input} onAnswer={onAnswer} {...props}/>));};await render();return render;}
async function click(selector:string){const element=host!.querySelector<HTMLElement>(selector);expect(element,selector).not.toBeNull();await act(async()=>element!.click());}
async function text(question:string,value:string){const element=host!.querySelector<HTMLTextAreaElement>('[data-tool-input-text="'+question+'"]');expect(element,question).not.toBeNull();await act(async()=>{Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value')!.set!.call(element,value);element!.dispatchEvent(new Event('input',{bubbles:true}));});}

it('collects the original option, text and Other without defaults and submits exact answers or explicit per-question/global skips only',async()=>{
 const answer=vi.fn(),fetch=vi.fn();vi.stubGlobal('fetch',fetch);await mount(answer);expect(host!.querySelectorAll('input:checked')).toHaveLength(0);expect(host!.querySelector('[type="password"]')).toBeNull();expect(host!.querySelector<HTMLButtonElement>('[data-tool-input-submit]')?.disabled).toBe(true);expect(answer).not.toHaveBeenCalled();
 await click('[data-tool-input-option="approach"][value="Small change"]');await text('approach','Keep the public API.');await text('details','Use the existing parser.');await click('[data-tool-input-other="validation"]');await text('validation','Run the relevant checks only.');expect(answer).not.toHaveBeenCalled();expect(host!.querySelector<HTMLButtonElement>('[data-tool-input-submit]')?.disabled).toBe(false);
 await click('[data-tool-input-submit]');expect(answer).toHaveBeenNthCalledWith(1,toolInputSelections);
 await click('[data-tool-input-question-skip="approach"]');await click('[data-tool-input-submit]');expect(answer).toHaveBeenNthCalledWith(2,[{question_id:'approach',kind:'skip'},toolInputSelections[1],toolInputSelections[2]]);
 await click('[data-tool-input-skip]');expect(answer).toHaveBeenNthCalledWith(3,toolInputRecord.request.codex.questions.map(question=>({question_id:question.id,kind:'skip'})));expect(fetch).not.toHaveBeenCalled();
});

it('preserves drafts for the same specification, clears changed sources and distinguishes saved answers, native delivery, closure and unknown writes read-only',async()=>{
 const answer=vi.fn(),render=await mount(answer);await click('[data-tool-input-option="approach"][value="Refactor"]');await text('approach','Use a narrow change.');await render({...toolInputRecord,version:2});expect(host!.querySelector<HTMLInputElement>('[data-tool-input-option="approach"][value="Refactor"]')?.checked).toBe(true);expect(host!.querySelector<HTMLTextAreaElement>('[data-tool-input-text="approach"]')?.value).toBe('Use a narrow change.');
 await render({...toolInputRecord,version:2},{disabled:true});expect(host!.querySelector<HTMLInputElement>('[data-tool-input-option="approach"][value="Refactor"]')?.matches(':disabled')).toBe(true);expect(answer).not.toHaveBeenCalled();await render({...toolInputRecord,version:3});expect(host!.querySelector<HTMLInputElement>('[data-tool-input-option="approach"][value="Refactor"]')?.checked).toBe(true);
 await render({...toolInputRecord,request:{...toolInputRecord.request,codex:{...toolInputRecord.request.codex,params_sha256:'4'.repeat(64)}}});expect(host!.querySelectorAll('input:checked')).toHaveLength(0);await click('[data-tool-input-option="approach"][value="Small change"]');await render({...toolInputRecord,id:'another-input'});expect(host!.querySelectorAll('input:checked')).toHaveLength(0);
 const observation={sequence:1,params_sha256:toolInputRecord.request.codex.params_sha256,answer_sha256:toolInputAnswer.answer_sha256,answer_observed:true,reply_started:false,reply_written:false,request_resolved:false,turn_closed:false};
 const saved:ToolInput={...toolInputRecord,state:'answered',version:2,answer:toolInputAnswer,allowed_actions:[],observation};await render(saved);expect(host!.querySelector('[data-tool-input-submit]')).toBeNull();expect(host!.textContent).toContain('Use the existing parser.');expect(host!.querySelector('[data-tool-input-status="saved"]')).not.toBeNull();expect(host!.querySelector('[data-tool-input-status="observed"]')).not.toBeNull();expect(host!.querySelector('[data-tool-input-status="written"]')).toBeNull();
 await render({...saved,observation:{...observation,sequence:2,reply_started:true,reply_written:true}});expect(host!.querySelector('[data-tool-input-status="written"]')).not.toBeNull();expect(host!.textContent).not.toContain('Model continued successfully');
 await render({...toolInputRecord,state:'closed',allowed_actions:[],observation:{...observation,answer_sha256:undefined,answer_observed:false,request_resolved:true}});expect(host!.querySelector('[data-tool-input-status="resolved"]')).not.toBeNull();expect(host!.querySelector('[data-tool-input-status="saved"]')).toBeNull();expect(host!.textContent).toContain('closed without a saved human answer');
 await render({...saved,observation:{...observation,sequence:3,reply_started:true,reply_written:false,failure_code:'user_input_reply_unknown'}});expect(host!.querySelector('[data-tool-input-status="unknown"]')).not.toBeNull();expect(host!.querySelector('[data-tool-input-submit]')).toBeNull();expect(answer).not.toHaveBeenCalled();
});
