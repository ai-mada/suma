import {expect,it} from 'vitest';
import {preferencesCommandPath,preferencesPath,readProjectNotificationPreferences,validPreferencesReceipt,validPreferencesRequest,validPreferencesScope,type PreferencesScope} from './project-notification-preferences-model';
import type {WriteRecoveryRequest} from './write-action';

const scope:PreferencesScope={project:'project-a',userID:'user-a',organizationID:'org-a'};
const preference=(version=0,muted=false,updated_at=version?'2026-10-31T03:04:05.123456789Z':'')=>({project_id:scope.project,user_id:scope.userID,version,followed_discussions_muted:muted,updated_at});
const request=(version=0,muted=true,key='original-operation'):WriteRecoveryRequest=>({path:preferencesCommandPath(scope),method:'POST',idempotency_key:key,body:{project_id:scope.project,expected_version:version,followed_discussions_muted:muted,idempotency_key:key}});
it('binds preference paths to valid original identity and project scope',()=>{
 expect(validPreferencesScope(scope)).toBe(true);
 expect(preferencesPath(scope)).toBe('/notification-preferences?project_id=project-a');
 expect(preferencesCommandPath(scope)).toBe('/notification-preferences/commands?project_id=project-a');
 for(const name of ['project','userID','organizationID'])for(const value of ['','.','..','has space','other/project','x'.repeat(65)])expect(validPreferencesScope({...scope,[name]:value})).toBe(false);
 expect(validPreferencesScope(null as unknown as PreferencesScope)).toBe(false);
});
it('reads only exact current-user preferences and a genuine unpersisted default',()=>{
 expect(readProjectNotificationPreferences(preference(),scope)).toEqual(preference());
 expect(readProjectNotificationPreferences(preference(1,true),scope)).toEqual(preference(1,true));
 expect(readProjectNotificationPreferences(preference(2147483647),scope)?.version).toBe(2147483647);
 for(const patch of [{project_id:'other'},{user_id:'other'},{version:-1},{version:1.5},{version:2147483648},{version:'1'},{followed_discussions_muted:0},{followed_discussions_muted:null},{updated_at:''},{updated_at:'2026-02-30T03:04:05Z'},{updated_at:'2026-10-31T03:04:05+00:00'},{extra:true}])expect(readProjectNotificationPreferences({...preference(1),...patch},scope)).toBeUndefined();
 expect(readProjectNotificationPreferences(preference(0,true),scope)).toBeUndefined();
 expect(readProjectNotificationPreferences(preference(0,false,'2026-10-31T03:04:05Z'),scope)).toBeUndefined();
 for(const key of Object.keys(preference())){const value:Record<string,unknown>={...preference()};delete value[key];expect(readProjectNotificationPreferences(value,scope)).toBeUndefined();}
});
it('preserves a closed original POST body and stable operation key across recovery',()=>{
 const original=request();expect(validPreferencesRequest(original,scope)).toBe(true);
 expect(validPreferencesRequest(request(2147483647,false),scope)).toBe(true);
 for(const patch of [{path:preferencesPath(scope)},{path:original.path+'&user_id=other'},{method:'GET'},{idempotency_key:'different-operation'},{extra:true}])expect(validPreferencesRequest({...original,...patch},scope)).toBe(false);
 for(const patch of [{project_id:'other'},{expected_version:-1},{expected_version:2147483648},{expected_version:NaN},{followed_discussions_muted:'false'},{user_id:scope.userID},{idempotency_key:'different-operation'}])expect(validPreferencesRequest({...original,body:{...original.body,...patch}},scope)).toBe(false);
 for(const key of Object.keys(original.body)){const body={...original.body};delete body[key];expect(validPreferencesRequest({...original,body},scope)).toBe(false);}
 for(const key of ['short','x'.repeat(129),' original-operation','original-operation ','original\noperation','original\ud800key','界'.repeat(43)])expect(validPreferencesRequest(request(0,true,key),scope)).toBe(false);
 expect(validPreferencesRequest(request(0,true,'界'.repeat(42)),scope)).toBe(true);
 expect(validPreferencesRequest(request(0,true,'x'.repeat(128)),scope)).toBe(true);
});
it('accepts original no-op or one-version change receipts without treating later state as that receipt',()=>{
 const receipt=(version:number,muted:boolean,key='original-operation')=>({idempotency_key:key,preferences:preference(version,muted)});
 expect(validPreferencesReceipt(receipt(0,false),request(0,false),scope)).toBe(true);
 expect(validPreferencesReceipt(receipt(1,true),request(),scope)).toBe(true);
 expect(validPreferencesReceipt(receipt(4,true),request(4,true),scope)).toBe(true);
 expect(validPreferencesReceipt(receipt(5,true),request(4,true),scope)).toBe(true);
 expect(validPreferencesReceipt(receipt(2147483647,false),request(2147483647,false),scope)).toBe(true);
 for(const candidate of [receipt(3,true),receipt(6,true),receipt(4,false),receipt(4,true,'another-operation'),{...receipt(4,true),extra:true},{preferences:preference(4,true)},receipt(2147483648,true)])expect(validPreferencesReceipt(candidate,request(4,true),scope)).toBe(false);
 expect(validPreferencesReceipt(receipt(4,true),request(4,true),{...scope,userID:'another-user'})).toBe(false);
 expect(validPreferencesReceipt(receipt(4,true),request(4,true),{...scope,project:'another-project'})).toBe(false);
});
it('rejects inherited and hidden fields at each private response or recovery boundary',()=>{
 const inherited=(value:object)=>Object.assign(Object.create({inherited:true}),value);
 const hidden=(value:object)=>Object.defineProperty({...value},'token',{value:'not-allowed'});
 const value=preference(1,true),req=request(),receipt={idempotency_key:req.idempotency_key,preferences:value};
 for(const wrap of [inherited,hidden]){
  expect(readProjectNotificationPreferences(wrap(value),scope)).toBeUndefined();
  expect(validPreferencesRequest(wrap(req),scope)).toBe(false);
  expect(validPreferencesRequest({...req,body:wrap(req.body)},scope)).toBe(false);
  expect(validPreferencesReceipt(wrap(receipt),req,scope)).toBe(false);
  expect(validPreferencesReceipt({...receipt,preferences:wrap(value)},req,scope)).toBe(false);
 }
 expect(readProjectNotificationPreferences({...value,[Symbol('private')]:true},scope)).toBeUndefined();
});
