import {Folder} from 'lucide-react';
import {label,type RecordItem} from '../api';
import {formatNumber,tr,useLocale} from '../i18n';
export function ProjectSettings({project}:{project:RecordItem}){
 useLocale();const version=Number.isSafeInteger(project.version)&&project.version>0?'v'+formatNumber(project.version):tr('workbench.not_reported');
 return <section className="panel settings-panel"><div className="panel-heading"><div><h2>{tr('workbench.project.title')}</h2><p>{tr('workbench.project.description')}</p></div><Folder size={20}/></div><dl><dt>{tr('workbench.project.name')}</dt><dd>{label(project)}</dd><dt>{tr('workbench.project.id')}</dt><dd><code>{project.id}</code></dd><dt>{tr('workbench.project.summary')}</dt><dd>{project.description||tr('workbench.project.not_set')}</dd><dt>{tr('workbench.project.version')}</dt><dd>{version}</dd></dl><p className="muted">{tr('workbench.project.scope')}</p></section>;
}
