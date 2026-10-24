import type {ToolEvent} from './tool-model';
import {renderNativeDiagnostic,tr as t,useLocale} from './i18n';

export function NativeToolMessage({event}:{event:ToolEvent}) {
 useLocale();
 if(!event.message)return null;
 const diagnostic=event.diagnostic;
 const display=event.schema_version===1 && event.protocol==='suma-jsonl-v1' && event.kind==='error' && diagnostic?.schema_version===1
  ? renderNativeDiagnostic(event.message,diagnostic.message_key,diagnostic.message_params)
  : {text:event.message,localized:false};
 return <>
  <p className="preserve" data-localized-diagnostic={display.localized || undefined}>{display.text}</p>
  {display.localized&&<details><summary>{t('development.facts.original_reason')}</summary><pre className="output">{event.message}</pre></details>}
 </>;
}
