'use strict';
const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const {smokeControl}=require('../scripts/startup-smoke.cjs');
const root=path.resolve(__dirname,'../../..');
const cache=path.join(root,'.cache/desktop');fs.mkdirSync(cache,{recursive:true});
const plan={root,platform:process.platform,arch:process.arch,host:{platform:process.platform,arch:process.arch},language:'en-US'};
function cleanup(raw){const checked=fs.realpathSync(raw);assert.equal(path.dirname(checked),fs.realpathSync(cache));assert.ok(path.basename(checked).startsWith('smoke-test-'));fs.rmSync(checked,{recursive:true,force:true})}
test('packaging smoke starts a service in isolated data and confirms its health',async()=>{
 const raw=fs.mkdtempSync(path.join(cache,'smoke-test-'));
 const program="const http=require('http'),fs=require('fs'),path=require('path');if(!process.env.SUMA_DATA_DIR||process.env.SUMA_TEST_INHERITED)process.exit(2);const tmp=path.join(process.env.SUMA_DATA_DIR,'tmp');if(['TEMP','TMP','TMPDIR'].some(name=>process.env[name]!==tmp)||!fs.statSync(tmp).isDirectory())process.exit(3);http.createServer((q,r)=>r.end('ok')).listen(Number(process.env.TEST_PORT),'127.0.0.1')";
 try{
  const result=await smokeControl(plan,raw,raw,{command:process.execPath,args:['-e',program],environment:{...process.env,SUMA_TEST_INHERITED:'secret'}});
  assert.equal(result,'passed');assert.ok(fs.existsSync(path.join(raw,'smoke-data')));
 }finally{cleanup(raw)}
});
test('packaging smoke rejects a control process that exits before readiness',async()=>{
 for(const language of ['en-US','zh-CN']){
  const raw=fs.mkdtempSync(path.join(cache,'smoke-test-'));
  try{await assert.rejects(smokeControl({...plan,language},raw,raw,{command:process.execPath,args:['-e','process.exit(9)']}),error=>error.code==='startup_smoke_failed'&&(language==='zh-CN'?/控制服务启动检查失败/:/control startup smoke check failed/).test(error.message))}
  finally{cleanup(raw)}
 }
});
