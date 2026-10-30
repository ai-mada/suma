import {useEffect,useLayoutEffect,useRef,useState} from 'react';
import {DiffEditor} from '@monaco-editor/react';
import type {editor} from 'monaco-editor';
import {prepareMonaco} from './monaco-local';
import type {RevisionEditorProps} from './SkillCatalogRevisionCompare';

export const prepareRevisionEditor=prepareMonaco;
export default function SkillCatalogRevisionEditor({left,right,scope,leftLabel,rightLabel,label}:RevisionEditorProps){
 const readTheme=()=>document.documentElement.dataset.theme==='dark'?'vs-dark':'light';
 const [theme,setTheme]=useState(readTheme),[sideBySide,setSideBySide]=useState(false),container=useRef<HTMLDivElement>(null);
 useEffect(()=>{const observer=new MutationObserver(()=>setTheme(readTheme()));observer.observe(document.documentElement,{attributes:true,attributeFilter:['data-theme']});return()=>observer.disconnect();},[]);
 useLayoutEffect(()=>{
  const element=container.current;if(!element)return;let live=true;
  const measure=(width:number)=>{if(live)setSideBySide(width>=600);};
  measure(element.getBoundingClientRect().width);
  const observer=typeof ResizeObserver==='undefined'?undefined:new ResizeObserver(entries=>{const entry=entries.find(value=>value.target===element);if(entry)measure(entry.contentRect.width);});
  observer?.observe(element);return()=>{live=false;observer?.disconnect();};
 },[]);
 const path=(item:typeof left,side:string)=>'suma-skill-compare:///'+encodeURIComponent(scope)+'/'+side+'/'+item.version+'/'+item.origin.revision+'/'+item.origin.sha256;
 const originalPath=path(left,'left'),modifiedPath=path(right,'right');
 const owned=useRef<{widget:editor.IStandaloneDiffEditor;models:editor.ITextModel[];uris:Set<string>}|undefined>(undefined);
 // The installed React wrapper releases models before its widget by default.
 // Detach our widget during layout cleanup, before the wrapper's passive
 // cleanup, then release only this comparison's fixed, private model URIs.
 useLayoutEffect(()=>()=>{
  const value=owned.current;owned.current=undefined;if(!value)return;
  if(value.widget.getModel())value.widget.setModel(null);
  for(const model of value.models)if(value.uris.has(model.uri.toString())&&!model.isDisposed())model.dispose();
 },[originalPath,modifiedPath]);
 return <div ref={container} style={{height:'100%',width:'100%',minWidth:0}}><DiffEditor key={originalPath+'\0'+modifiedPath} height="100%" original={left.content} modified={right.content} originalModelPath={originalPath} modifiedModelPath={modifiedPath} theme={theme} keepCurrentOriginalModel keepCurrentModifiedModel onMount={(widget,runtime)=>{const pair=widget.getModel();if(pair)owned.current={widget,models:[pair.original,pair.modified],uris:new Set([runtime.Uri.parse(originalPath).toString(),runtime.Uri.parse(modifiedPath).toString()])};}} options={{ariaLabel:label,originalAriaLabel:leftLabel,modifiedAriaLabel:rightLabel,readOnly:true,domReadOnly:true,originalEditable:false,automaticLayout:true,renderSideBySide:sideBySide,useInlineViewWhenSpaceIsLimited:true,enableSplitViewResizing:false,accessibilitySupport:'on',wordWrap:'on',fontSize:14,lineHeight:22,minimap:{enabled:false},scrollBeyondLastLine:false,renderOverviewRuler:false}}/></div>;
}
