import type {ApplicationReleaseRollout} from '../../../packages/contracts/src';
import {wellFormed} from './i18n/catalog';
import {UiError} from './i18n';

/** rc.CanonicalDigest marshals to JSON, decodes with UseNumber, then marshals
 * sorted maps. This bounded contract contains only safe integers and strings;
 * never use ordinary JSON.stringify field order as a Go canonical hash. */
function canonical(value:unknown):string{
 if(typeof value==='string'){if(!wellFormed(value))throw new UiError('release-rollouts.invalid_read');return JSON.stringify(value);}
 if(typeof value==='number'){if(!Number.isSafeInteger(value))throw new UiError('release-rollouts.invalid_read');return String(value);}
 if(typeof value==='boolean')return value?'true':'false';
 if(Array.isArray(value))return '['+value.map(canonical).join(',')+']';
 if(value&&typeof value==='object')return '{'+Object.keys(value).sort().map(key=>canonical(key)+':'+canonical((value as Record<string,unknown>)[key])).join(',')+'}';
 throw new UiError('release-rollouts.invalid_read');
}
async function sha(value:unknown){if(typeof crypto==='undefined'||!crypto.subtle)throw new UiError('release-rollouts.crypto_unavailable');const escaped:Record<string,string>={'<':'\\u003c','>':'\\u003e','&':'\\u0026','\u2028':'\\u2028','\u2029':'\\u2029'},source=canonical(value).replace(/[<>&\u2028\u2029]/g,character=>escaped[character]),result=await crypto.subtle.digest('SHA-256',new TextEncoder().encode(source));return Array.from(new Uint8Array(result),byte=>byte.toString(16).padStart(2,'0')).join('');}
export async function rolloutPreviewSHA(value:ApplicationReleaseRollout){const draft={...value,id:'',parent_run_id:'',snapshot_sha256:'',started_preview_sha256:'',created_by:'',created_at:'',members:value.members.map(member=>({...member,binding_id:''}))};return sha({protocol:'application-release-rollout-preview-v1',rollout:draft});}
export async function rolloutSnapshotSHA(value:ApplicationReleaseRollout){return sha({...value,snapshot_sha256:''});}
