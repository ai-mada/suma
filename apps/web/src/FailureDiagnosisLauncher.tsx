import {useCallback,useEffect,useId,useState} from 'react';
import {ChevronDown} from 'lucide-react';
import {ApiError,query,stateLabel,type Run,type Session} from './api';
import {useApi} from './api-context';
import {formatNumber,tr,UiError,useLocale} from './i18n';
import {toolError} from './tool-hooks';
import {ErrorNotice,Field,Loading} from './ui';
import {useDiagnosisRead} from './failure-diagnosis-hooks';
import {diagnosisLocator} from './failure-diagnosis-model';
import {FailureDiagnosisPanel} from './FailureDiagnosisPanel';
import './FailureDiagnosisPanel.css';

export type FailureDiagnosisLauncherProps={session:Session;project:string;runID:string;refresh?:number;onChanged?:()=>void|Promise<void>};
const t=(key:string,values?:Record<string,unknown>)=>tr('failure-diagnosis.'+key,values);
export function FailureDiagnosisLauncher(props:FailureDiagnosisLauncherProps){return <LauncherScope key={JSON.stringify([props.session.user.id,props.session.organization.id,props.project,props.runID])} {...props}/>;}
function LauncherScope({session,project,runID,refresh=0,onChanged}:FailureDiagnosisLauncherProps){
 useLocale();const api=useApi(),bodyID=useId(),[open,setOpen]=useState(false),[selected,setSelected]=useState(''),[tick,setTick]=useState(0),[forbidden,setForbidden]=useState<unknown>();
 const validID=/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,100}$/.test(runID);
 const failed=useCallback((error:unknown)=>{if(error instanceof ApiError&&[401,403].includes(error.status)){setForbidden(error);setSelected('');}},[]);
 const valid=useCallback((value:Run)=>!!value&&value.id===runID&&value.project_id===project&&Number.isSafeInteger(value.version)&&value.version>0&&typeof value.state==='string'&&Array.isArray(value.nodes)&&value.nodes.length<=10000&&value.nodes.every(node=>typeof node.id==='string'&&typeof node.state==='string'&&Number.isSafeInteger(node.attempt)&&node.attempt>=0)&&new Set(value.nodes.map(node=>node.id)).size===value.nodes.length,[project,runID]);
 const detail=useDiagnosisRead(api,'/runs/'+encodeURIComponent(runID)+query(project),open&&validID&&!forbidden,refresh+tick,valid,failed),run=detail.value;
 const choices=run?.nodes.filter(node=>['failed','reconciling'].includes(node.state)&&diagnosisLocator({run_id:runID,node_id:node.id,attempt:node.attempt}))||[],node=choices.find(node=>node.id===selected),unknown=!!run&&(run.state==='reconciling'||run.nodes.some(value=>value.state==='reconciling'));
 useEffect(()=>{if(run&&selected&&!choices.some(value=>value.id===selected))setSelected('');},[run,selected]);
 function canLeave(){return window.dispatchEvent(new Event('suma:navigate',{cancelable:true}));}
 const link='?'+new URLSearchParams({project,view:'development',sub:'runs',run:runID,...(node?{node:node.id,attempt:String(node.attempt)}:{})});
 return <section className="failure-diagnosis-panel fd-launcher"><button data-fd-launch-toggle type="button" className="fd-toggle" aria-expanded={open} aria-controls={bodyID} onClick={()=>{if(!open||canLeave())setOpen(value=>!value);}}><span>{t('launcher_title')}</span><ChevronDown size={16} aria-hidden="true"/></button>
  {open&&<div id={bodyID} className="fd-body"><p className="fd-muted">{t('launcher_help')}</p><div className="fd-actions"><a data-fd-launch-run href={link}>{t('open_source_run')}</a><button data-fd-launch-refresh type="button" disabled={detail.loading} onClick={()=>{if(canLeave()){setForbidden(undefined);setTick(value=>value+1);}}}>{t('refresh')}</button></div>
   {!validID&&<p className="warning">{t('launcher_invalid_run')}</p>}{detail.loading&&<Loading/>}
   {forbidden?<><p className="fd-warning">{t('launcher_access')}</p><ErrorNotice error={toolError(forbidden)}/></>:<>
    {Boolean(detail.error)&&<ErrorNotice error={detail.error instanceof UiError?t('launcher_invalid_run'):toolError(detail.error)} onRetry={()=>setTick(value=>value+1)}/>}
    {run&&<>{unknown&&<p className="fd-warning" role="status">{t('launcher_unknown')}</p>}{choices.length?<Field label={t('launcher_node')}><select name="fd_current_node" aria-label={t('launcher_node')} value={selected} disabled={detail.loading||!!detail.error} onChange={event=>{if(canLeave())setSelected(event.target.value);}}><option value="">{t('launcher_choose_node')}</option>{choices.map(value=><option key={value.id} value={value.id}>{value.name||value.id} · {stateLabel(value.state)} · {t('launcher_attempt',{value:formatNumber(value.attempt)})}</option>)}</select></Field>:<p className="fd-muted">{t('launcher_empty')}</p>}
     {node&&node.state==='failed'&&!unknown&&!detail.error&&<FailureDiagnosisPanel session={session} project={project} source={{run_id:run.id,node_id:node.id,attempt:node.attempt}} initiallyOpen refresh={refresh} onChanged={onChanged}/>}
    </>}
   </>}
  </div>}
 </section>;
}
