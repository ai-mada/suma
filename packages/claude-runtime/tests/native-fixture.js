// Owned protocol fixture only. No Claude implementation, vendor account,
// network endpoint or model is invoked. Production accepts a reviewed native
// executable; the SDK transport test explicitly launches this file with Node.
import fs from 'node:fs';
import readline from 'node:readline';
import assert from 'node:assert/strict';
const log=process.env.SUMA_CLAUDE_FIXTURE_LOG,send=value=>process.stdout.write(JSON.stringify(value)+'\n');
const permissionInput={command:'git status --short'},questionInput={questions:[{question:'选择数据库？',header:'数据库',options:[{label:'SQLite',description:'本地'},{label:'OceanBase',description:'团队'}],multiSelect:true}],answers:{}};
let stage=0,session='fixture-sdk-session';
const assistant=(id,name,input)=>send({type:'assistant',session_id:session,parent_tool_use_id:null,message:{id:'message-'+id,role:'assistant',content:[{type:'tool_use',id,name,input}],model:'claude-sonnet-5',usage:{input_tokens:1,output_tokens:0}}});
const request=(id,tool,input,toolID)=>send({type:'control_request',request_id:id,request:{subtype:'can_use_tool',tool_name:tool,input,tool_use_id:toolID}});
const toolResult=id=>send({type:'user',session_id:session,parent_tool_use_id:null,message:{role:'user',content:[{type:'tool_result',tool_use_id:id,content:'owned fixture result',is_error:false}]}});
readline.createInterface({input:process.stdin}).on('line',line=>{try{const value=JSON.parse(line);if(log)fs.appendFileSync(log,JSON.stringify(value)+'\n');
 if(value.type==='control_request'){
  if(value.request?.subtype==='initialize'){send({type:'control_response',response:{subtype:'success',request_id:value.request_id,response:{commands:[],models:[],account:{},output_style:'default',available_output_styles:[]}}});return;}
  if(value.request?.subtype==='interrupt'){send({type:'control_response',response:{subtype:'success',request_id:value.request_id,response:{}}});return;}
  throw Error('unexpected SDK control request');
 }
 if(value.type==='user'){
  assert.equal(stage,0);assert.equal(value.message.role,'user');stage=1;
  send({type:'system',subtype:'init',session_id:session,model:'claude-sonnet-5',claude_code_version:'2.1.314',permissionMode:'default',apiKeySource:'ANTHROPIC_API_KEY',cwd:process.cwd(),tools:['Read','Edit','Write','Glob','Grep','Bash','AskUserQuestion'],mcp_servers:[],slash_commands:[],uuid:'fixture-init'});
  setImmediate(()=>{assistant('tool-permission','Bash',permissionInput);request('native-permission-id','Bash',permissionInput,'tool-permission');});return;
 }
 if(value.type==='control_response'){
  const response=value.response;assert.equal(response.subtype,'success');const decision=response.response;assert.equal(decision.behavior,'allow');assert.equal(decision.updatedPermissions,undefined);
  if(stage===1){assert.equal(response.request_id,'native-permission-id');assert.deepEqual(decision.updatedInput,permissionInput);stage=2;toolResult('tool-permission');assistant('tool-question','AskUserQuestion',questionInput);request('native-question-id','AskUserQuestion',questionInput,'tool-question');return;}
  assert.equal(stage,2);assert.equal(response.request_id,'native-question-id');assert.deepEqual(decision.updatedInput,{...questionInput,answers:{'选择数据库？':'SQLite, OceanBase'}});stage=3;toolResult('tool-question');
  send({type:'result',subtype:'success',is_error:false,session_id:session,result:'fixture complete',permission_denials:[],num_turns:1,duration_ms:1,duration_api_ms:0,total_cost_usd:0,usage:{input_tokens:2,output_tokens:3,cache_read_input_tokens:0,cache_creation_input_tokens:0},modelUsage:{'claude-sonnet-5':{inputTokens:2,outputTokens:3,cacheReadInputTokens:0,cacheCreationInputTokens:0}},uuid:'fixture-result'});return;
 }
 throw Error('unexpected SDK input type');
}catch(error){if(log)fs.appendFileSync(log,JSON.stringify({fixture_error:error.message})+'\n');process.stderr.write('Owned SDK fixture protocol failed.\n');process.exitCode=2;process.stdin.destroy();}});
