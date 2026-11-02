import {useEffect,useLayoutEffect,useRef,useState} from 'react';
import type {PoolRunnerPairing,PoolRunnerPairingPage} from '../../../../packages/contracts/src';
import {ApiError} from '../api';
import {useApi,useApiView} from '../api-context';
import {ErrorNotice} from '../ui';
import {tr,UiError,useLocale} from '../i18n';
import {toolSessionStorage} from '../tool-hooks';
import {useWriteAction} from '../write-action';
import {PoolRunnerPairingCard,type PoolPairingCreateValues} from './PoolRunnerPairingCard';
import {poolPairingListPath,poolPairingPath,poolPairingStates,readPoolRunnerPairing,readPoolRunnerPairingPage,validPoolPairingScope,validPoolRunnerPairingReceipt,validPoolRunnerPairingRequest,type PoolPairingQuery,type PoolPairingScope} from './pool-runner-pairing-model';

export type PoolRunnerPairingPanelProps={poolID:string;disabled?:boolean;canCreate:boolean;onLockChange:(locked:boolean)=>void;onPaired:(value:PoolRunnerPairing)=>void};
const t=(key:string,values?:Record<string,unknown>)=>tr('pool-runner-pairing.'+key,values),locator=(value:unknown):value is string=>typeof value==='string'&&/^[a-f0-9]{32}$/.test(value),refused=(error:unknown)=>error instanceof ApiError&&[401,403,404].includes(error.status);
type Head={api:ReturnType<typeof useApi>;id:string;loading:boolean;value?:PoolRunnerPairing;error?:unknown};
type List={api:ReturnType<typeof useApi>;path:string;loading:boolean;page?:PoolRunnerPairingPage;error?:unknown};

