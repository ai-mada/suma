import {useCallback,useEffect,useLayoutEffect,useRef,useState} from 'react';
import type {ProjectKnowledgeReference} from '../../../packages/contracts/src';
import {ApiError} from './api';
import {useApi,useApiView} from './api-context';
import {KnowledgeRevisionFacts} from './KnowledgeFacts';
import type {KnowledgeRevision} from './knowledge-model';
import {knowledgeDetailPath,knowledgeRevisionPath,validKnowledgePins} from './task-conversation-knowledge-model';
import {readPublishedKnowledgeHighlight,readPublishedKnowledgeQualification,readPublishedKnowledgeRevision,type PublishedKnowledgeHighlight,type PublishedKnowledgeQualification,type PublishedKnowledgePreviewLocation} from './published-knowledge-preview-model';
import {tr,useLocale,UiError} from './i18n';
import {ErrorNotice} from './ui';
import './PublishedKnowledgePreview.css';

export type PublishedKnowledgePreviewProps={project:string;reference:ProjectKnowledgeReference;location?:PublishedKnowledgePreviewLocation;disabled?:boolean;accessBlocked?:boolean;refresh?:number;onChoose?:(reference:ProjectKnowledgeReference)=>void;onAccessDenied?:()=>void;onClose?:()=>void};
type Snapshot={loading:boolean;revision?:KnowledgeRevision;qualification?:PublishedKnowledgeQualification;error?:unknown;stale?:boolean;denied?:boolean};
const t=(key:string)=>tr('knowledge-preview.'+key);
const refusal=(e:unknown)=>e instanceof ApiError&&[401,403,404].includes(e.status);
const temporary=(e:unknown)=>e instanceof ApiError&&(e.status===0||e.status>=500)||e instanceof TypeError;

