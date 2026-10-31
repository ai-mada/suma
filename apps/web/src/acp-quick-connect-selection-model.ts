import {wellFormed} from './i18n/catalog';

export type ACPQuickInstallation={id:string;label:string;version?:string;kind:'native'|'wrapper_only'|'unverified'};
export type ACPQuickCatalogue={source:'acp_session_config';protocol_version:number;config_id:string;current_value?:string;models:{id:string;name:string;configuration_sha256?:string}[]};
export type ACPQuickDiscoveryPhase='idle'|'reading'|'ready'|'unsupported'|'failed'|'unknown'|'expired';
export type ACPQuickSelectionState={installations:ACPQuickInstallation[];installationID:string;sourceKey:string;catalogueSourceKey?:string;catalogue?:ACPQuickCatalogue;expiresAt?:string;phase:ACPQuickDiscoveryPhase;modelID:string};

const encoder=new TextEncoder();
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value)&&(Object.getPrototypeOf(value)===Object.prototype||Object.getPrototypeOf(value)===null);
const closed=(value:Record<string,unknown>,required:readonly string[],optional:readonly string[]=[])=>required.every(key=>Object.hasOwn(value,key))&&Reflect.ownKeys(value).every(key=>typeof key==='string'&&(required.includes(key)||optional.includes(key)));
const safeText=(value:unknown,limit=Infinity):value is string=>typeof value==='string'&&wellFormed(value)&&encoder.encode(value).length<=limit&&!/\p{Cc}/u.test(value);
const token=(value:unknown,limit=Infinity):value is string=>safeText(value,limit)&&value.length>0&&value.trim()===value;
const digest=(value:unknown)=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);

/** Internal selection data only. Public IPC observations must first pass their
 * own scope, installation and lifecycle validation before mapping here. */
export function validQuickACPCatalogue(value:unknown):value is ACPQuickCatalogue{
 if(!object(value)||!closed(value,['source','protocol_version','config_id','models'],['current_value'])||value.source!=='acp_session_config'||value.protocol_version!==1||!token(value.config_id)||!Array.isArray(value.models)||value.models.length>256)return false;
 if(Object.hasOwn(value,'current_value')&&!safeText(value.current_value,256))return false;
 const ids=new Set<string>();
 for(const model of value.models){
  if(!object(model)||!closed(model,['id','name'],['configuration_sha256'])||!token(model.id,256)||!safeText(model.name)||Object.hasOwn(model,'configuration_sha256')&&!digest(model.configuration_sha256)||ids.has(model.id))return false;
  ids.add(model.id);
 }
 return true;
}

function futureExpiry(value:unknown,now:number):boolean{
 if(typeof value!=='string'||!/^\d{4}-\d\d-\d\dT(?:[01]\d|2[0-3]):[0-5]\d:[0-5]\d(?:\.\d{1,9})?(?:Z|[+-](?:[01]\d|2[0-3]):[0-5]\d)$/.test(value)||!Number.isFinite(now))return false;
 const date=new Date(value.slice(0,10)+'T00:00:00Z'),expiry=Date.parse(value);
 return Number.isFinite(date.getTime())&&date.toISOString().slice(0,10)===value.slice(0,10)&&Number.isFinite(expiry)&&expiry>now;
}

export function selectedQuickACPModel(state:ACPQuickSelectionState,now=Date.now()):ACPQuickCatalogue['models'][number]|undefined{
 const installations=state.installations.filter(item=>item.id===state.installationID);
 if(installations.length!==1||installations[0].kind!=='native'||!token(installations[0].version)||state.phase!=='ready'||!state.sourceKey||state.sourceKey!==state.catalogueSourceKey||!futureExpiry(state.expiresAt,now)||!validQuickACPCatalogue(state.catalogue)||!token(state.modelID,256))return;
 // Keep the exact provider choice and identity; current_value is not consent.
 return state.catalogue.models.find(model=>model.id===state.modelID);
}
