import {useCallback,useEffect,useId,useLayoutEffect,useRef,useState} from 'react';
import {BookOpen,X} from 'lucide-react';
import type {ProjectKnowledgeDiscoveryHit} from '../../../packages/contracts/src';
import {ApiError} from './api';
import {useApi} from './api-context';
import {formatNumber,tr,UiError,useLocale} from './i18n';
import {ErrorNotice} from './ui';
import {PublishedKnowledgeDiscovery} from './PublishedKnowledgeDiscovery';
import {PublishedKnowledgePreview} from './PublishedKnowledgePreview';
import type {KnowledgeReference,KnowledgeSummary} from './knowledge-model';
import {conversationKnowledgeLimit,currentKnowledgePin,knowledgeDetailPath,knowledgeSelectionKey,sameKnowledgePin,validKnowledgePins,validKnowledgeSummary} from './task-conversation-knowledge-model';
import './TaskConversationKnowledge.css';

export type ProjectKnowledgePickerProps={project:string;taskID:string;userID:string;organizationID:string;selected:KnowledgeReference[];onChange:(pins:KnowledgeReference[])=>void;disabled:boolean;accessBlocked:boolean;refresh:number;onValidityChange:(key:string,valid:boolean)=>void;onAccessDenied:()=>void};

const t=(key:string,values?:Record<string,unknown>)=>tr('task-conversation-knowledge.'+key,values);
const refused=(error:unknown)=>error instanceof ApiError&&[401,403].includes(error.status);

/** Discovery pages never own selection. Exact publication pins are checked
 * separately before a parent may submit its original domain command. */
