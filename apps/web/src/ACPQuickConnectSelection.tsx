import {useEffect,useId,useState} from 'react';
import {RefreshCw,Search} from 'lucide-react';
import {formatDateTime,formatNumber,tr,useLocale} from './i18n';
import {selectedQuickACPModel,validQuickACPCatalogue,type ACPQuickSelectionState} from './acp-quick-connect-selection-model';
import './ACPQuickConnectSelection.css';

const t=(key:string,values?:Record<string,unknown>)=>tr('acp-quick-connect.'+key,values);
export type ACPQuickConnectSelectionProps={toolName:'Qoder'|'Cursor Agent';installationGuideURL:'https://docs.qoder.com/cli/installation'|'https://cursor.com/docs/cli/installation';state:ACPQuickSelectionState;disabled?:boolean;detecting?:boolean;canDiscover:boolean;onInspect:()=>void;onInstallationChange:(id:string)=>void;onDiscover:()=>void;onModelChange:(id:string)=>void;onRecover?:()=>void};

/** Controlled selection only. The owning desktop controller supplies exact
 * observed sources and performs authorized actions; rendering never probes. */
export function ACPQuickConnectSelection({toolName,installationGuideURL,state,disabled=false,detecting=false,canDiscover,onInspect,onInstallationChange,onDiscover,onModelChange,onRecover}:ACPQuickConnectSelectionProps){
 useLocale();const installationID=useId(),modelID=useId(),hintID=useId(),[now,setNow]=useState(()=>Date.now());
 useEffect(()=>{
  const current=Date.now();setNow(current);if(state.phase!=='ready'||!state.expiresAt)return;
  const remaining=Date.parse(state.expiresAt)-current;if(!Number.isFinite(remaining)||remaining<=0)return;
  const timer=window.setTimeout(()=>setNow(Date.now()),Math.min(remaining,2147483647));return()=>window.clearTimeout(timer);
 },[state.phase,state.expiresAt,state.sourceKey,state.catalogueSourceKey]);
 const installation=state.installations.find(item=>item.id===state.installationID),catalogue=validQuickACPCatalogue(state.catalogue)?state.catalogue:undefined;
 const sourceMatches=!!state.sourceKey&&state.sourceKey===state.catalogueSourceKey;
 const currentTime=Math.max(now,Date.now()),expires=state.expiresAt?Date.parse(state.expiresAt):NaN,expiryKnown=Number.isFinite(expires),fresh=expiryKnown&&expires>currentTime;
 const unknown=state.phase==='unknown',busy=disabled||detecting||state.phase==='reading',locked=busy||unknown;
 const native=installation?.kind==='native'&&!!installation.version;
 const choicesEnabled=!locked&&native&&state.phase==='ready'&&!!catalogue&&sourceMatches&&fresh;
 const choice=selectedQuickACPModel(state,currentTime),current=catalogue?.models.find(item=>item.id===catalogue.current_value);
 const status=state.phase!=='ready'?state.phase:!sourceMatches?'source_changed':!expiryKnown?'unverified':!fresh?'expired':!catalogue?'invalid_catalogue':!catalogue.models.length?'empty':'ready';
 return <section className="acp-quick-selection" aria-label={t('title',{tool:toolName})} data-acp-selection-ready={!!choice} aria-busy={detecting||state.phase==='reading'}>
  <div className="acp-quick-selection-heading"><p className="acp-quick-selection-intro">{t('intro',{tool:toolName})}</p><button type="button" data-acp-installations-inspect disabled={locked} onClick={onInspect}><Search size={15}/>{t(detecting?'detecting':'inspect')}</button></div>
  <div className="acp-quick-selection-field">
   <label htmlFor={installationID}>{t('installation')}</label>
   <select id={installationID} data-acp-installation-select value={state.installationID} disabled={locked||!state.installations.length} onChange={event=>{const found=state.installations.find(item=>item.id===event.target.value);if(!locked&&found?.kind==='native'&&found.version)onInstallationChange(found.id);}}>
    <option value="">{t('choose_installation')}</option>
    {state.installations.map(item=><option key={item.id} value={item.id} disabled={item.kind!=='native'||!item.version}>{t('installation_option',{name:item.label,version:item.version||t('version_unknown'),kind:t('kind_'+item.kind)})}</option>)}
   </select>
   {!state.installations.length&&!detecting&&<p className="acp-quick-selection-hint">{t('no_installations')}</p>}
   {!state.installations.some(item=>item.kind==='native'&&item.version)&&<a href={installationGuideURL} target="_blank" rel="noreferrer">{t('native_installation_guide')}</a>}
   {installation&&<p className="acp-quick-selection-hint">{t('version')}: {installation.version?<code>{installation.version}</code>:t('version_unknown')}</p>}
  </div>
  <div className="acp-quick-models">
   <div className="acp-quick-models-heading"><h4>{t('models')}</h4>{unknown&&onRecover?<button type="button" data-acp-observation-recover disabled={disabled||detecting} onClick={onRecover}><RefreshCw size={15}/>{t('recover')}</button>:<button type="button" data-acp-models-discover disabled={locked||!native||!canDiscover} onClick={onDiscover}><RefreshCw size={15}/>{t(state.phase==='reading'?'reading':'discover')}</button>}</div>
   <p className="acp-quick-selection-hint">{t('probe_notice')}</p>
   <p className="acp-quick-selection-status" role="status" data-acp-models-state={status}>{t('state_'+status)}</p>
   {catalogue&&state.phase==='ready'&&sourceMatches&&fresh&&<>
    <div className="acp-quick-selection-field"><label htmlFor={modelID}>{t('fixed_model')}</label><select id={modelID} data-acp-model-select aria-describedby={hintID} value={state.modelID} disabled={!choicesEnabled} onChange={event=>{const selected=selectedQuickACPModel({...state,modelID:event.target.value},Date.now());if(choicesEnabled&&selected)onModelChange(selected.id);}}><option value="">{t('choose_model')}</option>{catalogue.models.map(item=><option key={item.id} value={item.id}>{item.name?item.name+' · '+item.id:item.id}</option>)}</select></div>
   </>}
   {choice&&<p className="acp-quick-selected" data-acp-selected-model>{t('selected_model')}: <code>{choice.id}</code></p>}
   <details className="acp-quick-selection-details"><summary>{t('observation_details')}</summary><div className="acp-quick-selection-metadata">{catalogue&&<span>{t('model_count',{count:formatNumber(catalogue.models.length)})}</span>}{state.expiresAt&&<span>{t('expires')}: <time dateTime={state.expiresAt}>{formatDateTime(state.expiresAt,{dateStyle:'short',timeStyle:'short'})}</time></span>}</div><p className="acp-quick-selection-hint">{t('current_configuration')}: {current?<code>{current.id}</code>:t('current_unverified')}</p><p id={hintID} className="acp-quick-selection-hint">{t('selection_boundary')}</p></details>
  </div>
 </section>;
}
