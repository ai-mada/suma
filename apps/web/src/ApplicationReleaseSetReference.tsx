import {ArrowUpRight} from 'lucide-react';
import {tr,useLocale} from './i18n';
import {applicationHash,applicationHref,applicationID,applicationObject} from './project-applications-model';
import {releaseSetHref} from './application-release-sets-model';
import './ApplicationReleaseSets.css';

/** A signed plan's reverse locator is a reference, never a new group-level
 * permission. Unknown/invalid data cannot become a cross-domain destination. */
export function ApplicationReleaseSetReference({project,reference}:{project:string;reference:unknown}){
 useLocale();if(!applicationObject(reference)||!applicationID(reference.release_set_id)||!applicationHash(reference.release_set_sha256)||!applicationID(reference.application_id)||!applicationID(reference.application_version_id)||!applicationHash(reference.application_version_sha256))return null;
 const guard=(event:React.MouseEvent<HTMLAnchorElement>)=>{if(event.button===0&&!event.ctrlKey&&!event.metaKey&&!event.shiftKey&&!event.altKey&&!window.dispatchEvent(new Event('suma:navigate',{cancelable:true})))event.preventDefault();};
 return <section className="rs-original-link"><strong>{tr('application-release-sets.original_set')}</strong><div className="rs-links"><a href={releaseSetHref(project,reference.release_set_id)} onClick={guard}>{tr('application-release-sets.open_set')}<ArrowUpRight size={12}/></a><a href={applicationHref(project,reference.application_id,reference.application_version_id)} onClick={guard}>{tr('application-release-sets.open_application')}<ArrowUpRight size={12}/></a></div><small>{tr('application-release-sets.plan_reference_boundary')}</small><details><summary>{tr('application-release-sets.source_details')}</summary><code>{reference.release_set_sha256}</code><br/><code>{reference.application_version_sha256}</code></details></section>;
}