export function PublishedKnowledgePreview(props:PublishedKnowledgePreviewProps){
 const api=useApi(),view=useApiView(),owner=useRef({api,epoch:0});if(owner.current.api!==api)owner.current={api,epoch:owner.current.epoch+1};
 if(!view?.userID||!view.organizationID)return null;
 return <PreviewReader key={JSON.stringify([view.userID,view.organizationID,props.project,props.reference,owner.current.epoch])} {...props} userID={view.userID} organizationID={view.organizationID}/>;
}
function PreviewReader({project,reference,location,disabled=false,accessBlocked=false,refresh=0,onChoose,onAccessDenied,onClose,userID,organizationID}:PublishedKnowledgePreviewProps&{userID:string;organizationID:string}){
 useLocale();const api=useApi(),pinKey=JSON.stringify(reference),pin:ProjectKnowledgeReference={id:reference.id,revision:reference.revision,sha256:reference.sha256},validReference=validKnowledgePins([reference]);
 const [snapshot,setSnapshot]=useState<Snapshot>({loading:!accessBlocked}),current=useRef(snapshot),alive=useRef(true),caller=useRef(api),generation=useRef(0),controller=useRef<AbortController|undefined>(undefined),flight=useRef<Promise<void>|undefined>(undefined),denied=useRef(false),reportedDenial=useRef(false);
 const element=useRef<HTMLElement|null>(null),[wholeLocation,setWholeLocation]=useState<string|undefined>(undefined),[locateRequest,setLocateRequest]=useState(0),locateSource=useRef('');
 const parent=useRef({accessBlocked,disabled}),callbacks=useRef({onChoose,onAccessDenied,onClose});parent.current={accessBlocked,disabled};callbacks.current={onChoose,onAccessDenied,onClose};caller.current=api;
 const store=(value:Snapshot)=>{current.current=value;setSnapshot(value);};
 const deny=useCallback(()=>{denied.current=true;++generation.current;controller.current?.abort();flight.current=undefined;store({loading:false,denied:true});if(!reportedDenial.current){reportedDenial.current=true;callbacks.current.onAccessDenied?.();}},[]);
 useLayoutEffect(()=>{if(accessBlocked){denied.current=true;++generation.current;controller.current?.abort();flight.current=undefined;store({loading:false,denied:true});}},[accessBlocked]);
 const read=useCallback((replace=false):Promise<void>=>{
  if(parent.current.accessBlocked)return Promise.resolve();if(flight.current&&!replace)return flight.current;
  if(!validReference){store({loading:false,error:new UiError('knowledge-preview.invalid_read')});return Promise.resolve();}
  const ticket=++generation.current,abort=new AbortController();controller.current?.abort();controller.current=abort;
  const previous=!denied.current?current.current:undefined;store({loading:true,revision:previous?.revision,qualification:previous?.qualification,denied:denied.current});
  const owns=()=>alive.current&&caller.current===api&&generation.current===ticket&&!abort.signal.aborted&&!parent.current.accessBlocked;
  const load=async(path:string)=>{try{return await api<unknown>(path,undefined,'GET',{'X-Suma-Expected-User':userID,'X-Suma-Expected-Organization':organizationID},abort.signal);}catch(error){if(owns()&&refusal(error))deny();throw error;}};
  const job=(async()=>{
   const results=await Promise.allSettled([load(knowledgeRevisionPath(project,pin)),load(knowledgeDetailPath(project,pin.id))]);if(!owns())return;
   const failures=results.filter((result):result is PromiseRejectedResult=>result.status==='rejected');
   if(failures.length){const error=failures.find(result=>!temporary(result.reason))?.reason??failures[0].reason;store(temporary(error)&&previous?.revision?{...previous,loading:false,error,stale:true}:{loading:false,error});return;}
   const revisionResult=results[0],currentResult=results[1];if(revisionResult.status!=='fulfilled'||currentResult.status!=='fulfilled')return;
   try{const revision=readPublishedKnowledgeRevision(revisionResult.value,project,pin),qualification=readPublishedKnowledgeQualification(currentResult.value,project,pin);if(!owns())return;denied.current=false;reportedDenial.current=false;store({loading:false,qualification,revision:qualification.reason==='unavailable'?undefined:revision});}
   catch(error){if(owns())store({loading:false,error});}
  })();flight.current=job;void job.finally(()=>{if(flight.current===job)flight.current=undefined;});return job;
 },[api,project,pinKey,validReference,userID,organizationID,deny]);
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;++generation.current;controller.current?.abort();flight.current=undefined;};},[]);
 useEffect(()=>{void read(true);},[read,refresh,accessBlocked]);
 const hidden=accessBlocked||denied.current||!!snapshot.denied,revision=hidden?undefined:snapshot.revision;
 const locationKey=JSON.stringify(location),wholeText=wholeLocation===locationKey;let highlight:PublishedKnowledgeHighlight|undefined,locationError:unknown;
 if(revision)try{highlight=readPublishedKnowledgeHighlight(location,pin,revision.content);}catch(error){locationError=error;}
 const locationSource=JSON.stringify([project,pin,revision?.sha256,locationKey]),locationView=useRef({key:locationSource,highlight,invalid:!!locationError,hidden});locationView.current={key:locationSource,highlight,invalid:!!locationError,hidden};
 useLayoutEffect(()=>{const now=locationView.current;if(!locateRequest||locateSource.current!==now.key||!alive.current||caller.current!==api||now.hidden||now.invalid||!now.highlight||wholeText||current.current.loading)return;const match=element.current?.querySelector<HTMLElement>('[data-knowledge-preview-match]');match?.focus({preventScroll:true});match?.scrollIntoView?.({block:'center',behavior:'auto'});},[locateRequest,wholeText,locationSource,api]);
 const state=hidden?'denied':snapshot.loading?'loading':snapshot.stale?'stale':snapshot.error?'unavailable':locationError?'invalid_location':snapshot.qualification?.reason??'unavailable';
 const canChoose=!disabled&&!hidden&&!snapshot.loading&&!snapshot.stale&&!snapshot.error&&!locationError&&!!revision&&snapshot.qualification?.eligible===true;
 function choose(){const now=current.current;if(!alive.current||caller.current!==api||parent.current.accessBlocked||parent.current.disabled||denied.current||locationView.current.invalid||now.loading||now.stale||now.error||!now.revision||!now.qualification?.eligible)return;callbacks.current.onChoose?.({...pin});}
 function locate(){const now=locationView.current;if(!alive.current||caller.current!==api||parent.current.accessBlocked||now.hidden||now.invalid||!now.highlight||current.current.loading)return;locateSource.current=now.key;setWholeLocation(undefined);setLocateRequest(value=>value+1);}
 return <section ref={element} className="published-knowledge-preview" data-knowledge-published-preview aria-busy={snapshot.loading&&!hidden}>
  <header><h4>{revision?.title??t('title')}</h4>{onClose&&<button type="button" data-knowledge-preview-close onClick={()=>{if(alive.current&&caller.current===api)callbacks.current.onClose?.();}}>{t('close')}</button>}</header>
  <p className="knowledge-preview-reference" data-knowledge-preview-reference><span>{pin.id} · {tr('knowledge.revision',{revision:pin.revision})}</span><code>{pin.sha256}</code></p>
  <p className="knowledge-preview-status" data-knowledge-preview-state={state} role={state==='denied'?'alert':'status'}>{t('state.'+state)}</p>
  {!hidden&&!!snapshot.error&&<ErrorNotice error={snapshot.error}/>}
  {revision&&location!==undefined&&<div className="knowledge-preview-location" data-knowledge-preview-location={locationError?'invalid':wholeText?'whole':'valid'}>{!!locationError&&state!=='invalid_location'&&<p role="alert">{t('invalid_location')}</p>}{highlight&&<><p>{t('location_verified')}</p><div className="knowledge-preview-location-actions"><button type="button" data-knowledge-preview-locate disabled={snapshot.loading||hidden} onClick={locate}>{t('locate')}</button><button type="button" data-knowledge-preview-full disabled={snapshot.loading||hidden||wholeText} onClick={()=>{if(alive.current&&caller.current===api&&!locationView.current.invalid&&!locationView.current.hidden)setWholeLocation(locationKey);}}>{t('whole_text')}</button></div></>}</div>}
  {revision&&<div className="knowledge-preview-body" onClickCapture={event=>{if(event.target instanceof Element&&event.target.closest('a[href]')&&!window.dispatchEvent(new Event('suma:navigate',{cancelable:true})))event.preventDefault();}}><KnowledgeRevisionFacts project={project} revision={revision} collapseSources highlight={wholeText?undefined:highlight}/></div>}
  <p className="knowledge-preview-boundary">{t('boundary')}</p>
  <div className="knowledge-preview-actions">{onChoose&&<button type="button" data-knowledge-preview-choose disabled={!canChoose} onClick={choose}>{t('choose')}</button>}<button type="button" data-knowledge-preview-refresh disabled={accessBlocked||snapshot.loading&&!hidden} onClick={()=>void read()}>{t('refresh')}</button></div>
 </section>;
}
