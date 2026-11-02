import {useEffect,useId,useRef,useState} from 'react';
import {ArrowRight,Check,ChevronRight,Copy} from 'lucide-react';
import {tr,useLocale} from './i18n';
import {openCodeSetupCommands,type OpenCodeSetupInput,type OpenCodeSetupDraft} from './opencode-setup-guide-model';
import './OpenCodeSetupGuide.css';

const t=(key:string)=>tr('opencode-setup-guide.'+key);
const empty:OpenCodeSetupInput={shell:'',node:'',manifest:'',executable:'',provider:'',model:'',endpoint:'',credential:'',commands:''};
type TextField=Exclude<keyof OpenCodeSetupInput,'shell'>;
const limits:Record<TextField,number>={node:4096,manifest:4096,executable:4096,provider:100,model:256,endpoint:2048,credential:128,commands:4096};
export type OpenCodeSetupGuideProps={disabled?:boolean;onUseConfiguration?:(draft:OpenCodeSetupDraft)=>void};
/** A private machine-side preparation draft, not a provider connection test.
 * Only the Runner checks the reviewed asset closure and writes a new file. */
export function OpenCodeSetupGuide({disabled=false,onUseConfiguration}:OpenCodeSetupGuideProps={}){
 useLocale();const id=useId(),[input,setInput]=useState(empty),[copied,setCopied]=useState(''),[failed,setFailed]=useState(''),[copying,setCopying]=useState(false),result=openCodeSetupCommands(input),prepare=result?.prepare||'',owner=useRef({alive:true,revision:0,inflight:false});
 useEffect(()=>{owner.current.alive=true;return()=>{owner.current.alive=false;++owner.current.revision;};},[]);
 function update<K extends keyof OpenCodeSetupInput>(key:K,value:OpenCodeSetupInput[K]){if(disabled)return;++owner.current.revision;setInput(current=>({...current,[key]:value}));setCopied('');setFailed('');}
 function field(name:TextField){return <label className="opencode-setup-field" htmlFor={id+'-'+name}><span>{t('field.'+name)}</span><input id={id+'-'+name} name={'opencode-'+name} data-opencode-setup-field={name} value={input[name]} type={name==='endpoint'?'url':'text'} disabled={disabled} maxLength={limits[name]} autoComplete="off" spellCheck={false} onChange={event=>update(name,event.target.value)}/></label>;}
 async function copy(){if(disabled||!prepare||owner.current.inflight)return;const revision=owner.current.revision;owner.current.inflight=true;setCopying(true);setFailed('');try{await navigator.clipboard.writeText(prepare);if(owner.current.alive&&owner.current.revision===revision)setCopied(prepare);}catch{if(owner.current.alive&&owner.current.revision===revision)setFailed(prepare);}finally{owner.current.inflight=false;if(owner.current.alive)setCopying(false);}}
 return <section className="opencode-setup-guide" data-opencode-setup-guide aria-labelledby={id+'-title'}>
  <h3 id={id+'-title'}>{t('title')}</h3><p>{t('intro')}</p>
  <div className="opencode-setup-fields">{field('provider')}{field('model')}{field('endpoint')}{field('credential')}</div><p className="opencode-setup-muted">{t('credential_help')}</p>
  <details className="opencode-setup-machine" data-opencode-setup-machine><summary><ChevronRight size={16} aria-hidden="true"/>{t('machine')}</summary><p>{t('runtime_versions')}</p><div className="opencode-setup-fields"><label className="opencode-setup-field" htmlFor={id+'-shell'}><span>{t('field.shell')}</span><select id={id+'-shell'} name="opencode-shell" data-opencode-setup-field="shell" value={input.shell} disabled={disabled} onChange={event=>update('shell',event.target.value as OpenCodeSetupInput['shell'])}><option value="">{t('choose_shell')}</option><option value="powershell">PowerShell</option><option value="bash">Bash</option></select></label>{field('commands')}{field('node')}{field('executable')}{field('manifest')}</div><p className="opencode-setup-muted">{t('paths_help')}</p><p className="opencode-setup-muted">{t('new_file_help')}</p></details>
  <div className="opencode-setup-actions"><strong>{t('prepare_title')}</strong><button type="button" data-opencode-setup-copy disabled={disabled||!result||copying} onClick={()=>void copy()}>{copied===prepare&&prepare?<Check size={15} aria-hidden="true"/>:<Copy size={15} aria-hidden="true"/>}{t(copying?'copying':copied===prepare&&prepare?'copied':'copy')}</button></div>
  {result?<><pre data-opencode-setup-prepare tabIndex={0} aria-label={t('prepare_title')}>{prepare}</pre><details className="opencode-setup-review"><summary>{t('review')}</summary><pre data-opencode-setup-configuration tabIndex={0} aria-label={t('review')}>{JSON.stringify(result.request,null,2)}</pre></details></>:<p className="opencode-setup-incomplete" role="status">{t('incomplete')}</p>}
  {failed&&failed===prepare&&<p role="alert">{t('copy_failed')}</p>}
  {onUseConfiguration&&<button type="button" data-opencode-setup-use disabled={disabled||!result} onClick={()=>{if(!disabled&&result)onUseConfiguration({...result.draft});}}>{t('use_configuration')}<ArrowRight size={16} aria-hidden="true"/></button>}
  <p className="opencode-setup-muted">{t('boundary')}</p>
 </section>;
}
