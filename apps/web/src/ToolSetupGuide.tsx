import {useEffect,useId,useRef,useState} from 'react';
import {Check,Copy,Terminal} from 'lucide-react';
import {tr as t,useLocale} from './i18n';
import './ToolSetupGuide.css';
import {OctopHarnessSetup} from './OctopHarnessSetup';
import {QoderSetupGuide} from './QoderSetupGuide';
import {OpenCodeSetupGuide} from './OpenCodeSetupGuide';
import {CursorSetupGuide} from './CursorSetupGuide';
import {ClaudeSDKSetupGuide} from './ClaudeSDKSetupGuide';
import {CodexAppServerSetupGuide} from './CodexAppServerSetupGuide';
import {validCodexAppServerSetupDraft,type CodexAppServerSetupDraft} from './codex-app-server-setup-model';
import {validOpenCodeSetupDraft,type OpenCodeSetupDraft} from './opencode-setup-guide-model';
import {validQoderSetupDraft,type QoderSetupDraft} from './qoder-setup-guide-model';
import {preparationForPreset,preparationForLocalTool,type ToolPreparationPreset,type ToolPreparationSelection} from './tool-preparation-target';
import {validCursorSetupDraft,type CursorSetupDraft} from './cursor-setup-guide-model';
import {validClaudeSDKSetupDraft,type ClaudeSDKSetupDraft} from './claude-sdk-setup-model';
import {useApi,useApiView} from './api-context';
import {octopHarnessPreset} from './octop-execution-model';
import {DeveloperClientConnections} from './DeveloperClientConnections';
import {claudeCLIConfiguration,claudeCLIModel} from './claude-cli-model';
import {RemoteRunnerSetupGuide} from './RemoteRunnerSetupGuide';
import {LocalToolPreparationSetup} from './LocalToolPreparationSetup';
import {localToolPreparationTarget,validLocalToolPreparationDraft,type LocalToolPreparationDraft} from './local-tool-preparation-model';
import {CommandCompositionSetup} from './CommandCompositionSetup';
import {validCommandCompositionDraft,type CommandCompositionDraft} from './command-composition-model';
import type {ToolReportFeedbackSelection} from './tool-report-feedback-model';
import type {RunnerPairing} from '../../../packages/contracts/src';

type Preset=ToolPreparationPreset|'junit'|'operations'|'planner'|'extractor'|'diagnosis'|'native-coder'|'octop'|'recommendation';
type ConfigurationDraft=CursorSetupDraft|ClaudeSDKSetupDraft|CodexAppServerSetupDraft|OpenCodeSetupDraft|QoderSetupDraft|LocalToolPreparationDraft;
// These are local administrator templates, never a registration or live
// inventory. The existing observation/import flow remains the authority.
const presets={
 claude:claudeCLIConfiguration(),
 recommendation:{'suma-dispatch-recommendation':{adapter:'suma',protocol:'suma-jsonl-v1',executable:'node',args:[] as string[],input:'agent-jsonl',read_only:true,dispatch_recommendation:true,model_execution_protocol:'suma-model-execution-v1'}},
 octop:octopHarnessPreset,
 junit:{'junit-tests':{adapter:'command',protocol:'text-v1',executable:'node',args:['--test','--test-reporter=junit','--test-reporter-destination={{test_report_path}}'],input:'none',read_only:false,test_evidence:true,test_report:true}},
 operations:{'suma-intelligent-operations':{adapter:'suma',protocol:'suma-jsonl-v1',executable:'node',args:[] as string[],input:'agent-jsonl',read_only:true,intelligent_operations:true}},
 'native-coder':{'native-coder':{adapter:'suma',protocol:'suma-jsonl-v1',executable:'node',args:[] as string[],input:'agent-jsonl',read_only:false,model_execution_protocol:'suma-model-execution-v1'}},
 diagnosis:{'suma-failure-diagnosis':{adapter:'suma',protocol:'suma-jsonl-v1',executable:'node',args:[] as string[],input:'agent-jsonl',read_only:true,failure_diagnosis:true}},
 extractor:{'suma-knowledge-extractor':{adapter:'suma',protocol:'suma-jsonl-v1',executable:'node',args:[] as string[],input:'agent-jsonl',read_only:true,knowledge_extraction:true}},
 planner:{'suma-planner':{adapter:'suma',protocol:'suma-jsonl-v1',executable:'node',args:[] as string[],input:'agent-jsonl',read_only:true,planning:true}},
 codex:{'codex-develop':{adapter:'codex',protocol:'codex-jsonl-v1',executable:'codex',args:['exec','--json','--sandbox','workspace-write','--ephemeral','-'],input:'goal',read_only:false}},
 codebuddy:{'codebuddy-develop':{adapter:'codebuddy',protocol:'codebuddy-jsonl-v1',executable:'codebuddy',args:['-p','--input-format','text','--output-format','stream-json','--verbose','--permission-mode','acceptEdits'],input:'goal',read_only:false}},
 harness:{'harness-develop':{adapter:'harness',protocol:'dsh-multica-jsonl-v1',executable:'dsh',args:['--profile','multica','--stdio'],input:'dsh-jsonl',read_only:false}},
} as const;

