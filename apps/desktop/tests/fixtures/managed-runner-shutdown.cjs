'use strict';
const assert=require('node:assert/strict');

function shutdownLine(child,workerID,overrides){
 return JSON.stringify({type:'suma.local_runner.shutdown',worker_id:workerID,pid:child.pid,...overrides})+'\n';
}

function acknowledgeStop(child,input){
 const [configuration,request,tail]=input.split('\n');
 assert.equal(request,'stop');
 assert.equal(tail,'');
 child.stdout.write(shutdownLine(child,JSON.parse(configuration).worker_id));
}

module.exports={shutdownLine,acknowledgeStop};
