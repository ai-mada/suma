import type {ApplicationReleaseSetBlocker} from '../../../packages/contracts/src';
import {tr,useLocale} from './i18n';

const keys=new Set(['release_set_application_changed','release_set_quality_stale','release_set_source_unsupported','release_set_source_mismatch','release_set_candidate_stale','release_set_independent_test_required','release_set_reused_candidate','release_set_dependency_missing','release_set_dependency_conflict','release_set_dependency_incompatible','release_set_compatibility_unverified','release_set_environment_changed']);
/** Shared set-domain diagnostics retain unknown protocol codes as plain text. */
export function ReleaseSetBlockers({items}:{items:ApplicationReleaseSetBlocker[]}){useLocale();return items.length?<ul className="rs-warning" data-release-set-blockers>{items.map((item,index)=><li key={index}>{keys.has(item.code)?tr('application-release-sets.blocker.'+item.code):item.code}{item.application_id&&<> · <code>{item.application_id}</code></>}{item.ref_id&&<> · <code>{item.ref_id}</code></>}<details><summary>{tr('application-release-sets.original')}</summary><code>{item.code}</code></details></li>)}</ul>:null;}