export function ToolSetupGuide({id,expanded,onExpandedChange,project,pairedRunner,expectedWorkerID,presetRequest,onPairRunner,onReportSelection,onPrepareSharedPool,disabled}:{id?:string;expanded?:boolean;onExpandedChange?:(open:boolean)=>void;project?:string;pairedRunner?:RunnerPairing;expectedWorkerID?:string;presetRequest?:{preset:ToolPreparationPreset|'octop';nonce:number};onPairRunner?:()=>void;onReportSelection?:(selection:ToolReportFeedbackSelection)=>void;onPrepareSharedPool?:(localTool:ToolPreparationSelection['localTool'],sourceCurrent:()=>boolean)=>void;disabled?:boolean}={}){
 useLocale();
 const shared=typeof expectedWorkerID==='string'&&expectedWorkerID.startsWith('pool-');
 const targetPairing=!shared&&(!expectedWorkerID||pairedRunner?.worker_id===expectedWorkerID)?pairedRunner:undefined;
 const caller=useApi(),view=useApiView(),scope=JSON.stringify([view?.userID,view?.organizationID,project,expectedWorkerID,targetPairing?.id,targetPairing?.project_id,targetPairing?.control_url,targetPairing?.worker_id,targetPairing?.version]),remoteGuide=useRef<HTMLDivElement>(null);
 const owner=useRef({scope,caller,epoch:0,seenRequest:0});if(owner.current.scope!==scope||owner.current.caller!==caller)owner.current={scope,caller,epoch:owner.current.epoch+1,seenRequest:owner.current.seenRequest};
 const epoch=owner.current.epoch,sourceKey=scope+':'+epoch,navigationNonce=useRef(0),reportNavigationNonce=navigationNonce.current;
 const [navigation,setNavigation]=useState<{scope:string;caller:typeof caller;epoch:number;value:ToolPreparationSelection}>();
 const currentNavigation=navigation?.scope===scope&&navigation.caller===caller&&navigation.epoch===epoch?navigation:undefined;
 const [configurationRequest,setConfigurationRequest]=useState<{scope:string;caller:typeof caller;epoch:number;navigationNonce:number;value:(ConfigurationDraft|CommandCompositionDraft)&{nonce:number;selectionNonce?:number}}>();
 // Adopt only the explicitly selected output reference. The original Runner
 // guide still owns detection, registration and startup; no local result is inferred.
 function useComposedCommands(draft:CommandCompositionDraft){
  if(disabled||owner.current.epoch!==epoch||!validCommandCompositionDraft(draft))return;
  const selectionNonce=currentNavigation?.value.nonce,intent=navigationNonce.current;
  setConfigurationRequest(previous=>({scope,caller,epoch,navigationNonce:intent,value:{...draft,nonce:(previous?.value.nonce||0)+1,...(selectionNonce?{selectionNonce}:{})}}));
  requestAnimationFrame(()=>{if(owner.current.epoch!==epoch||navigationNonce.current!==intent)return;if(shared){remoteGuide.current?.querySelector<HTMLElement>('[data-shared-pool-output]')?.focus();return;}const details=remoteGuide.current?.querySelector('details');if(details){details.open=true;details.querySelector<HTMLInputElement>('[name="remote-runner-commands"]')?.focus();}});
 }
 function useRunnerConfiguration(draft:ConfigurationDraft){
  // The same monotonic selection generation fences ordinary and shared
  // adoption, including an old default-preset callback after A -> B -> A.
  if(disabled||owner.current.epoch!==epoch||sharedOwner.current.caller!==caller||sharedOwner.current.generation!==sharedGeneration||navigationNonce.current!==sharedNavigationNonce||selectedTarget?.localTool!==draft.localTool||!validCursorSetupDraft(draft)&&!validClaudeSDKSetupDraft(draft)&&!validCodexAppServerSetupDraft(draft)&&!validOpenCodeSetupDraft(draft)&&!validQoderSetupDraft(draft)&&!validLocalToolPreparationDraft(draft))return;
  let selection=currentNavigation?.value;
  if(!selection){
   // A fresh explicit native-path choice can establish the default Codex
   // selection; it does not resurrect an old request or select another tool.
   if(!validLocalToolPreparationDraft(draft))return;
   selection={localTool:draft.localTool,nonce:++navigationNonce.current};
   setNavigation({scope,caller,epoch,value:selection});
  }
  if(selection.nonce!==navigationNonce.current||selection.localTool!==draft.localTool)return;
  const selectionNonce=selection.nonce;
  setConfigurationRequest(previous=>({scope,caller,epoch,navigationNonce:selectionNonce,value:{...draft,nonce:(previous?.value.nonce||0)+1,selectionNonce}}));
  requestAnimationFrame(()=>{if(owner.current.epoch!==epoch||selectionNonce!==navigationNonce.current)return;if(shared){remoteGuide.current?.querySelector<HTMLElement>('[data-shared-pool-output]')?.focus();return;}const details=remoteGuide.current?.querySelector('details');if(details){details.open=true;details.querySelector('summary')?.focus();}});
 }
 const [selected,setSelected]=useState<Preset>('codex'),[copied,setCopied]=useState(''),[failed,setFailed]=useState(false);
 // A current selection is an ephemeral navigation source. A -> B -> A or an
 // original pending lock must not revive the old parent's asynchronous read.
 // Keep this separate from offline forms so blocking never erases their input.
 const sharedBasis=JSON.stringify([scope,epoch,selected,navigationNonce.current,shared,!!disabled,!!onPrepareSharedPool]);
 const sharedOwner=useRef({caller,basis:sharedBasis,generation:0});
 if(sharedOwner.current.caller!==caller||sharedOwner.current.basis!==sharedBasis)sharedOwner.current={caller,basis:sharedBasis,generation:sharedOwner.current.generation+1};
 const sharedGeneration=sharedOwner.current.generation,sharedNavigationNonce=navigationNonce.current,sharedAlive=useRef(true);
 useEffect(()=>{sharedAlive.current=true;return()=>{sharedAlive.current=false;};},[]);
 const [sharedCopy,setSharedCopy]=useState<{generation:number;path:string;failed:boolean}>();
 const selectedTarget=preparationForPreset(selected);
 const currentConfiguration=configurationRequest?.scope===scope&&configurationRequest.caller===caller&&configurationRequest.epoch===epoch&&configurationRequest.navigationNonce===navigationNonce.current&&configurationRequest.value.selectionNonce===currentNavigation?.value.nonce?configurationRequest.value:undefined;
 const sharedAvailable=shared&&!disabled&&!!view?.userID&&!!view.organizationID&&!!project&&expectedWorkerID!=='pool-'&&!!selectedTarget&&!!onPrepareSharedPool;
 function sharedSourceCurrent(){return sharedAlive.current&&navigationNonce.current===sharedNavigationNonce&&sharedOwner.current.caller===caller&&sharedOwner.current.basis===sharedBasis&&sharedOwner.current.generation===sharedGeneration&&owner.current.epoch===epoch&&owner.current.scope===scope;}
 function prepareSharedPool(){if(sharedAvailable&&selectedTarget&&sharedSourceCurrent())onPrepareSharedPool?.(selectedTarget.localTool,sharedSourceCurrent);}
 async function copySharedOutput(){
  if(!shared||disabled||!currentConfiguration||!sharedSourceCurrent())return;
  const path=currentConfiguration.commands;
  try{await navigator.clipboard.writeText(path);if(sharedSourceCurrent())setSharedCopy({generation:sharedGeneration,path,failed:false});}
  catch{if(sharedSourceCurrent())setSharedCopy({generation:sharedGeneration,path,failed:true});}
 }
 function selectPreset(preset:Preset){setSelected(preset);setCopied('');setFailed(false);const target=preparationForPreset(preset);const nonce=++navigationNonce.current;setNavigation(target?{scope,caller,epoch,value:{localTool:target.localTool,nonce}}:undefined);}
 useEffect(()=>{if(!presetRequest||owner.current.epoch!==epoch||!Number.isSafeInteger(presetRequest.nonce)||presetRequest.nonce<=owner.current.seenRequest)return;owner.current.seenRequest=presetRequest.nonce;if(preparationForPreset(presetRequest.preset)||presetRequest.preset==='octop')selectPreset(presetRequest.preset);},[presetRequest?.nonce,epoch]);
 const [launchMode,setLaunchMode]=useState<'native'|'node'>('native'),[entry,setEntry]=useState('');
 const entryHint=useId(),localID=useId(),guideID=id||localID;
 function configureRunner(preset:ToolPreparationPreset){
  if(disabled)return;if(selected!==preset)selectPreset(preset);else{setCopied('');setFailed(false);}
  requestAnimationFrame(()=>{if(owner.current.epoch!==epoch)return;document.getElementById(guideID)?.querySelector<HTMLInputElement|HTMLSelectElement>(preset==='cursor'?'[data-cursor-setup-field]':preset==='claude-sdk'?'[data-claude-sdk-setup-field]':preset==='codex-app-server'?'[data-codex-app-server-setup-field]':preset==='opencode'?'[data-opencode-setup-field]':'.tool-cli-setup-select select')?.focus();});
 }
 const [model,setModel]=useState(''),[endpoint,setEndpoint]=useState('');
 const [manifest,setManifest]=useState(''),[checks,setChecks]=useState(''),[allowRepair,setAllowRepair]=useState(false);
 const junit=selected==='junit',operations=selected==='operations',planner=selected==='planner',extractor=selected==='extractor',diagnosis=selected==='diagnosis',nativeCoder=selected==='native-coder',recommendation=selected==='recommendation',structured=operations||planner||extractor||diagnosis;
 const guideText=(name:'install'|'configure'|'register')=>selected==='claude'&&name==='install'?'claude-cli.install':recommendation?'dispatch-recommendation.setup_'+name:nativeCoder&&name==='configure'?'model-execution.setup_deployments':junit?'test-report-setup.'+name:operations?'intent-operations.setup_'+name:diagnosis?'failure-diagnosis-setup.'+name:extractor?'knowledge-extraction-setup.'+name:planner?'cli-setup.planner_'+name:'cli-setup.'+name;
 let validEndpoint=false;try{const u=new URL(endpoint);validEndpoint=(u.protocol==='https:'||u.protocol==='http:'&&['localhost','127.0.0.1','[::1]'].includes(u.hostname))&&!u.username&&!u.password&&!u.search&&!u.hash;}catch{/* Incomplete input is a draft, never configuration authority. */}
 const nodeMode=structured||nativeCoder||recommendation||!junit&&selected!=='codex'&&selected!=='claude'&&launchMode==='node';
 // The runner can be remote, so browser OS detection cannot select its launch
 // mode. This is a configuration preview; Go validates the actual local file.
 const validEntry=!/[\x00\r\n]/.test(entry)&&/\.(?:js|mjs|cjs)$/i.test(entry)&&(/^[a-z]:[\\/]/i.test(entry)||/^\/(?!\/)/.test(entry)||/^\\\\[^\\]+\\[^\\]+\\/.test(entry));
 const checkIDs=checks.split(',').map(value=>value.trim()).filter(Boolean),validManifest=!/[\x00\r\n]/.test(manifest)&&/\.json$/i.test(manifest)&&(/^[a-z]:[\\/]/i.test(manifest)||/^\/(?!\/)/.test(manifest)||/^\\\\[^\\]+\\[^\\]+\\/.test(manifest));
 const validNative=validManifest&&checkIDs.length>0&&checkIDs.length<=16&&new Set(checkIDs).size===checkIDs.length&&checkIDs.every(value=>/^[A-Za-z0-9._-]{1,100}$/.test(value))&&/\.(?:js|mjs)$/i.test(entry);
 const canCopy=(!nodeMode||validEntry)&&(!recommendation||/\.(?:js|mjs)$/i.test(entry))&&(!structured||/\.(?:js|mjs)$/i.test(entry)&&model.trim().length>0&&model.length<=200&&validEndpoint)&&(!nativeCoder||validNative);
 const [commandName,command]=Object.entries(presets[selected==='qoder'||selected==='opencode'||selected==='cursor'||selected==='claude-sdk'||selected==='codex-app-server'?'codex':selected])[0];
 const configuration=JSON.stringify({[commandName]:nativeCoder?{...command,args:[entry],native_workspace:{checks:checkIDs,runtime_manifest:manifest,...(allowRepair?{conflict_repair:true}:{})}}:structured?{...command,args:[entry],env:{SUMA_MODEL:model.trim(),SUMA_MODEL_BASE_URL:endpoint},env_from:operations||extractor||diagnosis?['SUMA_MODEL_API_KEY']:['SUMA_MODEL_API_KEY','SUMA_CONTROL_URL','SUMA_AGENT_TOKEN','SUMA_PROJECT_ID']}:nodeMode?{...command,executable:'node',args:[entry,...command.args]}:command},null,2);
 async function copy(){if(!canCopy)return;try{await navigator.clipboard.writeText(configuration);setCopied(configuration);setFailed(false);}catch{setFailed(true);}}
 const genericTarget=localToolPreparationTarget(selectedTarget?.localTool);
 const manualConfiguration=<>
   {selected!=='codex'&&selected!=='claude'&&!structured&&!nativeCoder&&!recommendation&&!junit&&<label className="tool-cli-setup-select">{t('cli-setup.launch_mode')}<select value={launchMode} onChange={event=>{setLaunchMode(event.target.value as 'native'|'node');setFailed(false);}}>
    <option value="native">{t('cli-setup.native_mode')}</option><option value="node">{t('cli-setup.node_mode')}</option>
   </select></label>}
   {nodeMode&&<div className="tool-cli-setup-entry">
    <label>{t('cli-setup.entry')}<input value={entry} onChange={event=>{setEntry(event.target.value);setFailed(false);}} placeholder="C:/tools/cli/entry.js" spellCheck={false} autoComplete="off" aria-describedby={entryHint} aria-invalid={entry!==''&&!validEntry}/></label>
    <p id={entryHint} className="muted">{t(structured?'cli-setup.planner_entry_help':'cli-setup.entry_help')}</p>
   </div>}
   {structured&&<div className="form-grid"><label>{t('cli-setup.model')}<input value={model} onChange={e=>setModel(e.target.value)} maxLength={200} autoComplete="off"/></label><label>{t('cli-setup.endpoint')}<input value={endpoint} onChange={e=>setEndpoint(e.target.value)} maxLength={2000} type="url" spellCheck={false} placeholder="https://provider.example/v1"/></label></div>}
   {selected==='claude'&&<p>{t('claude-cli.configured_model')}: <code>{claudeCLIModel}</code></p>}
   {nativeCoder&&<><label>{t('candidate-conflict-repair.local_manifest')}<input name="native_runtime_manifest" value={manifest} onChange={event=>setManifest(event.target.value)} maxLength={2000} spellCheck={false} autoComplete="off"/></label><label>{t('candidate-conflict-repair.local_checks')}<input name="native_check_ids" value={checks} onChange={event=>setChecks(event.target.value)} maxLength={1616} spellCheck={false} autoComplete="off"/></label><p className="muted">{t('candidate-conflict-repair.local_checks_help')}</p><label className="check"><input name="native_conflict_repair" type="checkbox" checked={allowRepair} onChange={event=>setAllowRepair(event.target.checked)}/><span>{t('candidate-conflict-repair.local_opt_in')}</span></label><p className="muted">{t('candidate-conflict-repair.local_policy')}</p></>}
   <ol><li>{t(guideText('install'))}</li><li>{t(guideText('configure'))}</li><li>{t('cli-setup.observe')}</li><li>{t(guideText('register'))}</li></ol>
   <div className="tool-cli-setup-actions"><strong>{t('cli-setup.local_config')}</strong><button type="button" disabled={!canCopy} onClick={()=>void copy()}>{canCopy&&copied===configuration?<Check size={14}/>:<Copy size={14}/>} {t(canCopy&&copied===configuration?'cli-setup.copied':'cli-setup.copy')}</button></div>
   <pre tabIndex={0} aria-label={t('cli-setup.local_config')}>{configuration}</pre>
   {failed&&<p role="alert">{t('cli-setup.copy_failed')}</p>}
   <p className="muted">{t(selected==='claude'?'claude-cli.setup_policy':recommendation?'dispatch-recommendation.setup_policy':junit?'test-report-setup.policy':operations?'intent-operations.setup_policy':diagnosis?'failure-diagnosis-setup.policy':extractor?'knowledge-extraction-setup.policy':planner?'cli-setup.planner_policy':selected==='harness'?'cli-setup.harness_dependency':selected==='codebuddy'?'cli-setup.codebuddy_policy':'cli-setup.codex_policy')}</p>
   <p className="muted">{t(planner?'cli-setup.planner_probe':'cli-setup.probe_help')}</p>
   <code className="tool-cli-setup-command">{structured||recommendation?'suma-runner --commands commands.local.json --allow-exec --describe-tools':'suma-runner --commands commands.local.json --allow-exec --repository REPOSITORY --base-commit COMMIT --describe-tools'}</code>
   <p className="muted">{t('cli-setup.shared_pool')}</p>
   <p className="muted">{t('cli-setup.boundary')}</p>
 </>;
 return <details id={guideID} className="tool-cli-setup" open={expanded} onToggle={event=>onExpandedChange?.(event.currentTarget.open)}>
  <summary><Terminal size={15}/>{t('cli-setup.title')}</summary>
  <div className="tool-cli-setup-body">
   <p className="muted">{t('cli-setup.intro')}</p>
   <label className="tool-cli-setup-select">{t('cli-setup.tool')}<select data-tool-setup-preset value={selected} onChange={event=>selectPreset(event.target.value as Preset)}>
    <option value="junit">{t('test-report-setup.tool')}</option><option value="recommendation">{t('dispatch-recommendation.setup_tool')}</option><option value="operations">{t('intent-operations.setup_tool')}</option><option value="native-coder">{t('candidate-conflict-repair.local_preset')}</option><option value="diagnosis">{t('failure-diagnosis-setup.tool')}</option><option value="extractor">{t('knowledge-extraction-setup.tool')}</option><option value="planner">{t('cli-setup.planner')}</option><option value="codex">Codex</option><option value="codex-app-server">{t('codex-app-server-setup.title')}</option><option value="codebuddy">CodeBuddy Code</option><option value="claude">{t('claude-cli.title')}</option><option value="claude-sdk">{t('claude-sdk-setup.title')}</option><option value="harness">DeepSeek Harness / dsh</option><option value="qoder">{t('acp-runtime.setup_title')}</option><option value="opencode">{t('opencode-setup-guide.title')}</option><option value="cursor">{t('cursor-setup-guide.title')}</option><option value="octop">{t('octop-execution.title')}</option>
   </select></label>
   {selected==='codex-app-server'?<CodexAppServerSetupGuide key={sourceKey+':CodexAppServerSetupGuide'} disabled={disabled} onUseConfiguration={useRunnerConfiguration}/>:selected==='qoder'?<QoderSetupGuide key={sourceKey+':QoderSetupGuide'} disabled={disabled} onUseConfiguration={useRunnerConfiguration}/>:selected==='opencode'?<OpenCodeSetupGuide key={sourceKey+':OpenCodeSetupGuide'} disabled={disabled} onUseConfiguration={useRunnerConfiguration}/>:selected==='cursor'?<CursorSetupGuide key={sourceKey+':CursorSetupGuide'} disabled={disabled} onUseConfiguration={useRunnerConfiguration}/>:selected==='claude-sdk'?<ClaudeSDKSetupGuide key={sourceKey+':ClaudeSDKSetupGuide'} disabled={disabled} onUseConfiguration={useRunnerConfiguration}/>:selected==='octop'?<OctopHarnessSetup/>:<>{genericTarget&&<LocalToolPreparationSetup key={sourceKey+':'+genericTarget.localTool} localTool={genericTarget.localTool} disabled={disabled} onUseConfiguration={useRunnerConfiguration}/>} {genericTarget?<details data-local-tool-manual><summary>{t('local-tool-preparation.manual_example')}</summary>{manualConfiguration}</details>:manualConfiguration}
   </>}
   {project&&<DeveloperClientConnections project={project}/>}
   <CommandCompositionSetup key={sourceKey+':CommandCompositionSetup'} disabled={disabled} onUseCommands={useComposedCommands}/>
   <div ref={remoteGuide}>{shared?<section className="tool-shared-pool-guide" data-shared-pool-guide><h4>{t('shared-tool-onboarding-guide.title')}</h4><p>{t('shared-tool-onboarding-guide.boundary')}</p><p><code>{expectedWorkerID}</code></p>{!sharedAvailable&&<p role="status">{t('shared-tool-onboarding-guide.unavailable')}</p>}<button type="button" data-shared-pool-prepare disabled={!sharedAvailable} onClick={prepareSharedPool}>{t('shared-tool-onboarding-guide.prepare')}</button>{currentConfiguration&&<div className="tool-shared-pool-output" data-shared-pool-output tabIndex={-1}><p>{t('shared-tool-onboarding-guide.output_hint')}</p><code>{currentConfiguration.commands}</code><button type="button" data-shared-pool-copy-output disabled={disabled} onClick={()=>void copySharedOutput()}>{t(sharedCopy?.generation===sharedGeneration&&sharedCopy.path===currentConfiguration.commands&&!sharedCopy.failed?'shared-tool-onboarding-guide.copied':'shared-tool-onboarding-guide.copy_output')}</button>{sharedCopy?.generation===sharedGeneration&&sharedCopy.failed&&<p role="alert">{t('cli-setup.copy_failed')}</p>}</div>}</section>:<RemoteRunnerSetupGuide key={sourceKey+':'+(preparationForPreset(selected)?'tools':selected)} project={project} pairedRunner={targetPairing} expectedWorkerID={expectedWorkerID} onReportSelection={onReportSelection?value=>{if(owner.current.epoch===epoch&&navigationNonce.current===reportNavigationNonce)onReportSelection(value);}:undefined} onPairRunner={onPairRunner} disabled={disabled} selectionRequest={currentNavigation?.value} configurationRequest={currentConfiguration} onConfigureCursor={()=>configureRunner('cursor')} onConfigureClaudeSDK={()=>configureRunner('claude-sdk')} onConfigureTool={localTool=>{const target=preparationForLocalTool(localTool);if(target)configureRunner(target.preset);}}/>}</div>
  </div>
 </details>;
}
