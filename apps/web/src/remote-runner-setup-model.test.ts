import {expect,it} from 'vitest';
import {remoteRunnerSetupCommands,type RemoteRunnerSetupInput} from './remote-runner-setup-model';

function input(shell:'powershell'|'bash',mode:'identity'|'environment'):RemoteRunnerSetupInput{
 const root=shell==='powershell'?"C:\\Users\\Dev's $(Write-Output unexpected)":"/home/dev's/$(printf unexpected)";
 return {mode,pairingID:'',identityPath:root+'/runner.identity.json',shell,control:'https://control.example.test/suma',project:'project-a',worker:'remote-original',commands:root+'/commands.json',state:root+'/runner.journal',localTool:'codex-jsonl',repository:root+'/checkout',baseCommit:'a'.repeat(40),allowInstalledDiscovery:false};
}
const quote=(value:string,shell:'powershell'|'bash')=>"'"+(shell==='powershell'?value.replaceAll("'","''"):value.replaceAll("'","'\"'\"'"))+"'";

it('T205 requires explicit discovery consent and creates only a maintenance connection without workspace or commands while leaving report and task startup distinct',()=>{
 for(const shell of ['powershell','bash'] as const)for(const mode of ['identity','environment','paired'] as const){const value:RemoteRunnerSetupInput={...input(shell,mode==='environment'?'environment':'identity'),mode,pairingID:'f'.repeat(32),allowInstalledDiscovery:true,commands:'',repository:'',baseCommit:''};const result=remoteRunnerSetupCommands(value);expect(result.discoveryStart,mode).toBeDefined();expect(result.discoveryStart).toContain('--allow-installed-tool-discovery --maintenance-only');expect(result.discoveryStart).not.toMatch(/--(?:commands|repository|base-commit|allow-exec|report-tools|prepare-tools)\b/);expect(result.start).toBeUndefined();expect(result.report).toBeUndefined();expect(remoteRunnerSetupCommands({...value,allowInstalledDiscovery:false}).discoveryStart).toBeUndefined();
  const full=remoteRunnerSetupCommands({...value,commands:'/srv/commands.json',repository:'/srv/repo',baseCommit:'a'.repeat(40)});expect(full.start).toContain('--allow-installed-tool-discovery');expect(full.start).not.toContain('--maintenance-only');expect(full.report).not.toContain('--allow-installed-tool-discovery');expect(full.report).not.toContain('--maintenance-only');
  const missing={...value} as Record<string,unknown>;delete missing.allowInstalledDiscovery;expect(remoteRunnerSetupCommands(missing)).toEqual({loopback:false});expect(remoteRunnerSetupCommands({...value,allowInstalledDiscovery:'true'})).toEqual({loopback:false});
 }
});

it('builds explicit ordinary Worker startup from the exact same public identity or environment scope in either shell',()=>{
 for(const shell of ['powershell','bash'] as const)for(const mode of ['identity','environment'] as const){
  const value=input(shell,mode),commands=remoteRunnerSetupCommands(value),literal=(text:string)=>quote(text,shell);
  const workspace=' --workspace '+literal(value.repository)+' --repository '+literal(value.repository)+' --base-commit '+literal(value.baseCommit)+' --allow-exec';
  const identityArgs=' --identity '+literal(value.identityPath)+' --control '+literal(value.control)+' --project '+literal(value.project)+' --worker '+literal(value.worker)+' --commands '+literal(value.commands)+workspace;
  const environmentArgs=' --control '+literal(value.control)+' --project '+literal(value.project)+' --worker '+literal(value.worker)+' --commands '+literal(value.commands)+' --state '+literal(value.state)+workspace;
  const env=shell==='powershell'?"if (-not $env:SUMA_RUNNER_TOKEN) { throw 'SUMA_RUNNER_TOKEN is required' }\n":': "${SUMA_RUNNER_TOKEN:?SUMA_RUNNER_TOKEN is required}"\n';
  expect(commands.start).toBe(mode==='identity'?'suma-runner'+identityArgs:env+'suma-runner'+environmentArgs);
  expect(commands.report).toBe(mode==='identity'?'suma-runner --report-tools'+identityArgs:env+'suma-runner --report-tools'+environmentArgs);
  expect(commands.prepare).toBe('suma-runner --prepare-tools '+literal(value.commands)+' --local-tool '+literal(value.localTool));
  const cursor=remoteRunnerSetupCommands({...value,localTool:'cursor-cli'});expect(cursor.prepare).toBeUndefined();expect(cursor.report).toBe(commands.report);expect(cursor.start).toBe(commands.start);
  for(const localTool of ['claude-code-sdk','opencode-sdk','qoder-acp','codex-app-server']){const special=remoteRunnerSetupCommands({...value,localTool});expect(special.prepare,localTool).toBeUndefined();expect(special.report).toBe(commands.report);expect(special.start).toBe(commands.start);}
  expect(commands.start).not.toMatch(/--(?:report-tools|pair-runner|prepare-tools)/);
  if(mode==='identity'){
   expect(commands.start).not.toMatch(/--state\b|SUMA_RUNNER_TOKEN/);
   const generic=remoteRunnerSetupCommands({...value,worker:''});expect(generic.start).toBeDefined();expect(generic.start).not.toContain('--worker');expect(generic.report).not.toContain('--worker');
   for(const worker of ['invalid worker','remote/other']){const invalid=remoteRunnerSetupCommands({...value,worker});expect(invalid.start).toBeUndefined();expect(invalid.report).toBeUndefined();}
  }
 }
});

