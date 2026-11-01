import {useId,useRef,useState} from 'react';
import {ArrowRight,Check,Copy,Terminal} from 'lucide-react';
import {tr,useLocale} from './i18n';
import {claudeSDKSetupCommands,type ClaudeSDKSetupDraft,type ClaudeSDKSetupInput} from './claude-sdk-setup-model';
import './ClaudeSDKSetupGuide.css';
export type ClaudeSDKSetupGuideProps={disabled?:boolean;onUseConfiguration?:(draft:ClaudeSDKSetupDraft)=>void};
const t=(key:string)=>tr('claude-sdk-setup.'+key);
const empty:ClaudeSDKSetupInput={shell:'',node:'',host:'',nativeCLI:'',model:'',credentialEnvRef:'',commands:''};
type TextField=Exclude<keyof ClaudeSDKSetupInput,'shell'>;
const limits:Record<TextField,number>={node:4096,host:4096,nativeCLI:4096,model:256,credentialEnvRef:128,commands:4096};
/** Only public settings and a copyable administrator draft. The original
 * Runner checks assets, versions and the new file before any execution. */
export function ClaudeSDKSetupGuide({disabled=false,onUseConfiguration}:ClaudeSDKSetupGuideProps={}){
 useLocale();const id=useId(),[input,setInput]=useState<ClaudeSDKSetupInput>(empty),[copied,setCopied]=useState(''),[failed,setFailed]=useState(''),[copying,setCopying]=useState(false);
 const result=claudeSDKSetupCommands(input),prepare=result?.prepare||'',copyState=useRef({prepare,inFlight:false});copyState.current.prepare=prepare;
 function update(name:keyof ClaudeSDKSetupInput,value:string){if(disabled)return;setInput(current=>({...current,[name]:value}));setCopied('');setFailed('');}
 function field(name:TextField,wide=true){const fieldID=id+'-'+name;return <label className={'claude-sdk-setup-field'+(wide?' claude-sdk-setup-wide':'')} htmlFor={fieldID}><span>{t('field.'+name)}</span><input id={fieldID} name={'claude-sdk-setup-'+name} data-claude-sdk-setup-field={name} value={input[name]} required disabled={disabled} maxLength={limits[name]} autoComplete="off" spellCheck={false} aria-describedby={fieldID+'-help'} onChange={event=>update(name,event.target.value)}/><span className="claude-sdk-setup-meta" id={fieldID+'-help'}>{t('help.'+name)}</span></label>;}
 async function copy(){
  if(disabled||!prepare||copying||copyState.current.inFlight)return;
  const original=prepare;copyState.current.inFlight=true;setCopying(true);setFailed('');
  try{await navigator.clipboard.writeText(original);if(copyState.current.prepare===original)setCopied(original);}
  catch{if(copyState.current.prepare===original)setFailed(original);}
  finally{copyState.current.inFlight=false;setCopying(false);}
 }
 function useConfiguration(){if(!disabled&&result&&onUseConfiguration)onUseConfiguration({...result.draft});}
 const copiedCurrent=!!prepare&&copied===prepare;
 return <section className="claude-sdk-setup-guide" data-claude-sdk-setup-guide aria-labelledby={id+'-title'}>
  <header className="claude-sdk-setup-heading"><Terminal size={19} aria-hidden="true"/><div><h3 id={id+'-title'}>{t('title')}</h3><p className="claude-sdk-setup-help">{t('intro')}</p><p className="claude-sdk-setup-meta">{t('versions')}</p></div></header>
  <ol className="claude-sdk-setup-steps">
   <li><h4>{t('configure_title')}</h4><div className="claude-sdk-setup-fields">
    <label className="claude-sdk-setup-field claude-sdk-setup-wide" htmlFor={id+'-shell'}><span>{t('field.shell')}</span><select id={id+'-shell'} name="claude-sdk-setup-shell" data-claude-sdk-setup-field="shell" value={input.shell} required disabled={disabled} aria-describedby={id+'-shell-help'} onChange={event=>update('shell',event.target.value)}><option value="">{t('choose_shell')}</option><option value="powershell">PowerShell</option><option value="bash">Bash</option></select><span className="claude-sdk-setup-meta" id={id+'-shell-help'}>{t('help.shell')}</span></label>
    {field('node')}{field('host')}{field('nativeCLI')}{field('model',false)}{field('credentialEnvRef',false)}{field('commands')}
   </div></li>
   <li><div className="claude-sdk-setup-command-heading"><h4>{t('prepare_title')}</h4><button type="button" className="primary" data-claude-sdk-setup-copy disabled={disabled||!prepare} aria-disabled={disabled||!prepare||copying} aria-busy={copying} onClick={()=>void copy()}>{copiedCurrent?<Check size={16} aria-hidden="true"/>:<Copy size={16} aria-hidden="true"/>}<span {...(copiedCurrent?{'data-claude-sdk-setup-copied':true}:{})}>{t(copying?'copying':copiedCurrent?'copied':'copy_prepare')}</span></button></div><p className="claude-sdk-setup-help">{t('prepare_help')}</p>
    {result?<><p className="claude-sdk-setup-meta" role="status">{t('draft_ready')}</p><pre className="claude-sdk-setup-command" data-claude-sdk-setup-prepare tabIndex={0} aria-label={t('prepare_title')}>{result.prepare}</pre></>:<p className="claude-sdk-setup-incomplete" role="status">{t('incomplete')}</p>}
    {!!prepare&&failed===prepare&&<p className="claude-sdk-setup-error" role="alert" data-claude-sdk-setup-copy-failed>{t('copy_failed')}</p>}
   </li>
   <li><h4>{t('continue_title')}</h4><p className="claude-sdk-setup-help">{t('continue_help')}</p>{onUseConfiguration&&<button type="button" className="claude-sdk-setup-use" data-claude-sdk-setup-use disabled={disabled||!result} onClick={useConfiguration}>{t('use_configuration')}<ArrowRight size={16} aria-hidden="true"/></button>}<p className="claude-sdk-setup-meta">{t('handoff_boundary')}</p></li>
  </ol>
  <details className="claude-sdk-setup-details"><summary>{t('details')}</summary><div className="claude-sdk-setup-detail-body"><p className="claude-sdk-setup-help">{t('verification_boundary')}</p>{result&&<><h4>{t('describe_title')}</h4><pre className="claude-sdk-setup-command" data-claude-sdk-setup-describe tabIndex={0} aria-label={t('describe_title')}>{result.describe}</pre></>}<p className="claude-sdk-setup-help">{t('observation_boundary')}</p></div></details>
 </section>;
}
