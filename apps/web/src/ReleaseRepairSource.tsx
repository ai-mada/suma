import {ArrowUpRight} from 'lucide-react';
import {formatNumber,tr,useLocale} from './i18n';
import {repairReleaseIssueHref} from './repair-release-handoff';
import type {ReleaseDeployment,ReleasePlan} from './release-model';

const identifier=(value:unknown)=>typeof value==='string'&&/^[A-Za-z0-9_-]{1,64}$/.test(value);

/** Selected, project-authorized details supply provenance. The route can
 * locate a detail but cannot establish an issue/cycle relationship. */
export function ReleaseRepairSource({project,selected,detail}:{project:string;selected:string;detail:ReleasePlan|ReleaseDeployment}){
 useLocale();
 const deployment='operation_id' in detail;
 const source=detail.repair_source;
 if(detail.project_id!==project||(deployment?detail.operation_id:detail.id)!==selected||!source||
    !identifier(source.issue_id)||!identifier(source.candidate_id)||!Number.isSafeInteger(source.cycle)||source.cycle<1||source.cycle>2147483646||
    !deployment&&source.candidate_id!==detail.content.candidate_id)return null;
 const context=deployment?'history':detail.content.action==='rollback'?'rollback':detail.execution_operation_id?'history':'forward';
 const href=repairReleaseIssueHref(project,source.issue_id,source.cycle);
 return <div className="release-task-basis" data-release-repair-source>
  <strong>{tr('release.repair_source.title')}</strong>
  <a href={href} onClick={event=>{if(!window.dispatchEvent(new Event('suma:navigate',{cancelable:true})))event.preventDefault();}}>
   {tr('release.repair_source.open_cycle',{cycle:formatNumber(source.cycle)})}<ArrowUpRight size={13} aria-hidden="true"/>
  </a>
  <small>{tr('release.repair_source.'+context)}</small>
 </div>;
}
