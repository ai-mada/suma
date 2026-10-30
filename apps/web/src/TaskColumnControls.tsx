import {useLayoutEffect,useRef,useState} from 'react';
import {ArrowDown,ArrowUp,Columns3,RotateCcw} from 'lucide-react';
import {tr,useLocale} from './i18n';
import {taskColumnIDs,taskColumns,moveTaskColumn,toggleTaskColumn,type TaskColumnID} from './task-columns-model';
import './TaskColumnControls.css';
const t=(key:string,values?:Record<string,unknown>)=>tr('task-columns.'+key,values);
export function TaskColumnControls({columns,disabled=false,onChange}:{columns?:string[];disabled?:boolean;onChange:(columns:string[]|undefined)=>void|boolean}){
 useLocale();const [open,setOpen]=useState(false),host=useRef<HTMLDetailsElement>(null),focus=useRef<{id:TaskColumnID;direction:'up'|'down'}|undefined>(undefined),visible=taskColumns(columns);
 useLayoutEffect(()=>{const target=focus.current;focus.current=undefined;if(!target)return;const button=host.current?.querySelector<HTMLButtonElement>('[data-column-'+target.direction+'="'+target.id+'"]'),checkbox=host.current?.querySelector<HTMLInputElement>('[data-column-check="'+target.id+'"]');if(button&&!button.matches(':disabled'))button.focus();else if(checkbox&&!checkbox.matches(':disabled'))checkbox.focus();},[columns]);
 const ordered=visible?[...visible,...taskColumnIDs.filter(id=>!visible.includes(id))]:[];
 function move(id:TaskColumnID,direction:-1|1){if(disabled)return;focus.current={id,direction:direction===-1?'up':'down'};const next=moveTaskColumn(columns,id,direction);if(next&&onChange(next)===false)focus.current=undefined;}
 return <details className="task-column-controls" ref={host} onToggle={event=>setOpen(event.currentTarget.open)}><summary><Columns3 size={16} aria-hidden="true"/>{t('title')}</summary>{open&&<div className="task-column-panel">
  {!visible?<p role="alert">{t('invalid')}</p>:<fieldset disabled={disabled}><legend className="sr-only">{t('title')}</legend>{ordered.map(id=>{const index=visible.indexOf(id),shown=index>=0;return <div key={id} className="task-column-option"><label><input type="checkbox" data-column-check={id} checked={shown} disabled={id==='title'} onChange={event=>{if(!disabled){const next=toggleTaskColumn(columns,id,event.target.checked);if(next)onChange(next);}}}/><span>{t('column.'+id)}{id==='title'&&<small>{t('fixed')}</small>}</span></label>{id!=='title'&&<span className="task-column-moves"><button type="button" className="icon-button" data-column-up={id} disabled={!shown||index<=1} aria-label={t('move_up',{name:t('column.'+id)})} onClick={()=>move(id,-1)}><ArrowUp size={15} aria-hidden="true"/></button><button type="button" className="icon-button" data-column-down={id} disabled={!shown||index===visible.length-1} aria-label={t('move_down',{name:t('column.'+id)})} onClick={()=>move(id,1)}><ArrowDown size={15} aria-hidden="true"/></button></span>}</div>;})}</fieldset>}
  <p className="task-column-hint">{t('hint')}</p><button type="button" data-columns-default disabled={disabled} onClick={()=>onChange(undefined)}><RotateCcw size={14} aria-hidden="true"/>{t('defaults')}</button>
 </div>}</details>;
}
