import {afterEach,beforeEach,expect,it} from 'vitest';
import type {LocalizationCatalog,WorkflowValidation} from '../../../packages/contracts/src';
import {renderDiagnostic,setLocale,setServerCatalog} from './i18n/index';
import {workflowValidationMessages} from './workflow-validation';

const source='invalid: original compiler evidence <raw>';
const catalog:LocalizationCatalog={
  version:'workflow-validation-fixture',template_format:'named-dot-v1',default_locale:'zh-CN',
  supported_locales:['zh-CN','en-US'],
  messages:[{key:'dag.validation.node',parameters:[{name:'node',kind:'text'}],
    texts:{'zh-CN':'节点 {{.node}} 无效','en-US':'Node {{.node}} is invalid.'}}],
};

beforeEach(async()=>{
  setServerCatalog(catalog);
  await setLocale('en-US',{persist:false});
});
afterEach(async()=>{await setLocale('zh-CN',{persist:false});});

it('preserves raw compiler diagnostics when metadata is absent, mismatched or unproven',()=>{
  const raw={valid:false,errors:[source],diagnostics:[]} as WorkflowValidation;
  expect(workflowValidationMessages(raw).map(renderDiagnostic)).toEqual([source]);
  expect(workflowValidationMessages({...raw,diagnostics:undefined} as unknown as WorkflowValidation).map(renderDiagnostic)).toEqual([source]);
  for(const d of [
    {message:'translated',localized:false,message_key:'dag.validation.node',message_params:{node:'a'}},
    {message:'',localized:true,message_key:'dag.validation.node',message_params:{node:'a'}},
    null,
  ]) {
    expect(workflowValidationMessages({...raw,diagnostics:[d]} as unknown as WorkflowValidation).map(renderDiagnostic)).toEqual([source]);
  }
  const diagnostic={message:'Unrelated translation',localized:true,message_key:'dag.validation.node',message_params:{node:'a'},locale:'en-US'};
  expect(workflowValidationMessages({...raw,diagnostics:[diagnostic,diagnostic]} as WorkflowValidation).map(renderDiagnostic)).toEqual([source]);
});

it('translates valid catalog metadata while retaining each original source without mutating the response',async()=>{
  const diagnostic={message:'节点 a 无效',localized:true,message_key:'dag.validation.node',message_params:{node:'a'},locale:'zh-CN'};
  const second={...diagnostic,message:'节点 b 无效',message_params:{node:'b'}};
  const result={valid:false,errors:[source,'invalid: second compiler evidence'],diagnostics:[diagnostic,second]} as WorkflowValidation;
  const before=JSON.stringify(result);
  for(const item of result.diagnostics) {Object.freeze(item.message_params);Object.freeze(item);}
  Object.freeze(result.errors);Object.freeze(result.diagnostics);Object.freeze(result);
  const projected=workflowValidationMessages(result);
  expect(projected).toEqual([{...diagnostic,message:source},{...second,message:'invalid: second compiler evidence'}]);
  expect(projected.map(renderDiagnostic)).toEqual(['Node a is invalid.','Node b is invalid.']);
  await setLocale('zh-CN',{persist:false});
  expect(projected.map(renderDiagnostic)).toEqual(['节点 a 无效','节点 b 无效']);
  expect(JSON.stringify(result)).toBe(before);
});

it.each([
  ['unknown key',{message_key:'dag.validation.missing',message_params:{node:'a'}}],
  ['empty key',{message_key:'',message_params:{node:'a'}}],
  ['missing parameters',{message_key:'dag.validation.node'}],
  ['extra parameters',{message_key:'dag.validation.node',message_params:{node:'a',extra:'unproven'}}],
  ['oversized parameter',{message_key:'dag.validation.node',message_params:{node:'x'.repeat(513)}}],
])('retains the corresponding raw compiler evidence for %s',(_name,metadata)=>{
  const diagnostic={message:'unproven display replacement',locale:'zh-CN',localized:true,...metadata};
  const result={valid:false,errors:[source],diagnostics:[diagnostic]} as WorkflowValidation;
  const before=JSON.stringify(result);
  expect(workflowValidationMessages(result).map(renderDiagnostic)).toEqual([source]);
  expect(JSON.stringify(result)).toBe(before);
});
