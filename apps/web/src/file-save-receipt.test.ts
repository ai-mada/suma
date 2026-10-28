import {webcrypto} from 'node:crypto';
import {afterEach,expect,it,vi} from 'vitest';
import {ApiError} from './api';
import {fileSaveReceipt,FileSaveRequest} from './file-saves';
afterEach(()=>vi.unstubAllGlobals());

it('binds acknowledgements to exact path, content and SHA256 of unnormalized UTF8 bytes',async()=>{
 vi.stubGlobal('crypto',webcrypto);
 const request={project_id:'p',path:'src/main.ts',content:'提交 🧪\r\n',expected_hash:'before',expected_context_sha256:'a'.repeat(64),idempotency_key:'original-operation'};
 const hash=Array.from(new Uint8Array(await webcrypto.subtle.digest('SHA-256',new TextEncoder().encode(request.content))),byte=>byte.toString(16).padStart(2,'0')).join('');
 const response={project_id:'p',path:request.path,content:request.content,hash,language:'typescript',workspace_name:'Original workspace',context_sha256:'a'.repeat(64)};
 expect(await fileSaveReceipt(response,request)).toEqual(response);
 for(const wrong of [null,{},[],{...response,project_id:'other-project'},{...response,path:'other.ts'},{...response,content:'提交 🧪\n'},{...response,hash:'0'.repeat(64)},{...response,hash:hash.toUpperCase()},{...response,language:4},{...response,workspace_name:undefined},{...response,workspace_name:''},{...response,context_sha256:'b'.repeat(64)},{...response,context_sha256:undefined}])expect(await fileSaveReceipt(wrong,request)).toBeUndefined();
 vi.stubGlobal('crypto',{});expect(await fileSaveReceipt(response,request)).toBeUndefined();
});

it('keeps unknown and reconciliation operations across later refusals but releases a first definite conflict',()=>{
 const source={project_id:'p',path:'a.ts',content:'before',hash:'before-sha',language:'typescript',workspace_name:'Original',context_sha256:'a'.repeat(64)};
 for(const status of [401,403,409]){
  const pending=new FileSaveRequest(),request=pending.begin('p',source,'submitted');
  pending.failed(new ApiError(0,'response_unknown','Uncertain acknowledgement'));
  pending.failed(new ApiError(status,'refused','A later refusal is not the old write receipt'));
  expect(pending.begin('other',{...source,path:'other.ts'},'edited')).toBe(request);
  expect(pending.unknown).toBe(true);expect(pending.source).toEqual(source);
  pending.completed();expect(pending.pending).toBeUndefined();expect(pending.unknown).toBe(false);
 }
 const prepared=new FileSaveRequest(),original=prepared.begin('p',source,'submitted');
 prepared.failed(new ApiError(409,'workspace_reconciliation_required','Prepared',{operation_id:original.idempotency_key,path:source.path}));
 prepared.failed(new ApiError(503,'unavailable','Unavailable'));
 expect(prepared.reconciliation).toBe(true);expect(prepared.reconciliationDetails?.operation_id).toBe(original.idempotency_key);
 const rejected=new FileSaveRequest();rejected.begin('p',source,'submitted');rejected.failed(new ApiError(409,'version_conflict','Known rejected before write'));
 expect(rejected.pending).toBeUndefined();expect(rejected.unknown).toBe(false);
});

it('freezes the original read context and accepts only exact authenticated not-accepted proof to settle uncertainty',()=>{
 const source={project_id:'p',path:'a.ts',content:'before',hash:'before-sha',language:'typescript',workspace_name:'Original',context_sha256:'a'.repeat(64)};
 const pending=new FileSaveRequest(),request=pending.begin('p',source,'submitted');
 expect(request.expected_context_sha256).toBe('a'.repeat(64));
 source.context_sha256='b'.repeat(64);
 pending.failed(new ApiError(0,'response_unknown','Lost acknowledgement'));
 const proof={project_id:'p',path:'a.ts',operation_id:request.idempotency_key,expected_context_sha256:'a'.repeat(64),not_accepted:true};
 for(const [status,code,details] of [
  [403,'workspace_context_changed',proof],[500,'workspace_context_changed',proof],
  [409,'workspace_context_changed',{...proof,project_id:'other'}],
  [409,'workspace_context_changed',{...proof,path:'other.ts'}],
  [409,'workspace_context_changed',{...proof,operation_id:'other'}],
  [409,'workspace_context_changed',{...proof,expected_context_sha256:'b'.repeat(64)}],
  [409,'workspace_context_changed',{...proof,not_accepted:false}],
  [409,'workspace_context_changed',{}],
 ] as const){pending.failed(new ApiError(status,code,'Not sufficient proof',details));expect(pending.pending).toBe(request);expect(pending.unknown).toBe(true);}
 pending.failed(new ApiError(409,'workspace_context_changed','Original operation not accepted',proof));
 expect(pending.pending).toBeUndefined();expect(pending.unknown).toBe(false);
});

it('never releases a prepared operation on a context-change proof',()=>{
 const source={project_id:'p',path:'a.ts',content:'before',hash:'before-sha',language:'typescript',workspace_name:'Original',context_sha256:'a'.repeat(64)};
 const pending=new FileSaveRequest(),request=pending.begin('p',source,'submitted');
 pending.failed(new ApiError(409,'workspace_reconciliation_required','Prepared',{operation_id:request.idempotency_key,path:source.path}));
 pending.failed(new ApiError(409,'workspace_context_changed','Not accepted',{project_id:'p',path:source.path,operation_id:request.idempotency_key,expected_context_sha256:source.context_sha256,not_accepted:true}));
 expect(pending.pending).toBe(request);expect(pending.reconciliation).toBe(true);expect(pending.unknown).toBe(true);
});
