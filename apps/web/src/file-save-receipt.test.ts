import {webcrypto} from 'node:crypto';
import {afterEach,expect,it,vi} from 'vitest';
import {ApiError} from './api';
import {fileSaveReceipt,FileSaveRequest} from './file-saves';
afterEach(()=>vi.unstubAllGlobals());

it('binds acknowledgements to exact path, content and SHA256 of unnormalized UTF8 bytes',async()=>{
 vi.stubGlobal('crypto',webcrypto);
 const request={project_id:'p',path:'src/main.ts',content:'提交 🧪\r\n',expected_hash:'before',idempotency_key:'original-operation'};
 const hash=Array.from(new Uint8Array(await webcrypto.subtle.digest('SHA-256',new TextEncoder().encode(request.content))),byte=>byte.toString(16).padStart(2,'0')).join('');
 const response={path:request.path,content:request.content,hash,language:'typescript'};
 expect(await fileSaveReceipt(response,request)).toEqual(response);
 for(const wrong of [null,{},[],{...response,path:'other.ts'},{...response,content:'提交 🧪\n'},{...response,hash:'0'.repeat(64)},{...response,hash:hash.toUpperCase()},{...response,language:4}])expect(await fileSaveReceipt(wrong,request)).toBeUndefined();
 vi.stubGlobal('crypto',{});expect(await fileSaveReceipt(response,request)).toBeUndefined();
});

it('keeps unknown and reconciliation operations across later refusals but releases a first definite conflict',()=>{
 const source={path:'a.ts',content:'before',hash:'before-sha'};
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
