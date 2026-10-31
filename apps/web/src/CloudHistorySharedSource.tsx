import type {CloudHistorySharedObservation} from '../../../packages/contracts/src';
import {formatNumber,tr,useLocale} from './i18n';
import './CloudHistorySharing.css';
const t=(key:string,values?:Record<string,unknown>)=>tr('workbuddy-history-sharing.'+key,values);
/** Adopted message metadata never grants access to the owner's private load. */
export function CloudHistorySharedSource({source}:{source:CloudHistorySharedObservation}){
 useLocale();return <aside className="cloud-history-shared-source" data-cloud-history-shared-source aria-label={t('source_title')}><strong>{t('source_title')}</strong><p>{t('source_summary',{count:formatNumber(source.entries.length),state:t('load_state_'+source.load_state)})}</p><p>{t('scope')}</p><details><summary>{t('source_details')}</summary><dl><div><dt>{t('source_run')}</dt><dd><code>{source.source_run_id}</code></dd></div><div><dt>{t('source_load')}</dt><dd><code>{source.load_id}</code> · {formatNumber(source.load_version)}</dd></div><div><dt>{t('source_sequences')}</dt><dd>{source.entries.map(entry=>formatNumber(entry.sequence)).join(' / ')}</dd></div><div><dt>{t('source_body_hash')}</dt><dd><code>{source.body_sha256}</code></dd></div></dl></details></aside>;
}
