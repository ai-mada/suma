import {useEffect,useRef,useState} from 'react';
import {ExternalLink} from 'lucide-react';
import type {WorkspaceMetadata} from '../../../packages/contracts/src';
import {useApi,useApiView} from './api-context';
import {validWorkspaceMetadata,type DesktopDirectoryRequest} from './desktop-workspace-model';
import {tr,useLocale} from './i18n';

type Props={project:string;metadata:WorkspaceMetadata;active:boolean;disabled?:boolean};
type CursorEditorRequest=DesktopDirectoryRequest&{expected_binding_sha256:string};
type OpenCursorEditor=(request:CursorEditorRequest)=>Promise<unknown>;
const id=/^[a-f0-9]{32}$/;
const codes=['desktop_unavailable','editor_unavailable','editor_launch_failed','workspace_unavailable','access_changed','busy'] as const;
type Status='requested'|'request_unconfirmed'|typeof codes[number];
const t=(key:string)=>tr('desktop-cursor-editor.'+key);

function cursorEditorBridge():{openCursorEditor:OpenCursorEditor}|undefined{
 const raw=(window as unknown as {sumaDesktop?:unknown}).sumaDesktop;
 if(!raw||typeof raw!=='object'||Array.isArray(raw))return;
 const bridge=raw as Record<string,unknown>;
 if(bridge.platform==='win32'&&bridge.version==='1.0.0'&&typeof bridge.openCursorEditor==='function')return raw as {openCursorEditor:OpenCursorEditor};
}

function readStatus(raw:unknown):Status{
 if(!raw||typeof raw!=='object'||Array.isArray(raw))return 'request_unconfirmed';
 const result=raw as Record<string,unknown>;
 const keys=Object.keys(result).sort().join(',');
 if(keys==='status'&&result.status==='requested')return 'requested';
 if(keys==='code,status'&&result.status==='failed'){
  const code=codes.find(code=>code===result.code);
  if(code)return code;
 }
 return 'request_unconfirmed';
}

/** The desktop process resolves and authorizes the folder. The renderer only
 * supplies the active view identifiers and discards receipts from old views. */
export function DesktopCursorEditorButton({project,metadata,active,disabled}:Props){
 const api=useApi(),view=useApiView(),bridge=cursorEditorBridge();
 const owner=useRef({api,bridge,epoch:0});
 if(owner.current.api!==api||owner.current.bridge!==bridge)owner.current={api,bridge,epoch:owner.current.epoch+1};
 if(!active||!bridge||!view||![project,view.userID,view.organizationID].every(value=>id.test(value))||
    !validWorkspaceMetadata(metadata,project)||!metadata.configured||!metadata.desktop_binding_available||!metadata.binding_sha256)return null;
 return <CursorEditorAction key={JSON.stringify([project,view.userID,view.organizationID,metadata.binding_sha256,owner.current.epoch])}
  bridge={bridge} scope={{project_id:project,user_id:view.userID,tenant_id:view.organizationID,expected_binding_sha256:metadata.binding_sha256}} disabled={disabled}/>;
}

function CursorEditorAction({bridge,scope,disabled}:Readonly<{
 bridge:{openCursorEditor:OpenCursorEditor};scope:Omit<CursorEditorRequest,'locale'>;disabled?:boolean;
}>){
 const locale=useLocale(),mounted=useRef(false),inFlight=useRef(false);
 const [pending,setPending]=useState(false),[status,setStatus]=useState<Status>();
 useEffect(()=>{mounted.current=true;return()=>{mounted.current=false;};},[]);
 async function open(){
  if(disabled||inFlight.current)return;
  inFlight.current=true;setPending(true);setStatus(undefined);
  let next:Status;
  try{next=readStatus(await bridge.openCursorEditor({...scope,locale}));}
  catch{next='request_unconfirmed';}
  finally{inFlight.current=false;}
  if(mounted.current){setPending(false);setStatus(next);}
 }
 return <div className="inline wrap" data-desktop-cursor-editor>
  <button type="button" disabled={disabled} aria-disabled={pending||disabled||undefined} aria-busy={pending} onClick={()=>void open()}>
   <ExternalLink size={14} aria-hidden="true"/>{t(pending?'requesting':'open')}
  </button>
  <small role="status" className="muted">{status?t(status):null}</small>
 </div>;
}
