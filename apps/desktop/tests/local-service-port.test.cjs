'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),net=require('node:net');
test('stable desktop port rejects malformed overrides instead of selecting another endpoint',()=>{
 const {selectLocalServicePort}=require('../src/local-service-port.cjs');
 assert.equal(selectLocalServicePort({}),4319);for(const value of ['1024','4321','65535'])assert.equal(selectLocalServicePort({SUMA_DESKTOP_PORT:value}),Number(value));
 for(const value of ['', '0','80','1023','65536',' 4319','4319 ','+4319','4319.0','1e3','abc',4319])assert.throws(()=>selectLocalServicePort({SUMA_DESKTOP_PORT:value}));
});
test('occupied stable endpoint fails and never attaches to the listener',async()=>{
 const {reserveLocalServicePort}=require('../src/local-service-port.cjs'),server=net.createServer();
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));const port=server.address().port;
 try{await assert.rejects(reserveLocalServicePort(port));}finally{await new Promise(resolve=>server.close(resolve));}
 assert.equal(await reserveLocalServicePort(port),port);
});
