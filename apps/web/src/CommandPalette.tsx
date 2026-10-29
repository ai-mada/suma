import {useEffect,useId,useMemo,useRef,useState,type KeyboardEvent} from 'react';
import {ArrowUpRight,FileText,Search} from 'lucide-react';
import {useApi} from './api-context';
import {formatNumber,tr,useLocale,UiError} from './i18n';
import {Modal,ErrorNotice} from './ui';
import type {WorkbenchView} from './navigation';
import {pageMatches,palettePages,paletteQueryValid,paletteSearchResult,type CommandPaletteTarget,type PalettePage,type PaletteSearchResult,type PaletteSearchRow} from './command-palette-model';
import './CommandPalette.css';

export type {CommandPaletteTarget} from './command-palette-model';
export type CommandPaletteProps={
 open:boolean;onClose:()=>void;project:string;projectName?:string;view:WorkbenchView;sub:string;
 userID:string;organizationID:string;
 onNavigate:(target:CommandPaletteTarget)=>boolean;
 onSource:(source:unknown)=>boolean;
};
type RecentPages={scope:string;ids:string[]};
type Option={id:string;label:string;group:'recent'|'context'|'pages'|'results';page?:PalettePage;result?:PaletteSearchRow};
type ReadState={query:string;result?:PaletteSearchResult;error?:unknown};
const t=(key:string,parameters?:Record<string,unknown>)=>tr('command-palette.'+key,parameters);

export function CommandPalette(props:CommandPaletteProps){
 useLocale();
 const scope=JSON.stringify([props.userID,props.organizationID,props.project]);
 // Only static page identifiers are retained, never private search titles or
 // result bodies. Changing the loaded account or project discards this memory.
 const recent=useRef<RecentPages>({scope,ids:[]});
 if(recent.current.scope!==scope)recent.current={scope,ids:[]};
 return <Modal open={props.open} onClose={props.onClose} title={t('title')} description={t('description')}>
  {props.open&&<PaletteBody key={scope} {...props} recent={recent.current}/>}
 </Modal>;
}

