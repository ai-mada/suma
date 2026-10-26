import {useCallback,useEffect,useRef,useState} from 'react';
import {BookOpen,RefreshCw,X} from 'lucide-react';
import type {SkillCatalogDetail,SkillCatalogPage,SkillCatalogRevisionPage,SkillCatalogSummary} from '../../../packages/contracts/src';
import {ApiError,query,type Session} from './api';
import {useApi} from './api-context';
import {formatDateTime,formatNumber,tr,UiError,useLocale} from './i18n';
import {toolError,toolSessionStorage} from './tool-hooks';
import {Empty,ErrorNotice,Field,Loading} from './ui';
import {useLeaveGuard,useWriteAction,WriteButton,WriteFields,WriteNotice} from './write-action';
import type {KnowledgeDetail,KnowledgePage,KnowledgeSummary} from './knowledge-model';
import {catalogID,catalogInteger,catalogListPath,catalogRecoveryValid,catalogSHA,catalogTerms,catalogText,validCatalogSummary,type SkillCatalogFilters} from './skill-catalog-model';
import {useCatalogPage,useCatalogRead} from './skill-catalog-hooks';
import './SkillCatalogWorkspace.css';

type Props={session:Session;project:string;selected:string;filters:SkillCatalogFilters;onFilter:(patch:Partial<SkillCatalogFilters>)=>void;onSelect:(id:string)=>void;refresh:number;onRefresh:()=>void|Promise<void>;sourceId?:string};
type Editor={kind:'publish'|'update'|'retire'|'adopt';entry?:SkillCatalogDetail;knowledgeId:string;source?:KnowledgeDetail;domain:string;scenarios:string;tags:string;reason:string;ack:boolean;dirty:boolean};
const t=(key:string,values?:Record<string,unknown>)=>tr('skill-catalog.'+key,values);
const itemPath=(id:string)=>'/skill-catalog/'+encodeURIComponent(id);
const eligibleSource=(value:KnowledgeSummary)=>value.kind==='skill'&&value.state==='published'&&value.published_eligible&&value.eligibility==='current'&&value.published_revision>0&&catalogSHA(value.published_sha256);
const publishedSource=(value:KnowledgeDetail)=>eligibleSource(value)&&value.published?.revision===value.published_revision&&value.published.sha256===value.published_sha256;
const makeEditor=(kind:Editor['kind'],entry?:SkillCatalogDetail,sourceId=''):Editor=>({kind,entry,knowledgeId:entry?.origin.knowledge_id||sourceId,domain:entry?.domain||'',scenarios:entry?.scenarios.join(', ')||'',tags:entry?.tags.join(', ')||'',reason:'',ack:false,dirty:false});
const revisionKey=(item:SkillCatalogDetail)=>item.id+':'+item.version;

