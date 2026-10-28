import {test} from 'node:test';
import assert from 'node:assert/strict';
import {validateFinal,stableCallIdentity,parseExecute} from '../src/contract.js';

const identity={sessionID:'ses_test',userMessageID:'msg_user',providerID:'fixed',model:'fixed-model'};
const complete=()=>({info:{id:'msg_assistant',role:'assistant',sessionID:'ses_test',parentID:'msg_user',providerID:'fixed',modelID:'fixed-model',time:{created:1,completed:2},finish:'stop'},parts:[{type:'text',text:'done'}]});
test('whole-turn success requires a correlated final completed assistant, never a step or HTTP acknowledgement',()=>{
 assert.deepEqual(validateFinal(complete(),identity),{assistant_message_id:'msg_assistant',completed_at:2});
 for(const mutate of [(x:any)=>x.info.role='user',(x:any)=>x.info.parentID='foreign',(x:any)=>x.info.sessionID='foreign',(x:any)=>x.info.providerID='other',(x:any)=>x.info.modelID='other',(x:any)=>delete x.info.time.completed,(x:any)=>x.info.finish='tool-calls',(x:any)=>x.info.finish='length',(x:any)=>x.info.error={name:'MessageAbortedError'},(x:any)=>x.parts.push({type:'tool',state:{status:'running'}})]){const value=complete();mutate(value);assert.throws(()=>validateFinal(value,identity));}
 assert.throws(()=>validateFinal({type:'step_finish'},identity));
});
test('stable tool identities separate calls in one assistant message and refuse absent runtime call IDs',()=>{
 const context={sessionID:'ses_a',messageID:'msg_a',callID:'call_one'};
 assert.equal(stableCallIdentity('attempt',context,'write'),'["attempt","ses_a","msg_a","call_one","write"]');
 assert.notEqual(stableCallIdentity('attempt',context,'write'),stableCallIdentity('attempt',{...context,callID:'call_two'},'write'));
 for(const bad of [{...context,callID:undefined},{...context,callID:''},{...context,messageID:''},{...context,sessionID:'bad\nvalue'}])assert.throws(()=>stableCallIdentity('attempt',bad,'write'));
});
test('closed execute protocol rejects hidden configuration and conflict repair authority',()=>{
 const bridge={version:1,endpoint:'http://127.0.0.1:1234/mcp',token:'test-secret-token',policy_sha256:'a'.repeat(64),request_id:'request'};
 assert.equal(parseExecute({v:1,type:'execute',request_id:'request',goal:'fix',workspace_bridge:bridge}).goal,'fix');
 for(const value of [{v:1,type:'execute',request_id:'request',goal:'fix',workspace_bridge:bridge,model:'foreign'},{v:1,type:'execute',request_id:'request',goal:'fix',workspace_bridge:{...bridge,conflict_repair_protocol:'suma.candidate-conflict-repair.v1'}}])assert.throws(()=>parseExecute(value));
});
