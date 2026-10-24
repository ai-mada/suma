import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {nativeCatalog,nativeDisplay,type NativeMessageKey} from '../src/display.js';

test('native catalog preserves all 46 bilingual definitions in named-dot-v1 grammar',()=>{
 const entries=Object.entries(nativeCatalog).map(([key,entry])=>({key,en:entry.en,zh:entry.zh,params:Object.entries('params' in entry?entry.params:{}).map(([name,kind])=>({name,kind}))})).sort((a,b)=>a.key<b.key?-1:a.key>b.key?1:0);
 assert.equal(entries.length,46);
 // Captured from the pre-unification SDK catalog at 7ff1b1453a4beaf8893855296b39683c907bf048,
 // with the two status placeholders normalized to the common grammar.
 assert.equal(createHash('sha256').update(JSON.stringify(entries)).digest('hex'),'f5024b58f1cf720fc052163bc7735cf392dd36eb9994c325009f339bd45805bb');
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
