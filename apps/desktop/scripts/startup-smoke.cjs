'use strict';
const fs=require('node:fs');
const path=require('node:path');
const net=require('node:net');
const {spawn}=require('node:child_process');
const {randomBytes}=require('node:crypto');
const {failure}=require('./build-plan.cjs');
const {controlEnvironment}=require('../src/workspace-grants.cjs');
const {waitForHealth,terminateService}=require('../src/lifecycle.cjs');

async function availablePort(){
 return new Promise((resolve,reject)=>{const server=net.createServer();server.once('error',reject);server.listen(0,'127.0.0.1',()=>{const port=server.address().port;server.close(error=>error?reject(error):resolve(port))})});
}
async function smokeControl(plan,raw,resources,options={}){
 if(plan.platform!==plan.host.platform||plan.arch!==plan.host.arch)return 'not_run_cross_target';
 const data=path.join(raw,'smoke-data');fs.mkdirSync(data,{mode:0o700});
 const temporary=path.join(data,'tmp');fs.mkdirSync(temporary,{mode:0o700});
 const port=await availablePort(),origin=`http://127.0.0.1:${port}`;
 const parent=Object.fromEntries(Object.entries(options.environment||process.env).filter(([name])=>!/^SUMA_/i.test(name)));
 const env=controlEnvironment(parent,{data,web:path.join(resources,'web'),origin,key:randomBytes(32)});
 Object.assign(env,{TEMP:temporary,TMP:temporary,TMPDIR:temporary});
 env.SUMA_SOURCE_INSPECTOR_NODE=path.join(resources,'source-inspector',plan.platform==='win32'?'node.exe':'node');
 env.SUMA_SOURCE_INSPECTOR_ENTRY=path.join(resources,'source-inspector','source-inspector.bundle.mjs');
 if(options.command)env.TEST_PORT=String(port);
 const binary=options.command||path.join(resources,'control',plan.platform==='win32'?'suma-control.exe':'suma-control');
 const child=spawn(binary,options.args||['--addr',`127.0.0.1:${port}`],{cwd:data,env,stdio:'ignore',windowsHide:true});
 let exited=false;child.once('error',()=>{exited=true});child.once('exit',()=>{exited=true});
 try{await waitForHealth(origin,()=>exited,15000,plan.language);return 'passed'}
 catch{throw failure('startup_smoke_failed','',plan.language)}
 finally{await terminateService(child,5000,plan.language)}
}
module.exports={smokeControl};
