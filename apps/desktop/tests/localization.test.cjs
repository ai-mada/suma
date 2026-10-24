'use strict';
const {test}=require('node:test');
const assert=require('node:assert/strict');
const http=require('node:http');
const vm=require('node:vm');
const fs=require('node:fs');
const path=require('node:path');
const {normalizeLocale,resolveLocale,message,localError}=require('../src/localization.cjs');
const {waitForHealth}=require('../src/lifecycle.cjs');

test('desktop canonical locales and explicit environment remain local preferences',()=>{
 for(const locale of ['zh','zh-Hans','zh_CN','zh-SG'])assert.equal(normalizeLocale(locale),'zh-CN');
 for(const locale of ['en','en-US','en-GB'])assert.equal(normalizeLocale(locale),'en-US');
 for(const locale of ['zh-TW','zh-Hant','en-Cyrl','invalid','en\n'])assert.equal(normalizeLocale(locale),null);
 assert.equal(resolveLocale('en-US',{SUMA_LANGUAGE:'zh'}),'zh-CN');
 assert.equal(resolveLocale('en-US',{}),'en-US');
 assert.equal(resolveLocale('zh-TW',{}),'zh-CN');
});
test('desktop own errors translate while original exception diagnostics survive',()=>{
 assert.equal(message('zh-CN','startup_failed'),'速码无法启动');
 assert.equal(message('en-US','startup_failed'),'Suma could not start');
 const error=localError('zh-CN','termination_failed',new Error('original OS diagnostic ENOENT'));
 assert.equal(error.code,'termination_failed');
 assert.match(error.message,/终止失败/);assert.match(error.message,/original OS diagnostic ENOENT/);
 assert.equal(message('en-US','future_code'),'future_code');
});
test('desktop health errors use requested language on a real local endpoint',async()=>{
 const server=http.createServer((_req,res)=>{res.writeHead(503);res.end()});
 await new Promise(r=>server.listen(0,'127.0.0.1',r));
 const origin=`http://127.0.0.1:${server.address().port}`;
 try{
  await assert.rejects(()=>waitForHealth(origin,()=>true,500,'zh-CN'),/启动前退出/);
  await assert.rejects(()=>waitForHealth(origin,()=>false,20,'en-US'),/did not become healthy/);
 }finally{await new Promise(r=>server.close(r))}
});

// 执行真正的主进程入口，但隔离 Electron/文件存在性；只验证启动失败交互，
// 不把此测试称为实际安装包或真实 Electron 窗口验收。
test('desktop main reads app locale after ready and shows bilingual startup failures',async()=>{
 const source=fs.readFileSync(path.join(__dirname,'../src/main.cjs'),'utf8');
 for(const [appLocale,title,fragment] of [['en-US','Suma could not start','bundled control'],['zh-CN','速码无法启动','内置控制']]){
  const dialogs=[];let ready=false;let quit=0;
  const app={isPackaged:false,requestSingleInstanceLock:()=>true,on:()=>{},quit:()=>quit++,whenReady:()=>Promise.resolve().then(()=>{ready=true}),getLocale:()=>{assert.equal(ready,true);return appLocale}};
  const context={__dirname:path.join(__dirname,'../src'),process:{platform:process.platform,env:{}},require:id=>{
   if(id==='electron')return {app,BrowserWindow:class{},dialog:{showErrorBox:(...args)=>dialogs.push(args)}};
   if(id==='node:fs')return {existsSync:()=>false,mkdirSync:()=>{throw Error('must not create data for missing bundle')}};
   if(id.startsWith('./'))return require(path.join(__dirname,'../src',id));
   return require(id);
  }};
  vm.runInNewContext(source,context);
  await new Promise(resolve=>setImmediate(resolve));
  assert.equal(dialogs.length,1);assert.equal(dialogs[0][0],title);assert.ok(dialogs[0][1].includes(fragment));assert.equal(quit,1);
 }
});
