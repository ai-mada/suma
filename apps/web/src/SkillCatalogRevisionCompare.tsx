import {useCallback,useEffect,useId,useState,type ComponentType} from 'react';
import {FileDiff} from 'lucide-react';
import type {SkillCatalogDetail,SkillCatalogRevisionPage} from '../../../packages/contracts/src';
import {ApiError,query} from './api';
import {useApi,useApiView} from './api-context';
import {formatNumber,tr,UiError,useLocale} from './i18n';
import {Loading,ErrorNotice,Field} from './ui';
import {toolError} from './tool-hooks';
import {useCatalogPage} from './skill-catalog-hooks';
import {canAdoptComparedCurrent,comparisonRevisions,validComparisonRevision,validRevisionPage} from './skill-revision-compare-model';
import './SkillCatalogRevisionCompare.css';

export type SkillCatalogRevisionCompareProps={project:string;current:SkillCatalogDetail;refresh:number;blocked?:boolean;adoptionBlocked?:boolean;onAccessDenied:(error:unknown)=>void;onAdoptCurrent:()=>void;onRefreshCurrent?:()=>void};
export type RevisionEditorProps={left:SkillCatalogDetail;right:SkillCatalogDetail;scope:string;leftLabel:string;rightLabel:string;label:string};
const t=(key:string,values?:Record<string,unknown>)=>tr('skill-revision-compare.'+key,values);
const revisionKey=(item:SkillCatalogDetail)=>item.id+':'+item.version;
export function SkillCatalogRevisionCompare(props:SkillCatalogRevisionCompareProps){
 const view=useApiView(),api=useApi(),item=props.current;
 const scope=JSON.stringify([view?.userID,view?.organizationID,props.project,item.id,item.version,item.origin.project_id,item.origin.knowledge_id,item.origin.knowledge_version,item.origin.revision,item.origin.sha256]);
 return <RevisionCompare key={scope} {...props} scope={scope} api={api}/>;
}
type ScopedCompareProps=SkillCatalogRevisionCompareProps&{scope:string;api:ReturnType<typeof useApi>};
function RevisionCompare(props:ScopedCompareProps){
 useLocale();const region=useId(),[open,setOpen]=useState(false);
 return <details className="src-compare" data-skill-compare onToggle={event=>setOpen(event.currentTarget.open)}><summary aria-controls={region}><FileDiff size={15} aria-hidden="true"/>{t('title')}</summary>{open&&<RevisionCompareBody key={JSON.stringify([props.refresh,!!props.blocked])} {...props} region={region}/>}</details>;
}
// An expansion owns a new read lifetime. Closing unmounts cached text and
// models, so reopening cannot render an earlier success before its fresh GET.
function RevisionCompareBody({project,current,refresh,blocked=false,adoptionBlocked=false,onAccessDenied,onAdoptCurrent,onRefreshCurrent,scope,api,region}:ScopedCompareProps&{region:string}){
 useLocale();const instance=useId(),[denied,setDenied]=useState<unknown>(),[selected,setSelected]=useState<{left?:number;right?:number}>({}),[mode,setMode]=useState<'diff'|'text'>('diff'),[editor,setEditor]=useState<{component?:ComponentType<RevisionEditorProps>;error?:boolean}>({}),[retry,setRetry]=useState(0);
 const onError=useCallback((error:unknown)=>{if(error instanceof ApiError&&[401,403].includes(error.status)){setDenied(error);onAccessDenied(error);}},[onAccessDenied]);
 const validate=useCallback((page:SkillCatalogRevisionPage)=>validRevisionPage(page,current),[current]);
 const history=useCatalogPage<SkillCatalogDetail,SkillCatalogRevisionPage>(api,'/skill-catalog/'+encodeURIComponent(current.id)+'/revisions'+query(project)+'&limit=20',!blocked&&!denied&&validComparisonRevision(current),refresh,validate,onError,revisionKey);
 const readable=!blocked&&!denied&&!history.loading&&!history.error&&!!history.value&&validComparisonRevision(current);
 const revisions=readable?comparisonRevisions(current,history.value!.items):[],left=revisions.find(item=>item.version===(selected.left??revisions.find(item=>item.version!==current.version)?.version)),right=revisions.find(item=>item.version===(selected.right??current.version)),pair=!!left&&!!right&&left.version!==right.version;
 useEffect(()=>{
  if(!readable||!pair||mode!=='diff'||editor.component)return;
  let live=true;setEditor({});
  void import('./SkillCatalogRevisionEditor').then(async module=>{await module.prepareRevisionEditor();if(live)setEditor({component:module.default});}).catch(()=>{if(live)setEditor({error:true});});
  return()=>{live=false;};
 },[readable,pair,mode,retry,editor.component]);
 const versionLabel=(item:SkillCatalogDetail)=>t('option',{version:formatNumber(item.version),revision:formatNumber(item.origin.revision)})+(item.version===current.version?' · '+t('current'):'');
 function selector(side:'left'|'right',item:SkillCatalogDetail|undefined){return <div className="src-side"><Field label={t(side)}><select data-compare-left={side==='left'?true:undefined} data-compare-right={side==='right'?true:undefined} value={item?.version??''} onChange={event=>setSelected(value=>({...value,[side]:Number(event.target.value)}))}>{!item&&<option value="">{t('choose')}</option>}{revisions.map(value=><option key={value.version} value={value.version}>{versionLabel(value)}</option>)}</select></Field>{item&&<div className="src-source"><span>{t('source',{revision:formatNumber(item.origin.revision),version:formatNumber(item.origin.knowledge_version)})}</span><code aria-label={t('sha')}>{item.origin.sha256}</code></div>}</div>;}
 const Editor=editor.component;
 return <section id={region} className="src-body" aria-label={t('title')} aria-busy={history.loading||blocked}>
  {denied?<ErrorNotice error={toolError(denied)}/>:blocked||history.loading?<Loading/>:history.error?<ErrorNotice error={toolError(history.error)}/>:!validComparisonRevision(current)?<ErrorNotice error={new UiError('skill-revision-compare.invalid_read')}/>:readable&&<>
   <p className="src-hint">{t('intro')}</p><div className="src-selectors">{selector('left',left)}{selector('right',right)}</div>
   {!pair?<p className="src-hint" role="status">{t(revisions.length<2?'not_enough':'different')}</p>:<><div className="src-actions"><button type="button" data-compare-text aria-pressed={mode==='text'} onClick={()=>setMode(value=>value==='diff'?'text':'diff')}>{t(mode==='diff'?'plain_text':'show_diff')}</button></div>
    {mode==='text'?<div data-compare-plain className="src-plain">{[left!,right!].map((item,index)=><section key={index} aria-label={t(index===0?'left':'right')}><h4>{versionLabel(item)}</h4><pre>{item.content}</pre></section>)}</div>:editor.error?<div className="src-editor-error" role="alert"><p>{t('editor_failed')}</p><button type="button" data-compare-editor-retry onClick={()=>{setEditor({});setRetry(value=>value+1);}}>{t('retry_editor')}</button></div>:Editor?<div className="src-editor"><Editor key={left!.version+':'+right!.version} left={left!} right={right!} scope={scope+instance} leftLabel={t('left')+' · '+versionLabel(left!)} rightLabel={t('right')+' · '+versionLabel(right!)} label={t('editor_label')}/></div>:<Loading/>}
   </>}
   <div className="src-footer"><p className="src-hint">{t('boundary')}</p>{pair&&canAdoptComparedCurrent(current,left,right)&&<button data-compare-adopt type="button" disabled={adoptionBlocked} onClick={()=>{if(!adoptionBlocked)onAdoptCurrent();}}>{t('adopt_current',{version:formatNumber(current.version)})}</button>}{history.value?.next_cursor&&<button type="button" data-compare-more onClick={()=>void history.read(true)}>{t('more')}</button>}</div>
  </>}
  {!!history.error&&!denied&&<button className="src-retry" data-compare-retry type="button" onClick={()=>onRefreshCurrent?onRefreshCurrent():void history.read()}>{t('retry_read')}</button>}
 </section>;
}
