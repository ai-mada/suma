import {CircleHelp} from 'lucide-react';
import {tr,useLocale} from './i18n';
import {toolConnectionDiagnostic,type ToolConnectionDiagnosticAction} from './tool-connection-diagnostics-model';
import './ToolConnectionDiagnostic.css';

export type ToolConnectionDiagnosticProps={
 reasonCode?:string;
 originalOperationPending?:boolean;
 disabled?:boolean;
 onDetect?:()=>void;
 onConfigure?:()=>void;
 onWorkspace?:()=>void;
 onGuide?:()=>void;
 onReconcile?:()=>void;
};
const t=(key:string)=>tr('tool-connection-diagnostics.'+key);

/** Shows only actions for which the current connection owner supplied a handler.
 * Rendering and expanding the source code never perform an operation. */
export function ToolConnectionDiagnostic(props:ToolConnectionDiagnosticProps){
 useLocale();
 const value=toolConnectionDiagnostic(props.reasonCode,props.originalOperationPending);
 if(!value)return null;
 const callbacks:Partial<Record<ToolConnectionDiagnosticAction,()=>void>>={detect:props.onDetect,configure:props.onConfigure,workspace:props.onWorkspace,guide:props.onGuide,reconcile:props.onReconcile};
 const actions=value.actions.filter(action=>typeof callbacks[action]==='function');
 return <section className="tool-connection-diagnostic" data-tool-connection-diagnostic data-tool-connection-reason={value.reason} aria-label={t('title')}>
  <div className="tool-connection-diagnostic-copy" role="status"><CircleHelp size={18} aria-hidden="true"/><div><strong>{t('reason.'+value.reason)}</strong><p>{t('hint.'+value.reason)}</p></div></div>
  {actions.length>0&&<div className="tool-connection-diagnostic-actions">{actions.map(action=><button key={action} type="button" data-tool-connection-action={action} disabled={props.disabled} onClick={()=>callbacks[action]?.()}>{t('action.'+action)}</button>)}</div>}
  {value.rawCode&&<details className="tool-connection-diagnostic-source"><summary>{t('source_code')}</summary><code>{value.rawCode}</code></details>}
 </section>;
}
