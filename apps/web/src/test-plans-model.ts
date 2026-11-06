import type {QualityTestCase,TaskContextRecord,TestPlanView,TestPlanRevision,TestPlanExecution,TestPlanReport,TestPlanReportRef,TestCaseResult,IssueRepairCaseOrigin} from '../../../packages/contracts/src';
import type {WriteRecoveryRequest} from './write-action';

export const planStates=['draft','published','retired'] as const;
export type TestPlanState=(typeof planStates)[number];
export type TestPlanCatalogFilter={state?:TestPlanState;taskID?:string;invalid?:boolean};
export function isTestPlanState(value:unknown):value is TestPlanState{
 return typeof value==='string'&&(planStates as readonly string[]).includes(value);
}
export function testPlanCatalogPath(project:string,filter:TestPlanCatalogFilter,limit=20){
 const query=new URLSearchParams({project_id:project});
 if(filter.state)query.set('state',filter.state);
 if(filter.taskID)query.set('task_id',filter.taskID);
 query.set('limit',String(limit));
 return testPlanPath()+'?'+query;
}
export const planEligibility=['none','current','source_changed','retired'] as const;
export const executionStates=['pending','passed','failed','incomplete','source_changed','retired','run_changed','unknown'] as const;
export const caseStatuses=['not_run','passed','failed','error','skipped','unknown','ambiguous'] as const;
export const packageEvidenceStates=['pending','verified','unverified','failed','unknown','skipped','cancelled'] as const;
export const reportEligibility=['current','invalid','unknown','source_changed','stale_attempt'] as const;
export const testPlanPath=(id='')=>'/test-plans'+(id?'/'+encodeURIComponent(id):'');
export const planID=(value:unknown):value is string=>typeof value==='string'&&/^[A-Za-z0-9_-]{1,64}$/.test(value);
export const planHash=(value:unknown):value is string=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const text=(value:unknown):value is string=>typeof value==='string';
const integer=(value:unknown):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>=0;
const positive=(value:unknown):value is number=>integer(value)&&value>0;
const oneOf=(value:unknown,values:readonly string[])=>typeof value==='string'&&values.includes(value);
const strings=(value:unknown):value is string[]=>Array.isArray(value)&&value.every(text);
const indices=(value:unknown):value is number[]=>Array.isArray(value)&&value.every(value=>positive(value));
const bytes=(value:string)=>new TextEncoder().encode(value).length;
export const planText=(value:unknown,max:number,empty=false):value is string=>text(value)&&(empty||!!value.trim())&&bytes(value)<=max;
const unique=(values:unknown[])=>new Set(values).size===values.length;
const knownKeys=(value:Record<string,unknown>,keys:readonly string[])=>Object.keys(value).every(key=>keys.includes(key));

export const originalTestCasePanelPrefix='test-results:';
export type OriginalTestCaseLocator={revision:number;sha256:string;caseKey:string;issueID:string};
const testRunID=(value:unknown):value is string=>text(value)&&/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,100}$/.test(value);
export function validIssueRepairCaseOrigin(value:unknown):value is IssueRepairCaseOrigin{
 return object(value)&&knownKeys(value,['plan_id','run_id','case_key','plan_revision','plan_sha256'])&&
  planID(value.plan_id)&&testRunID(value.run_id)&&planID(value.case_key)&&positive(value.plan_revision)&&planHash(value.plan_sha256);
}
export function parseOriginalTestCasePanel(panel:string):OriginalTestCaseLocator|undefined{
 if(!panel.startsWith(originalTestCasePanelPrefix))return;
 const parts=panel.slice(originalTestCasePanelPrefix.length).split(':');
 const [version,sha256,caseKey,issueID]=parts,revision=Number(version);
 if(parts.length===4&&String(revision)===version&&positive(revision)&&planHash(sha256)&&planID(caseKey)&&planID(issueID))return {revision,sha256,caseKey,issueID};
}
export function validOriginalTestCaseRoute(params:URLSearchParams):boolean{
 return ['project','view','sub','record','run','panel'].every(key=>params.getAll(key).length===1)&&
  params.get('view')==='quality'&&params.get('sub')==='tests'&&!params.has('node')&&!params.has('attempt')&&
  planID(params.get('project'))&&planID(params.get('record'))&&testRunID(params.get('run'))&&
  !!parseOriginalTestCasePanel(params.get('panel')||'');
}
export function originalTestCaseHref(project:string,issue:string,origin:IssueRepairCaseOrigin):string{
 const panel=originalTestCasePanelPrefix+[origin.plan_revision,origin.plan_sha256,origin.case_key,issue].join(':');
 return '?'+new URLSearchParams({project,view:'quality',sub:'tests',record:origin.plan_id,run:origin.run_id,panel});
}
export function originalFailedCase(execution:TestPlanExecution,locator:OriginalTestCaseLocator):TestCaseResult|undefined{
 if(execution.revision!==locator.revision||execution.plan_sha256!==locator.sha256)return;
 return execution.cases.find(item=>item.case_key===locator.caseKey&&item.issue_id===locator.issueID&&
  (item.status==='failed'||item.status==='error')&&item.evidence.length>0);
}

