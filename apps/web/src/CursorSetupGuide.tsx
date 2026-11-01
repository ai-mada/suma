import {useId,useRef,useState} from 'react';
import {ArrowRight,Check,Copy,Terminal} from 'lucide-react';
import {tr,useLocale} from './i18n';
import {cursorSetupCommands,type CursorSetupDraft,type CursorSetupInput} from './cursor-setup-guide-model';
import './CursorSetupGuide.css';

export type CursorSetupGuideProps={disabled?:boolean;onUseConfiguration?:(draft:CursorSetupDraft)=>void};
const t=(key:string)=>tr('cursor-setup-guide.'+key);
const empty:CursorSetupInput={shell:'',packageRoot:'',reviewedVersion:'',model:'',credentialEnvRef:'',commands:''};
type TextField=Exclude<keyof CursorSetupInput,'shell'>;
const limits:Record<TextField,number>={packageRoot:4096,reviewedVersion:256,model:256,credentialEnvRef:128,commands:4096};

/** Public administrator draft only. The target Runner owns package inspection,
 * fixed arguments and file creation; this view never starts or registers it. */
export function CursorSetupGuide({disabled=false,onUseConfiguration}:CursorSetupGuideProps={}){
 useLocale();const id=useId(),[input,setInput]=useState<CursorSetupInput>(empty),[copied,setCopied]=useState(''),[failed,setFailed]=useState(''),[copying,setCopying]=useState(false);
 const result=cursorSetupCommands(input),prepare=result?.prepare||'',copyState=useRef({prepare,inFlight:false});copyState.current.prepare=prepare;
 function update(name:keyof CursorSetupInput,value:string){if(disabled)return;setInput(current=>({...current,[name]:value}));setCopied('');setFailed('');}
 function field(name:TextField,wide=true){const fieldID=id+'-'+name;return <label className={'cursor-setup-field'+(wide?' cursor-setup-field-wide':'')} htmlFor={fieldID}><span>{t('field.'+name)}</span><input id={fieldID} name={'cursor-setup-'+name} data-cursor-setup-field={name} value={input[name]} required disabled={disabled} maxLength={limits[name]} autoComplete="off" spellCheck={false} aria-describedby={fieldID+'-help'} onChange={event=>update(name,event.target.value)}/><span id={fieldID+'-help'} className="cursor-setup-meta">{t('help.'+name)}</span></label>;}
 async function copy(){
  if(disabled||!prepare||copying||copyState.current.inFlight)return;const original=prepare;copyState.current.inFlight=true;setCopying(true);setFailed('');
  try{await navigator.clipboard.writeText(original);if(copyState.current.prepare===original)setCopied(original);}
  catch{if(copyState.current.prepare===original)setFailed(original);}
  finally{copyState.current.inFlight=false;setCopying(false);}
 }
 function useConfiguration(){if(!disabled&&result&&onUseConfiguration)onUseConfiguration({...result.draft});}
 const copiedCurrent=!!prepare&&copied===prepare;
 return <section className="cursor-setup-guide" data-cursor-setup-guide aria-labelledby={id+'-title'}>
  <header className="cursor-setup-heading"><Terminal size={19} aria-hidden="true"/><div><h3 id={id+'-title'}>{t('title')}</h3><p className="cursor-setup-help">{t('intro')}</p></div></header>
  <ol className="cursor-setup-steps">
   <li><h4>{t('configure_title')}</h4><div className="cursor-setup-fields">
    <label className="cursor-setup-field cursor-setup-field-wide" htmlFor={id+'-shell'}><span>{t('field.shell')}</span><select id={id+'-shell'} name="cursor-setup-shell" data-cursor-setup-field="shell" value={input.shell} required disabled={disabled} aria-describedby={id+'-shell-help'} onChange={event=>update('shell',event.target.value)}><option value="">{t('choose_shell')}</option><option value="powershell">PowerShell</option><option value="bash">Bash</option></select><span id={id+'-shell-help'} className="cursor-setup-meta">{t('help.shell')}</span></label>
    {field('packageRoot')}{field('reviewedVersion',false)}{field('model',false)}{field('credentialEnvRef')}{field('commands')}
   </div><a className="cursor-setup-install" href="https://cursor.com/docs/cli/installation" target="_blank" rel="noopener noreferrer">{t('installation')}</a></li>
   <li><div className="cursor-setup-command-heading"><h4>{t('prepare_title')}</h4><button type="button" className="primary" data-cursor-setup-copy disabled={disabled||!prepare} aria-disabled={disabled||!prepare||copying} aria-busy={copying} onClick={()=>void copy()}>{copiedCurrent?<Check size={16} aria-hidden="true"/>:<Copy size={16} aria-hidden="true"/>}<span {...(copiedCurrent?{'data-cursor-setup-copied':true}:{})}>{t(copying?'copying':copiedCurrent?'copied':'copy_prepare')}</span></button></div>
    <p className="cursor-setup-help">{t('prepare_help')}</p>
    {result?<><p className="cursor-setup-meta" role="status">{t('draft_ready')}</p><pre className="cursor-setup-command" data-cursor-setup-prepare tabIndex={0} aria-label={t('prepare_title')}>{result.prepare}</pre></>:<p className="cursor-setup-incomplete" role="status">{t('incomplete')}</p>}
    {!!prepare&&failed===prepare&&<p className="cursor-setup-error" role="alert" data-cursor-setup-copy-failed>{t('copy_failed')}</p>}
   </li>
   <li><h4>{t('continue_title')}</h4><p className="cursor-setup-help">{t('continue_help')}</p>
    {onUseConfiguration&&<button type="button" className="cursor-setup-use" data-cursor-setup-use disabled={disabled||!result} onClick={useConfiguration}>{t('use_configuration')}<ArrowRight size={16} aria-hidden="true"/></button>}
    <p className="cursor-setup-meta">{t('handoff_boundary')}</p>
   </li>
  </ol>
  <details className="cursor-setup-details"><summary>{t('details')}</summary><div className="cursor-setup-detail-body">
   <p className="cursor-setup-help">{t('verification_boundary')}</p>
   {result&&<><p className="cursor-setup-meta">{t('derived_entry')}</p><code className="cursor-setup-derived">{result.request.executable}</code><h4>{t('describe_title')}</h4><pre className="cursor-setup-command" data-cursor-setup-describe tabIndex={0} aria-label={t('describe_title')}>{result.describe}</pre></>}
   <p className="cursor-setup-help">{t('observation_boundary')}</p>
  </div></details>
 </section>;
}