export function SkillCatalogWorkspace(props:Props){return <CatalogView key={JSON.stringify([props.session.user.id,props.session.organization.id,props.project])} {...props}/>;}
function CatalogView({session,project,selected,filters,onFilter,onSelect,refresh,onRefresh,sourceId=''}:Props){
 useLocale();const api=useApi(),[tick,setTick]=useState(0),[search,setSearch]=useState(filters),[editor,setEditor]=useState<Editor>(),[historyOpen,setHistoryOpen]=useState(false),[historyVersion,setHistoryVersion]=useState<number>(),[forbidden,setForbidden]=useState<unknown>(),[notice,setNotice]=useState<{kind:'saved'|'adopted';id:string}>(),alive=useRef(true),openedSource=useRef(''),callbacks=useRef({onSelect,onRefresh});callbacks.current={onSelect,onRefresh};
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;};},[]);
 useEffect(()=>setSearch(filters),[filters.q,filters.domain,filters.scenario,filters.state]);
 useEffect(()=>{setHistoryOpen(false);setHistoryVersion(undefined);},[selected]);
 const onError=useCallback((error:unknown)=>{if(error instanceof ApiError&&[401,403].includes(error.status)){setForbidden(error);setEditor(undefined);setNotice(undefined);}},[]);
 const pageValid=useCallback((page:SkillCatalogPage)=>Array.isArray(page.items)&&page.items.length<=20&&catalogInteger(page.total)&&page.total>=page.items.length&&typeof page.next_cursor==='string'&&typeof page.can_manage==='boolean'&&typeof page.can_adopt==='boolean'&&page.items.every(validCatalogSummary),[]);
 const list=useCatalogPage<SkillCatalogSummary,SkillCatalogPage>(api,catalogListPath(project,filters),true,refresh+tick,pageValid,onError);
 const detailValid=useCallback((value:SkillCatalogDetail)=>validCatalogSummary(value)&&value.id===selected&&typeof value.content==='string',[selected]);
 const detail=useCatalogRead<SkillCatalogDetail>(api,itemPath(selected)+query(project),!!selected,refresh+tick,detailValid,onError),current=detail.value;
 const historyValid=useCallback((page:SkillCatalogRevisionPage)=>Array.isArray(page.items)&&page.items.length<=20&&catalogInteger(page.total)&&page.total>=page.items.length&&typeof page.next_cursor==='string'&&page.items.every(item=>validCatalogSummary(item)&&item.id===selected&&typeof item.content==='string'),[selected]);
 const history=useCatalogPage<SkillCatalogDetail,SkillCatalogRevisionPage>(api,itemPath(selected)+'/revisions'+query(project)+'&limit=20',!!selected&&historyOpen,refresh+tick,historyValid,onError,revisionKey);
 const managing=editor?.kind==='publish'||editor?.kind==='update',knowledgeID=managing?editor.knowledgeId:'';
 const sourcesValid=useCallback((page:KnowledgePage)=>Array.isArray(page.items)&&page.items.length<=50&&catalogInteger(page.total)&&page.total>=page.items.length&&page.items.every(item=>item.project_id===project&&item.kind==='skill'),[project]);
 const sources=useCatalogPage<KnowledgeSummary,KnowledgePage>(api,'/project-knowledge'+query(project)+'&kind=skill&state=published&limit=50',managing&&!!list.value?.can_manage,refresh+tick,sourcesValid,onError);
 const sourceValid=useCallback((value:KnowledgeDetail)=>value.project_id===project&&value.id===knowledgeID&&value.kind==='skill'&&!!value.latest&&catalogInteger(value.version),[project,knowledgeID]);
 const source=useCatalogRead<KnowledgeDetail>(api,'/project-knowledge/'+encodeURIComponent(knowledgeID)+query(project),!!knowledgeID&&managing,refresh+tick,sourceValid,onError);
 useEffect(()=>{if(source.value)setEditor(previous=>previous&&['publish','update'].includes(previous.kind)&&previous.knowledgeId===source.value!.id&&!previous.source?{...previous,source:source.value,ack:false}:previous);},[source.value]);
 useEffect(()=>{if(sourceId&&catalogID(sourceId)&&list.value?.can_manage&&openedSource.current!==sourceId){openedSource.current=sourceId;setEditor(makeEditor('publish',undefined,sourceId));}},[sourceId,list.value?.can_manage]);
 const action=useWriteAction({scope:JSON.stringify(['skill-catalog',session.user.id,session.organization.id,project]),storage:toolSessionStorage(),validateRequest:request=>catalogRecoveryValid(request,project)}),locked=action.busy||!!action.pending||action.persistenceBlocked;
 useLeaveGuard(!!editor?.dirty,action.setError);
 const stale=detail.loading||!!detail.error,historical=historyVersion!==undefined&&historyVersion!==current?.version,shown=historical?history.value?.items.find(item=>item.version===historyVersion):current;
 const scenarios=editor?catalogTerms(editor.scenarios,8,80,true):undefined,tags=editor?catalogTerms(editor.tags,16,40):undefined;
 const writeAllowed=!!editor&&!locked&&!action.conflict&&catalogText(editor.reason,2000)&&(editor.kind==='publish'||editor.kind==='update'?list.value?.can_manage&&!!editor.source&&publishedSource(editor.source)&&catalogText(editor.domain,80)&&!!scenarios&&!!tags&&editor.ack&&(editor.kind==='publish'||editor.entry?.can_manage&&editor.entry.allowed_actions.includes('update')):editor.kind==='adopt'?editor.entry?.can_adopt&&editor.entry.allowed_actions.includes('adopt')&&editor.ack:editor.entry?.can_manage&&editor.entry.allowed_actions.includes('retire'));
 function canLeave(){return !locked&&window.dispatchEvent(new Event('suma:navigate',{cancelable:true}));}
 function edit(patch:Partial<Editor>){setEditor(value=>value?{...value,...patch,dirty:true}:value);setNotice(undefined);}
 async function completed(value:unknown,path:string){
  if(value===undefined||!alive.current)return;const result=value as {id?:unknown;project_id?:unknown};if(!catalogID(result.id)||path.endsWith('/adoptions')&&result.project_id!==project){action.setError(new UiError('skill-catalog.invalid_read'));return;}
  setEditor(undefined);setHistoryVersion(undefined);setNotice({kind:path.endsWith('/adoptions')?'adopted':'saved',id:result.id});if(!path.endsWith('/adoptions'))callbacks.current.onSelect(result.id);setTick(value=>value+1);
  try{await callbacks.current.onRefresh();}catch{if(alive.current)action.setError(new UiError('skill-catalog.refresh_failed'));}
 }
 async function submit(){
  if(!editor||!writeAllowed)return;let path:string,body:Record<string,unknown>,method='POST';
  if(editor.kind==='publish'||editor.kind==='update'){
   const fixed=editor.source!;path=editor.kind==='publish'?'/skill-catalog':itemPath(editor.entry!.id);method=editor.kind==='publish'?'POST':'PATCH';body={project_id:project,knowledge_id:fixed.id,expected_knowledge_version:fixed.version,published_revision:fixed.published_revision,published_sha256:fixed.published_sha256,expected_version:editor.entry?.version||0,domain:editor.domain,scenarios,tags,reason:editor.reason,acknowledge_sharing:true};
  }else{path=itemPath(editor.entry!.id)+(editor.kind==='adopt'?'/adoptions':'/commands');body={project_id:project,expected_version:editor.entry!.version,reason:editor.reason,...(editor.kind==='adopt'?{origin_sha256:editor.entry!.origin.sha256,acknowledge_draft:true}:{action:'retire'})};}
  setNotice(undefined);await completed(await action.run(path,body,method),path);
 }
 async function rebase(){
  if(!editor)throw new UiError('skill-catalog.refresh_failed');
  const latest=editor.entry?await detail.read():undefined;if(editor.entry&&(!latest||latest.id!==editor.entry.id))throw new UiError('skill-catalog.refresh_failed');
  const freshSource=managing?await source.read():undefined;if(managing&&(!freshSource||!publishedSource(freshSource)))throw new UiError('skill-catalog.source_unavailable');
  const page=await list.read();if(!page)throw new UiError('skill-catalog.refresh_failed');
  if(latest&&!(editor.kind==='adopt'?latest.can_adopt&&latest.allowed_actions.includes('adopt'):latest.can_manage&&latest.allowed_actions.includes(editor.kind==='update'?'update':'retire'))||managing&&!page.can_manage)throw new UiError('skill-catalog.access_changed');
  setEditor({...editor,entry:latest||editor.entry,source:freshSource||editor.source,ack:false});
 }
 if(forbidden)return <section className="panel"><h2>{t('title')}</h2><ErrorNotice error={toolError(forbidden)} onRetry={()=>{setForbidden(undefined);setTick(value=>value+1);}}/></section>;
 return <div className="skill-catalog" data-detail={!!selected} data-sharing={editor?.kind==='publish'}>
  {(action.error&&!editor||action.pending||notice)&&<div className="sc-recovery">{!editor&&<WriteNotice action={action} onRebase={rebase}/>} {action.pending&&<><strong>{t('original_operation')}</strong><p>{t('recovery_hint')}</p><p className="sc-raw">{String(action.pending.body.reason||'')}</p><details className="sc-disclosure"><summary>{t('fixed_request')}</summary><pre className="sc-content">{JSON.stringify(action.pending.body,null,2)}</pre></details><button data-catalog-restore type="button" disabled={action.busy} onClick={()=>{const request=action.pending;if(request)void action.run(request.path,request.body,request.method).then(value=>completed(value,request.path));}}>{t('restore')}</button></>}{notice&&<p className="sc-success" role="status">{t(notice.kind)}{notice.kind==='adopted'&&<> <a data-catalog-draft href={'?'+new URLSearchParams({project,view:'knowledge',sub:'project-knowledge',record:notice.id,knowledge_kind:'skill'})}>{t('open_draft')}</a></>}</p>}</div>}
  <section className="panel sc-list" aria-label={t('title')} aria-busy={list.loading}>
   <div className="sc-heading"><div><h2>{t('title')}</h2><p className="sc-muted">{t('intro')}</p></div><BookOpen size={20} aria-hidden="true"/></div>
   <form className="sc-filter" data-catalog-filter onSubmit={event=>{event.preventDefault();if(canLeave()&&new TextEncoder().encode(search.q).length<=200&&(search.domain===''||catalogText(search.domain,80))&&(search.scenario===''||catalogText(search.scenario,80)))onFilter(search);}}><div className="sc-filter-grid">
    <Field label={t('domain')}><input name="catalog_domain" value={search.domain} maxLength={80} disabled={locked} onChange={event=>setSearch({...search,domain:event.target.value})}/></Field><Field label={t('scenario')}><input name="catalog_scenario" value={search.scenario} maxLength={80} disabled={locked} onChange={event=>setSearch({...search,scenario:event.target.value})}/></Field>
    <Field label={t('state')}><select name="catalog_state" value={search.state} disabled={locked} onChange={event=>setSearch({...search,state:event.target.value as SkillCatalogFilters['state']})}><option value="all">{t('all_states')}</option><option value="published">{t('state.published')}</option><option value="retired">{t('state.retired')}</option></select></Field>
    <div className="sc-filter-search"><input name="catalog_q" aria-label={t('keywords')} placeholder={t('keywords')} maxLength={200} value={search.q} disabled={locked} onChange={event=>setSearch({...search,q:event.target.value})}/><button disabled={locked}>{t('filter')}</button></div>
   </div></form>
   <div className="sc-actions">{list.value?.can_manage&&<button data-catalog-publish className="primary" type="button" disabled={locked||!!editor} onClick={()=>{action.clearConflict();setEditor(makeEditor('publish'));}}>{t('share')}</button>}<button className="icon-button" type="button" aria-label={t('refresh')} disabled={locked||list.loading} onClick={()=>void list.read()}><RefreshCw size={16}/></button></div>
   <p className="sc-muted">{t('approval_boundaries')}</p>{sourceId&&list.value&&!list.value.can_manage&&<p className="sc-readonly">{t('sharing_unavailable')}</p>}
   {Boolean(list.error)&&<ErrorNotice error={toolError(list.error)} onRetry={()=>void list.read()}/>}{list.loading&&!list.value&&<Loading/>}
   <ul className="sc-list-items">{list.value?.items.map(item=><li key={item.id}><button data-catalog-select={item.id} className="sc-entry" type="button" aria-current={selected===item.id} disabled={locked} onClick={()=>{if(canLeave())onSelect(item.id);}}><strong>{item.title}</strong><p>{item.summary}</p><span className="sc-meta"><span>{item.domain}</span><span>{t('version',{value:formatNumber(item.version)})}</span><span>{t('state.'+item.state)}</span></span><span className="sc-tags">{item.scenarios.map(value=><span key={value}>{value}</span>)}</span><small>{t('eligibility.'+item.eligibility)}</small></button></li>)}</ul>
   {list.value&&!list.value.items.length&&!list.loading&&<Empty title={t('empty')} description={t('empty_hint')}/>}
   {list.value&&<div className="sc-pager"><span>{t('loaded',{loaded:formatNumber(list.value.items.length),total:formatNumber(list.value.total)})}</span>{list.value.next_cursor&&<button data-catalog-more type="button" disabled={locked||list.loading} onClick={()=>void list.read(true)}>{t('more')}</button>}</div>}
  </section>
  <section className="panel sc-detail" aria-label={t('details')} aria-busy={detail.loading}>
   {editor?.kind==='publish'?<CatalogEditor editor={editor} edit={edit} cancel={()=>setEditor(undefined)} action={action} writeAllowed={!!writeAllowed} submit={submit} source={source} sources={sources} project={project} rebase={rebase}/>:selected?<><div className="sc-heading"><button className="sc-back" type="button" disabled={locked} onClick={()=>{if(canLeave())onSelect('');}}>{t('back')}</button><span className="sc-muted">{current?t('version',{value:formatNumber(current.version)}):selected}</span><button className="icon-button" type="button" aria-label={t('close')} disabled={locked} onClick={()=>{if(canLeave())onSelect('');}}><X size={17}/></button></div>
    {Boolean(detail.error)&&<ErrorNotice error={toolError(detail.error)} onRetry={()=>void detail.read()}/>}{detail.loading&&!current&&<Loading/>}
    {shown&&<><h2>{shown.title}</h2><p className="sc-muted">{shown.summary}</p><div className="sc-meta"><span>{shown.domain}</span><span>{t('state.'+shown.state)}</span><span>{t('version',{value:formatNumber(shown.version)})}</span></div><div className="sc-tags">{[...shown.scenarios,...shown.tags].map((value,index)=><span key={index}>{value}</span>)}</div>
     <p className={shown.eligibility==='current'?'sc-muted':'sc-warning'}>{t('eligibility.'+shown.eligibility)}</p>{historical&&<div className="sc-callout"><p>{t('historical')}</p><button data-catalog-current type="button" disabled={locked} onClick={()=>setHistoryVersion(undefined)}>{t('show_current')}</button></div>}
     <pre data-catalog-content className="sc-content">{shown.content}</pre><p className="sc-muted">{t('no_permissions')}</p>
     <details className="sc-disclosure"><summary>{t('origin')}</summary><div className="sc-source"><p>{t('origin_revision',{value:formatNumber(shown.origin.revision)})}</p><code>{shown.origin.project_id} · {shown.origin.knowledge_id}</code><code>{shown.origin.sha256}</code><p>{t('reviewed_by',{value:shown.reviewed_by})} · {formatDateTime(shown.updated_at)}</p><p className="sc-raw">{shown.reason}</p><p className="sc-muted">{t('origin_boundary')}</p></div></details>
    </>}
    <details className="sc-disclosure" data-catalog-history onToggle={event=>setHistoryOpen(event.currentTarget.open)}><summary>{t('history')}</summary>{historyOpen&&<>{history.loading&&<Loading/>}{Boolean(history.error)&&<ErrorNotice error={toolError(history.error)} onRetry={()=>void history.read()}/>}<div className="sc-history">{history.value?.items.map(item=><button data-catalog-revision={item.version} key={item.version} type="button" disabled={locked} aria-pressed={shown?.version===item.version} onClick={()=>setHistoryVersion(item.version)}><strong>{t('version',{value:formatNumber(item.version)})} · {item.title}</strong><span className="sc-muted">{item.domain} · {formatDateTime(item.updated_at)}</span></button>)}</div>{history.value?.next_cursor&&<button type="button" disabled={locked||history.loading} onClick={()=>void history.read(true)}>{t('more_history')}</button>}</>}</details>
    {current&&!historical&&<div className="sc-actions">{current.can_adopt&&current.allowed_actions.includes('adopt')&&<button data-catalog-adopt className="primary" type="button" disabled={locked||stale||!!editor} onClick={()=>{action.clearConflict();setEditor(makeEditor('adopt',current));}}>{t('adopt')}</button>}{current.can_manage&&current.allowed_actions.includes('update')&&(current.origin.project_id===project?<button data-catalog-update type="button" disabled={locked||stale||!!editor} onClick={()=>{action.clearConflict();setEditor(makeEditor('update',current));}}>{t('update')}</button>:<a data-catalog-manage-source className="sc-link" aria-disabled={locked||stale||!!editor} href={'?'+new URLSearchParams({project:current.origin.project_id,view:'knowledge',sub:'skill-catalog',record:current.id})} onClick={event=>{if(locked||stale||editor||!canLeave())event.preventDefault();}}>{t('manage_source')}</a>)}{current.can_manage&&current.allowed_actions.includes('retire')&&<button data-catalog-retire type="button" disabled={locked||stale||!!editor} onClick={()=>{action.clearConflict();setEditor(makeEditor('retire',current));}}>{t('retire')}</button>}</div>}
    {editor&&<CatalogEditor editor={editor} edit={edit} cancel={()=>setEditor(undefined)} action={action} writeAllowed={!!writeAllowed} submit={submit} source={source} sources={sources} project={project} rebase={rebase}/>}
   </>:<Empty title={t('no_selection')} description={t('selection_hint')}/>}
  </section>
 </div>;
}
type SourceRead=ReturnType<typeof useCatalogRead<KnowledgeDetail>>;
type SourcePage=ReturnType<typeof useCatalogPage<KnowledgeSummary,KnowledgePage>>;
function CatalogEditor({editor,edit,cancel,action,writeAllowed,submit,source,sources,project,rebase}:{editor:Editor;edit:(patch:Partial<Editor>)=>void;cancel:()=>void;action:ReturnType<typeof useWriteAction>;writeAllowed:boolean;submit:()=>Promise<void>;source:SourceRead;sources:SourcePage;project:string;rebase:()=>Promise<void>}){
 const managing=editor.kind==='publish'||editor.kind==='update',fixed=editor.source,locked=action.busy||!!action.pending;
 return <form className="sc-editor" data-catalog-editor onSubmit={event=>{event.preventDefault();void submit();}}><div className="sc-editor-heading"><h3>{t(editor.kind==='publish'?'share':editor.kind==='update'?'update':editor.kind==='adopt'?'adopt':'retire')}</h3><button type="button" disabled={locked} onClick={cancel}>{t('cancel')}</button></div>
  <WriteNotice action={action} onRebase={rebase}/><p className="sc-muted">{t(managing?'sharing_hint':editor.kind==='adopt'?'adoption_hint':'retirement_hint')}</p><WriteFields action={action}>
   {managing&&<><Field label={t('source_skill')}><select name="catalog_source" value={editor.knowledgeId} disabled={editor.kind==='update'} onChange={event=>edit({knowledgeId:event.target.value,source:undefined,ack:false})}><option value="">{t('choose_source')}</option>{sources.value?.items.filter(eligibleSource).map(item=><option value={item.id} key={item.id}>{item.published_title||item.title} · {t('origin_revision',{value:formatNumber(item.published_revision)})}</option>)}{editor.knowledgeId&&!sources.value?.items.some(item=>item.id===editor.knowledgeId)&&<option value={editor.knowledgeId}>{editor.knowledgeId}</option>}</select></Field>
    {sources.loading&&<Loading/>}{Boolean(sources.error)&&<ErrorNotice error={toolError(sources.error)} onRetry={()=>void sources.read()}/>}{sources.value?.next_cursor&&<button type="button" disabled={sources.loading||locked} onClick={()=>void sources.read(true)}>{t('more_sources')}</button>}{sources.value&&!sources.value.items.some(eligibleSource)&&!sources.loading&&<p className="sc-muted">{t('no_sources')} <a href={'?'+new URLSearchParams({project,view:'knowledge',sub:'project-knowledge',knowledge_kind:'skill'})}>{t('open_project_skills')}</a></p>}
    {source.loading&&!fixed&&<Loading/>}{Boolean(source.error)&&<ErrorNotice error={toolError(source.error)} onRetry={()=>void source.read()}/>}
    {fixed&&<div className="sc-callout"><strong>{fixed.published?.title||fixed.title}</strong><p>{t('origin_revision',{value:formatNumber(fixed.published_revision)})} · {t('source_version',{value:formatNumber(fixed.version)})}</p>{fixed.has_draft&&<p>{t('draft_does_not_replace')}</p>}{!publishedSource(fixed)&&<p className="sc-warning">{t('source_unavailable')}</p>}<details className="sc-disclosure"><summary>{t('review_shared_text')}</summary><pre data-catalog-published-source className="sc-content">{fixed.published?.content||''}</pre><code>{fixed.published_sha256}</code></details></div>}
    <Field label={t('domain')} hint={t('domain_hint')}><input name="share_domain" maxLength={80} value={editor.domain} onChange={event=>edit({domain:event.target.value,ack:false})}/></Field><Field label={t('scenarios')} hint={t('scenarios_hint')}><input name="share_scenarios" value={editor.scenarios} onChange={event=>edit({scenarios:event.target.value,ack:false})}/></Field><Field label={t('tags')} hint={t('tags_hint')}><input name="share_tags" value={editor.tags} onChange={event=>edit({tags:event.target.value,ack:false})}/></Field>
   </>}
   {editor.entry&&<div className="sc-callout"><strong>{editor.entry.title}</strong><p>{t('version',{value:formatNumber(editor.entry.version)})}</p><code>{editor.entry.origin.sha256}</code></div>}
   <Field label={t('reason')} hint={t('reason_hint')}><textarea name="catalog_reason" rows={2} maxLength={2000} value={editor.reason} onChange={event=>edit({reason:event.target.value})}/></Field>
   {editor.kind!=='retire'&&<label className="sc-check"><input name={managing?'acknowledge_sharing':'acknowledge_draft'} type="checkbox" checked={editor.ack} onChange={event=>edit({ack:event.target.checked})}/>{t(managing?'sharing_ack':'adoption_ack')}</label>}
   <WriteButton action={action} disabled={!writeAllowed}>{t(editor.kind==='publish'?'confirm_share':editor.kind==='update'?'confirm_update':editor.kind==='adopt'?'confirm_adopt':'confirm_retire')}</WriteButton>
  </WriteFields>
 </form>;
}
