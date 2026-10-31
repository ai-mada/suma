import {useState} from 'react';
import {Check,Copy} from 'lucide-react';
import {Field} from './ui';
import {tr,useLocale} from './i18n';
import {wellFormed} from './i18n/catalog';
const t=(key:string)=>tr('acp-runtime.'+key);
const encoder=new TextEncoder();
const safe=(value:string,max:number)=>value.length>0&&value.trim()===value&&wellFormed(value)&&encoder.encode(value).length<=max&&!/[\p{Cc}\u2028\u2029]/u.test(value);
const reserved=new Set(['HOME','USERPROFILE','PATH','APPDATA','LOCALAPPDATA','TEMP','TMP','TMPDIR','COMSPEC','SYSTEMROOT','SYSTEMDRIVE','WINDIR','PATHEXT']);
/** Administrator draft only. The Runner validates the actual native file,
 * exact version and credentials; this form grants no execution eligibility. */
export function QoderSetupGuide(){
 useLocale();const [input,setInput]=useState({executable:'',version:'',model:'',credential:''}),[copied,setCopied]=useState(''),[failed,setFailed]=useState('');
 const windows=/^[a-z]:[\\/]/i.test(input.executable),native=safe(input.executable,4096)&&(windows?/\.exe$/i.test(input.executable):/^\/(?!\/)/.test(input.executable)&&! /\.(?:cmd|bat|ps1|sh|js|mjs|cjs)$/i.test(input.executable));
 const credential=!input.credential||/^[A-Z_][A-Z0-9_]{0,127}$/.test(input.credential)&&!/^SUMA_|^CODEX/.test(input.credential)&&!reserved.has(input.credential);
 const valid=native&&safe(input.version,256)&&safe(input.model,256)&&credential;
 const serialized=valid?JSON.stringify({'qoder-develop':{adapter:'qoder',protocol:'acp-stdio-v1',executable:input.executable,args:['--acp'],input:'acp-stdio',read_only:false,acp:{runtime:'qoder',reviewed_version:input.version,model:input.model,...(input.credential?{credential_env_ref:input.credential}:{})}}},null,2):'';
 function field(name:keyof typeof input,key:string,max:number){return <Field label={t(key)}><input name={'qoder-'+name} value={input[name]} maxLength={max} autoComplete="off" spellCheck={false} onChange={event=>{setInput(current=>({...current,[name]:event.target.value}));setFailed('');}}/></Field>;}
 async function copy(){if(!serialized)return;try{await navigator.clipboard.writeText(serialized);setCopied(serialized);setFailed('');}catch{setFailed(serialized);}}
 return <section aria-label={t('setup_title')}>
  <p className="muted">{t('setup_intro')}</p>
  {field('executable','setup_executable',4096)}
  <div className="form-grid">{field('version','setup_version',256)}{field('model','setup_model',256)}</div>
  <p className="muted">{t('setup_version_hint')}</p>
  {field('credential','setup_credential_ref',128)}<p className="muted">{t('setup_credential_hint')}</p>
  <div className="tool-cli-setup-actions"><strong>{tr('cli-setup.local_config')}</strong><button type="button" data-qoder-setup-copy disabled={!serialized} onClick={()=>void copy()}>{serialized&&copied===serialized?<Check size={14}/>:<Copy size={14}/>} {tr(serialized&&copied===serialized?'cli-setup.copied':'cli-setup.copy')}</button></div>
  {serialized?<pre data-qoder-setup-config tabIndex={0} aria-label={tr('cli-setup.local_config')}>{serialized}</pre>:<p className="muted">{t('setup_invalid')}</p>}
  {serialized&&failed===serialized&&<p role="alert">{tr('cli-setup.copy_failed')}</p>}
  <ol><li>{tr('cli-setup.observe')}</li><li>{t('setup_register')}</li></ol>
  <code className="tool-cli-setup-command">suma-runner --commands commands.local.json --allow-exec --repository REPOSITORY --base-commit COMMIT --describe-tools</code>
  <p className="muted">{t('setup_no_scan')}</p><p className="muted">{t('setup_policy')}</p>
 </section>;
}
