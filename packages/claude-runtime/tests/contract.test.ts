import test from 'node:test';
import assert from 'node:assert/strict';
import {sha256,questions,nativeAnswers,answerDigest,permissionDigest,parseLine,parseRequest,inputJSON,type InputRequest,type PermissionSource} from '../src/contract.js';
const raw=JSON.stringify({questions:[{question:'选择数据库？',header:'数据库',options:[{label:'SQLite',description:'本地'},{label:'OceanBase',description:'团队'}],multiSelect:true}],answers:{}});
const basis={session_id:'session-original',tool_use_id:'tool-original',request_id:'request-native',callback_id:'callback-host',configuration_sha256:'a'.repeat(64),params_sha256:sha256(raw)};
const source:InputRequest={...basis,input_json:raw,questions:questions(raw)};

test('T192 exact native question identity preserves original labels and order',()=>{
 const answer={outcome:'answered' as const,answers:[{question_id:'q1',option_ids:['o2','o1']}]};assert.deepEqual(nativeAnswers(source,answer),{'选择数据库？':'SQLite, OceanBase'});
 assert.equal(answerDigest(source,answer),answerDigest(source,{...answer,answers:[{question_id:'q1',option_ids:['o1','o2']}]}));
 assert.throws(()=>nativeAnswers(source,{outcome:'answered',answers:[{question_id:'q1',option_ids:['o1','o1']}]}));
 for(const patch of [{answers:{'选择数据库？':'preset'}},{metadata:{}},{annotations:{}}])assert.throws(()=>questions(JSON.stringify({...JSON.parse(raw),...patch})));
 assert.throws(()=>questions(JSON.stringify({questions:[{...JSON.parse(raw).questions[0],options:[{label:'same',description:''},{label:'same',description:''}]}]})));
});
test('T192 decisions use UTF8 byte-length prefixes and bind every native source',()=>{
 const permission:PermissionSource={...basis,params_sha256:sha256('{}'),tool_name:'Bash',input_json:'{}'};const decision={outcome:'reject_once' as const,reason:'拒绝这一次'};
 const digest=permissionDigest(permission,decision);assert.match(digest,/^[a-f0-9]{64}$/);for(const field of ['request_id','session_id','tool_use_id','callback_id'] as const)assert.notEqual(permissionDigest({...permission,[field]:permission[field]+'2'},decision),digest);
 assert.notEqual(permissionDigest(permission,{...decision,reason:'不同'}),digest);
 assert.throws(()=>permissionDigest(permission,{outcome:'allow_once',updatedPermissions:[]} as never));
});
test('T192 bounded private frames reject duplicate keys and malformed Unicode',()=>{
 for(const raw of ['{"v":1,"v":1}','{"value":{"a":1,"a":2}}','{"value":"\\ud800"}','{"value":'+ '['.repeat(40)+'0'+']'.repeat(40)+'}'])assert.throws(()=>parseLine(raw));
 const value={v:1,type:'execute',request_id:'original',goal:'中文🙂',configuration_sha256:'a'.repeat(64)};assert.deepEqual(parseRequest(parseLine(JSON.stringify(value))),value);assert.throws(()=>parseRequest({...value,env:{}}));assert.throws(()=>parseRequest({...value,goal:'x'.repeat(262145)}));
 const large={text:'x'.repeat(64000)};assert.equal(inputJSON(large),JSON.stringify(large));assert.throws(()=>inputJSON({text:'x'.repeat(65536)}));
});
test('T192 callback source refuses credentials and non-JSON payload mutation',()=>{
 assert.throws(()=>inputJSON({command:'echo fixture-api-secret'},'fixture-api-secret'));
 assert.throws(()=>inputJSON({api_key:'hidden'}));assert.throws(()=>inputJSON({text:'\ud800'}));assert.throws(()=>inputJSON({value:undefined}));
 const circular:Record<string,unknown>={};circular.next=circular;assert.throws(()=>inputJSON(circular));
 assert.equal(inputJSON({file_path:'source.ts',content:'ordinary text'}),' {"file_path":"source.ts","content":"ordinary text"}'.trim());
});
