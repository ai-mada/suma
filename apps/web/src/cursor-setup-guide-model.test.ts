import {expect,it} from 'vitest';
import {cursorSetupCommands,validCursorSetupDraft,type CursorSetupInput} from './cursor-setup-guide-model';
const input=(shell:'powershell'|'bash',windows=shell==='powershell'):CursorSetupInput=>({shell,packageRoot:windows?"C:\\tools\\Cursor 包 O'Brien $(Write-Output unsafe)":"/srv/Cursor 包 O'Brien $(printf unsafe)",reviewedVersion:'2026.10.22-27dcf5f',model:"模型 O'Brien $(untrusted)",credentialEnvRef:'CURSOR_API_KEY',commands:windows?"C:\\配置 O'Brien $(whoami)\\commands.local.json":"/srv/配置 O'Brien $(whoami)/commands.local.json"});
it('constructs only the five-field UTF-8 stdin request with literal quoting, local encoding recovery and an explicit independent describe draft',()=>{
 for(const shell of ['powershell','bash'] as const)for(const windows of [true,false]){
  const value=input(shell,windows),result=cursorSetupCommands(value);expect(result).toBeDefined();const out=result!;
  const expected={adapter_id:'cursor-cli',executable:value.packageRoot+(windows?'\\node.exe':'/node'),reviewed_version:value.reviewedVersion,model:value.model,credential_env_ref:'CURSOR_API_KEY'};expect(out.request).toEqual(expected);expect(out.draft).toEqual({shell,commands:value.commands,localTool:'cursor-cli'});expect(validCursorSetupDraft(out.draft)).toBe(true);
  if(shell==='powershell'){
   const encoded=/\n\s*('(?:[^']|'')*') \| suma-runner --prepare-tools /.exec(out.prepare);expect(encoded).not.toBeNull();expect(JSON.parse(encoded![1].slice(1,-1).replaceAll("''","'"))).toEqual(expected);
   const positions=[out.prepare.indexOf('$cursorSetupPreviousEncoding = $OutputEncoding'),out.prepare.indexOf('try {'),out.prepare.indexOf('[System.Text.UTF8Encoding]::new($false)'),out.prepare.indexOf(' | suma-runner '),out.prepare.indexOf('finally {'),out.prepare.indexOf('$OutputEncoding = $cursorSetupPreviousEncoding')];expect(positions.every((position,index)=>position>=0&&(index===0||position>positions[index-1]))).toBe(true);expect(out.prepare.startsWith('& {\n')).toBe(true);expect(out.prepare).not.toMatch(/\$global:|SetEnvironmentVariable/);
  }else{
   const encoded=/^printf '%s' (.*) \| suma-runner --prepare-tools /.exec(out.prepare);expect(encoded).not.toBeNull();expect(JSON.parse(encoded![1].slice(1,-1).replaceAll("'\"'\"'","'"))).toEqual(expected);expect(out.prepare).not.toContain('OutputEncoding');
  }
  expect(out.prepare).toContain('--prepare-acp-stdin');expect(out.prepare).not.toMatch(/--local-tool|--allow-exec|--force|--yolo/);expect(out.describe).toContain('--allow-exec --repository \'REPOSITORY\' --base-commit \'COMMIT\' --describe-tools');expect(out.describe).not.toContain('--prepare-acp-stdin');expect(out.describe).not.toContain('CURSOR_API_KEY');expect(value).toEqual(input(shell,windows));
 }
 expect(cursorSetupCommands({...input('bash'),packageRoot:'/srv/cursor\\'})?.request.executable).toBe('/srv/cursor\\/node');
});
it('rejects incomplete, nonlocal, malformed or private configuration rather than inventing a platform, model or credential',()=>{
 const value=input('bash');expect(validCursorSetupDraft({shell:'bash',commands:value.commands,localTool:'cursor-cli'})).toBe(true);
 for(const patch of [{shell:''},{shell:'cmd'},{packageRoot:'relative/agent'},{packageRoot:'\\\\server\\share\\Cursor'},{packageRoot:'//server/share/Cursor'},{commands:'commands.json'},{commands:'/srv/commands.txt'},{credentialEnvRef:''},{credentialEnvRef:'PATH'},{credentialEnvRef:'SUMA_RUNNER_TOKEN'},{credentialEnvRef:'OPENAI_API_KEY'},{model:''},{reviewedVersion:''},{model:'界'.repeat(86)},{model:'bad\nmodel'},{model:'\ud800'},{packageRoot:'/srv/bad\0root'},{executable:'/other/node'},{args:['--force']},{sha256:'a'.repeat(64)},{api_key:'secret'}])expect(cursorSetupCommands({...value,...patch})).toBeUndefined();
 for(const patch of [{shell:''},{localTool:'qoder-acp'},{commands:'relative.json'},{commands:'//server/commands.json'},{nonce:1},{token:'secret'}])expect(validCursorSetupDraft({shell:'bash',commands:value.commands,localTool:'cursor-cli',...patch})).toBe(false);
 const symbol=Symbol('private');expect(cursorSetupCommands({...value,[symbol]:'not allowed'})).toBeUndefined();expect(cursorSetupCommands(null)).toBeUndefined();expect(cursorSetupCommands([])).toBeUndefined();
});
