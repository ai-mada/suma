import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {nativeCatalog,nativeDisplay,type NativeMessageKey} from '../src/display.js';

test('native catalog preserves the original 50 definitions and paired purpose extensions',()=>{
 const entries=Object.entries(nativeCatalog).map(([key,entry])=>({key,en:entry.en,zh:entry.zh,params:Object.entries('params' in entry?entry.params:{}).map(([name,kind])=>({name,kind}))})).sort((a,b)=>a.key<b.key?-1:a.key>b.key?1:0);
 assert.equal(entries.length,52);
 // T-P4-020 authority-generated baseline: the original 46 definitions plus
 // four dedicated workspace diagnostics; unknown keys remain unaccepted.
 const original=entries.filter(entry=>!['native.request.invalid_purpose','native.knowledge.tools_forbidden'].includes(entry.key));
 assert.equal(original.length,50);
 assert.equal(createHash('sha256').update(JSON.stringify(original)).digest('hex'),'5fc7a465774ab720caca5e87a90a8a7c1f2d5924fece17697a97701ae7cd28b3');
 assert.deepEqual(nativeCatalog['native.request.invalid_purpose'],{en:'The selected purpose requires its dedicated read-only model configuration without a workspace bridge.',zh:'所选用途需要对应的专用只读模型配置，且不能连接工作区。'});
 assert.deepEqual(nativeCatalog['native.knowledge.tools_forbidden'],{en:'Knowledge extraction cannot request or execute tools.',zh:'知识提炼不允许请求或执行工具。'});
 for(const entry of entries){
  const params=entry.key==='native.endpoint.rejected'?{status:0}:entry.key==='native.catalog.probe_exited'?{status:'{{.other}} <tag> 原始'}:{};
  for(const locale of ['zh-CN','en-US'] as const){
   const display=nativeDisplay(entry.key as NativeMessageKey,locale,params)!;
   assert.ok(display,entry.key);
   assert.equal(display.localized_message,(locale==='en-US'?entry.en:entry.zh).replace('{{.status}}',String(params.status)));
   assert.equal(display.message_key,entry.key);
   assert.equal(display.locale,locale);
  }
 }
});

test('native text parameters reject unpaired UTF16 surrogates and preserve valid UTF8 boundaries',()=>{
 for(const status of ['\ud800','\udfff','prefix\ud800suffix','\udfff\ud800','\u0000','\u0085','a'.repeat(513),'码'.repeat(171)]){
  assert.equal(nativeDisplay('native.catalog.probe_exited','en-US',{status}),undefined,'invalid text must not replace original evidence');
 }
 for(const status of ['a'.repeat(512),'码'.repeat(170),'😀'.repeat(128),'{{.status}} <tag> 原始']){
  assert.equal(nativeDisplay('native.catalog.probe_exited','en-US',{status})?.localized_message,'CLI probe exited '+status);
 }
});