export function newQualityCase():QualityTestCase{return {key:'case-'+crypto.randomUUID().slice(0,12),title:'',description:'',preconditions:'',steps:[],expected_result:'',priority:'medium',covers:[],selector:{class_name:'',name:''}};}
export function taskCriteria(value:unknown):string[]{return Array.isArray(value)?value.filter(text):typeof value==='string'?value.split('\n').filter(value=>value.trim()):[];}
export function validTaskContext(value:unknown):value is TaskContextRecord{return object(value)&&planID(value.id)&&positive(value.version)&&text(value.title)&&text(value.description)&&strings(value.acceptance_criteria);}
export function validQualityCase(value:unknown):value is QualityTestCase{return object(value)&&knownKeys(value,['key','title','description','preconditions','steps','expected_result','priority','covers','selector'])&&planID(value.key)&&['title','description','preconditions','expected_result'].every(key=>text(value[key]))&&strings(value.steps)&&oneOf(value.priority,['urgent','high','medium','low'])&&indices(value.covers)&&object(value.selector)&&knownKeys(value.selector,['class_name','name'])&&text(value.selector.class_name)&&text(value.selector.name);}

/** Draft validation never invents a publish decision. Incomplete intended
 * checks remain editable; publication uses current server-side authority. */
export function planDraftErrors(title:string,description:string,cases:QualityTestCase[],criteria:string[]):string[]{
 const out:string[]=[];
 if(!planText(title,200))out.push('title_required');
 if(bytes(description)>4000||bytes(JSON.stringify(cases))>128*1024)out.push('text_limit');
 if(cases.length>100)out.push('case_limit');
 const keys=new Set<string>(),selectors=new Set<string>();
 for(const item of cases){
  if(!validQualityCase(item)||keys.has(item.key)){out.push('invalid_key');continue;}keys.add(item.key);
  if(bytes(item.title)>200||[item.description,item.preconditions,item.expected_result].some(value=>bytes(value)>4000)||item.steps.length>20||item.steps.some(value=>bytes(value)>1000)||bytes(item.selector.class_name)>500||bytes(item.selector.name)>500)out.push('text_limit');
  if(item.covers.length>100||!unique(item.covers)||item.covers.some(index=>index>criteria.length))out.push('covers_invalid');
  if(item.selector.name){const selector=JSON.stringify([item.selector.class_name,item.selector.name]);if(selectors.has(selector))out.push('selector_duplicate');selectors.add(selector);}
 }
 return [...new Set(out)];
}
function revision(value:unknown,task:string):value is TestPlanRevision{
 return object(value)&&positive(value.revision)&&oneOf(value.state,planStates)&&text(value.title)&&text(value.description)&&planHash(value.sha256)&&text(value.created_by)&&text(value.created_at)&&object(value.source)&&validTaskContext(value.source.task)&&value.source.task.id===task&&planHash(value.source.task_basis_sha256)&&planHash(value.source.sha256)&&(value.source.requirement===undefined||value.source.requirement===null||validTaskContext(value.source.requirement))&&Array.isArray(value.cases)&&value.cases.length<=100&&value.cases.every(validQualityCase)&&unique(value.cases.map(item=>item.key));
}
export function validTestPlan(value:unknown,project:string):value is TestPlanView{
 if(!object(value)||value.project_id!==project||!planID(value.id)||!planID(value.task_id)||!positive(value.version)||!oneOf(value.state,planStates)||!positive(value.latest_revision)||!integer(value.published_revision)||value.published_revision>value.latest_revision||typeof value.has_draft!=='boolean'||typeof value.basis_current!=='boolean'||!oneOf(value.eligibility,planEligibility)||!strings(value.allowed_actions)||value.allowed_actions.some(action=>!['save','publish','retire'].includes(action))||!text(value.created_at)||!text(value.updated_at))return false;
 if(!revision(value.latest,value.task_id)||value.latest.revision!==value.latest_revision)return false;
 if(value.published_revision>0&&(!revision(value.published,value.task_id)||value.published.revision!==value.published_revision))return false;
 return true;
}
export function validPlanPage<T>(value:unknown,item:(value:unknown)=>value is T):value is {items:T[];total:number;next_cursor:string}{return object(value)&&integer(value.total)&&Array.isArray(value.items)&&value.items.length<=value.total&&value.items.every(item)&&typeof value.next_cursor==='string';}
function reportRef(value:unknown):value is TestPlanReportRef{
 if(!object(value)||!planID(value.id)||!text(value.run_id)||!text(value.node_id)||!positive(value.attempt)||!positive(value.fence)||!text(value.worker_id)||!text(value.source_commit)||!text(value.protocol)||!oneOf(value.eligibility,reportEligibility)||!Number.isSafeInteger(value.process_exit_code)||typeof value.outcome_unknown!=='boolean'||!text(value.reported_at)||!['total','passed','failed','errors','skipped','unmapped'].every(key=>integer(value[key])))return false;
 const known=value.eligibility==='current'||value.eligibility==='source_changed';
 return known?planHash(value.sha256)&&value.protocol==='suma-junit-report-v1':value.sha256===''||planHash(value.sha256);
}
function caseResult(value:unknown):value is TestCaseResult{
 if(!object(value)||!planID(value.case_key)||!text(value.title)||!oneOf(value.status,caseStatuses)||!indices(value.covers)||!text(value.issue_id)||!Array.isArray(value.evidence))return false;
 return value.evidence.every(item=>object(item)&&planID(item.report_id)&&text(item.node_id)&&positive(item.attempt)&&positive(item.fence)&&text(item.worker_id)&&text(item.source_commit)&&oneOf(item.status,caseStatuses)&&text(item.message)&&integer(item.duration_ms));
}
const dagNodeID=(value:unknown):value is string=>text(value)&&/^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$/.test(value);
const sourceCommit=(value:unknown):value is string=>text(value)&&/^(?:[a-f0-9]{40}|[a-f0-9]{64})$/.test(value);
function packageEvidence(value:unknown,basisCurrent:boolean):value is TestPlanExecution['package_evidence'][number]{
 if(!object(value)||!dagNodeID(value.node_id)||!dagNodeID(value.source_node_id)||value.node_id===value.source_node_id||!oneOf(value.state,packageEvidenceStates)||typeof value.can_create_issue!=='boolean')return false;
 if(value.attempt!==undefined&&!positive(value.attempt))return false;
 if(value.fence!==undefined&&(!positive(value.fence)||!positive(value.attempt)))return false;
 if(value.issue_id!==undefined&&(!planID(value.issue_id)||value.can_create_issue))return false;
 if(value.can_create_issue&&(!basisCurrent||value.state!=='failed'||!positive(value.attempt)||!positive(value.fence)))return false;
 if(![value.worker_id,value.source_worker_id].every(worker=>worker===undefined||planText(worker,200)))return false;
 if(value.state==='verified')return basisCurrent&&positive(value.attempt)&&planID(value.artifact_id)&&planHash(value.manifest_sha256)&&sourceCommit(value.source_commit);
 return [value.artifact_id,value.manifest_sha256,value.source_commit,value.source_worker_id].every(fact=>fact===undefined);
}
export function validTestPlanExecution(value:unknown,plan:string,task?:string):value is TestPlanExecution{
 if(!object(value)||value.plan_id!==plan||!planID(value.task_id)||task!==undefined&&value.task_id!==task||!integer(value.revision)||!(value.revision===0?value.plan_sha256==='':planHash(value.plan_sha256))||!text(value.run_id)||!text(value.run_state)||!text(value.created_at)||!oneOf(value.state,executionStates)||typeof value.basis_current!=='boolean'||!strings(value.source_commits)||!['total','passed','failed','errors','skipped','not_run'].every(key=>integer(value[key]))||!Array.isArray(value.cases)||value.cases.length>100||value.cases.length!==value.total||!value.cases.every(caseResult)||!unique(value.cases.map(item=>item.case_key))||!Array.isArray(value.reports)||!value.reports.every(reportRef)||!unique(value.reports.map(item=>item.id)))return false;
 if(!['passed','failed','errors','skipped','not_run'].every(key=>Number(value[key])<=Number(value.total)))return false;
 const packages=value.package_evidence;
 const basisCurrent=value.basis_current;
 if(!Array.isArray(packages)||packages.length>1000||!packages.every(item=>packageEvidence(item,basisCurrent))||!unique(packages.map(item=>item.node_id)))return false;
 if(value.revision===0)return value.total===0&&value.reports.length===0&&packages.length===0&&value.state!=='passed';
 if(value.state==='passed'&&(value.total===0||value.passed!==value.total||!value.basis_current||value.run_state!=='succeeded'||!value.reports.some(item=>item.eligibility==='current')||packages.some(item=>!['verified','skipped'].includes(item.state))))return false;
 const reports=value.reports as TestPlanReportRef[],cases=value.cases as TestCaseResult[];
 return reports.every(item=>item.run_id===value.run_id)&&cases.every(item=>item.evidence.every(evidence=>reports.some(report=>report.id===evidence.report_id&&report.node_id===evidence.node_id&&report.attempt===evidence.attempt&&report.fence===evidence.fence&&report.worker_id===evidence.worker_id&&report.source_commit===evidence.source_commit)));
}
export function validTestPlanReport(value:unknown,run:string,id:string):value is TestPlanReport{return reportRef(value)&&value.id===id&&value.run_id===run&&object(value)&&planText(value.xml,256*1024,true)&&Array.isArray(value.cases);}

