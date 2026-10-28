import {useState} from 'react';
import {Check,Copy} from 'lucide-react';
import {Field} from './ui';
import {tr,useLocale} from './i18n';
import {octopHarnessConfiguration,type OctopHarnessInputs} from './octop-execution-model';
const t=(key:string)=>tr('octop-execution.'+key);
const initial:OctopHarnessInputs={python:'',bundle:'',manifest:'',checks:'',provider:'',model:'',endpoint:'',keyEnvironment:'OCTOP_MODEL_API_KEY',maxSteps:'8',maxOutputTokens:'4096'};
/** A local administrator configuration draft. Rendering or copying never
 * installs a runtime, reads an API key or changes a registered tool profile. */
export function OctopHarnessSetup(){
 useLocale();const [input,setInput]=useState(initial),[copied,setCopied]=useState(''),[failed,setFailed]=useState(''),config=octopHarnessConfiguration(input),serialized=config?JSON.stringify(config,null,2):'';
 function field(name:keyof OctopHarnessInputs,type:'text'|'url'|'number'='text',maxLength=2048){return <Field label={t('setup_'+name)}><input name={'octop-'+name} value={input[name]} onChange={event=>{setInput(current=>({...current,[name]:event.target.value}));setFailed('');}} type={type} autoComplete="off" spellCheck={false} maxLength={maxLength} {...(type==='number'?{min:1,max:name==='maxSteps'?32:32768,step:1}:{})}/></Field>;}
 async function copy(){if(!serialized)return;setFailed('');try{await navigator.clipboard.writeText(serialized);setCopied(serialized);}catch{setFailed(serialized);}}
 return <section aria-label={t('setup_title')}>
  <p className="muted">{t('setup_intro')}</p>
  <div className="form-grid">{field('provider','text',100)}{field('model','text',200)}</div>{field('endpoint','url')}
  <details className="connection-guidance"><summary>{t('setup_runtime')}</summary><p className="muted">{t('setup_runtime_help')}</p>{field('python')}{field('bundle')}{field('manifest')}{field('checks','text',1616)}<p className="muted">{t('setup_checks_help')}</p></details>
  <details className="connection-guidance"><summary>{t('setup_parameters')}</summary>{field('keyEnvironment','text',100)}<p className="muted">{t('setup_key_help')}</p><div className="form-grid">{field('maxSteps','number')}{field('maxOutputTokens','number')}</div></details>
  <p className="muted">{t('observe_only')}</p>
  <div className="tool-cli-setup-actions"><strong>{tr('cli-setup.local_config')}</strong><button type="button" data-octop-setup-copy disabled={!serialized} onClick={()=>void copy()}>{serialized&&copied===serialized?<Check size={14}/>:<Copy size={14}/>} {tr(serialized&&copied===serialized?'cli-setup.copied':'cli-setup.copy')}</button></div>
  {serialized?<pre data-octop-setup-config tabIndex={0} aria-label={tr('cli-setup.local_config')}>{serialized}</pre>:<p className="muted">{t('setup_incomplete')}</p>}
  {serialized&&failed===serialized&&<p role="alert">{tr('cli-setup.copy_failed')}</p>}
  <details className="connection-guidance"><summary>{t('setup_next')}</summary><ol><li>{t('setup_probe')}</li><li>{t('setup_import')}</li><li>{t('setup_dispatch')}</li></ol><p className="muted">{t('workspace_boundary')}</p><p className="muted">{t('setup_product_boundary')}</p></details>
 </section>;
}
