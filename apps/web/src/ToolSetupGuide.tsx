import {useId,useState} from 'react';
import {Check,Copy,Terminal} from 'lucide-react';
import {tr as t,useLocale} from './i18n';
import './ToolSetupGuide.css';
import {OctopHarnessSetup} from './OctopHarnessSetup';
import {octopHarnessPreset} from './octop-execution-model';

type Preset='junit'|'operations'|'codex'|'codebuddy'|'harness'|'planner'|'extractor'|'diagnosis'|'native-coder'|'octop'|'recommendation';
// These are local administrator templates, never a registration or live
// inventory. The existing observation/import flow remains the authority.
const presets={
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

export function ToolSetupGuide(){
 useLocale();
 const [selected,setSelected]=useState<Preset>('codex'),[copied,setCopied]=useState(''),[failed,setFailed]=useState(false);
 const [launchMode,setLaunchMode]=useState<'native'|'node'>('native'),[entry,setEntry]=useState('');
 const entryHint=useId();
 const [model,setModel]=useState(''),[endpoint,setEndpoint]=useState('');
 const [manifest,setManifest]=useState(''),[checks,setChecks]=useState(''),[allowRepair,setAllowRepair]=useState(false);
 const junit=selected==='junit',operations=selected==='operations',planner=selected==='planner',extractor=selected==='extractor',diagnosis=selected==='diagnosis',nativeCoder=selected==='native-coder',recommendation=selected==='recommendation',structured=operations||planner||extractor||diagnosis;
 const guideText=(name:'install'|'configure'|'register')=>recommendation?'dispatch-recommendation.setup_'+name:nativeCoder&&name==='configure'?'model-execution.setup_deployments':junit?'test-report-setup.'+name:operations?'intent-operations.setup_'+name:diagnosis?'failure-diagnosis-setup.'+name:extractor?'knowledge-extraction-setup.'+name:planner?'cli-setup.planner_'+name:'cli-setup.'+name;
 let validEndpoint=false;try{const u=new URL(endpoint);validEndpoint=(u.protocol==='https:'||u.protocol==='http:'&&['localhost','127.0.0.1','[::1]'].includes(u.hostname))&&!u.username&&!u.password&&!u.search&&!u.hash;}catch{/* Incomplete input is a draft, never configuration authority. */}
 const nodeMode=structured||nativeCoder||recommendation||!junit&&selected!=='codex'&&launchMode==='node';
 // The runner can be remote, so browser OS detection cannot select its launch
 // mode. This is a configuration preview; Go validates the actual local file.
 const validEntry=!/[\x00\r\n]/.test(entry)&&/\.(?:js|mjs|cjs)$/i.test(entry)&&(/^[a-z]:[\\/]/i.test(entry)||/^\/(?!\/)/.test(entry)||/^\\\\[^\\]+\\[^\\]+\\/.test(entry));
 const checkIDs=checks.split(',').map(value=>value.trim()).filter(Boolean),validManifest=!/[\x00\r\n]/.test(manifest)&&/\.json$/i.test(manifest)&&(/^[a-z]:[\\/]/i.test(manifest)||/^\/(?!\/)/.test(manifest)||/^\\\\[^\\]+\\[^\\]+\\/.test(manifest));
 const validNative=validManifest&&checkIDs.length>0&&checkIDs.length<=16&&new Set(checkIDs).size===checkIDs.length&&checkIDs.every(value=>/^[A-Za-z0-9._-]{1,100}$/.test(value))&&/\.(?:js|mjs)$/i.test(entry);
 const canCopy=(!nodeMode||validEntry)&&(!recommendation||/\.(?:js|mjs)$/i.test(entry))&&(!structured||/\.(?:js|mjs)$/i.test(entry)&&model.trim().length>0&&model.length<=200&&validEndpoint)&&(!nativeCoder||validNative);
 const [commandName,command]=Object.entries(presets[selected])[0];
 const configuration=JSON.stringify({[commandName]:nativeCoder?{...command,args:[entry],native_workspace:{checks:checkIDs,runtime_manifest:manifest,...(allowRepair?{conflict_repair:true}:{})}}:structured?{...command,args:[entry],env:{SUMA_MODEL:model.trim(),SUMA_MODEL_BASE_URL:endpoint},env_from:operations||extractor||diagnosis?['SUMA_MODEL_API_KEY']:['SUMA_MODEL_API_KEY','SUMA_CONTROL_URL','SUMA_AGENT_TOKEN','SUMA_PROJECT_ID']}:nodeMode?{...command,executable:'node',args:[entry,...command.args]}:command},null,2);
 async function copy(){if(!canCopy)return;try{await navigator.clipboard.writeText(configuration);setCopied(configuration);setFailed(false);}catch{setFailed(true);}}
 return <details className="tool-cli-setup">
  <summary><Terminal size={15}/>{t('cli-setup.title')}</summary>
  <div className="tool-cli-setup-body">
   <p className="muted">{t('cli-setup.intro')}</p>
   <label className="tool-cli-setup-select">{t('cli-setup.tool')}<select value={selected} onChange={event=>{setSelected(event.target.value as Preset);setFailed(false);}}>
    <option value="junit">{t('test-report-setup.tool')}</option><option value="recommendation">{t('dispatch-recommendation.setup_tool')}</option><option value="operations">{t('intent-operations.setup_tool')}</option><option value="native-coder">{t('candidate-conflict-repair.local_preset')}</option><option value="diagnosis">{t('failure-diagnosis-setup.tool')}</option><option value="extractor">{t('knowledge-extraction-setup.tool')}</option><option value="planner">{t('cli-setup.planner')}</option><option value="codex">Codex</option><option value="codebuddy">CodeBuddy Code</option><option value="harness">DeepSeek Harness / dsh</option><option value="octop">{t('octop-execution.title')}</option>
   </select></label>
   {selected==='octop'?<OctopHarnessSetup/>:<>
   {selected!=='codex'&&!structured&&!nativeCoder&&!recommendation&&!junit&&<label className="tool-cli-setup-select">{t('cli-setup.launch_mode')}<select value={launchMode} onChange={event=>{setLaunchMode(event.target.value as 'native'|'node');setFailed(false);}}>
    <option value="native">{t('cli-setup.native_mode')}</option><option value="node">{t('cli-setup.node_mode')}</option>
   </select></label>}
   {nodeMode&&<div className="tool-cli-setup-entry">
    <label>{t('cli-setup.entry')}<input value={entry} onChange={event=>{setEntry(event.target.value);setFailed(false);}} placeholder="C:/tools/cli/entry.js" spellCheck={false} autoComplete="off" aria-describedby={entryHint} aria-invalid={entry!==''&&!validEntry}/></label>
    <p id={entryHint} className="muted">{t(structured?'cli-setup.planner_entry_help':'cli-setup.entry_help')}</p>
   </div>}
   {structured&&<div className="form-grid"><label>{t('cli-setup.model')}<input value={model} onChange={e=>setModel(e.target.value)} maxLength={200} autoComplete="off"/></label><label>{t('cli-setup.endpoint')}<input value={endpoint} onChange={e=>setEndpoint(e.target.value)} maxLength={2000} type="url" spellCheck={false} placeholder="https://provider.example/v1"/></label></div>}
   {nativeCoder&&<><label>{t('candidate-conflict-repair.local_manifest')}<input name="native_runtime_manifest" value={manifest} onChange={event=>setManifest(event.target.value)} maxLength={2000} spellCheck={false} autoComplete="off"/></label><label>{t('candidate-conflict-repair.local_checks')}<input name="native_check_ids" value={checks} onChange={event=>setChecks(event.target.value)} maxLength={1616} spellCheck={false} autoComplete="off"/></label><p className="muted">{t('candidate-conflict-repair.local_checks_help')}</p><label className="check"><input name="native_conflict_repair" type="checkbox" checked={allowRepair} onChange={event=>setAllowRepair(event.target.checked)}/><span>{t('candidate-conflict-repair.local_opt_in')}</span></label><p className="muted">{t('candidate-conflict-repair.local_policy')}</p></>}
   <ol><li>{t(guideText('install'))}</li><li>{t(guideText('configure'))}</li><li>{t('cli-setup.observe')}</li><li>{t(guideText('register'))}</li></ol>
   <div className="tool-cli-setup-actions"><strong>{t('cli-setup.local_config')}</strong><button type="button" disabled={!canCopy} onClick={()=>void copy()}>{canCopy&&copied===configuration?<Check size={14}/>:<Copy size={14}/>} {t(canCopy&&copied===configuration?'cli-setup.copied':'cli-setup.copy')}</button></div>
   <pre tabIndex={0} aria-label={t('cli-setup.local_config')}>{configuration}</pre>
   {failed&&<p role="alert">{t('cli-setup.copy_failed')}</p>}
   <p className="muted">{t(recommendation?'dispatch-recommendation.setup_policy':junit?'test-report-setup.policy':operations?'intent-operations.setup_policy':diagnosis?'failure-diagnosis-setup.policy':extractor?'knowledge-extraction-setup.policy':planner?'cli-setup.planner_policy':selected==='harness'?'cli-setup.harness_dependency':selected==='codebuddy'?'cli-setup.codebuddy_policy':'cli-setup.codex_policy')}</p>
   <p className="muted">{t(planner?'cli-setup.planner_probe':'cli-setup.probe_help')}</p>
   <code className="tool-cli-setup-command">{structured||recommendation?'suma-runner --commands commands.local.json --allow-exec --describe-tools':'suma-runner --commands commands.local.json --allow-exec --repository REPOSITORY --base-commit COMMIT --describe-tools'}</code>
   <p className="muted">{t('cli-setup.shared_pool')}</p>
   <p className="muted">{t('cli-setup.boundary')}</p>
   </>}
  </div>
 </details>;
}
