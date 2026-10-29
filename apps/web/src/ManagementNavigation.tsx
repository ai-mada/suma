import {Building2,BrainCircuit,FolderKanban,ReceiptText,Users,Wrench,ArrowRight,Plus} from 'lucide-react';
import {formatNumber,tr as t,useLocale} from './i18n';
import {label,type Session} from './api';
import {managementArea,managementAreas} from './management-navigation';
import './management.css';
const icons={projects:FolderKanban,teams:Users,models:BrainCircuit,tools:Wrench,billing:ReceiptText,organization:Building2};
export function ManagementNavigation({active,sub,onNavigate}:{active:boolean;sub:string;onNavigate:(sub:string)=>void}){
 useLocale();const current=managementArea(sub);
 return <><div className="sidebar-label">{t('management.title')}</div><nav className="management-nav" aria-label={t('management.title')}>{managementAreas.map(area=>{const Icon=icons[area.id],selected=active&&current.id===area.id;return <button key={area.id} className={selected?'active':''} aria-current={selected?'page':undefined} onClick={()=>onNavigate(area.start)}><Icon size={18}/><span>{t('management.area.'+area.id)}</span>{selected&&<span className="active-dot"/>}</button>;})}</nav></>;
}
export function ProjectDirectory({session,onOpen,onCreate}:{session:Session;onOpen:(id:string)=>void;onCreate:()=>void}){
 useLocale();return <section className="management-directory" data-project-directory>
  <div className="management-heading"><div><h2>{t('management.project_directory')}</h2><p>{t('management.project_count',{count:formatNumber(session.projects.length)})}</p></div>{['owner','admin'].includes(session.user.role||'')&&<button className="primary" onClick={onCreate}><Plus size={15}/>{t('shell.create_project')}</button>}</div>
  <div className="management-projects">{session.projects.map(project=><button className="panel" key={project.id} onClick={()=>onOpen(project.id)}><span className="management-project-icon"><FolderKanban size={22}/></span><span><strong>{label(project)}</strong><small>{project.description||t('management.project_scope')}</small><code>{project.id}</code></span><ArrowRight size={17}/></button>)}</div>
 </section>;
}
