'use strict';
// Targeted resource-lifecycle fault injection. No UI bundle, API, credentials,
// Worker, Git candidate, model or vendor journey is repeated here.
const fs=require('node:fs'),path=require('node:path'),http=require('node:http'),assert=require('node:assert/strict');
const{closeOwnedNative}=require('./close-native.cjs'),root=path.resolve(__dirname,'../../..');
async function main(){
 const directory=path.join(root,'.cache/test-dispatch-bindings-216/cleanup',new Date().toISOString().replace(/[-:.]/g,''));fs.mkdirSync(directory,{recursive:true});
 const result={passed:false,scope:'Owned native close only; normal and forced cleanup fault injection',processes:[]};
 const server=http.createServer((_request,response)=>{response.writeHead(404);response.end();});await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));const origin='http://127.0.0.1:'+server.address().port;
 try{
  const{_electron}=require(process.env.SUMA_PLAYWRIGHT_MODULE);
  for(const forced of [false,true]){
   const name=forced?'forced':'normal',dir=path.join(directory,name);fs.mkdirSync(dir);
   const env={};for(const[key,value]of Object.entries(process.env))if(['path','systemroot','windir','comspec','pathext','home','userprofile','homedrive','homepath','appdata','localappdata','programdata','programfiles','programfiles(x86)','programw6432'].includes(key.toLowerCase()))env[key]=value;
   Object.assign(env,{SUMA_FIXTURE_ORIGIN:origin,SUMA_FIXTURE_DIRECTORY:dir,SUMA_FIXTURE_PARTITION:'suma-fixture-close-native-'+name,TEMP:dir,TMP:dir});
   const app=await _electron.launch({executablePath:path.join(root,'apps/desktop/node_modules/electron/dist/electron.exe'),args:[path.join(__dirname,'native.cjs')],cwd:root,env,timeout:15000}),child=app.process(),exit=new Promise(resolve=>child.once('exit',(code,signal)=>resolve({code,signal})));
   await app.firstWindow();
   const owner=forced?{process:()=>child,close:()=>new Promise(()=>{})}:app;
   const closed=await closeOwnedNative(owner,exit,forced?200:15000);result.processes.push({mode:name,pid:child.pid,...closed});
   assert.equal(closed.forced,forced);assert.ok(child.exitCode!==null||child.signalCode!==null);if(!forced)assert.deepEqual(closed.native_exit,{code:0,signal:null});
  }
  result.passed=true;
 }catch(error){result.error=String(error);process.exitCode=1;}
 finally{await new Promise(resolve=>server.close(resolve));result.origin=origin;fs.writeFileSync(path.join(directory,'result.json'),JSON.stringify(result,null,2)+'\n');}
 console.log(JSON.stringify({passed:result.passed,directory,processes:result.processes}));
}
main().catch(error=>{console.error(String(error));process.exitCode=1;});