/** Recover exact domain commands, never arbitrary API paths, credentials or
 * caller-supplied passing execution facts. The server checks current authority. */
export function testPlanRecovery(request:WriteRecoveryRequest,project:string){
 const body=request.body;if(!object(body)||body.project_id!==project||typeof body.idempotency_key!=='string'||body.idempotency_key!==request.idempotency_key)return false;
 const path=request.path,base='/test-plans',identifier='[A-Za-z0-9_-]{1,64}';
 if(request.method==='POST'&&path===base)return knownKeys(body,['project_id','task_id','expected_task_version','expected_requirement_version','title','description','cases','idempotency_key'])&&planID(body.task_id)&&positive(body.expected_task_version)&&integer(body.expected_requirement_version)&&text(body.title)&&text(body.description)&&Array.isArray(body.cases)&&body.cases.every(validQualityCase);
 if(request.method==='PUT'&&new RegExp('^'+base+'/'+identifier+'$').test(path))return knownKeys(body,['project_id','expected_version','expected_task_version','expected_requirement_version','title','description','cases','idempotency_key'])&&positive(body.expected_version)&&positive(body.expected_task_version)&&integer(body.expected_requirement_version)&&text(body.title)&&text(body.description)&&Array.isArray(body.cases)&&body.cases.every(validQualityCase);
 if(request.method==='POST'&&new RegExp('^'+base+'/'+identifier+'/commands$').test(path))return knownKeys(body,['project_id','expected_version','action','reason','idempotency_key'])&&positive(body.expected_version)&&oneOf(body.action,['publish','retire'])&&planText(body.reason,2000);
 if(request.method==='POST'&&new RegExp('^'+base+'/'+identifier+'/executions/'+identifier+'/defects$').test(path))return knownKeys(body,['project_id','expected_version','case_key','reason','idempotency_key'])&&positive(body.expected_version)&&planID(body.case_key)&&planText(body.reason,2000);
 if(request.method==='POST'&&new RegExp('^'+base+'/'+identifier+'/executions/'+identifier+'/package-defects$').test(path))return knownKeys(body,['project_id','expected_version','node_id','expected_attempt','expected_fence','reason','idempotency_key'])&&positive(body.expected_version)&&dagNodeID(body.node_id)&&positive(body.expected_attempt)&&positive(body.expected_fence)&&planText(body.reason,2000);
 return false;
}
