import {ArrowRight, Check, CircleHelp, Radio, Server, ShieldCheck, Sparkles} from 'lucide-react';
import {formatNumber, tr as t, useLocale} from './i18n';
import type {ToolProfile} from './tool-model';
import './ToolReadiness.css';

export type ToolFilter = 'all'|'available'|'paused'|'attention'|'revoked';
export function matchesToolFilter(profile:ToolProfile, filter:ToolFilter) {
  if (filter==='all') return true;
  if (filter==='available') return profile.state==='enabled'&&profile.available;
  if (filter==='attention') return profile.state==='enabled'&&!profile.available;
  return profile.state===filter;
}

export function ToolReadiness({items,total,uncertain,identitiesKnown,identityCount,disabled,filtered=false,onClearFilters,onWorker,onRegister,onGuide,onSelect}:{
  items?:ToolProfile[];total:number;uncertain:boolean;identitiesKnown:boolean;identityCount:number;disabled:boolean;
  filtered?:boolean;onClearFilters?:()=>void;
  onWorker?:()=>void;onRegister:()=>void;onGuide:()=>void;onSelect:(id:string)=>void;
}) {
  useLocale();
  const ready=items?.filter(item=>matchesToolFilter(item,'available')).length??0;
  const paused=items?.find(item=>item.state==='paused');
  const blocked=items?.find(item=>matchesToolFilter(item,'attention'));
  const next=uncertain||!items?'refresh':filtered&&!items.length?'filtered_empty':!items.length?identitiesKnown&&identityCount===0?'identity':'register':blocked?'attention':paused?'review':ready?'ready':'register';
  const setup=next==='register';
  const action=next==='filtered_empty'?onClearFilters:next==='identity'?onWorker:next==='review'?()=>onSelect(paused!.id):next==='attention'?()=>onSelect(blocked!.id):onRegister;
  const title=next==='filtered_empty'?'tool-workspace.no_matches':setup?'tool-quick-connect.next':next==='identity'&&!onWorker?'tool-worker-identities.needs_node':'tool-workspace.next.'+next;
  const help=next==='filtered_empty'?'tool-workspace.no_matches_hint':setup?'tool-quick-connect.next_hint':next==='identity'&&!onWorker?'tool-worker-identities.request_node':'tool-workspace.help.'+next;
  const label=next==='filtered_empty'?'tool-workspace.clear_filters':setup?'tool-quick-connect.title':'tool-workspace.action.'+next;
  return <section className="tool-readiness" data-tool-readiness-state={next} aria-label={t('tool-workspace.readiness')}>
    <div className="tool-readiness-main">
      <span className="tool-readiness-icon"><Sparkles size={18} aria-hidden="true"/></span>
      <div><h3>{t(title)}</h3><p>{t(help)}</p></div>
      {!['refresh','ready'].includes(next)&&action&&<button type="button" className="text-button" data-tool-readiness-next data-tool-readiness-clear-filters={next==='filtered_empty'?'':undefined} disabled={disabled} onClick={action}>{t(label)}<ArrowRight size={14} aria-hidden="true"/></button>}
      {next==='ready'&&<span className="tool-status-pill available"><Check size={14} aria-hidden="true"/>{t('tool-workspace.eligible')}</span>}
    </div>
    <div className="tool-readiness-foot">
      <div className="tool-readiness-meta">
        <span><Server size={14} aria-hidden="true"/>{identitiesKnown?t('tool-workspace.identities',{count:formatNumber(identityCount)}):t('tool-workspace.identity_unknown')}</span>
        <span data-tool-readiness-scope><Radio size={14} aria-hidden="true"/>{uncertain?t('tool-workspace.unconfirmed'):t(filtered?'tool-workspace.filtered_scope':'tool-workspace.loaded_scope',{count:formatNumber(items?.length??0),total:formatNumber(total)})}</span>
      </div>
      <button type="button" className="text-button" disabled={disabled} onClick={onGuide}><CircleHelp size={14} aria-hidden="true"/>{t('tool-workspace.setup_guide')}</button>
      <details className="tool-journey-disclosure">
    <summary>{t('tool-workspace.setup_journey')}</summary>
    <ol className="tool-setup-journey" aria-label={t('tool-workspace.setup_journey')}>
      {['identity','probe','register','review'].map((step,index)=><li key={step}><span>{formatNumber(index+1)}</span><div><strong>{t('tool-workspace.step.'+step)}</strong><small>{t('tool-workspace.step_help.'+step)}</small></div>{index===3?<ShieldCheck size={16} aria-hidden="true"/>:<ArrowRight size={14} aria-hidden="true"/>}</li>)}
    </ol>
      </details>
    </div>
  </section>;
}
