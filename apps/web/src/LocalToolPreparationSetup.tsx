import {useId,useLayoutEffect,useRef,useState} from 'react';
import {ArrowRight,Check,Copy} from 'lucide-react';
import {useApi,useApiView} from './api-context';
import {tr,useLocale} from './i18n';
import {localToolPreparationCommand,localToolPreparationTarget,validLocalToolPreparationDraft,type LocalToolPreparationDraft,type LocalToolPreparationTarget} from './local-tool-preparation-model';
import './LocalToolPreparationSetup.css';

export type LocalToolPreparationSetupProps={localTool:string;disabled?:boolean;onUseConfiguration?:(draft:LocalToolPreparationDraft)=>void};
const t=(key:string,values?:Record<string,unknown>)=>tr('local-tool-preparation.'+key,values);

/** A local file reference only. The parent guide retains project/Worker
 * ownership; this component never probes, sends or executes the command. */
export function LocalToolPreparationSetup(props:LocalToolPreparationSetupProps){
  const caller=useApi(),view=useApiView(),basis=JSON.stringify([view?.userID,view?.organizationID,props.localTool]);
  const source=useRef({caller,basis,epoch:0});if(source.current.caller!==caller||source.current.basis!==basis)source.current={caller,basis,epoch:source.current.epoch+1};
  const target=localToolPreparationTarget(props.localTool),epoch=source.current.epoch;
  useLocale();
  if(!target)return <p role="status" data-local-tool-preparation-unavailable>{t('unsupported')}</p>;
  const owns=()=>source.current.caller===caller&&source.current.basis===basis&&source.current.epoch===epoch;
  return <PreparationScope key={epoch} target={target} disabled={props.disabled} onUseConfiguration={props.onUseConfiguration} owns={owns}/>;
}
function PreparationScope({target,disabled=false,onUseConfiguration,owns}:{target:LocalToolPreparationTarget;disabled?:boolean;onUseConfiguration?:LocalToolPreparationSetupProps['onUseConfiguration'];owns:()=>boolean}){
  useLocale();const id=useId(),[shell,setShell]=useState<''|'powershell'|'bash'>(''),[commands,setCommands]=useState(''),[copying,setCopying]=useState(false),[receipt,setReceipt]=useState<{revision:number;command:string;state:'copied'|'failed'}>();
  const owner=useRef({alive:true,disabled,revision:0,inflight:false});
  // Disable/re-enable is a new adoption generation even if all field values
  // later match the exact old draft again.
  if(owner.current.disabled!==disabled){owner.current.disabled=disabled;owner.current.revision++;}
  const latest=useRef({shell,commands,localTool:target.localTool,onUseConfiguration});latest.current={shell,commands,localTool:target.localTool,onUseConfiguration};
  useLayoutEffect(()=>{owner.current.alive=true;return()=>{owner.current.alive=false;owner.current.revision++;};},[]);
  const candidate={shell,commands,localTool:target.localTool},draft=validLocalToolPreparationDraft(candidate)?candidate:undefined,result=localToolPreparationCommand(draft),active=!disabled&&owns();
  const currentReceipt=active&&receipt?.revision===owner.current.revision&&receipt.command===result?.prepare?receipt:undefined;
  function editable(){return owner.current.alive&&!owner.current.disabled&&owns();}
  function edited(){owner.current.revision++;setReceipt(undefined);}
  async function copy(){
    if(!editable()||owner.current.inflight)return;
    const {shell,commands,localTool}=latest.current,command=localToolPreparationCommand({shell,commands,localTool});if(!command)return;
    const revision=owner.current.revision;owner.current.inflight=true;setCopying(true);setReceipt(undefined);
    try{await navigator.clipboard.writeText(command.prepare);if(editable()&&owner.current.revision===revision)setReceipt({revision,command:command.prepare,state:'copied'});}
    catch{if(editable()&&owner.current.revision===revision)setReceipt({revision,command:command.prepare,state:'failed'});}
    finally{owner.current.inflight=false;if(owner.current.alive&&owns())setCopying(false);}
  }
  function useConfiguration(){
    if(!editable()||owner.current.inflight)return;
    const {shell,commands,localTool}=latest.current,value={shell,commands,localTool};
    if(validLocalToolPreparationDraft(value)&&localToolPreparationCommand(value))latest.current.onUseConfiguration?.({...value});
  }
  return <section className="local-tool-preparation" data-local-tool-preparation data-local-tool={target.localTool} aria-labelledby={id+'-title'}>
    <h3 id={id+'-title'}>{t('title')}</h3><p>{t('intro',{tool:t('tool.'+target.localTool)})}</p>
    <div className="local-tool-preparation-fields"><label htmlFor={id+'-shell'}><span>{t('shell')}</span><select id={id+'-shell'} data-local-tool-shell value={shell} disabled={disabled} onChange={event=>{if(editable()){edited();setShell(event.target.value as typeof shell);}}}><option value="">{t('choose_shell')}</option><option value="powershell">PowerShell</option><option value="bash">Bash</option></select></label>
      <label htmlFor={id+'-output'}><span>{t('output')}</span><input id={id+'-output'} data-local-tool-output value={commands} disabled={disabled} maxLength={4096} autoComplete="off" spellCheck={false} aria-describedby={id+'-output-help'} aria-invalid={!!commands&&!!shell&&!draft||undefined} onChange={event=>{if(editable()){edited();setCommands(event.target.value);}}}/><small id={id+'-output-help'}>{t('output_help')}</small></label></div>
    <div className="local-tool-preparation-actions"><button type="button" data-local-tool-copy disabled={!active||!result||copying} onClick={()=>void copy()}>{currentReceipt?.state==='copied'?<Check size={15} aria-hidden="true"/>:<Copy size={15} aria-hidden="true"/>}{t(copying?'copying':currentReceipt?.state==='copied'?'copied':'copy')}</button>
      {onUseConfiguration&&<button type="button" data-local-tool-use disabled={!active||!draft||copying} onClick={useConfiguration}>{t('use')}<ArrowRight size={15} aria-hidden="true"/></button>}</div>
    {active&&result?<pre tabIndex={0} data-local-tool-preview aria-label={t('preview')}>{result.prepare}</pre>:<p role="status" data-local-tool-status="incomplete">{t(disabled?'blocked':'incomplete')}</p>}
    {currentReceipt&&<p role={currentReceipt.state==='failed'?'alert':'status'} data-local-tool-status={currentReceipt.state}>{t(currentReceipt.state==='failed'?'copy_failed':'copy_boundary')}</p>}
    <p className="local-tool-preparation-boundary">{t('boundary')}</p>
  </section>;
}
