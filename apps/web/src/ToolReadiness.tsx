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

export function ToolReadiness({items,total,uncertain,identitiesKnown,identityCount,disabled,onWorker,onRegister,onGuide,onSelect}:{
  items?:ToolProfile[];total:number;uncertain:boolean;identitiesKnown:boolean;identityCount:number;disabled:boolean;
  onWorker:()=>void;onRegister:()=>void;onGuide:()=>void;onSelect:(id:string)=>void;
}) {
  useLocale();
  const ready=items?.filter(item=>matchesToolFilter(item,'available')).length??0;
  const paused=items?.find(item=>item.state==='paused');
  const blocked=items?.find(item=>matchesToolFilter(item,'attention'));
  const next=uncertain||!items?'refresh':!items.length?identitiesKnown&&identityCount===0?'identity':'register':blocked?'attention':paused?'review':ready?'ready':'register';
  const action=next==='identity'?onWorker:next==='review'?()=>onSelect(paused!.id):next==='attention'?()=>onSelect(blocked!.id):onRegister;
  return <section className="tool-readiness" aria-label={t('tool-workspace.readiness')}>
    <div className="tool-readiness-main">
      <span className="tool-readiness-icon"><Sparkles size={20}/></span>
      <div><h3>{t('tool-workspace.next_step')}</h3><strong>{t('tool-workspace.next.'+next)}</strong><p>{t('tool-workspace.help.'+next)}</p></div>
      {!['refresh','ready'].includes(next)&&<button className="primary" disabled={disabled} onClick={action}>{t('tool-workspace.action.'+next)}<ArrowRight size={14}/></button>}
      {next==='ready'&&<span className="tool-status-pill available"><Check size={14}/>{t('tool-workspace.eligible')}</span>}
    </div>
    <div className="tool-readiness-foot">
      <span><Server size={14}/>{identitiesKnown?t('tool-workspace.identities',{count:formatNumber(identityCount)}):t('tool-workspace.identity_unknown')}</span>
      <span><Radio size={14}/>{uncertain?t('tool-workspace.unconfirmed'):t('tool-workspace.loaded_scope',{count:formatNumber(items?.length??0),total:formatNumber(total)})}</span>
      <button className="text-button" onClick={onGuide}><CircleHelp size={14}/>{t('tool-workspace.setup_guide')}</button>
    </div>
    <details key={items?.length?'configured':'empty'} className="tool-journey-disclosure" open={!items?.length}>
    <summary>{t('tool-workspace.setup_journey')}</summary>
    <ol className="tool-setup-journey" aria-label={t('tool-workspace.setup_journey')}>
      {['identity','probe','register','review'].map((step,index)=><li key={step}><span>{formatNumber(index+1)}</span><div><strong>{t('tool-workspace.step.'+step)}</strong><small>{t('tool-workspace.step_help.'+step)}</small></div>{index===3?<ShieldCheck size={16}/>:<ArrowRight size={14}/>}</li>)}
    </ol>
    </details>
  </section>;
}
