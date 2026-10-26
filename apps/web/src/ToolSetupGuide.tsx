import {useId,useState} from 'react';
import {Check,Copy,Terminal} from 'lucide-react';
import {tr as t,useLocale} from './i18n';
import './ToolSetupGuide.css';

type Preset='codex'|'codebuddy'|'harness';
// These are local administrator templates, never a registration or live
// inventory. The existing observation/import flow remains the authority.
const presets={
 codex:{'codex-develop':{adapter:'codex',protocol:'codex-jsonl-v1',executable:'codex',args:['exec','--json','--sandbox','workspace-write','--ephemeral','-'],input:'goal',read_only:false}},
 codebuddy:{'codebuddy-develop':{adapter:'codebuddy',protocol:'codebuddy-jsonl-v1',executable:'codebuddy',args:['-p','--input-format','text','--output-format','stream-json','--verbose','--permission-mode','acceptEdits'],input:'goal',read_only:false}},
 harness:{'harness-develop':{adapter:'harness',protocol:'dsh-multica-jsonl-v1',executable:'dsh',args:['--profile','multica','--stdio'],input:'dsh-jsonl',read_only:false}},
} as const;

export function ToolSetupGuide(){
 useLocale();
 const [selected,setSelected]=useState<Preset>('codex'),[copied,setCopied]=useState(''),[failed,setFailed]=useState(false);
 const [launchMode,setLaunchMode]=useState<'native'|'node'>('native'),[entry,setEntry]=useState('');
 const entryHint=useId();
 const nodeMode=selected!=='codex'&&launchMode==='node';
 // The runner can be remote, so browser OS detection cannot select its launch
 // mode. This is a configuration preview; Go validates the actual local file.
 const validEntry=!/[\x00\r\n]/.test(entry)&&/\.(?:js|mjs|cjs)$/i.test(entry)&&(/^[a-z]:[\\/]/i.test(entry)||/^\/(?!\/)/.test(entry)||/^\\\\[^\\]+\\[^\\]+\\/.test(entry));
 const canCopy=!nodeMode||validEntry;
 const [commandName,command]=Object.entries(presets[selected])[0];
 const configuration=JSON.stringify({[commandName]:nodeMode?{...command,executable:'node',args:[entry,...command.args]}:command},null,2);
 async function copy(){if(!canCopy)return;try{await navigator.clipboard.writeText(configuration);setCopied(configuration);setFailed(false);}catch{setFailed(true);}}
 return <details className="tool-cli-setup">
  <summary><Terminal size={15}/>{t('cli-setup.title')}</summary>
  <div className="tool-cli-setup-body">
   <p className="muted">{t('cli-setup.intro')}</p>
   <label className="tool-cli-setup-select">{t('cli-setup.tool')}<select value={selected} onChange={event=>{setSelected(event.target.value as Preset);setFailed(false);}}>
    <option value="codex">Codex</option><option value="codebuddy">CodeBuddy Code</option><option value="harness">DeepSeek Harness / dsh</option>
   </select></label>
   {selected!=='codex'&&<label className="tool-cli-setup-select">{t('cli-setup.launch_mode')}<select value={launchMode} onChange={event=>{setLaunchMode(event.target.value as 'native'|'node');setFailed(false);}}>
    <option value="native">{t('cli-setup.native_mode')}</option><option value="node">{t('cli-setup.node_mode')}</option>
   </select></label>}
   {nodeMode&&<div className="tool-cli-setup-entry">
    <label>{t('cli-setup.entry')}<input value={entry} onChange={event=>{setEntry(event.target.value);setFailed(false);}} placeholder="C:/tools/cli/entry.js" spellCheck={false} autoComplete="off" aria-describedby={entryHint} aria-invalid={entry!==''&&!validEntry}/></label>
    <p id={entryHint} className="muted">{t('cli-setup.entry_help')}</p>
   </div>}
   <ol><li>{t('cli-setup.install')}</li><li>{t('cli-setup.configure')}</li><li>{t('cli-setup.observe')}</li><li>{t('cli-setup.register')}</li></ol>
   <div className="tool-cli-setup-actions"><strong>{t('cli-setup.local_config')}</strong><button type="button" disabled={!canCopy} onClick={()=>void copy()}>{canCopy&&copied===configuration?<Check size={14}/>:<Copy size={14}/>} {t(canCopy&&copied===configuration?'cli-setup.copied':'cli-setup.copy')}</button></div>
   <pre tabIndex={0} aria-label={t('cli-setup.local_config')}>{configuration}</pre>
   {failed&&<p role="alert">{t('cli-setup.copy_failed')}</p>}
   <p className="muted">{t(selected==='harness'?'cli-setup.harness_dependency':selected==='codebuddy'?'cli-setup.codebuddy_policy':'cli-setup.codex_policy')}</p>
   <p className="muted">{t('cli-setup.probe_help')}</p>
   <code className="tool-cli-setup-command">suma-runner --commands commands.local.json --allow-exec --repository REPOSITORY --base-commit COMMIT --describe-tools</code>
   <p className="muted">{t('cli-setup.shared_pool')}</p>
   <p className="muted">{t('cli-setup.boundary')}</p>
  </div>
 </details>;
}
