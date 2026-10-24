const {test}=require('node:test');
const assert=require('node:assert/strict');
const http=require('node:http');
const {spawn}=require('node:child_process');
const {waitForHealth,allowedNavigation,terminateService}=require('../src/lifecycle.cjs');
test('waits for health and rejects an exited control service',async()=>{
 const server=http.createServer((_req,res)=>{res.writeHead(200,{'Content-Type':'application/json'});res.end('{"status":"ok"}')});
 await new Promise(r=>server.listen(0,'127.0.0.1',r));const url=`http://127.0.0.1:${server.address().port}`;
 try{await waitForHealth(url,()=>false,1000);await assert.rejects(()=>waitForHealth(url,()=>true,1000),/exited/)}finally{await new Promise(r=>server.close(r))}
});
test('service termination resolves only after the owned process exits',async()=>{const child=spawn(process.execPath,['-e','setTimeout(()=>{},5000)'],{stdio:'ignore',windowsHide:true});try{await new Promise((r,j)=>{child.once('spawn',r);child.once('error',j)});await terminateService(child,250);assert.notEqual(child.exitCode===null && child.signalCode===null,true)}finally{if(child.exitCode===null)child.kill()}});
test('navigation stays within the owned control origin',()=>{
 assert.equal(allowedNavigation('http://127.0.0.1:4310/development','http://127.0.0.1:4310'),true);
 for(const candidate of ['https://evil.test','javascript:alert(1)','file:///etc/passwd','http://127.0.0.1:4311'])assert.equal(allowedNavigation(candidate,'http://127.0.0.1:4310'),false);
});
