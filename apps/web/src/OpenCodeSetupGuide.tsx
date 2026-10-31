import {useState} from 'react';
import {Check,Copy} from 'lucide-react';
import {tr,useLocale} from './i18n';
import {openCodeSetupConfiguration,type OpenCodeSetupInput} from './opencode-setup-guide-model';

const t=(key:string)=>tr('opencode-setup-guide.'+key);
const empty:OpenCodeSetupInput={node:'',bundle:'',manifest:'',executable:'',provider:'',model:'',endpoint:'',credential:'',checks:''};
const paragraph={fontSize:14,lineHeight:1.7} as const;
/** A remote/local Runner administrator copies a draft. Only the actual Runner
 * can verify files, assets, configured checks and execution eligibility. */
export function OpenCodeSetupGuide(){
 useLocale();const[input,setInput]=useState(empty),[copied,setCopied]=useState(''),[failed,setFailed]=useState(''),configuration=openCodeSetupConfiguration(input),serialized=configuration?JSON.stringify(configuration,null,2):'';
 function field(name:keyof OpenCodeSetupInput,maxLength=4096){return <label style={{display:'grid',gap:6,minWidth:0,fontSize:14,marginBlock:12}}>{t('field.'+name)}<input name={'opencode-'+name} value={input[name]} type={name==='endpoint'?'url':'text'} maxLength={maxLength} autoComplete="off" spellCheck={false} style={{width:'100%',minWidth:0,minHeight:44,fontSize:14}} onChange={event=>{setInput(current=>({...current,[name]:event.target.value}));setFailed('');}}/></label>;}
 async function copy(){if(!serialized)return;try{await navigator.clipboard.writeText(serialized);setCopied(serialized);setFailed('');}catch{setFailed(serialized);}}
 return <section data-opencode-setup-guide aria-label={t('title')}>
  <p style={paragraph}>{t('intro')}</p><p style={paragraph}>{t('runtime_versions')}</p>
  {field('node')}{field('bundle')}{field('manifest')}{field('executable')}
  <p style={paragraph}>{t('paths_help')}</p>
  <div className="form-grid">{field('provider',100)}{field('model',256)}</div>
  {field('endpoint',2048)}{field('credential',128)}<p style={paragraph}>{t('credential_help')}</p>
  {field('checks',2048)}<p style={paragraph}>{t('checks_help')}</p>
  <div className="tool-cli-setup-actions"><strong>{tr('cli-setup.local_config')}</strong><button type="button" data-opencode-setup-copy disabled={!serialized} style={{minHeight:44,fontSize:14}} onClick={()=>void copy()}>{serialized&&copied===serialized?<Check size={14}/>:<Copy size={14}/>} {tr(serialized&&copied===serialized?'cli-setup.copied':'cli-setup.copy')}</button></div>
  {serialized?<pre data-opencode-setup-config tabIndex={0} style={{fontSize:14}} aria-label={tr('cli-setup.local_config')}>{serialized}</pre>:<p style={paragraph} role="status">{t('incomplete')}</p>}
  {serialized&&failed===serialized&&<p role="alert">{tr('cli-setup.copy_failed')}</p>}
  <details className="connection-guidance"><summary>{t('next')}</summary><ol><li style={paragraph}>{t('save')}</li><li style={paragraph}>{t('verify')}</li><li style={paragraph}>{t('register')}</li></ol><p style={paragraph}>{t('boundary')}</p></details>
 </section>;
}
