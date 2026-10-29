import type {OverviewAttentionItem,OverviewAttentionPage,OverviewDelivery,OverviewMilestones} from '../../../../packages/contracts/src';
export const attentionKinds=['review','blocked','overdue'] as const;
export type AttentionKind=typeof attentionKinds[number];
export type AttentionItem=OverviewAttentionItem;
export type AttentionPage=OverviewAttentionPage;
export type OverviewActivity={id?:string;title?:string;message?:string;action?:string;type?:string;actor_name?:string;actor?:string;at?:string;created_at?:string;kind?:string};
export type DeliverySummary=Partial<OverviewDelivery>;
export type OverviewData={
 project?:{id?:string;title?:string;name?:string;description?:string};
 counts?:Record<string,number|{total?:number}>;
 delivery?:DeliverySummary;
 attention?:Partial<Record<AttentionKind|'reconciling',AttentionPage>>;
 milestones?:OverviewMilestones;
 activity?:OverviewActivity[];
 as_of?:string;
};
export function overviewCount(value:unknown):number|undefined{return typeof value==='number'&&Number.isSafeInteger(value)&&value>=0?value:undefined;}
export function acceptancePercent(delivery:DeliverySummary|undefined):number|undefined{
 const scope=overviewCount(delivery?.scope_tasks),accepted=overviewCount(delivery?.accepted_tasks);
 return delivery?.scope==='whole_project'&&scope!==undefined&&scope>0&&accepted!==undefined&&accepted<=scope?accepted/scope*100:undefined;
}
export function validAttentionPage(value:unknown,kind:AttentionKind):value is AttentionPage{
 if(!value||typeof value!=='object')return false;
 const page=value as AttentionPage;
 return page.kind===kind&&overviewCount(page.total)!==undefined&&Array.isArray(page.items)&&page.items.length<=50&&page.items.length<=page.total&&typeof page.next_cursor==='string'&&typeof page.as_of==='string'&&Number.isFinite(Date.parse(page.as_of))&&page.date_basis==='UTC'&&page.items.every(item=>item&&typeof item==='object'&&['id','title','status','owner_id','owner_name','run_id','run_state','updated_at','due_date'].every(key=>typeof item[key as keyof AttentionItem]==='string')&&!!item.id);
}
export function emptyOverview(data:OverviewData):boolean{
 return data.delivery?.scope==='whole_project'&&data.delivery.total_tasks===0&&['requirements','tasks','issues'].every(key=>{const count=data.counts?.[key];return (typeof count==='number'?count:count?.total)===0;});
}
export function activityPresentation(item:OverviewActivity):'approved'|'created'|'updated'|'review'|'conversation'|'execution'|'warning'|'neutral'{
 const kind=item.kind||item.type||'';
 if(kind==='task_conversation.posted')return 'conversation';
 if(kind.endsWith('.approved')||kind==='task.accepted')return 'approved';
 if(kind==='task.reviewed'||kind.endsWith('.reviewed'))return 'review';
 if(/(?:\.|_)created$/.test(kind))return 'created';
 if(/(?:\.|_)updated$/.test(kind))return 'updated';
 if(/(?:\.|_)(?:started|dispatched)$/.test(kind))return 'execution';
 if(/(?:\.|_)(?:failed|rejected|blocked|acceptance_invalidated)$/.test(kind))return 'warning';
 return 'neutral';
}
