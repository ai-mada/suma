import type {ReleaseWorker} from '../../../packages/contracts/src';

export type ToolWorkerIdentitySource=Readonly<Pick<ReleaseWorker,'worker_id'|'status'|'expires_at'>>;
export type ToolWorkerIdentityState='current'|'expired'|'revoked'|'unknown';
export type ToolWorkerIdentityFilter='current'|'attention'|'revoked'|'all';
export type ToolWorkerIdentityCounts=Record<ToolWorkerIdentityState|'attention'|'all',number>;

function expiryTime(value:string):number{
 // Check the calendar before Date.parse can normalize an impossible date.
 const match=typeof value==='string'&&value.trim()===value?/^(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d)(?:\.(\d{1,9}))?(?:Z|[+-]\d\d:\d\d)$/.exec(value):null;
 if(!match)return Number.NaN;
 const canonical=match[1]+'.'+(match[2]??'').padEnd(3,'0').slice(0,3)+'Z',wall=Date.parse(canonical);
 if(!Number.isFinite(wall)||new Date(wall).toISOString()!==canonical)return Number.NaN;
 return Date.parse(value);
}

/** Credential presentation only: current does not mean online, authorized for
 * an action, or available for dispatch. The caller supplies one clock snapshot. */
export function classifyToolWorkerIdentity(worker:ToolWorkerIdentitySource,now:number):ToolWorkerIdentityState{
 if(worker.status==='revoked')return 'revoked';
 if(worker.status!=='active'||!Number.isFinite(now))return 'unknown';
 const expiry=expiryTime(worker.expires_at);
 if(!Number.isFinite(expiry))return 'unknown';
 return expiry<=now?'expired':'current';
}

/** Counts cover query-matched records already read, before the category filter;
 * readCount is the input size, never a server total. Keep objects and order. */
export function projectToolWorkerIdentities<T extends ToolWorkerIdentitySource>(
 records:readonly T[],now:number,filter:ToolWorkerIdentityFilter='current',query='',
):{items:T[];counts:ToolWorkerIdentityCounts;readCount:number}{
 const items:T[]=[],counts:ToolWorkerIdentityCounts={current:0,expired:0,revoked:0,unknown:0,attention:0,all:0};
 const needle=query.trim().toLowerCase();
 for(const worker of records){
  if(!worker.worker_id.toLowerCase().includes(needle))continue;
  const state=classifyToolWorkerIdentity(worker,now),attention=state==='expired'||state==='unknown';
  counts[state]++;counts.all++;if(attention)counts.attention++;
  if(filter==='all'||filter==='attention'&&attention||filter===state)items.push(worker);
 }
 return {items,counts,readCount:records.length};
}
