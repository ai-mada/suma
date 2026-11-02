import {useId,useLayoutEffect,useRef,useState} from 'react';
import {ArrowRight,Check,Copy} from 'lucide-react';
import {useApi,useApiView} from './api-context';
import {tr,useLocale} from './i18n';
import {qoderSetupCommands,validQoderSetupDraft,type QoderSetupDraft,type QoderSetupInput} from './qoder-setup-guide-model';
import './QoderSetupGuide.css';

export type QoderSetupGuideProps={disabled?:boolean;onUseConfiguration?:(draft:QoderSetupDraft)=>void};
const t=(key:string)=>tr('qoder-setup.'+key);
const emptyInput=():QoderSetupInput=>({shell:'',executable:'',reviewedVersion:'',model:'',credentialEnvRef:'',commands:''});
const fieldNames={executable:'executable',reviewedVersion:'version',model:'model',credentialEnvRef:'credential',commands:'commands'} as const;

export function QoderSetupGuide(props:QoderSetupGuideProps){
  const caller=useApi(),view=useApiView(),basis=JSON.stringify([view?.userID,view?.organizationID]);
  const source=useRef({caller,basis,epoch:0});
  if(source.current.caller!==caller||source.current.basis!==basis)source.current={caller,basis,epoch:source.current.epoch+1};
  const epoch=source.current.epoch,owns=()=>source.current.caller===caller&&source.current.basis===basis&&source.current.epoch===epoch;
  return <QoderSetupScope key={epoch} {...props} owns={owns}/>;
}
function QoderSetupScope({disabled=false,onUseConfiguration,owns}:QoderSetupGuideProps&{owns:()=>boolean}){
  useLocale();const id=useId(),[input,setInput]=useState(emptyInput),[copying,setCopying]=useState(false),[receipt,setReceipt]=useState<{revision:number;command:string;state:'copied'|'failed'}>();
  const owner=useRef({alive:true,disabled,revision:0,inflight:false});
  if(owner.current.disabled!==disabled){owner.current.disabled=disabled;owner.current.revision++;}
  const latest=useRef({input,onUseConfiguration});latest.current={input,onUseConfiguration};
  useLayoutEffect(()=>{owner.current.alive=true;return()=>{owner.current.alive=false;owner.current.revision++;};},[]);
  const result=qoderSetupCommands(input),active=!disabled&&owns(),draft={shell:input.shell,commands:input.commands,localTool:'qoder-acp'},currentReceipt=active&&receipt?.revision===owner.current.revision&&receipt.command===result?.prepare?receipt:undefined;
  function editable(){return owner.current.alive&&!owner.current.disabled&&owns();}
  function update<K extends keyof QoderSetupInput>(key:K,value:QoderSetupInput[K]){if(!editable())return;owner.current.revision++;setReceipt(undefined);setInput(old=>({...old,[key]:value}));}
  async function copy(){
    if(!editable()||owner.current.inflight)return;
    const command=qoderSetupCommands(latest.current.input);if(!command)return;
    const revision=owner.current.revision;owner.current.inflight=true;setCopying(true);setReceipt(undefined);
    try{await navigator.clipboard.writeText(command.prepare);if(editable()&&owner.current.revision===revision)setReceipt({revision,command:command.prepare,state:'copied'});}
    catch{if(editable()&&owner.current.revision===revision)setReceipt({revision,command:command.prepare,state:'failed'});}
    finally{owner.current.inflight=false;if(owner.current.alive&&owns())setCopying(false);}
  }
  function useConfiguration(){
    if(!editable()||owner.current.inflight||!qoderSetupCommands(latest.current.input))return;
    const value={shell:latest.current.input.shell,commands:latest.current.input.commands,localTool:'qoder-acp'};
    if(validQoderSetupDraft(value))latest.current.onUseConfiguration?.({...value});
  }
  const manual=result?JSON.stringify({'qoder-develop':{adapter:'qoder',protocol:'acp-stdio-v1',executable:result.request.executable,args:['--acp'],input:'acp-stdio',read_only:false,acp:{runtime:'qoder',reviewed_version:result.request.reviewed_version,model:result.request.model,...(result.request.credential_env_ref?{credential_env_ref:result.request.credential_env_ref}:{})}}},null,2):'';
  function field(key:Exclude<keyof QoderSetupInput,'shell'>,label:string,hook:string,max:number,help?:string){return <label htmlFor={id+'-'+key}><span>{t(label)}</span><input id={id+'-'+key} name={'qoder-'+fieldNames[key]} {...{['data-qoder-setup-'+hook]:true}} value={input[key]} maxLength={max} disabled={disabled} autoComplete="off" spellCheck={false} aria-describedby={help?id+'-'+key+'-help':undefined} onChange={event=>update(key,event.target.value)}/>{help&&<small id={id+'-'+key+'-help'}>{t(help)}</small>}</label>;}
  return <section className="qoder-setup" data-qoder-setup aria-labelledby={id+'-title'}>
    <h3 id={id+'-title'}>{t('title')}</h3><p>{t('intro')}</p>
    <div className="qoder-setup-fields">{field('model','model','model',256,'model_help')}{field('credentialEnvRef','credential','credential',128,'credential_help')}</div>
    <fieldset className="qoder-setup-machine"><legend>{t('machine')}</legend><div className="qoder-setup-fields">
      {field('executable','executable','executable',4096,'executable_help')}{field('reviewedVersion','version','version',256,'version_help')}
      <label htmlFor={id+'-shell'}><span>{t('shell')}</span><select id={id+'-shell'} name="qoder-shell" data-qoder-setup-shell value={input.shell} disabled={disabled} onChange={event=>update('shell',event.target.value as QoderSetupInput['shell'])}><option value="">{t('choose_shell')}</option><option value="powershell">PowerShell</option><option value="bash">Bash</option></select></label>
      {field('commands','output','output',4096,'output_help')}
    </div></fieldset>
    <div className="qoder-setup-actions"><button type="button" data-qoder-setup-copy disabled={!active||!result||copying} onClick={()=>void copy()}>{currentReceipt?.state==='copied'?<Check size={15} aria-hidden="true"/>:<Copy size={15} aria-hidden="true"/>}{t(copying?'copying':currentReceipt?.state==='copied'?'copied':'copy')}</button>
      {onUseConfiguration&&<button type="button" data-qoder-setup-use disabled={!active||!result||!validQoderSetupDraft(draft)||copying} onClick={useConfiguration}>{t('use')}<ArrowRight size={15} aria-hidden="true"/></button>}</div>
    {active&&result?<><pre tabIndex={0} data-qoder-setup-preview aria-label={t('preview')}>{result.prepare}</pre><details className="qoder-setup-private"><summary>{t('request')}</summary><pre tabIndex={0} data-qoder-setup-request>{JSON.stringify(result.request,null,2)}</pre></details></>:<p role="status" data-qoder-setup-status="incomplete">{t(disabled?'blocked':'incomplete')}</p>}
    {currentReceipt&&<p role={currentReceipt.state==='failed'?'alert':'status'} data-qoder-setup-status={currentReceipt.state}>{t(currentReceipt.state==='failed'?'copy_failed':'copy_boundary')}</p>}
    <p className="qoder-setup-boundary">{t('boundary')}</p>
    <details className="qoder-setup-manual"><summary>{t('manual')}</summary><p>{t('manual_help')}</p>{active&&manual&&<pre tabIndex={0} data-qoder-setup-config>{manual}</pre>}</details>
  </section>;
}