export function PoolRunnerPairingPanel(props:PoolRunnerPairingPanelProps){
 const api=useApi(),view=useApiView(),identity=JSON.stringify([view?.organizationID,view?.userID,props.poolID]),owner=useRef({api,identity,epoch:0});
 if(owner.current.api!==api||owner.current.identity!==identity)owner.current={api,identity,epoch:owner.current.epoch+1};
 if(!view)return null;const scope={poolID:props.poolID,organizationID:view.organizationID,userID:view.userID};if(!validPoolPairingScope(scope))return null;
 const epoch=owner.current.epoch,owns=()=>owner.current.api===api&&owner.current.identity===identity&&owner.current.epoch===epoch;
 return <PairingScope key={identity+':'+epoch} {...props} scope={scope} owns={owns}/>;
}
function PairingScope({scope,disabled=false,canCreate,onLockChange,onPaired,owns}:PoolRunnerPairingPanelProps&{scope:PoolPairingScope;owns:()=>boolean}){
 useLocale();const api=useApi(),storage=toolSessionStorage(),scopeKey=JSON.stringify([scope.organizationID,scope.userID,scope.poolID,'pool_runner']),locatorKey='suma-pool-runner-pairing:'+scopeKey;
 const [selected,setSelected]=useState(()=>{try{const value=storage?.getItem(locatorKey);return value===null||value===undefined?'':locator(value)?value:'unreadable-locator';}catch{return 'unreadable-locator';}}),[head,setHead]=useState<Head>(),[list,setList]=useState<List>(),[revision,setRevision]=useState(0),[denied,setDenied]=useState(false),[localError,setLocalError]=useState<unknown>(),[query,setQuery]=useState<PoolPairingQuery>({status:'active',q:'',limit:20,cursor:''}),[search,setSearch]=useState(''),[cursors,setCursors]=useState<string[]>(['']);
 const alive=useRef(false),headEpoch=useRef(0),listEpoch=useRef(0),latest=useRef({selected,disabled,canCreate,onLockChange,onPaired});latest.current={selected,disabled,canCreate,onLockChange,onPaired};
 const headers={'X-Suma-Expected-User':scope.userID,'X-Suma-Expected-Organization':scope.organizationID};
 const action=useWriteAction({scope:'pool-runner-pairing:'+scopeKey,storage,validateRequest:request=>validPoolRunnerPairingRequest(request,scope),validateResponse:(value,request)=>{
  if(!alive.current||!owns()||!validPoolRunnerPairingReceipt(value,request,scope))return false;
  if(request.path===poolPairingPath(scope.poolID)){try{if(!storage)throw Error();storage.setItem(locatorKey,readPoolRunnerPairing(value,scope).id);}catch{setLocalError(new UiError('pool-runner-pairing.locator_failed'));return false;}}
  return alive.current&&owns();
 }});
 const pending=!!action.pending,locked=pending||action.busy,invalidLocator=!!selected&&!locator(selected),writeDenied=[401,403,404].includes(action.errorStatus||0),hidden=denied||writeDenied;
 const current=head?.api===api&&head.id===selected?head:undefined,pairing=hidden?undefined:current?.value,loading=!hidden&&(locator(selected)&&!current||!!current?.loading),stale=!!selected&&(!current?.value||current.loading||!!current.error),path=poolPairingListPath(scope,query),currentList=list?.api===api&&list.path===path?list:undefined;
 const error=invalidLocator?new UiError('pool-runner-pairing.invalid_locator'):localError||action.error||current?.error;
 function deny(){setDenied(true);setHead(undefined);setList(undefined);setLocalError(new UiError('pool-runner-pairing.access_changed'));}
 function rememberID(id:string){try{if(!storage)throw Error();storage.setItem(locatorKey,id);}catch{setLocalError(new UiError('pool-runner-pairing.locator_failed'));return false;}setSelected(id);return true;}
 function refresh(){if(disabled||action.busy||!owns())return;setLocalError(undefined);setHead(before=>before?{...before,loading:true,error:undefined}:before);if(!action.pending)action.clearConflict();setRevision(value=>value+1);}
 useLayoutEffect(()=>{latest.current.onLockChange(locked);},[locked]);
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;++headEpoch.current;++listEpoch.current;latest.current.onLockChange(false);};},[]);
 useEffect(()=>{if(writeDenied)deny();},[writeDenied]);
 useEffect(()=>{
  if(disabled)return;const ticket=++listEpoch.current,controller=new AbortController(),live=()=>alive.current&&owns()&&listEpoch.current===ticket&&!controller.signal.aborted;
  setList(before=>({api,path,loading:true,...(before?.api===api&&before.path===path?{page:before.page}:{})}));
  void api<unknown>(path,undefined,'GET',headers,controller.signal).then(raw=>{if(!live())return;const page=readPoolRunnerPairingPage(raw,scope,query);setList({api,path,loading:false,page});setDenied(false);if(writeDenied)action.setError('');}).catch(failure=>{if(!live())return;if(refused(failure))deny();else setList({api,path,loading:false,error:failure});});
  return()=>controller.abort();
 },[api,path,revision,disabled,invalidLocator]);
 useEffect(()=>{
  if(!locator(selected)||disabled)return;const id=selected,ticket=++headEpoch.current,controller=new AbortController(),live=()=>alive.current&&owns()&&headEpoch.current===ticket&&latest.current.selected===id&&!controller.signal.aborted;
  setHead(before=>({api,id,loading:true,...(before?.api===api&&before.id===id?{value:before.value}:{})}));
  void api<unknown>(poolPairingPath(scope.poolID)+'/'+encodeURIComponent(id),undefined,'GET',headers,controller.signal).then(raw=>{if(!live())return;const value=readPoolRunnerPairing(raw,scope);if(value.id!==id)throw new UiError('pool-runner-pairing.invalid_read');setHead({api,id,loading:false,value});setDenied(false);if(writeDenied)action.setError('');}).catch(failure=>{if(!live())return;if(refused(failure))deny();else setHead(before=>({api,id,loading:false,error:failure,...(before?.api===api&&before.id===id?{value:before.value}:{})}));});
  return()=>controller.abort();
 },[api,selected,revision,disabled]);
 async function run(route:string,body:Record<string,unknown>,method='POST'){
  const value=await action.run<unknown>(route,body,method);if(!alive.current||!owns())return;
  if(value){const receipt=readPoolRunnerPairing(value,scope);if(rememberID(receipt.id)){setHead({api,id:receipt.id,loading:true,value:receipt});setRevision(number=>number+1);}}
 }
 async function create(values:PoolPairingCreateValues){if(disabled||hidden||locked||stale||action.conflict||action.persistenceBlocked||!canCreate||!owns())return;await run(poolPairingPath(scope.poolID),{pool_id:scope.poolID,...values});}
 async function command(actionName:'approve'|'reject'){
  const value=current?.value;if(disabled||hidden||locked||stale||loading||!value||!owns()||!(actionName==='approve'?value.state==='proposed'&&value.can_approve:['waiting','proposed'].includes(value.state)))return;
  await run(poolPairingPath(scope.poolID)+'/'+value.id+'/commands',{pool_id:scope.poolID,action:actionName,expected_version:value.version});
 }
 async function restore(){const original=action.pending;if(disabled||hidden||action.busy||!original||!owns())return;await run(original.path,original.body,original.method);}
 function choose(id:string){if(disabled||locked||hidden||!currentList?.page?.items.some(value=>value.id===id)||!locator(id)||!owns())return;++headEpoch.current;setHead(undefined);setLocalError(undefined);action.clearConflict();rememberID(id);}
 function resetLocator(){
  if(disabled||locked||hidden||!invalidLocator||!currentList?.page||currentList.loading||currentList.error||!owns())return;
  try{if(!storage)throw Error();storage.removeItem(locatorKey);}catch{setLocalError(new UiError('pool-runner-pairing.reset_failed'));return;}
  ++headEpoch.current;setHead(undefined);setSelected('');setLocalError(undefined);setRevision(value=>value+1);
 }
 function changeQuery(status:string,q:string,cursor=''){if(disabled||locked)return;try{const next={status,q:q.trim(),limit:20,cursor};poolPairingListPath(scope,next);setLocalError(undefined);setQuery(next);if(!cursor)setCursors(['']);}catch{setLocalError(new UiError('pool-runner-pairing.query_invalid'));}}
 return <div className="pool-runner-pairing" data-pool-runner-pairing data-current-pairing={pairing?.id||''}>
  <details className="pool-pairing-history"><summary>{t('history')}</summary><form className="pool-pairing-history-search" onSubmit={event=>{event.preventDefault();changeQuery(query.status,search);}}><label><span>{t('status')}</span><select data-pool-runner-pairing-status value={query.status} disabled={disabled||locked} onChange={event=>changeQuery(event.target.value,query.q)}>{['active','all',...poolPairingStates].map(state=><option key={state} value={state}>{t('state.'+state)}</option>)}</select></label><label><span>{t('search')}</span><input value={search} maxLength={120} disabled={disabled||locked} onChange={event=>setSearch(event.target.value)}/></label><button disabled={disabled||locked}>{t('search')}</button></form>
   {!!currentList?.error&&<ErrorNotice error={currentList.error}/>} {currentList?.loading&&<p role="status">{t('loading')}</p>}{!hidden&&currentList?.page&&<><p className="pool-pairing-muted">{t('list_count',{count:currentList.page.total})}</p><ul>{currentList.page.items.map(value=><li key={value.id}><button type="button" data-pool-runner-pairing-select={value.id} aria-pressed={selected===value.id} disabled={disabled||locked||currentList.loading} onClick={()=>choose(value.id)}><span>{value.name}</span><span>{t('state.'+value.state)}</span></button></li>)}</ul><div className="pool-pairing-actions"><button type="button" data-pool-runner-pairing-previous disabled={disabled||locked||currentList.loading||cursors.length<2} onClick={()=>{const next=cursors.slice(0,-1);setCursors(next);changeQuery(query.status,query.q,next.at(-1)||'');}}>{t('previous')}</button><button type="button" data-pool-runner-pairing-next disabled={disabled||locked||currentList.loading||!currentList.page.next_cursor} onClick={()=>{const cursor=currentList.page?.next_cursor;if(cursor){setCursors(before=>[...before,cursor]);changeQuery(query.status,query.q,cursor);}}}>{t('next')}</button></div></>}
  </details>
  {invalidLocator&&!pending&&<div className="pool-pairing-locator-recovery"><p className="pool-pairing-muted">{t('reset_help')}</p><button type="button" className="text-button" data-pool-runner-pairing-reset-locator disabled={disabled||locked||hidden||!currentList?.page||currentList.loading||!!currentList.error} onClick={resetLocator}>{t('reset_locator')}</button></div>}
  <PoolRunnerPairingCard scope={scope} pairing={pairing} loading={loading} stale={stale} disabled={disabled||hidden||action.busy||!pending&&action.persistenceBlocked} refreshDisabled={disabled||action.busy} canCreate={canCreate&&!hidden&&!!currentList?.page&&!currentList.loading&&!currentList.error} pending={pending} error={error} onCreate={values=>void create(values)} onRefresh={refresh} onApprove={()=>void command('approve')} onReject={()=>void command('reject')} onRestore={()=>void restore()} onContinue={()=>{const value=current?.value;if(!disabled&&!hidden&&!locked&&!stale&&!loading&&value?.state==='approved'&&owns())latest.current.onPaired(value);}}/>
 </div>;
}
