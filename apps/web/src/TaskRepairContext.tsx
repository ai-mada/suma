import {useId} from 'react';
import {ArrowUpRight} from 'lucide-react';
import {tr,useLocale,formatNumber} from './i18n';
import {taskDeliveryHref} from './task-delivery-navigation-model';
import {automationTriggerPanelPrefix} from './navigation';
import {taskDeliveryRepair} from './task-delivery-model';
import {canLeave,DeliveryReference} from './TaskDeliveryReference';
import './TaskDeliveryPanel.css';
const t=(key:string)=>tr('task-delivery.'+key);

export function TaskRepairContext({project,repair:source,locked=false,onOpenIssue,onOpenRun}:{
 project:string;repair:unknown;locked?:boolean;
 onOpenIssue?:(id:string)=>void;onOpenRun?:(target:{run:string})=>void;
}){
 useLocale();
 const heading=useId(),repair=taskDeliveryRepair(source);
 const issueHref=repair&&taskDeliveryHref(project,{kind:'issue',id:repair.issueID});
 if(!repair||!issueHref)return null;
 const {automation,regression}=repair;
 const triggerHref=(rule:string,trigger:string)=>'?'+new URLSearchParams({project,view:'development',sub:'automations',record:rule,panel:automationTriggerPanelPrefix+trigger});
 return <section className="task-delivery-section task-delivery-repair" data-task-delivery-repair aria-labelledby={heading}>
  <h4 id={heading}>{t('repair.title')}</h4>
  <p className="task-delivery-muted">{tr('issue-repair.cycle',{cycle:formatNumber(repair.cycle)})} · {t('repair.state.'+repair.state)}</p>
  <DeliveryReference kind="repair-issue" id={repair.issueID} href={issueHref} locked={locked} canLeave={canLeave}
   onNavigate={onOpenIssue?()=>onOpenIssue(repair.issueID):undefined}>{t('repair.open_issue')}<ArrowUpRight size={13} aria-hidden="true"/></DeliveryReference>
  {automation&&<div>
   <p className="task-delivery-muted"><strong>{automation.ruleName}</strong> · {tr('automation.run_origin.context',{
    source:tr(automation.source==='cron'?'automation.scheduled_trigger':automation.source==='manual'?'automation.manual_trigger':'automation.signed_event_trigger'),
    revision:formatNumber(automation.revision),
   })}</p>
   <div className="task-delivery-repair-links">
    <DeliveryReference kind="repair-run" id={automation.runID} href={'?'+new URLSearchParams({project,view:'development',sub:'runs',run:automation.runID})} locked={locked} canLeave={canLeave}
     onNavigate={onOpenRun?()=>onOpenRun({run:automation.runID}):undefined}>{tr('failure-diagnosis.open_source_run')}<ArrowUpRight size={13} aria-hidden="true"/></DeliveryReference>
    <DeliveryReference kind="repair-trigger" id={automation.triggerID} href={triggerHref(automation.ruleID,automation.triggerID)} locked={locked} canLeave={canLeave}>{tr('automation.run_origin.open')}<ArrowUpRight size={13} aria-hidden="true"/></DeliveryReference>
    {automation.pauseID&&<DeliveryReference kind="repair-pause" id={automation.pauseID} href={triggerHref(automation.ruleID,automation.pauseID)} locked={locked} canLeave={canLeave}>{tr('automation.run_origin.pause_open')}<ArrowUpRight size={13} aria-hidden="true"/></DeliveryReference>}
   </div>
  </div>}
  {regression&&<div>
   <p>{tr('issue-repair.bound_revision',{revision:formatNumber(regression.revision),count:formatNumber(regression.count)})}</p>
   <p className="task-delivery-muted">{t('repair.regression_scope')}</p>
   <DeliveryReference kind="repair-plan" id={regression.planID} href={'?'+new URLSearchParams({project,view:'quality',sub:'tests',record:regression.planID})} locked={locked} canLeave={canLeave}>{tr('test-report-setup.open_plan')}<ArrowUpRight size={13} aria-hidden="true"/></DeliveryReference>
  </div>}
 </section>;
}
