import {useEffect, useId, useLayoutEffect, useRef, useState} from 'react';
import {FileJson, Upload} from 'lucide-react';
import {renderDiagnostic, tr as t, UiError, useLocale} from './i18n';
import './ToolInventoryField.css';

/** Local file reading is a draft input. Only the existing parsed profile is
 * sent on explicit registration; neither file bytes nor its name are uploaded. */
export function ToolInventoryField({value,error,disabled,onChange,onReading}:{
  value:string;error:unknown;disabled:boolean;onChange:(text:string)=>void;onReading:(reading:boolean)=>void;
}) {
  useLocale();
  const id=useId(),epoch=useRef(0),change=useRef(onChange);
  useLayoutEffect(()=>{change.current=onChange;},[onChange]);
  const [reading,setReading]=useState(false),[failure,setFailure]=useState<unknown>(''),[filename,setFilename]=useState('');
  useEffect(()=>()=>{epoch.current++;},[]);
  const problem=failure||error;
  function edit(text:string){epoch.current++;setReading(false);onReading(false);setFailure('');setFilename('');onChange(text);}
  async function read(file?:File){
    if(!file)return;
    const generation=++epoch.current;
    setFailure('');setFilename('');
    if(!/\.json$/i.test(file.name)||file.size>256*1024){
      setReading(false);onReading(false);
      setFailure(new UiError(file.size>256*1024?'tool-workspace.file_too_large':'tool-workspace.file_json_only'));
      return;
    }
    setReading(true);onReading(true);
    try{
      const text=await file.text();
      if(generation!==epoch.current)return;
      change.current(text);setFilename(file.name);
    }catch{
      if(generation===epoch.current)setFailure(new UiError('tool-workspace.file_read_failed'));
    }finally{
      if(generation===epoch.current){setReading(false);onReading(false);}
    }
  }
  return <div className="tool-inventory-field">
    <div className="tool-inventory-heading">
      <label htmlFor={id}>{t('development.tools.local_non_secret_tool_inventory')}</label>
      <label className="tool-file-button"><Upload size={14} aria-hidden="true"/>{t('tool-workspace.import_file')}
        <input data-tool-inventory-file type="file" accept=".json,application/json" aria-label={t('tool-workspace.import_file')} disabled={disabled} onChange={event=>{const file=event.currentTarget.files?.[0];event.currentTarget.value='';void read(file);}}/>
      </label>
    </div>
    <p id={id+'-help'} className="muted">{t('tool-workspace.inventory_help')}</p>
    <textarea id={id} rows={6} value={value} disabled={disabled} spellCheck={false} aria-invalid={!!problem} aria-describedby={id+'-help'+(problem?' '+id+'-error':'')} onChange={event=>edit(event.target.value)} placeholder={'{"items":[{"profile":{…},"available":true,"reason":""}]}'}/>
    <div className="tool-file-status" role="status">{reading?t('tool-workspace.reading_file'):filename&&<><FileJson size={14} aria-hidden="true"/>{filename}</>}</div>
    {!!problem&&<p id={id+'-error'} data-tool-import-error className="tool-inline-error" role="alert">{renderDiagnostic(problem)}</p>}
  </div>;
}
