import {Building2,BrainCircuit,FolderKanban,ReceiptText,Users,Wrench} from 'lucide-react';
import {tr as t,useLocale} from './i18n';
import {managementArea,managementAreas} from './management-navigation';
import './management.css';
const icons={projects:FolderKanban,teams:Users,models:BrainCircuit,tools:Wrench,billing:ReceiptText,organization:Building2};
export function ManagementNavigation({active,sub,onNavigate}:{active:boolean;sub:string;onNavigate:(sub:string)=>void}){
 useLocale();const current=managementArea(sub);
 return <><div className="sidebar-label">{t('management.title')}</div><nav className="management-nav" aria-label={t('management.title')}>{managementAreas.map(area=>{const Icon=icons[area.id],selected=active&&current.id===area.id;return <button key={area.id} className={selected?'active':''} aria-label={t('management.area.'+area.id)} title={t('management.area.'+area.id)} aria-current={selected?'page':undefined} onClick={()=>onNavigate(area.start)}><Icon size={18}/><span>{t('management.area.'+area.id)}</span>{selected&&<span className="active-dot"/>}</button>;})}</nav></>;
}
export {ProjectDirectory} from './ProjectPortfolio';
