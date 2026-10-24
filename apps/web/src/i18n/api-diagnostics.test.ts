// @vitest-environment jsdom
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {api,ApiError} from '../api';
import {renderDiagnostic,setLocale} from './index';
beforeEach(async()=>{await setLocale('en-US',{persist:false});});
afterEach(()=>{vi.unstubAllGlobals();vi.restoreAllMocks();});
it('keeps authoritative HTTP status when a JSON error includes conflicting presentation fields',async()=>{
 vi.stubGlobal('fetch',vi.fn(async()=>new Response(JSON.stringify({error:{code:'forbidden',message:'Original refusal',status:200,localDiagnostic:{ui_key:'common.loading'}}}),{status:403})));
 const error=await api('/fixture').catch(error=>error);expect(error).toBeInstanceOf(ApiError);expect(error.status).toBe(403);expect(error.code).toBe('forbidden');expect(renderDiagnostic(error)).toBe('Original refusal');
});
it('retains a local parameter identity for a non-JSON failure instead of freezing rendered prose',async()=>{
 vi.stubGlobal('fetch',vi.fn(async()=>new Response('<h1>upstream unavailable</h1>',{status:503})));
 const error=await api('/fixture').catch(error=>error);expect(error.status).toBe(503);expect(renderDiagnostic(error)).toBe('Request failed (503); its error receipt was incomplete.');await setLocale('zh-CN',{persist:false});expect(renderDiagnostic(error)).toBe('请求失败（503），错误回执未完整读取。');expect(fetch).toHaveBeenCalledTimes(1);
});
it('relocalizes a missing JSON error message with its exact response status',async()=>{
 vi.stubGlobal('fetch',vi.fn(async()=>new Response(JSON.stringify({error:{code:'upstream'}}),{status:502})));
 const error=await api('/fixture').catch(error=>error);expect(renderDiagnostic(error)).toBe('Request failed (502)');await setLocale('zh-CN',{persist:false});expect(renderDiagnostic(error)).toBe('请求失败（502）');expect(error.code).toBe('upstream');
});
it('keeps a malformed success unconfirmed and does not turn it into a normal failure',async()=>{
 vi.stubGlobal('fetch',vi.fn(async()=>new Response('<html>incomplete receipt</html>',{status:200})));
 const error=await api('/fixture',{original:'body'}).catch(error=>error);expect(error.status).toBe(0);expect(error.code).toBe('response_unknown');await setLocale('zh-CN',{persist:false});expect(renderDiagnostic(error)).toContain('操作结果尚未确认');expect(fetch).toHaveBeenCalledTimes(1);
});
