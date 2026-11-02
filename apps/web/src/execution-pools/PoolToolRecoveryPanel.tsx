import {useCallback,useEffect,useRef,useState} from 'react';
import {RefreshCw} from 'lucide-react';
import type {ExecutionPool,ExecutionPoolRunner,PoolRunnerReadiness} from '../../../../packages/contracts/src';
import {ApiError} from '../api';
import {useApi,useApiView} from '../api-context';
import {formatDateTime,tr,useLocale} from '../i18n';
import {toolDiscoveryTimestampNs} from '../tool-discovery-model';
import {poolRunnerReadinessPath,readPoolRunnerReadiness} from './pool-config-connection-model';
import {serializePoolToolRecoveryPin,validPoolToolRecoveryPin,type PoolToolRecoveryPin} from './pool-tool-recovery-model';
import {serializePoolToolSetupPin,validPoolToolSetupPin,type PoolToolSetupPin} from './pool-tool-setup-model';
import './PoolToolRecoveryPanel.css';

export type PoolToolTargetPin=PoolToolRecoveryPin|PoolToolSetupPin;
/** Two explicit navigation purposes share the original Pool read boundary.
 * A setup target never acquires an invented original tool profile. */
export function serializePoolToolTargetPin(pin:PoolToolTargetPin):string{return validPoolToolSetupPin(pin)?serializePoolToolSetupPin(pin):serializePoolToolRecoveryPin(pin);}
export type PoolToolRecovery={pin:PoolToolTargetPin;scope:PoolRunnerReadiness['scope']};
export type PoolToolRecoverySnapshot={runner:ExecutionPoolRunner;recovery:PoolToolRecovery;expiresAt:number};
type Props={pin:PoolToolTargetPin;pool:Pick<ExecutionPool,'id'|'owner_tenant_id'|'version'|'status'>;disabled?:boolean;refresh:number;onSnapshot:(value:PoolToolRecoverySnapshot|undefined)=>void;onPrepare:(value:PoolToolRecoverySnapshot)=>void;onManage?:(runner:ExecutionPoolRunner)=>void;onSourceInvalidated?:()=>void};
const t=(key:string,values?:Record<string,unknown>)=>tr('pool-tool-recovery.'+key,values);
const record=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
function readRunner(raw:unknown,pin:PoolToolTargetPin):ExecutionPoolRunner|undefined{
  if(!record(raw))return;
  const required=['id','pool_id','worker_id','name','version','status','max_slots','credential_version','credential_expires_at','allowed_actions','created_at'];
  if(!required.every(key=>Object.hasOwn(raw,key))||Object.keys(raw).some(key=>!required.includes(key)&&key!=='last_seen_at')||raw.id!==pin.runner_id||raw.pool_id!==pin.pool_id||raw.worker_id!==pin.worker_id||typeof raw.name!=='string'||typeof raw.status!=='string'||!['active','revoked'].includes(raw.status)||!Number.isSafeInteger(raw.version)||Number(raw.version)<1||!Number.isSafeInteger(raw.credential_version)||Number(raw.credential_version)<1||!Number.isSafeInteger(raw.max_slots)||Number(raw.max_slots)<1||Number(raw.max_slots)>128||!Array.isArray(raw.allowed_actions)||new Set(raw.allowed_actions).size!==raw.allowed_actions.length||raw.allowed_actions.some(value=>!['rotate_credential','revoke'].includes(value)))return;
  try{toolDiscoveryTimestampNs(raw.credential_expires_at);toolDiscoveryTimestampNs(raw.created_at);if(Object.hasOwn(raw,'last_seen_at'))toolDiscoveryTimestampNs(raw.last_seen_at);}catch{return;}
  return structuredClone(raw) as unknown as ExecutionPoolRunner;
}

/** The locator is only an intent. Both reads authenticate the current Pool
 * owner; neither reads project records or issues a command on its own. */
