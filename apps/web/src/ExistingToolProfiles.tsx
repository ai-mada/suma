import {useId} from 'react';
import {ArrowRight,Play} from 'lucide-react';
import {formatNumber,renderDiagnostic,tr,useLocale} from './i18n';
import type {ToolProfile} from './tool-model';
import './ExistingToolProfiles.css';

export type ExistingToolProfilesProps={
 items:readonly ToolProfile[];
 selectedID:string;
 disabled?:boolean;
 onSelect:(id:string)=>void;
 onEnable:(profile:ToolProfile)=>void;
 onOpen?:(profile:ToolProfile)=>void;
};
const t=(key:string,values?:Record<string,unknown>)=>tr('tool-profile-reuse.'+key,values);
function ProfileFacts({profile}:{profile:ToolProfile}){
 return <div className="existing-tool-facts"><div className="existing-tool-heading"><strong>{profile.name}</strong><span className="existing-tool-state" data-state={profile.state}>{t('state.'+profile.state)}</span></div><div className="existing-tool-meta"><code>{profile.profile.command_id}</code><span>{t('version',{version:formatNumber(profile.version)})}</span><span>{t(profile.available?'available':'unavailable')}</span></div>{(profile.reason_diagnostic||profile.reason)&&<p className="existing-tool-reason">{renderDiagnostic(profile.reason_diagnostic??profile.reason)}</p>}</div>;
}

/** The parent owns the complete, scoped match query and original write receipt.
 * Selecting or viewing an existing profile never reports a new registration. */
export function ExistingToolProfiles({items,selectedID,disabled=false,onSelect,onEnable,onOpen}:ExistingToolProfilesProps){
 useLocale();const choiceName=useId(),current=items.filter(profile=>profile.state==='paused'||profile.state==='enabled'),history=items.filter(profile=>profile.state==='revoked'),selected=current.find(profile=>profile.id===selectedID);
 return <section className="existing-tools" data-existing-tools aria-label={t('title')}>
  <header><h4>{t('title')}</h4><p>{t('intro')}</p></header>
  {current.length>0&&<>
   <fieldset className="existing-tool-options" disabled={disabled}><legend>{t('choose')}</legend><div className="existing-tool-option-list">{current.map(profile=><label className="existing-tool-choice" data-selected={selectedID===profile.id} key={profile.id}><input data-existing-tool-choice type="radio" name={choiceName} value={profile.id} checked={selectedID===profile.id} onChange={()=>onSelect(profile.id)}/><ProfileFacts profile={profile}/></label>)}</div></fieldset>
   {!selected&&<p className="existing-tool-selection-hint" role="status">{t('select_required')}</p>}
   <div className="existing-tool-actions">
    {selected?.state==='paused'&&<button type="button" className="primary" data-existing-tool-enable={selected.id} disabled={disabled} onClick={()=>onEnable(selected)}><Play size={15} aria-hidden="true"/>{t('enable')}</button>}
    {selected?.state==='enabled'&&onOpen&&<button type="button" className="primary" data-existing-tool-open={selected.id} disabled={disabled} onClick={()=>onOpen(selected)}>{t('open')}<ArrowRight size={15} aria-hidden="true"/></button>}
   </div>
  </>}
  {history.length>0&&<details className="existing-tool-history" data-existing-tool-history><summary>{t('history',{count:formatNumber(history.length)})}</summary><p className="existing-tool-history-hint">{t('history_hint')}</p><ul>{history.map(profile=><li key={profile.id}><ProfileFacts profile={profile}/>{onOpen&&<button type="button" data-existing-tool-open={profile.id} disabled={disabled} onClick={()=>onOpen(profile)}>{t('history_open')}<ArrowRight size={15} aria-hidden="true"/></button>}</li>)}</ul></details>}
 </section>;
}
