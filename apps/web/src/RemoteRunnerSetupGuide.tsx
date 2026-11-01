import {useEffect,useId,useRef,useState} from 'react';
import {Check,Copy,Network} from 'lucide-react';
import type {RunnerPairing} from '../../../packages/contracts/src';
import {tr,useLocale} from './i18n';
import {readRunnerPairing} from './runner-pairing-model';
import {remoteRunnerLocalTools,remoteRunnerSetupCommands,type RemoteRunnerSetupInput} from './remote-runner-setup-model';
import {validCursorSetupDraft,type CursorSetupDraft} from './cursor-setup-guide-model';
import {validClaudeSDKSetupDraft,type ClaudeSDKSetupDraft} from './claude-sdk-setup-model';
import './RemoteRunnerSetupGuide.css';

const t=(key:string)=>tr('remote-runner-setup.'+key);
const toolNames:Record<(typeof remoteRunnerLocalTools)[number],string>={'codex-jsonl':'Codex CLI','codebuddy-cli':'CodeBuddy Code','dsh-multica':'DeepSeek Harness / dsh','claude-code-cli':'Claude Code','claude-code-sdk':'Claude SDK','cursor-cli':'Cursor Agent','qoder-acp':'Qoder CLI','codex-app-server':'Codex App Server','opencode-sdk':'OpenCode SDK'};

/** The target machine and its original Runner remain the only execution
 * authority. This guide builds public drafts and never sends or runs them. */