export function PoolToolRecoveryPanel(props:Props){
  const api=useApi(),view=useApiView(),key=JSON.stringify([view?.userID,view?.organizationID,props.pool.id,props.pool.owner_tenant_id,props.pool.version,props.pool.status,props.pin]);
  const owner=useRef({api,key,epoch:0});if(owner.current.api!==api||owner.current.key!==key)owner.current={api,key,epoch:owner.current.epoch+1};
  const epoch=owner.current.epoch,owns=()=>owner.current.api===api&&owner.current.key===key&&owner.current.epoch===epoch;
  const authorized=!!view?.userID&&view.organizationID===props.pool.owner_tenant_id&&props.pool.id===props.pin.pool_id&&(validPoolToolRecoveryPin(props.pin)||validPoolToolSetupPin(props.pin));
  return <RecoveryScope key={epoch} {...props} owns={owns} authorized={authorized}/>;
}
function RecoveryScope({pin,pool,disabled=false,refresh,onSnapshot,onPrepare,onManage,onSourceInvalidated,owns,authorized}:Props&{owns:()=>boolean;authorized:boolean}){
  useLocale();const api=useApi(),[phase,setPhase]=useState<'reading'|'read'|'restricted'|'unknown'|'expired'|'blocked'>('reading'),[value,setValue]=useState<{runner:ExecutionPoolRunner;readiness:PoolRunnerReadiness;expiresAt:number;prepareExpiresAt:number}>();
  const alive=useRef(true),intent=useRef(0),abort=useRef<AbortController|undefined>(undefined),latest=useRef({disabled,authorized,onSnapshot,onPrepare,onManage,onSourceInvalidated,owns});latest.current={disabled,authorized,onSnapshot,onPrepare,onManage,onSourceInvalidated,owns};
  const current=()=>alive.current&&latest.current.owns()&&!latest.current.disabled&&latest.current.authorized;
  const read=useCallback(async(prepare=false)=>{
    if(!current())return;
    const generation=++intent.current,controller=new AbortController();abort.current?.abort();abort.current=controller;
    const started=performance.now();setPhase('reading');setValue(undefined);latest.current.onSnapshot(undefined);
    const ownsRead=()=>current()&&!controller.signal.aborted&&intent.current===generation;
    try{
      const path=poolRunnerReadinessPath(pin.pool_id,pin.runner_id,pin.tenant_id,pin.project_id)!;
      const [rawRunner,rawReadiness]=await Promise.all([api<unknown>('/execution-pools/'+encodeURIComponent(pin.pool_id)+'/runners/'+encodeURIComponent(pin.runner_id),undefined,'GET',undefined,controller.signal),api<unknown>(path,undefined,'GET',undefined,controller.signal)]);
      if(!ownsRead())return;
      const runner=readRunner(rawRunner,pin),readiness=readPoolRunnerReadiness(rawReadiness,{poolID:pin.pool_id,runnerID:pin.runner_id,workerID:pin.worker_id,tenantID:pin.tenant_id,projectID:pin.project_id});
      if(!runner||!readiness||runner.version!==readiness.runner.version||runner.status!==readiness.runner.status||runner.credential_version!==readiness.runner.credential_version||runner.credential_expires_at!==readiness.runner.credential_expires_at)throw Error('Invalid exact Pool source');
      const elapsed=performance.now()-started,remaining=30000-elapsed;
      const prepareRemaining=Math.min(30000,Number((toolDiscoveryTimestampNs(readiness.runner.credential_expires_at)-toolDiscoveryTimestampNs(readiness.checked_at))/1000000n))-elapsed;
      const expiresAt=performance.now()+Math.max(0,remaining),prepareExpiresAt=performance.now()+Math.max(0,prepareRemaining),snapshot={runner,readiness,expiresAt,prepareExpiresAt};setValue(snapshot);setPhase(remaining>0?'read':'expired');
      const sourceAuthorityValid=pool.status==='active'&&readiness.pool_status==='active'&&runner.status==='active'&&readiness.runner.credential_state==='valid'&&readiness.scope.grant_status==='approved';
      if(!sourceAuthorityValid)latest.current.onSourceInvalidated?.();
      else if(prepareRemaining>0){const result={runner,recovery:{pin,scope:readiness.scope},expiresAt:prepareExpiresAt};latest.current.onSnapshot(result);if(prepare)latest.current.onPrepare(result);}
    }catch(error){if(ownsRead()){const denied=error instanceof ApiError&&[401,403,404].includes(error.status);setValue(undefined);setPhase(denied?'restricted':'unknown');latest.current.onSnapshot(undefined);if(denied)latest.current.onSourceInvalidated?.();}}
  },[api,pin,pool.status]);
  useEffect(()=>{alive.current=true;return()=>{alive.current=false;intent.current++;abort.current?.abort();};},[]);
  useEffect(()=>{if(disabled||!authorized){intent.current++;abort.current?.abort();setValue(undefined);setPhase(authorized?'blocked':'restricted');latest.current.onSnapshot(undefined);if(!authorized&&latest.current.owns())latest.current.onSourceInvalidated?.();}else void read();},[disabled,authorized,refresh,read]);
  useEffect(()=>{if(!value)return;const timer=setInterval(()=>{if(performance.now()>=value.prepareExpiresAt)latest.current.onSnapshot(undefined);if(performance.now()>=value.expiresAt){intent.current++;setPhase('expired');}},1000);return()=>clearInterval(timer);},[value]);
  const canPrepare=current()&&phase==='read'&&!!value&&performance.now()<value.prepareExpiresAt&&pool.status==='active'&&value.readiness.pool_status==='active'&&value.runner.status==='active'&&value.readiness.runner.credential_state==='valid'&&value.readiness.scope.grant_status==='approved';
  const canManage=current()&&value&&pool.status==='active'&&phase==='read'&&performance.now()<value.expiresAt&&onManage;
  const setup=validPoolToolSetupPin(pin);
  return <section className="pool-tool-recovery" data-pool-tool-recovery data-pool-tool-intent={setup?'setup':'recovery'} data-target={serializePoolToolTargetPin(pin)}>
    <header><div><h3>{setup?tr('shared-tool-onboarding-pool.title'):t('title')}</h3><p>{t('target',{worker:pin.worker_id,project:pin.project_id})}</p>{setup&&<p>{tr('shared-tool-onboarding-pool.tool',{tool:pin.adapter_id})}</p>}</div><button type="button" data-pool-recovery-refresh disabled={!current()||phase==='reading'} onClick={()=>void read()}><RefreshCw size={15} aria-hidden="true"/>{t('refresh')}</button></header>
    <p className="muted">{setup?tr('shared-tool-onboarding-pool.boundary'):t('boundary')}</p>
    <p role="status" data-pool-recovery-state={phase}>{t('phase.'+phase)}</p>
    {value&&<p className="muted">{t('as_of',{at:formatDateTime(value.readiness.checked_at)})}</p>}
    {value&&phase==='read'&&!value.readiness.scope.can_open_project&&<p>{tr('shared-tool-onboarding-pool.project_restricted')}</p>}
    {value&&(!canPrepare)&&phase==='read'&&<p>{t(value.runner.status!=='active'||value.readiness.runner.credential_state!=='valid'?'identity_unavailable':value.readiness.scope.grant_status!=='approved'?'grant_unavailable':!value.readiness.scope.can_open_project?'project_restricted':'pool_unavailable')}</p>}
    <div className="pool-row-actions"><button type="button" className="primary" data-pool-recovery-prepare disabled={!canPrepare} onClick={()=>void read(true)}>{t('prepare')}</button>
      {value&&canManage&&(value.runner.status!=='active'||value.readiness.runner.credential_state!=='valid')&&<button type="button" data-pool-recovery-manage onClick={()=>{if(current())latest.current.onManage?.(value.runner);}}>{t('manage_identity')}</button>}</div>
  </section>;
}