it('keeps preparation and one-time reporting separate and never emits startup for an invalid current scope or command',()=>{
 for(const mode of ['identity','environment'] as const){
  const value=input('bash',mode);
  for(const patch of [{control:'http://remote.example.test'},{control:'https://user:pass@control.example.test'},{control:'https://control.example.test?query=1'},{project:''},{commands:'relative.json'},{commands:'/tmp/not-json.txt'},{repository:'relative-checkout'},{baseCommit:'main'},{shell:''},{localTool:'unknown-runtime'},...(mode==='identity'?[{identityPath:''},{identityPath:'relative.identity.json'}]:[{worker:''},{state:'relative-journal'}])]){
   const commands=remoteRunnerSetupCommands({...value,...patch});expect(commands.start).toBeUndefined();expect(commands.report).toBeUndefined();
  }
  const preparedOnly=remoteRunnerSetupCommands({...value,control:''});expect(preparedOnly.prepare).toBeDefined();expect(preparedOnly.report).toBeUndefined();expect(preparedOnly.start).toBeUndefined();
  expect(remoteRunnerSetupCommands({...value,token:'must-not-enter-a-command'})).toEqual({loopback:false});
 }
});

it('resumes the exact paired Runner default identity with independent report and startup commands in both shells',()=>{
 for(const shell of ['powershell','bash'] as const){
  const value:RemoteRunnerSetupInput={...input(shell,'identity'),mode:'paired',pairingID:'f'.repeat(32),worker:'remote-'+'b'.repeat(32),identityPath:'',state:''};
  const commands=remoteRunnerSetupCommands(value),literal=(text:string)=>quote(text,shell);
  const args=' --paired-runner '+literal(value.pairingID)+' --control '+literal(value.control)+' --project '+literal(value.project)+' --worker '+literal(value.worker)+' --commands '+literal(value.commands)+' --workspace '+literal(value.repository)+' --repository '+literal(value.repository)+' --base-commit '+literal(value.baseCommit)+' --allow-exec';
  expect(commands.start).toBe('suma-runner'+args);expect(commands.report).toBe('suma-runner --report-tools'+args);
  expect(commands.prepare).toBe('suma-runner --prepare-tools '+literal(value.commands)+' --local-tool '+literal(value.localTool));
  expect(commands.start).not.toMatch(/--(?:identity|state|pair-runner|report-tools)|SUMA_RUNNER_TOKEN/);expect(commands.report).not.toMatch(/--(?:identity|state|pair-runner)|SUMA_RUNNER_TOKEN/);
  for(const patch of [{pairingID:''},{pairingID:'f'.repeat(31)},{pairingID:'F'.repeat(32)},{pairingID:'other-pairing'},{worker:''},{worker:'invalid worker'},{project:''},{project:'another/project'},{control:''},{control:'http://remote.example.test'},{control:'https://user:pass@control.example.test'},{commands:'relative.json'},{repository:''},{baseCommit:'HEAD'}]){
   const invalid=remoteRunnerSetupCommands({...value,...patch});expect(invalid.report).toBeUndefined();expect(invalid.start).toBeUndefined();
  }
  const missingIdentity=remoteRunnerSetupCommands({...value,pairingID:''});expect(missingIdentity.prepare).toBe(commands.prepare);expect(missingIdentity.report).toBeUndefined();expect(missingIdentity.start).toBeUndefined();
 }
});
