import type {DispatchRecommendationChoice} from '../../../packages/contracts/src';
import {wellFormed} from './i18n/catalog';
import {UiError} from './i18n';

/** Encoder for this single five-field Go struct, not a general JSON canonical
 * format. Its field order and required nulls mirror the public Choice contract.
 * Go encoding/json also HTML-escapes strings and both line separators. */
export function dispatchChoiceJSON(choice:DispatchRecommendationChoice):string{
 const keys=['candidate_id','max_steps','max_output_tokens','reasoning_effort','reason'];
 const string=(value:unknown)=>typeof value==='string'&&wellFormed(value);
 const integer=(value:unknown)=>value===null||typeof value==='number'&&Number.isSafeInteger(value)&&value>0;
 if(!choice||typeof choice!=='object'||Array.isArray(choice)||Object.keys(choice).length!==keys.length||!keys.every(key=>Object.hasOwn(choice,key))||!string(choice.candidate_id)||!integer(choice.max_steps)||!integer(choice.max_output_tokens)||choice.reasoning_effort!==null&&!string(choice.reasoning_effort)||!string(choice.reason))throw new UiError('dispatch-recommendation.invalid_choice');
 const source=JSON.stringify({candidate_id:choice.candidate_id,max_steps:choice.max_steps,max_output_tokens:choice.max_output_tokens,reasoning_effort:choice.reasoning_effort,reason:choice.reason});
 const escaped:Record<string,string>={'<':'\\u003c','>':'\\u003e','&':'\\u0026','\u2028':'\\u2028','\u2029':'\\u2029'};
 return source.replace(/[<>&\u2028\u2029]/g,character=>escaped[character]);
}
export async function dispatchChoiceSHA(choice:DispatchRecommendationChoice):Promise<string>{
 const digest=await crypto.subtle.digest('SHA-256',new TextEncoder().encode(dispatchChoiceJSON(choice)));
 return Array.from(new Uint8Array(digest),byte=>byte.toString(16).padStart(2,'0')).join('');
}
