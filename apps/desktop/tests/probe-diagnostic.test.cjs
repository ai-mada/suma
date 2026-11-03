'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {validInstalledToolReport}=require('../src/installed-tools.cjs');

test('T226 preserves only fixed probe diagnostics across all nine installation tuples and rejects null, aliases, unknown fields and contradictory facts',()=>{
 const products={'codex-jsonl':'Codex','codebuddy-cli':'CodeBuddy Code','dsh-multica':'DeepSeek Harness / dsh','claude-code-cli':'Claude Code','cursor-cli':'Cursor','qoder-acp':'Qoder CLI','codex-app-server':'Codex','opencode-sdk':'OpenCode','claude-code-sdk':'Claude Code'};
 const diagnostic={stage:'help',code:'help_contract_mismatch',next_action:'guide'};
 const check=row=>validInstalledToolReport({protocol:'suma-installed-tools-v1',items:[row]},row.adapter_id);
 for(const [adapter_id,product] of Object.entries(products)){
  const row={adapter_id,product,installed:true,launch_supported:false,reason_code:'probe_failed',diagnostic};
  const safe=check(row);assert.deepEqual(safe?.items[0].diagnostic,diagnostic);assert.notEqual(safe.items[0].diagnostic,diagnostic);assert.equal(safe.items[0].reason_code,'probe_failed');
  for(const invalid of [null,undefined,{}, {...diagnostic,nextAction:'guide'}, {...diagnostic,stage:'version'}, {...diagnostic,next_action:'scan'}, {...diagnostic,code:'stderr_guessed'}, {...diagnostic,stderr:'private-output'}, {stage:'version',code:'stop_unverified',next_action:'scan'}])assert.equal(check({...row,diagnostic:invalid}),undefined);
  assert.equal(check({...row,diagnostic:{...diagnostic,stage:'source',code:'permission_denied'}})?.items[0].diagnostic.code,'permission_denied');
  assert.equal(check({...row,diagnostic:{stage:'version',code:'stop_unverified',next_action:'none'}})?.items[0].diagnostic.next_action,'none');
  const absent={adapter_id,product,installed:false,launch_supported:false,reason_code:'not_installed'};assert.deepEqual(check(absent).items[0],absent);assert.equal(check({...absent,diagnostic}),undefined);
  const incomplete={...absent,reason_code:'scan_incomplete',diagnostic:{stage:'resolve',code:'source_unavailable',next_action:'guide'}};assert.equal(check(incomplete).items[0].reason_code,'scan_incomplete');assert.equal(check({...incomplete,installed:true}),undefined);assert.equal(check({...incomplete,version:'not-verified'}),undefined);assert.equal(check({...incomplete,diagnostic}),undefined);
 }
 const privateCommand={executable:'private-local-path',env:{PRIVATE_VALUE:'private-secret'}};
 const row={adapter_id:'codebuddy-cli',product:'CodeBuddy Code',installed:true,launch_supported:false,reason_code:'probe_failed',diagnostic,command:privateCommand};
 assert.deepEqual(check(row).items[0],{adapter_id:row.adapter_id,product:row.product,installed:true,launch_supported:false,reason_code:'probe_failed',diagnostic});assert.equal(JSON.stringify(check(row)).includes('private-'),false);assert.equal(check({...row,raw_stderr:'private-output'}),undefined);
 const cursor={adapter_id:'cursor-cli',product:'Cursor',installed:true,launch_supported:false,reason_code:'acp_configuration_required',executable:process.execPath.replace(/[^\\/]+$/,'node.exe'),version:'reviewed-package-version',executable_sha256:'a'.repeat(64)};
 assert.equal(check(cursor)?.items[0].executable_sha256,cursor.executable_sha256);
 const {version,...versionless}=cursor;assert.equal(check(versionless),undefined);
 for(const change of [{diagnostic},{command:privateCommand},{executable_sha256:'invalid'},{executable_sha256:['a'.repeat(64)]},{launch_supported:true},{version:' unreviewed whitespace '},{executable:process.execPath.replace(/[^\\/]+$/,'arbitrary.exe')}])assert.equal(check({...cursor,...change}),undefined);
 const failedCursor={...cursor,reason_code:'probe_failed',diagnostic};assert.equal(check(failedCursor),undefined);
});
