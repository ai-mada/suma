import {beforeEach,expect,it} from 'vitest';
import {renderDisplayDiagnostic} from './display-diagnostic';
import {setLocale,setServerCatalog,renderDiagnostic} from './i18n';

// Independent public-contract fixtures, never used as a production source catalog.
const cases = [
 ['profile_revoked','工具档案已永久撤销','Tool profile has been permanently revoked'],
 ['profile_paused','工具档案已暂停，尚未授权派发','Tool profile is paused and is not authorized for dispatch'],
 ['activation_revoked','工具档案启用人的当前审批权限已失效','The enabling user no longer has current approval authority'],
 ['worker_expired','执行节点凭证已过期或撤销','Worker credential has expired or been revoked'],
 ['worker_creator_revoked','执行节点登记人的当前项目权限已失效','The worker registering user no longer has current project authority'],
 ['worker_unregistered','执行节点未登记、未获项目授权或机器凭证已撤销','Worker is unregistered, unauthorized for this project, or its machine credential is revoked'],
 ['pool_paused','资源池已暂停，停止新派发','Execution pool is paused; new dispatch is stopped'],
 ['pool_unavailable','资源池当前状态不允许新派发','Execution pool does not currently permit new dispatch'],
 ['pool_expired','资源池执行节点凭证已过期','Execution pool worker credential has expired'],
 ['inventory_missing','执行节点尚未上报工具清单','Worker has not reported tool inventory'],
 ['inventory_stale','工具清单已过期，等待执行节点重新探测','Tool inventory has expired; waiting for worker reprobe'],
 ['command_missing','当前执行节点未声明此命令','Current worker has not declared this command'],
 ['profile_mismatch','当前工具配置、版本或程序摘要与固定档案不匹配','Current tool configuration, version, or executable digest does not match the pinned profile'],
 ['dispatchable','执行节点已报告可领取；模型认证、执行与验收结果需分别核验','Worker reports dispatch readiness; model authentication, execution, and acceptance must be checked separately'],
] as const;
const catalog = {version:'display-test',template_format:'named-dot-v1',default_locale:'zh-CN',supported_locales:['zh-CN','en-US'],messages:[
 ...cases.map(([key,source,en])=>({key:'display.tool.'+key,parameters:[],texts:{'zh-CN':source,'en-US':en}})),
 {key:'display.tool.reported_unavailable',parameters:[{name:'reason',kind:'text'}],texts:{'zh-CN':'执行节点报告工具不可用：{{.reason}}','en-US':'Worker reported the tool unavailable: {{.reason}}'}},
]};
beforeEach(async()=>{await setLocale('en-US',{persist:false});setServerCatalog(catalog);});
it.each(cases)('proves the complete %s source before displaying catalog text',async(key,source,en)=>{
 const hint={schema_version:1,message_key:'display.tool.'+key};
 expect(renderDisplayDiagnostic(source,hint)).toEqual({text:en,localized:true});
 expect(renderDisplayDiagnostic(source+' ',hint)).toEqual({text:source+' ',localized:false});
 expect(renderDisplayDiagnostic(source,undefined)).toEqual({text:source,localized:false});
 await setLocale('zh-CN',{persist:false});expect(renderDisplayDiagnostic(source,hint)).toEqual({text:source,localized:true});
});
it.each(cases)('does not give %s ambient authority through either generic diagnostic path',async(key,source,en)=>{
 expect(renderDiagnostic(source)).toBe(source);
 const message='External tool returned an unrelated original error';
 expect(renderDiagnostic({message,message_key:'display.tool.'+key})).toBe(message);
 // Even a valid schema/source pair must use the dedicated bound renderer.
 expect(renderDiagnostic({message:source,schema_version:1,message_key:'display.tool.'+key})).toBe(source);
 await setLocale('zh-CN',{persist:false});expect(renderDiagnostic(en)).toBe(en);
});
it.each([
 undefined,null,[],{schema_version:2,message_key:'display.tool.profile_paused'},
 {schema_version:1,message_key:'display.tool.future'},
 {schema_version:1,message_key:'native.endpoint.rejected'},
 {schema_version:1,message_key:'display.tool.profile_paused',extra:true},
 {schema_version:1,message_key:'display.tool.profile_paused',message_params:{extra:'text'}},
 {schema_version:1,message_key:'display.tool.profile_paused',message_params:null},
 {schema_version:1,message_key:'display.tool.profile_paused',message_params:[]},
])('retains source for invalid or foreign identity %j',hint=>{
 const source='工具档案已暂停，尚未授权派发';expect(renderDisplayDiagnostic(source,hint)).toEqual({text:source,localized:false});
});
it('treats external reasons as data without recursive translation, mutation or truncation',()=>{
 const reason='native.endpoint.rejected {{.reason}} <script>用户文字</script>';
 const hint={schema_version:1,message_key:'display.tool.reported_unavailable',message_params:{reason}};
 const before=JSON.stringify(hint);
 expect(renderDisplayDiagnostic('执行节点报告工具不可用：'+reason,hint)).toEqual({text:'Worker reported the tool unavailable: '+reason,localized:true});
 expect(JSON.stringify(hint)).toBe(before);
});
it.each(['', '\uD800', 'a\nreason', 'a'.repeat(513), '中'.repeat(171), 2, null])('rejects invalid external reason %j without changing raw source',reason=>{
 const source='执行节点报告工具不可用：'+reason;
 expect(renderDisplayDiagnostic(source,{schema_version:1,message_key:'display.tool.reported_unavailable',message_params:{reason}})).toEqual({text:source,localized:false});
});
it('accepts the exact 512-byte text boundary',()=>{
 const reason='a'.repeat(512);expect(renderDisplayDiagnostic('执行节点报告工具不可用：'+reason,{schema_version:1,message_key:'display.tool.reported_unavailable',message_params:{reason}}).localized).toBe(true);
});
it('falls back until a compatible catalog arrives, without using generic source aliases',()=>{
 const source='工具档案已暂停，尚未授权派发',hint={schema_version:1,message_key:'display.tool.profile_paused'};
 setServerCatalog({...catalog,messages:[]});expect(renderDisplayDiagnostic(source,hint)).toEqual({text:source,localized:false});
 setServerCatalog(catalog);expect(renderDisplayDiagnostic(source,hint).localized).toBe(true);
 setServerCatalog({...catalog,messages:[{key:'display.tool.profile_paused',parameters:[{name:'reason',kind:'text'}],texts:{'zh-CN':'{{.reason}}','en-US':'{{.reason}}'}}]});
 expect(renderDisplayDiagnostic(source,hint)).toEqual({text:source,localized:false});
});