function PaletteBody({recent,...props}:CommandPaletteProps&{recent:RecentPages}){
 const locale=useLocale(),api=useApi();
 const [query,setQuery]=useState(''),[read,setRead]=useState<ReadState>(),[retry,setRetry]=useState(0),[selection,setSelection]=useState<string>();
 const input=useRef<HTMLInputElement>(null),request=useRef(0),liveQuery=useRef('');
 const listID=useId(),hintID=useId(),scopeID=useId();
 const trimmed=query.trim(),valid=paletteQueryValid(trimmed);
 liveQuery.current=trimmed;

 useEffect(()=>{
  const generation=++request.current,controller=new AbortController();
  if(!trimmed||!props.project||!valid){setRead(undefined);return()=>controller.abort();}
  setRead(undefined);
  const timer=setTimeout(()=>{
   void api<unknown>('/search',{project_id:props.project,query:trimmed,types:[]},undefined,undefined,controller.signal)
    .then(value=>{
     if(controller.signal.aborted||generation!==request.current||liveQuery.current!==trimmed)return;
     let result:PaletteSearchResult;
     try{result=paletteSearchResult(props.project,value);}catch{throw new UiError('command-palette.invalid_response');}
     setRead({query:trimmed,result});
    }).catch(error=>{
     if(!controller.signal.aborted&&generation===request.current&&liveQuery.current===trimmed)setRead({query:trimmed,error});
    });
  },250);
  return()=>{clearTimeout(timer);controller.abort();};
 },[api,trimmed,props.project,valid,retry]);

 const currentRead=read?.query===trimmed?read:undefined;
 const available=useMemo(()=>palettePages.filter(page=>!page.projectRequired||Boolean(props.project)),[props.project]);
 const options=useMemo<Option[]>(()=>{
  const matching=available.filter(page=>pageMatches(tr(page.labelKey),tr(page.keywordsKey),trimmed));
  const recentIDs=trimmed?[]:recent.ids;
  const pages=matching.map(page=>({id:'page:'+page.id,label:tr(page.labelKey),page,
   group:recentIDs.includes(page.id)?'recent' as const:page.target.view===props.view?'context' as const:'pages' as const}));
  const order={recent:0,context:1,pages:2,results:3};
  pages.sort((left,right)=>order[left.group]-order[right.group]||
   (left.group==='recent'?recentIDs.indexOf(left.page.id)-recentIDs.indexOf(right.page.id):0));
  const results=(currentRead?.result?.items||[]).map(result=>({id:'source:'+result.id,label:result.title,result,group:'results' as const}));
  return [...pages,...results];
 },[available,trimmed,props.view,currentRead,recent,locale]);
 const active=options.find(option=>option.id===selection)||options[0];
 const activeIndex=active?options.indexOf(active):-1;
 const optionID=(index:number)=>listID+'-'+index;
 const busy=Boolean(trimmed&&props.project&&valid&&!currentRead);
 const error=!valid?new UiError('command-palette.query_limit'):currentRead?.error;

 useEffect(()=>{
  if(activeIndex>=0)document.getElementById(optionID(activeIndex))?.scrollIntoView?.({block:'nearest'});
 },[activeIndex,listID]);

 function activate(option:Option|undefined){
  if(!option)return;
  const allowed=option.page?props.onNavigate(option.page.target):option.result&&currentRead?.query===liveQuery.current?props.onSource(option.result.source):false;
  if(allowed){
   if(option.page)recent.ids=[option.page.id,...recent.ids.filter(id=>id!==option.page!.id)].slice(0,5);
   props.onClose();
  }else input.current?.focus();
 }
 function onKeyDown(event:KeyboardEvent<HTMLInputElement>){
  if(event.nativeEvent.isComposing)return;
  const recognized=['ArrowDown','ArrowUp','Home','End','Enter'].includes(event.key);
  if(!recognized)return;
  event.preventDefault();event.stopPropagation();
  if(event.key==='Enter'){activate(active);return;}
  if(!options.length)return;
  const index=event.key==='Home'?0:event.key==='End'?options.length-1:event.key==='ArrowDown'?(activeIndex+1)%options.length:(activeIndex-1+options.length)%options.length;
  setSelection(options[index].id);
 }
 const groups=['recent','context','pages','results'] as const;
 return <div className="command-palette" data-command-palette>
  <div className="command-palette-scope" id={scopeID}><span>{t('scope')}</span><strong>{props.projectName||props.project||t('choose_project')}</strong></div>
  <div className="command-palette-input"><Search size={19} aria-hidden="true"/>
   <input ref={input} autoFocus role="combobox" aria-label={t('input')} aria-expanded="true" aria-autocomplete="list" aria-controls={listID} aria-activedescendant={activeIndex>=0?optionID(activeIndex):undefined} aria-describedby={scopeID+' '+hintID} value={query} maxLength={500} placeholder={t('placeholder')} onChange={event=>{liveQuery.current=event.target.value.trim();setQuery(event.target.value);setSelection(undefined);}} onKeyDown={onKeyDown}/>
   {query&&<button type="button" className="command-palette-clear" onClick={()=>{liveQuery.current='';setQuery('');setSelection(undefined);input.current?.focus();}}>{t('clear')}</button>}
  </div>
  {Boolean(error)&&<ErrorNotice error={error} onRetry={valid&&props.project?()=>setRetry(value=>value+1):undefined}/>}
  <div className="command-palette-options" id={listID} role="listbox" aria-label={t('results')} aria-busy={busy}>
   {groups.map(group=>{
    const rows=options.filter(option=>option.group===group);
    return rows.length?<div role="group" aria-label={t('group.'+group)} key={group}>
     <div className="command-palette-group" aria-hidden="true">{t('group.'+group)}</div>
     {rows.map(option=>{
      const index=options.indexOf(option),chosen=option.id===active?.id;
      return <div key={option.id} id={optionID(index)} role="option" aria-selected={chosen} className="command-palette-option" onMouseMove={()=>setSelection(option.id)} onMouseDown={event=>event.preventDefault()} onClick={()=>activate(option)}>
       {option.result?<FileText size={17} aria-hidden="true"/>:<ArrowUpRight size={17} aria-hidden="true"/>}
       <div><strong>{option.label}</strong>{option.result&&<><small className="command-palette-snippet">{option.result.snippet}</small><small>{t('kind.'+option.result.kind)}{option.result.revision?<> · {t('revision',{version:formatNumber(option.result.revision)})}</>:null}</small></>}</div>
       {option.page?.target.view===props.view&&option.page.target.sub===props.sub?<span className="command-palette-current">{t('current')}</span>:null}
      </div>;
     })}
    </div>:null;
   })}
  </div>
  <div className="command-palette-status" role="status" aria-live="polite">
   {busy?t('searching'):!error&&trimmed&&!options.length?t('empty'):!props.project?t('choose_project'):''}
   {currentRead?.result?.truncated&&<span>{t('bounded',{limit:formatNumber(currentRead.result.limit),total:formatNumber(currentRead.result.total)})}</span>}
  </div>
  <div className="command-palette-footer" id={hintID}><span><kbd>↑</kbd><kbd>↓</kbd> {t('move')} <kbd>Enter</kbd> {t('open')}</span><span><kbd>Esc</kbd> {t('close')}</span></div>
 </div>;
}
