import {useId,useState} from 'react';
import {Check,Copy,Network} from 'lucide-react';
import {tr,useLocale} from './i18n';
import {remoteRunnerLocalTools,remoteRunnerSetupCommands,type RemoteRunnerSetupInput} from './remote-runner-setup-model';
import './RemoteRunnerSetupGuide.css';

const t=(key:string)=>tr('remote-runner-setup.'+key);
const toolNames:Record<(typeof remoteRunnerLocalTools)[number],string>={'codex-jsonl':'Codex CLI','codebuddy-cli':'CodeBuddy Code','dsh-multica':'DeepSeek Harness / dsh','claude-code-cli':'Claude Code','cursor-cli':'Cursor','qoder-acp':'Qoder CLI','codex-app-server':'Codex App Server','opencode-sdk':'OpenCode SDK'};

/** The target machine and its original Runner remain the only execution
 * authority. This guide builds public drafts and never sends or runs them. */
export function RemoteRunnerSetupGuide({project}:{project?:string}){
 useLocale();const hint=useId(),[input,setInput]=useState<RemoteRunnerSetupInput>({shell:'',control:'',project:project||'',worker:'',commands:'',state:'',localTool:'',repository:'',baseCommit:''}),[copied,setCopied]=useState(''),[failed,setFailed]=useState(''),commands=remoteRunnerSetupCommands(input);
 function update(name:keyof RemoteRunnerSetupInput,value:string){setInput(current=>({...current,[name]:value}));setFailed('');}
 function field(name:'control'|'project'|'worker'|'commands'|'state'|'repository'|'baseCommit',maxLength=4096){return <label className="remote-runner-field">{t('field.'+name)}<input name={'remote-runner-'+name} value={input[name]} type={name==='control'?'url':'text'} maxLength={maxLength} autoComplete="off" spellCheck={false} aria-describedby={hint} onChange={event=>update(name,event.target.value)}/></label>;}
 async function copy(kind:'prepare'|'report'){const command=commands[kind];if(!command)return;try{await navigator.clipboard.writeText(command);setCopied(command);setFailed('');}catch{setFailed(command);}}
 function command(kind:'prepare'|'report'){const value=commands[kind];return <div className="remote-runner-command"><div className="remote-runner-command-heading"><strong>{t('command.'+kind)}</strong><button type="button" data-remote-runner-copy={kind} disabled={!value} onClick={()=>void copy(kind)}>{value&&copied===value?<Check size={15} aria-hidden="true"/>:<Copy size={15} aria-hidden="true"/>}{t(value&&copied===value?'copied':'copy')}</button></div>{value?<pre tabIndex={0} data-remote-runner-command={kind} aria-label={t('command.'+kind)}>{value}</pre>:<p className="remote-runner-incomplete" role="status">{t('incomplete.'+kind)}</p>}{value&&failed===value&&<p className="remote-runner-warning" role="alert">{t('copy_failed')}</p>}</div>;}
 return <details className="remote-runner-setup" data-remote-runner-setup>
  <summary><Network size={17} aria-hidden="true"/>{t('title')}</summary>
  <div className="remote-runner-body">
   <p>{t('intro')}</p><p id={hint} className="remote-runner-help">{t('public_only')}</p>
   <ol className="remote-runner-steps">
    <li><h4>{t('prepare.title')}</h4><p>{t('prepare.intro')}</p>
     <label className="remote-runner-field">{t('field.shell')}<select name="remote-runner-shell" value={input.shell} onChange={event=>update('shell',event.target.value)}><option value="">{t('choose_shell')}</option><option value="powershell">PowerShell</option><option value="bash">Bash</option></select></label>
     <p className="remote-runner-help">{t('shell_help')}</p>{field('commands')}
     <label className="remote-runner-field">{t('field.localTool')}<select name="remote-runner-localTool" value={input.localTool} onChange={event=>update('localTool',event.target.value)}><option value="">{t('all_tools')}</option>{remoteRunnerLocalTools.map(tool=><option key={tool} value={tool}>{toolNames[tool]}</option>)}</select></label>
     {command('prepare')}<p className="remote-runner-help">{t('prepare.boundary')}</p>
    </li>
    <li><h4>{t('report.title')}</h4><p>{t('report.intro')}</p>{field('control',2048)}
     {commands.loopback&&<p className="remote-runner-warning" data-remote-runner-loopback role="status">{t('loopback')}</p>}
     <div className="remote-runner-fields">{field('project',200)}{field('worker',200)}</div>{field('state')}
     {field('repository')}{field('baseCommit',64)}<p className="remote-runner-help">{t('baseline_help')}</p>
     <p className="remote-runner-help">{t('token_help')}</p>{command('report')}<p className="remote-runner-help">{t('report.boundary')}</p>
    </li>
    <li><h4>{t('reuse.title')}</h4><p>{t('reuse.intro')}</p></li>
    <li><h4>{t('enable.title')}</h4><p>{t('enable.intro')}</p></li>
   </ol>
  </div>
 </details>;
}
