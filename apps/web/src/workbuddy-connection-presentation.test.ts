import {expect,it} from 'vitest';
import {
 workBuddyConnectionReason,
 workBuddyReusableScopes,
 workBuddyScopeDifference,
} from './workbuddy-connection-presentation';

const read='user.task.readable',invoke='user.task.invokable';

it('projects only fixed reasons from the current public connection without deriving actions',()=>{
 expect(workBuddyConnectionReason({state:'refresh_unknown',last_error:'workbuddy_effect_unknown'})).toBe('effect_unknown');
 expect(workBuddyConnectionReason({state:'reauthorization_required',last_error:'workbuddy_reauthorization_required'})).toBe('reauthorization_required');
 expect(workBuddyConnectionReason({state:'authorization_unknown',last_error:''})).toBe('effect_unknown');
 expect(workBuddyConnectionReason({state:'refresh_unknown',last_error:''})).toBe('effect_unknown');
 expect(workBuddyConnectionReason({state:'reauthorization_required',last_error:''})).toBe('reauthorization_required');
 for(const state of ['connected','authorizing','refreshing','disconnected']){
  expect(workBuddyConnectionReason({state,last_error:''})).toBeUndefined();
 }
});

it('never echoes unknown provider or machine error text even when the public state has a known reason',()=>{
 for(const last_error of ['private-token-at-C:/private/account','workbuddy_effect_unknown: private details',' workbuddy_reauthorization_required','WORKBUDDY_EFFECT_UNKNOWN']){
  for(const state of ['connected','refresh_unknown','reauthorization_required']){
   const connection=Object.freeze({state,last_error}),before=JSON.stringify(connection);
   expect(workBuddyConnectionReason(connection)).toBe('generic');
   expect(JSON.stringify(connection)).toBe(before);
  }
 }
});

it('reuses only still-requestable current permissions in available order without selecting newly available permissions',()=>{
 const current=Object.freeze([invoke,read,invoke,'retired.scope']);
 const available=Object.freeze(['new.scope',read,read,invoke]);
 const before=JSON.stringify({current,available});
 expect(workBuddyReusableScopes(current,available)).toEqual([read,invoke]);
 expect(workBuddyReusableScopes([read,invoke],[read])).toEqual([read]);
 expect(workBuddyReusableScopes([],available)).toEqual([]);
 expect(workBuddyReusableScopes(current,[])).toEqual([]);
 expect(JSON.stringify({current,available})).toBe(before);
});

it('compares the exact current grant to the explicit request and exposes unavailable scopes without rewriting the request',()=>{
 const current=Object.freeze([invoke,read,'retired.scope',invoke]);
 const requested=Object.freeze([read,'new.scope','unavailable.request',read]);
 const available=Object.freeze(['new.scope',read]);
 const before=JSON.stringify({current,requested,available});
 expect(workBuddyScopeDifference(current,requested,available)).toEqual({
  added:['new.scope','unavailable.request'],
  removed:[invoke,'retired.scope'],
  retained:[read],
  unavailable:[invoke,'retired.scope','unavailable.request'],
 });
 expect(JSON.stringify({current,requested,available})).toBe(before);
});

it('keeps explicit removal distinct from reuse and does not invent additions from the available catalogue',()=>{
 expect(workBuddyScopeDifference([read,invoke],[],[read,invoke,'new.scope'])).toEqual({added:[],removed:[read,invoke],retained:[],unavailable:[]});
 expect(workBuddyScopeDifference([invoke,read],[read,invoke],[read,invoke,'new.scope'])).toEqual({added:[],removed:[],retained:[read,invoke],unavailable:[]});
 expect(workBuddyScopeDifference([],[],[])).toEqual({added:[],removed:[],retained:[],unavailable:[]});
});
