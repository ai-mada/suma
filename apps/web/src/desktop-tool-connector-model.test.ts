import {expect,it} from 'vitest';
import {desktopToolBridge,readDesktopToolResult,validDesktopToolScope} from './desktop-tool-connector-model';

const scope={project:'1'.repeat(32),userID:'2'.repeat(32),organizationID:'3'.repeat(32),adapterID:'codex-jsonl'};
const connection={project_id:scope.project,adapter_id:'codex-jsonl',phase:'connected',worker_id:'desktop-'+'4'.repeat(32),base_commit:'a'.repeat(40),expires_at:'2099-01-01T00:00:00Z',repository_name:'project'};
it('accepts only the supported native function and exact current identity scope',()=>{
 const manageLocalTool=async()=>({});
 expect(desktopToolBridge({platform:'win32',version:'1.0.0',manageLocalTool})).toBeDefined();
 for(const value of [{platform:'web',version:'1.0.0',manageLocalTool},{platform:'linux',version:'2.0.0',manageLocalTool},{platform:'linux',version:'1.0.0',manageLocalTool:true}])expect(desktopToolBridge(value)).toBeUndefined();
 expect(validDesktopToolScope(scope)).toBe(true);
 for(const change of [{project:'p'},{userID:'A'.repeat(32)},{organizationID:''},{adapterID:'planned-tool'}])expect(validDesktopToolScope({...scope,...change})).toBe(false);
});
it('rejects leaked fields and mismatched connection facts before presenting a node',()=>{
 expect(readDesktopToolResult({status:'ok',connection},scope)).toEqual({status:'ok',connection});
 for(const change of [{project_id:'5'.repeat(32)},{adapter_id:'codebuddy-cli'},{phase:'healthy'},{worker_id:undefined},{base_commit:'main'},{expires_at:'tomorrow'},{repository_name:'secret\nline'},{token:'secret'},{path:'D:/private'},{command:['run']}])expect(readDesktopToolResult({status:'ok',connection:{...connection,...change}},scope)).toBeUndefined();
 expect(readDesktopToolResult({status:'ok',connection,token:'secret'},scope)).toBeUndefined();
 expect(readDesktopToolResult({status:'failed',code:'authorization_required'},scope)).toEqual({status:'failed',code:'authorization_required'});
 expect(readDesktopToolResult({status:'failed',code:'failed',environment:{}},scope)).toBeUndefined();
});
it('keeps Qoder fixed configuration facts scoped to the ACP connection',()=>{
 const qoderScope={...scope,adapterID:'qoder-acp'},qoderConnection={...connection,adapter_id:'qoder-acp',model:'model:一',tool_version:'Qoder 1.2.3',executable_sha256:'a'.repeat(64),configuration_sha256:'b'.repeat(64)};
 expect(validDesktopToolScope(qoderScope)).toBe(true);
 expect(readDesktopToolResult({status:'ok',connection:qoderConnection},qoderScope)).toEqual({status:'ok',connection:qoderConnection});
 for(const patch of [{model:'bad\nmodel'},{model:'x'.repeat(257)},{tool_version:''},{executable_sha256:'bad'},{configuration_sha256:'A'.repeat(64)},{acp_selection:{installation_id:'private'}}])expect(readDesktopToolResult({status:'ok',connection:{...qoderConnection,...patch}},qoderScope)).toBeUndefined();
 expect(readDesktopToolResult({status:'ok',connection:{...qoderConnection,adapter_id:scope.adapterID}},scope)).toBeUndefined();
});