export function ProjectKnowledgePicker(props:ProjectKnowledgePickerProps){
 const api=useApi(),caller=useRef({api,epoch:0});
 if(caller.current.api!==api)caller.current={api,epoch:caller.current.epoch+1};
 return <PickerScope key={JSON.stringify([props.userID,props.organizationID,props.project,props.taskID,caller.current.epoch])} {...props}/>;
}
function PickerScope({project,userID,organizationID,selected,onChange,disabled,accessBlocked,refresh,onValidityChange,onAccessDenied}:ProjectKnowledgePickerProps){
 useLocale();const api=useApi(),id=useId(),[expanded,setExpanded]=useState(false),[preview,setPreview]=useState<KnowledgeReference>(),[conflict,setConflict]=useState(false),[deniedAt,setDeniedAt]=useState<number>();
 const [hydrated,setHydrated]=useState<{key:string;items:KnowledgeSummary[]}>(),[error,setError]=useState<unknown>(),[retry,setRetry]=useState(0);
 const alive=useRef(true),epoch=useRef(0),controller=useRef<AbortController|undefined>(undefined),notified=useRef<number|undefined>(undefined);
 const callbacks=useRef({onChange,onValidityChange,onAccessDenied});callbacks.current={onChange,onValidityChange,onAccessDenied};
 const selectionKey=knowledgeSelectionKey(selected,refresh),pinsValid=validKnowledgePins(selected),hidden=accessBlocked||deniedAt===refresh;
 const current=useRef({selectionKey,hidden,disabled,selected,preview,refresh});current.current={selectionKey,hidden,disabled,selected,preview,refresh};
 const deny=useCallback(()=>{if(!alive.current||current.current.refresh!==refresh)return;controller.current?.abort();setHydrated(undefined);setPreview(undefined);setDeniedAt(refresh);if(notified.current!==refresh){notified.current=refresh;callbacks.current.onAccessDenied();}},[refresh]);
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;controller.current?.abort();};},[]);
 const items=!hidden&&hydrated?.key===selectionKey?hydrated.items:[],ready=!selected.length||pinsValid&&!hidden&&hydrated?.key===selectionKey&&items.length===selected.length&&!error;
 useLayoutEffect(()=>{callbacks.current.onValidityChange(selectionKey,!!ready);},[selectionKey,ready]);
 useEffect(()=>{
  setHydrated(undefined);setError(undefined);setConflict(false);
  if(hidden||!selected.length)return;
  if(!pinsValid){setError(new UiError('task-conversation-knowledge.invalid_read'));return;}
  const abort=new AbortController(),generation=++epoch.current;controller.current?.abort();controller.current=abort;
  const rows:KnowledgeSummary[]=[];let next=0,failure:unknown;
  const active=()=>alive.current&&!abort.signal.aborted&&generation===epoch.current&&current.current.selectionKey===selectionKey&&!current.current.hidden;
  async function worker(){
   while(active()&&next<selected.length){
    const pin=selected[next++];
    try{
     const value=await api<unknown>(knowledgeDetailPath(project,pin.id),undefined,'GET',{'X-Suma-Expected-User':userID,'X-Suma-Expected-Organization':organizationID},abort.signal);
     if(!active())return;
     if(!validKnowledgeSummary(value,project)||value.id!==pin.id)throw new UiError('task-conversation-knowledge.invalid_read');
     if(currentKnowledgePin(value,project,pin))rows.push(value);
    }catch(reason){if(!active())return;if(refused(reason)){deny();return;}if(!(reason instanceof ApiError&&reason.status===404))failure=reason;}
   }
  }
  void Promise.all(Array.from({length:Math.min(3,selected.length)},worker)).then(()=>{if(!active())return;if(failure)setError(failure);else setHydrated({key:selectionKey,items:rows});});
  return()=>abort.abort();
 },[api,project,userID,organizationID,selectionKey,hidden,pinsValid,retry,deny]);
 function openPreview(reference:KnowledgeReference){if(!alive.current||current.current.hidden||current.current.disabled||!validKnowledgePins([reference]))return;setConflict(false);setPreview(reference);setExpanded(true);}
 function previewHit(hit:ProjectKnowledgeDiscoveryHit){if(hit.project_id===project)openPreview(hit.reference);}
 function choose(reference:KnowledgeReference){
  const now=current.current;if(!alive.current||now.hidden||now.disabled||!now.preview||!sameKnowledgePin(reference,now.preview)||!validKnowledgePins([reference])||!validKnowledgePins(now.selected))return;
  const existing=now.selected.find(pin=>pin.id===reference.id);
  if(existing&&!sameKnowledgePin(existing,reference)){setConflict(true);return;}
  if(!existing&&now.selected.length>=conversationKnowledgeLimit)return;
  setConflict(false);setPreview(undefined);if(!existing)callbacks.current.onChange([...now.selected,reference]);
 }
 function remove(id:string){const now=current.current;if(alive.current&&!now.hidden&&!now.disabled){setConflict(false);callbacks.current.onChange(now.selected.filter(pin=>pin.id!==id));}}
 const limitReached=!!preview&&!selected.some(pin=>pin.id===preview.id)&&selected.length>=conversationKnowledgeLimit;
 if(hidden)return null;
 return <div className="conversation-knowledge project-knowledge-picker" data-project-knowledge-picker aria-label={t('title')}>
  <div className="conversation-knowledge-heading"><button type="button" data-knowledge-toggle aria-expanded={expanded} aria-controls={id} disabled={disabled} onClick={()=>setExpanded(value=>!value)}><BookOpen size={14} aria-hidden="true"/>{t('title')}<span>{formatNumber(selected.length)}/{conversationKnowledgeLimit}</span></button><small>{t('hint')}</small></div>
  {!!selected.length&&<ul className="conversation-knowledge-selected" aria-label={t('selected')}>
   {selected.map(pin=>{const entry=items.find(item=>item.id===pin.id);return <li key={pin.id} data-knowledge-selected={pin.id} className={entry?'':'is-unavailable'}><span><strong>{entry?.published_title||t(hydrated?.key===selectionKey?'unavailable':'checking',{id:pin.id})}</strong><small>{t('revision',{revision:formatNumber(pin.revision)})} · <code>{pin.id}</code></small></span><div className="conversation-knowledge-selected-actions"><button type="button" className="text-button" data-knowledge-selected-preview={pin.id} disabled={disabled} onClick={()=>openPreview(pin)}>{t('preview')}</button><button type="button" data-knowledge-remove className="icon-button" disabled={disabled} aria-label={t('remove',{name:entry?.published_title||pin.id})} onClick={()=>remove(pin.id)}><X size={13} aria-hidden="true"/></button></div></li>;})}
  </ul>}
  {!!error&&<ErrorNotice error={error} onRetry={()=>setRetry(value=>value+1)}/>}
  {!!selected.length&&!ready&&<p className="conversation-knowledge-warning" role="status">{t('verify_required')}</p>}
  {conflict&&<p className="conversation-knowledge-warning" role="alert">{t('conflict')}</p>}
  {expanded&&<div id={id} className="conversation-knowledge-picker">
   <PublishedKnowledgeDiscovery project={project} disabled={disabled} accessBlocked={hidden} refresh={refresh} onPreview={previewHit} onAccessDenied={deny}/>
   {preview&&<div className="conversation-knowledge-preview"><PublishedKnowledgePreview project={project} reference={preview} disabled={disabled} accessBlocked={hidden} refresh={refresh} onChoose={pinsValid&&!limitReached?choose:undefined} onAccessDenied={deny} onClose={()=>setPreview(undefined)}/>{limitReached&&<p className="conversation-knowledge-warning" role="status">{t('limit',{limit:conversationKnowledgeLimit})}</p>}</div>}
  </div>}
 </div>;
}
