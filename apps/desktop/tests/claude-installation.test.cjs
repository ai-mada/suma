'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),path=require('node:path');
const {validInstalledToolReport}=require('../src/installed-tools.cjs');
test('Claude native installation observation crosses the bridge without launch authority',()=>{
 const report=validInstalledToolReport({protocol:'suma-installed-tools-v1',items:[{adapter_id:'claude-code-cli',product:'Claude Code',installed:true,launch_supported:true,reason_code:'workspace_required',executable:path.resolve('owned-claude.exe'),version:'2.1.314 (Claude Code)',command:{executable:'private-native-path',args:['private-launch-arguments'],env:{secret:'not-for-renderer'}}}]},'claude-code-cli');
 assert.ok(report);assert.equal(report.items[0].product,'Claude Code');assert.equal(report.items[0].launch_supported,true);assert.equal(Object.hasOwn(report.items[0],'command'),false);assert.equal(JSON.stringify(report).includes('private-launch-arguments'),false);assert.equal(JSON.stringify(report).includes('not-for-renderer'),false);
});
