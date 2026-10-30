'use strict';
const net=require('node:net');
function selectLocalServicePort(environment={}){
 const value=environment.SUMA_DESKTOP_PORT;
 if(value===undefined)return 4319;
 if(typeof value!=='string'||!(/^[1-9][0-9]{0,4}$/).test(value)||Number(value)<1024||Number(value)>65535)throw Error('Invalid desktop service port');
 return Number(value);
}
function reserveLocalServicePort(port){
 if(!Number.isInteger(port)||port<1024||port>65535)return Promise.reject(Error('Invalid desktop service port'));
 return new Promise((resolve,reject)=>{
  const listener=net.createServer();listener.once('error',()=>reject(Error('Desktop service port is unavailable')));
  listener.listen({port,host:'127.0.0.1',exclusive:true},()=>listener.close(error=>error?reject(Error('Desktop service port is unavailable')):resolve(port)));
 });
}
module.exports={selectLocalServicePort,reserveLocalServicePort};
