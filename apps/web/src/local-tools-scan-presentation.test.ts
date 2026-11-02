import {expect,it} from 'vitest';
import {projectLocalToolsScan} from './local-tools-scan-presentation';
import type {InstalledToolFact} from './tool-quick-connect-model';

const missing=(adapter_id:string,product:string):InstalledToolFact=>({adapter_id,product,installed:false,launch_supported:false,reason_code:'not_installed'});

it('keeps distinct CLI and SDK observations in original order and preserves frozen source objects',()=>{
 const cli:InstalledToolFact={adapter_id:'codex-jsonl',product:'Codex',installed:true,launch_supported:true,reason_code:'workspace_required',executable:'C:/tools/codex.exe',version:'codex-cli 1.0.0'};
 const sdk:InstalledToolFact={adapter_id:'codex-app-server',product:'Codex',installed:true,launch_supported:false,reason_code:'configuration_required',executable:cli.executable,version:cli.version,executable_sha256:'a'.repeat(64)};
 const absentClaude=missing('claude-code-sdk','Claude Code'),absentOpenCode=missing('opencode-sdk','OpenCode');
 const items=Object.freeze([absentClaude,cli,absentOpenCode,sdk].map(item=>Object.freeze(item)));
 const before=JSON.stringify(items),result=projectLocalToolsScan(items);
 expect(result.primary).toEqual([cli,sdk]);expect(result.other).toEqual([absentClaude,absentOpenCode]);
 expect(result.primary[0]).toBe(cli);expect(result.primary[1]).toBe(sdk);expect(result.other[0]).toBe(absentClaude);expect(result.other[1]).toBe(absentOpenCode);
 expect(result.detectedCount).toBe(2);expect(result.attentionCount).toBe(0);expect(JSON.stringify(items)).toBe(before);
});

it('keeps the selected unobserved protocol visible without inventing an observation for an unknown selection',()=>{
 const cli=missing('claude-code-cli','Claude Code'),sdk=missing('claude-code-sdk','Claude Code'),opencode=missing('opencode-sdk','OpenCode');
 const items=Object.freeze([cli,sdk,opencode]);
 const selected=projectLocalToolsScan(items,'claude-code-sdk');
 expect(selected.primary).toEqual([sdk]);expect(selected.primary[0]).toBe(sdk);expect(selected.other).toEqual([cli,opencode]);
 expect(selected.detectedCount).toBe(0);expect(selected.attentionCount).toBe(0);
 expect(projectLocalToolsScan(items,'unknown-adapter')).toEqual({primary:[],other:[cli,sdk,opencode],detectedCount:0,attentionCount:0});
});

it('retains false installation anomalies and counts installation observations without inferring execution health',()=>{
 const editorOnly:InstalledToolFact={adapter_id:'cursor-cli',product:'Cursor',installed:false,launch_supported:false,reason_code:'cursor_editor_only'};
 const unsupported:InstalledToolFact={adapter_id:'claude-code-sdk',product:'Claude Code',installed:true,launch_supported:false,reason_code:'version_unsupported',executable:'C:/tools/claude.exe',executable_sha256:'b'.repeat(64),version:'2.1.999'};
 const absent=missing('qoder-acp','Qoder CLI');
 const result=projectLocalToolsScan(Object.freeze([editorOnly,absent,unsupported]));
 expect(result.primary).toEqual([editorOnly,unsupported]);expect(result.other).toEqual([absent]);
 expect(result.detectedCount).toBe(1);expect(result.attentionCount).toBe(1);
 expect(result.primary[0]).toBe(editorOnly);expect(result.primary[1]).toBe(unsupported);expect(unsupported.launch_supported).toBe(false);
 expect(projectLocalToolsScan([])).toEqual({primary:[],other:[],detectedCount:0,attentionCount:0});
});
