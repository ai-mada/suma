import {expect,it} from 'vitest';
import {remoteRunnerSetupCommands,type RemoteRunnerSetupInput} from './remote-runner-setup-model';
import {localToolPreparationTarget,validLocalToolPreparationDraft,localToolPreparationCommand} from './local-tool-preparation-model';
import {preparationForLocalTool} from './tool-preparation-target';
import {runnerSetupLiteral} from './cursor-setup-guide-model';

const generic=['codex-jsonl','codebuddy-cli','dsh-multica','claude-code-cli'] as const;
const input:RemoteRunnerSetupInput={mode:'identity',pairingID:'',identityPath:'/srv/runner.identity.json',shell:'bash',control:'https://control.example.test',project:'project',worker:'worker',commands:'/srv/commands.json',state:'',localTool:'codex-jsonl',repository:'/srv/repository',baseCommit:'a'.repeat(40),allowInstalledDiscovery:false,allowToolPreparation:false};

it('T220 explicit generic preparation shares the stable local JSON output boundary without changing original all-tools or special modes',()=>{
 for(const localTool of generic){
  expect(remoteRunnerSetupCommands({...input,localTool}).prepare).toBe("suma-runner --prepare-tools '/srv/commands.json' --local-tool '"+localTool+"'");
  for(const commands of ['\\\\host\\share\\commands.json','/srv/../commands.json','D:/tools./commands.json','D:/commands.json:secret','D:commands.json','/srv/commands.txt'])expect(remoteRunnerSetupCommands({...input,localTool,commands}).prepare,commands).toBeUndefined();
 }
 expect(remoteRunnerSetupCommands({...input,localTool:''}).prepare).toBe("suma-runner --prepare-tools '/srv/commands.json'");
 for(const localTool of ['cursor-cli','qoder-acp','claude-code-sdk','codex-app-server','opencode-sdk'])expect(remoteRunnerSetupCommands({...input,localTool}).prepare).toBeUndefined();
});

it('T220 closes four fixed-tool drafts and preserves shell literals without granting execution or accepting special protocol inputs',()=>{
 for(const localTool of generic){
  expect(localToolPreparationTarget(localTool)).toBe(preparationForLocalTool(localTool));
  for(const shell of ['powershell','bash'] as const){
   const commands=shell==='powershell'?"D:/Dev's $(Write-Output unexpected)/commands.JSON":"/srv/dev's/$(printf unexpected)/commands.json";
   const draft={shell,commands,localTool};expect(validLocalToolPreparationDraft(draft)).toBe(true);
   const prepare=localToolPreparationCommand(draft)!.prepare;
   expect(prepare).toBe('suma-runner --prepare-tools '+runnerSetupLiteral(commands,shell)+' --local-tool '+runnerSetupLiteral(localTool,shell));
   expect(prepare).not.toMatch(/--(?:control|project|worker|identity|pool-identity|allow-exec|report-tools|prepare-sdk-stdin)\b/);
   for(const value of [null,[],{...draft,token:'must-not-be-used'},{...draft,argv:[]},{...draft,shell:''},{...draft,commands:commands+'\n'},{...draft,commands:'/srv/'+ '界'.repeat(1400)+'.json'},{shell,commands},Object.assign(Object.create({}),draft),{...draft,[Symbol('extra')]:true}]){expect(validLocalToolPreparationDraft(value)).toBe(false);expect(localToolPreparationCommand(value)).toBeUndefined();}
  }
 }
 for(const value of ['claude-code-sdk','opencode-sdk','cursor-cli','qoder-acp','codex-app-server','octop','',null,{}]){expect(localToolPreparationTarget(value)).toBeUndefined();expect(localToolPreparationCommand({shell:'bash',commands:'/srv/commands.json',localTool:value})).toBeUndefined();}
 const nullPrototype=Object.assign(Object.create(null),{shell:'bash',commands:'/srv/commands.json',localTool:'codex-jsonl'});expect(validLocalToolPreparationDraft(nullPrototype)).toBe(true);
});