type ConfigurationRequest=(CursorSetupDraft|ClaudeSDKSetupDraft)&{nonce:number};
type Props={project?:string;pairedRunner?:RunnerPairing;expectedWorkerID?:string;onPairRunner?:()=>void;disabled?:boolean;configurationRequest?:ConfigurationRequest;onConfigureCursor?:()=>void;onConfigureClaudeSDK?:()=>void};
function readConfigurationRequest(value:unknown):ConfigurationRequest|undefined{
 if(!value||typeof value!=='object'||Array.isArray(value)||![Object.prototype,null].includes(Object.getPrototypeOf(value)))return;
 const row=value as Record<string,unknown>;
 if(Reflect.ownKeys(row).length!==4||!['shell','commands','localTool','nonce'].every(key=>Object.hasOwn(row,key))||!Number.isSafeInteger(row.nonce)||(row.nonce as number)<=0)return;
 const draft={shell:row.shell,commands:row.commands,localTool:row.localTool};
 return row.localTool==='cursor-cli'&&validCursorSetupDraft(draft)||row.localTool==='claude-code-sdk'&&validClaudeSDKSetupDraft(draft)?{...draft,nonce:row.nonce as number}:undefined;
}
export function RemoteRunnerSetupGuide(props:Props){
 let pairing:RunnerPairing|undefined;
 if(props.pairedRunner)try{const value=readRunnerPairing(props.pairedRunner,props.project||'');if(value.state==='approved'&&value.worker_id)pairing=value;}catch{/* A foreign or malformed source cannot seed a connection draft. */}
 const target=!props.expectedWorkerID||pairing?.worker_id===props.expectedWorkerID?pairing:undefined;
 const scope=JSON.stringify([props.project,props.expectedWorkerID,target?.id,target?.project_id,target?.control_url,target?.worker_id,target?.version]);
 // A request already observed for one public target cannot seed another target.
 const request=readConfigurationRequest(props.configurationRequest),owner=useRef<{scope:string;request:ConfigurationRequest}|undefined>(undefined);
 if(request&&(!owner.current||request.nonce>owner.current.request.nonce))owner.current={scope,request};
 const currentRequest=request&&owner.current?.scope===scope&&JSON.stringify(request)===JSON.stringify(owner.current.request)?request:undefined;
 return <RemoteRunnerSetupScope key={scope} {...props} configurationRequest={currentRequest} pairedRunner={target} disabled={props.disabled||!!props.pairedRunner&&!pairing} invalidTarget={!!props.pairedRunner&&!pairing}/>;
}
function RemoteRunnerSetupScope({project,pairedRunner,expectedWorkerID,onPairRunner,configurationRequest,onConfigureCursor,onConfigureClaudeSDK,disabled=false,invalidTarget=false}:Props&{invalidTarget?:boolean}){
 useLocale();const hint=useId(),[input,setInput]=useState<RemoteRunnerSetupInput>({mode:expectedWorkerID&&!pairedRunner?'identity':'paired',pairingID:pairedRunner?.id||'',identityPath:'',shell:'',control:pairedRunner?.control_url||'',project:project||'',worker:expectedWorkerID||pairedRunner?.worker_id||'',commands:'',state:'',localTool:'',repository:'',baseCommit:''}),[copied,setCopied]=useState(''),[failed,setFailed]=useState('');
 const appliedNonce=useRef(0);
 useEffect(()=>{
  if(disabled||!configurationRequest||configurationRequest.nonce<=appliedNonce.current)return;
  appliedNonce.current=configurationRequest.nonce;
  const {shell,commands,localTool}=configurationRequest;
  setInput(current=>({...current,shell,commands,localTool}));setCopied('');setFailed('');
 },[configurationRequest?.nonce,disabled]);
 const effective={...input,...(input.mode!=='environment'?{project:project||''}:{}),...(input.mode==='identity'&&!pairedRunner&&!expectedWorkerID?{worker:''}:{}),...(expectedWorkerID?{worker:expectedWorkerID}:{}),...(pairedRunner?{control:pairedRunner.control_url,project:pairedRunner.project_id,worker:pairedRunner.worker_id||'',pairingID:pairedRunner.id}:{})},commands=remoteRunnerSetupCommands(effective);
 const cursor=input.localTool==='cursor-cli',claudeSDK=input.localTool==='claude-code-sdk';
 function fixed(name:keyof RemoteRunnerSetupInput){return name==='project'&&input.mode!=='environment'||name==='worker'&&!!expectedWorkerID||!!pairedRunner&&['control','project','worker','pairingID'].includes(name);}
 function update(name:keyof RemoteRunnerSetupInput,value:string){if(disabled||fixed(name))return;setInput(current=>({...current,[name]:value}));setFailed('');}
 function field(name:'control'|'project'|'worker'|'commands'|'state'|'repository'|'baseCommit'|'identityPath'|'pairingID',maxLength=4096){return <label className="remote-runner-field">{t('field.'+name)}<input name={'remote-runner-'+name} value={effective[name]} type={name==='control'?'url':'text'} maxLength={maxLength} disabled={disabled} readOnly={fixed(name)} autoComplete="off" spellCheck={false} aria-describedby={hint} onChange={event=>update(name,event.target.value)}/></label>;}
 async function copy(kind:'prepare'|'report'|'start'){const command=commands[kind];if(disabled||!command)return;try{await navigator.clipboard.writeText(command);setCopied(command);setFailed('');}catch{setFailed(command);}}
 function command(kind:'prepare'|'report'|'start'){const value=commands[kind];return <div className="remote-runner-command"><div className="remote-runner-command-heading"><strong>{t('command.'+kind)}</strong><button type="button" data-remote-runner-copy={kind} disabled={disabled||!value} onClick={()=>void copy(kind)}>{value&&copied===value?<Check size={15} aria-hidden="true"/>:<Copy size={15} aria-hidden="true"/>}{t(value&&copied===value?'copied':kind==='start'?'start.copy':'copy')}</button></div>{value?<pre tabIndex={0} data-remote-runner-command={kind} aria-label={t('command.'+kind)}>{value}</pre>:<p className="remote-runner-incomplete" role="status">{t(kind==='report'&&input.mode!=='environment'?'incomplete.'+input.mode:'incomplete.'+kind)}</p>}{value&&failed===value&&<p className="remote-runner-warning" role="alert">{t('copy_failed')}</p>}</div>;}
 return <details className="remote-runner-setup" data-remote-runner-setup open={!!pairedRunner||!!expectedWorkerID}>
  <summary><Network size={17} aria-hidden="true"/>{t('title')}</summary>
  <div className="remote-runner-body">
   <p>{t('intro')}</p><p id={hint} className="remote-runner-help">{t('public_only')}</p>
   {pairedRunner&&<p className="remote-runner-help" data-remote-runner-target>{t('target_hint')}</p>}{invalidTarget&&<p className="remote-runner-warning" role="alert">{t('invalid_target')}</p>}
   {onPairRunner&&<div className="remote-runner-command"><p>{t('pair_intro')}</p><div className="remote-runner-command-heading"><button type="button" data-remote-runner-pair disabled={disabled} onClick={()=>{if(!disabled)onPairRunner();}}><Network size={15} aria-hidden="true"/>{t('pair')}</button></div></div>}
   <ol className="remote-runner-steps">
    <li><h4>{t(cursor?'cursor.title':claudeSDK?'claude_sdk.title':'prepare.title')}</h4><p>{t(cursor?'cursor.intro':claudeSDK?'claude_sdk.intro':'prepare.intro')}</p>
     <label className="remote-runner-field">{t('field.shell')}<select name="remote-runner-shell" value={input.shell} disabled={disabled} onChange={event=>update('shell',event.target.value)}><option value="">{t('choose_shell')}</option><option value="powershell">PowerShell</option><option value="bash">Bash</option></select></label>
     <p className="remote-runner-help">{t('shell_help')}</p>{field('commands')}
     <label className="remote-runner-field">{t('field.localTool')}<select name="remote-runner-localTool" value={input.localTool} disabled={disabled} onChange={event=>update('localTool',event.target.value)}><option value="">{t('all_tools')}</option>{remoteRunnerLocalTools.map(tool=><option key={tool} value={tool}>{toolNames[tool]}</option>)}</select></label>
     {cursor?<div className="remote-runner-command remote-runner-cursor-setup" data-remote-runner-cursor-setup><p>{t('cursor.configuration_help')}</p>{onConfigureCursor&&<div className="remote-runner-command-heading"><button type="button" data-remote-runner-configure-cursor disabled={disabled} onClick={()=>{if(!disabled)onConfigureCursor();}}>{t('cursor.configure')}</button></div>}</div>:claudeSDK?<div className="remote-runner-command remote-runner-claude-sdk-setup" data-remote-runner-claude-sdk-setup><p>{t('claude_sdk.configuration_help')}</p>{onConfigureClaudeSDK&&<div className="remote-runner-command-heading"><button type="button" data-remote-runner-configure-claude-sdk disabled={disabled} onClick={()=>{if(!disabled)onConfigureClaudeSDK();}}>{t('claude_sdk.configure')}</button></div>}</div>:command('prepare')}<p className="remote-runner-help">{t('prepare.boundary')}</p>
    </li>
    <li><h4>{t('report.title')}</h4><p>{t('report.intro')}</p>
     <label className="remote-runner-field">{t('field.mode')}<select name="remote-runner-mode" value={input.mode} disabled={disabled} onChange={event=>update('mode',event.target.value)}><option value="paired">{t('mode.paired')}</option><option value="identity">{t('mode.identity')}</option><option value="environment">{t('mode.environment')}</option></select></label>
      {input.mode==='paired'?<>{field('control',2048)}{field('project',200)}{field('pairingID',32)}{field('worker',200)}<p className="remote-runner-help">{t('paired_help')}</p></>:input.mode==='identity'?<>{field('control',2048)}{field('project',200)}{expectedWorkerID&&field('worker',200)}{field('identityPath')}<p className="remote-runner-help">{t('identity_help')}</p></>:<>{field('control',2048)}
      <div className="remote-runner-fields">{field('project',200)}{field('worker',200)}</div>{field('state')}
     </>}
     {commands.loopback&&<p className="remote-runner-warning" data-remote-runner-loopback role="status">{t('loopback')}</p>}
     {field('repository')}{field('baseCommit',64)}<p className="remote-runner-help">{t('baseline_help')}</p>
     {input.mode==='environment'&&<p className="remote-runner-help">{t('token_help')}</p>}{command('report')}<p className="remote-runner-help">{t('report.boundary')}</p>
    </li>
    <li><h4>{t('reuse.title')}</h4><p>{t('reuse.intro')}</p></li>
    <li><h4>{t('enable.title')}</h4><p>{t('enable.intro')}</p></li>
    <li data-remote-runner-start><h4>{t('start.title')}</h4><p>{t('start.intro')}</p>{command('start')}<p className="remote-runner-help">{t('start.boundary')}</p></li>
   </ol>
  </div>
 </details>;
}
