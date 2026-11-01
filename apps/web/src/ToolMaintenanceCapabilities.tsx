import type {ToolMaintenanceStatus} from '../../../packages/contracts/src';
import {readToolMaintenanceStatus} from './tool-maintenance-model';
import {tr,useLocale} from './i18n';
export type ToolMaintenanceCapabilitiesProps={maintenance?:ToolMaintenanceStatus;expired?:boolean;unverified?:boolean;onRefresh?:()=>void;disabled?:boolean};
export function ToolMaintenanceCapabilities({maintenance,expired,unverified,onRefresh,disabled}:ToolMaintenanceCapabilitiesProps){
 useLocale();const status=readToolMaintenanceStatus(maintenance)?.status,state=unverified?'unknown':status?.state==='supported'&&expired?'expired':status?.state??'unknown',t=(key:string)=>tr('tool-reinspection.maintenance.'+key);
 return <section className="tool-maintenance" data-tool-maintenance-capabilities><p data-tool-maintenance-state={state}><strong>{t('title')}</strong> · {t('state.'+state)}</p><p className="tool-reinspection-boundary">{t('boundary')}</p>{status&&<details><summary>{t('source')}</summary><dl>{(['protocol','instance_id','checked_at','observed_at','expires_at'] as const).filter(key=>status[key]).map(key=><div key={key}><dt>{t(key)}</dt><dd><code>{status[key]}</code></dd></div>)}</dl></details>}{onRefresh&&<button type="button" data-tool-maintenance-refresh disabled={disabled} onClick={onRefresh}>{t('refresh')}</button>}</section>;
}
