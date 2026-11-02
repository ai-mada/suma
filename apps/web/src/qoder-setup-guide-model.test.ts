import {expect,it} from 'vitest';
import {qoderSetupCommands,validQoderSetupDraft,type QoderSetupInput} from './qoder-setup-guide-model';

it('T221 preserves exact private ACP input, shell literals and the existing path/credential boundaries without inventing a native observation',()=>{
  const input:QoderSetupInput={shell:'powershell',executable:"D:/Tools/O'Brien & $(not-run)/qoder.exe",reviewedVersion:'Qoder reviewed 1.2.3',model:"reviewed-'model'",credentialEnvRef:'',commands:"D:/new/O'Brien & $(not-run).json"};
  const result=qoderSetupCommands(input)!;
  expect(result.request).toEqual({adapter_id:'qoder-acp',executable:input.executable,reviewed_version:'Qoder reviewed 1.2.3',model:"reviewed-'model'",credential_env_ref:''});
  expect(Object.keys(result.request)).toHaveLength(5);
  expect(result.prepare).toContain("'D:/new/O''Brien & $(not-run).json' --prepare-acp-stdin");
  expect(result.prepare).toContain('"model":"reviewed-\'\'model\'\'"');
  expect(result.prepare).toContain('$OutputEncoding = [System.Text.UTF8Encoding]::new($false)');
  expect(result.prepare).toContain('finally {\n    $OutputEncoding = $qoderSetupPreviousEncoding');
  expect(result.prepare).not.toMatch(/--allow-exec|--describe-tools|--version|--local-tool|--acp(?:\s|$)/);
  const bash=qoderSetupCommands({...input,shell:'bash',executable:"/opt/O'Brien/qoder",credentialEnvRef:'LOCAL_QODER_TOKEN',commands:"/tmp/O'Brien.json"})!;
  expect(bash.prepare).toContain("printf '%s' '");
  expect(bash.prepare).toContain("'/tmp/O'\"'\"'Brien.json' --prepare-acp-stdin");
  expect(bash.request.credential_env_ref).toBe('LOCAL_QODER_TOKEN');
  for(const invalid of [null,[],{...input,extra:true},{...input,credentialEnvRef:undefined},{...input,model:null},{...input,reviewedVersion:' reviewed'},{...input,model:'é'.repeat(129)},{...input,reviewedVersion:'x\ud800'},{...input,model:'x\n--unsafe'},{...input,shell:''},{...input,commands:'relative.json'},{...input,commands:'//host/share/out.json'},{...input,commands:'D:/new/out.txt'},{...input,executable:'D:/tools/qoder.cmd'},{...input,executable:'/opt/qoder.js'},{...input,executable:'D:/tools/../qoder.exe'}])expect(qoderSetupCommands(invalid),JSON.stringify(invalid)).toBeUndefined();
  for(const credentialEnvRef of ['SUMA_AGENT_TOKEN','CODEX_HOME','HOME','PATH','LANG','NODE_OPTIONS','OPENAI_API_KEY','ANTHROPIC_API_KEY','secret=value','LOWER_case'])expect(qoderSetupCommands({...input,credentialEnvRef}),credentialEnvRef).toBeUndefined();
  const draft={shell:'bash',commands:'/tmp/new.json',localTool:'qoder-acp'};
  expect(validQoderSetupDraft(draft)).toBe(true);
  for(const invalid of [{...draft,executable:'/opt/qoder'},{...draft,localTool:'cursor-cli'},{...draft,commands:'/tmp/new.txt'},{...draft,shell:''}])expect(validQoderSetupDraft(invalid)).toBe(false);
  expect(input.credentialEnvRef).toBe('');
});
