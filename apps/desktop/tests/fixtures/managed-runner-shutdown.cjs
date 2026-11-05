'use strict';
const assert=require('node:assert/strict');

function shutdownLine(child,workerID,operationID,overrides){
 return JSON.stringify({type:'suma.local_runner.shutdown',worker_id:workerID,pid:child.pid,operation_id:operationID,...overrides})+'\n';
}

function acknowledgeStop(child,input){
 const [configuration,request,tail]=input.split('\n');
 // A failed intent write sends plain EOF so the child can still drain work.
 if(request===''&&tail===undefined)return;
 const operation=/^stop ([a-f0-9]{32})$/.exec(request);
 assert.ok(operation);
 assert.equal(tail,'');
 child.stdout.write(shutdownLine(child,JSON.parse(configuration).worker_id,operation[1]));
}

module.exports={shutdownLine,acknowledgeStop};
