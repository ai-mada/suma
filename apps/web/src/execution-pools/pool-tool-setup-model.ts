import type {ProjectPoolRunnerPage} from '../../../../packages/contracts/src';
import type {AdapterDescriptor} from '../AdapterConnections';
import {toolPreparationTarget,preparationForLocalTool} from '../tool-preparation-target';
import {toolDiscoveryTimestampNs} from '../tool-discovery-model';
import {validPoolConfigID} from './pool-config-preparation-model';

/** New-tool preparation intent. Unlike recovery, there is no existing profile
 * or replacement authority. Both destinations read current permissions again. */
export type PoolToolSetupPin={
 pool_id:string; runner_id:string; worker_id:string; tenant_id:string;
 project_id:string; adapter_id:string; adapter:string; protocol:string;
};
const fields=['pool_id','runner_id','worker_id','tenant_id','project_id','adapter_id','adapter','protocol'] as const;
const prefix='pool-tool-setup:';
const object=(v:unknown):v is Record<string,unknown>=>!!v&&typeof v==='object'&&!Array.isArray(v)&&[Object.prototype,null].includes(Object.getPrototypeOf(v));
export function validPoolToolSetupPin(value:unknown):value is PoolToolSetupPin{
 if(!object(value)||Reflect.ownKeys(value).length!==fields.length||!fields.every(k=>Object.hasOwn(value,k))||!fields.filter(k=>k!=='worker_id').every(k=>validPoolConfigID(value[k])))return false;
 const target=preparationForLocalTool(value.adapter_id);
 return !!target&&value.worker_id==='pool-'+value.runner_id&&value.adapter===target.adapter&&value.protocol===target.protocol;
}
/** A closed tuple carries public scope only, never paths, commands or secrets. */
export function serializePoolToolSetupPin(pin:PoolToolSetupPin):string{
 if(!validPoolToolSetupPin(pin))throw Error('Invalid Pool tool setup navigation');
 return prefix+fields.map(k=>pin[k]).join(':');
}
export function parsePoolToolSetupPin(panel:string):PoolToolSetupPin|undefined{
 if(typeof panel!=='string'||!panel.startsWith(prefix)||panel.length>1000)return;
 const values=panel.slice(prefix.length).split(':');if(values.length!==fields.length)return;
 const candidate=Object.fromEntries(fields.map((k,i)=>[k,values[i]]));return validPoolToolSetupPin(candidate)?candidate:undefined;
}
export function createPoolToolSetupPin(adapter:AdapterDescriptor,workerID:string,tenantID:string,projectID:string,page:ProjectPoolRunnerPage|undefined):PoolToolSetupPin|undefined{
 if(!toolPreparationTarget(adapter)||!validPoolConfigID(tenantID)||!validPoolConfigID(projectID)||!page||!Array.isArray(page.items))return;
 try{toolDiscoveryTimestampNs(page.as_of);}catch{return;}
 const rows=page.items.filter(r=>object(r)&&r.worker_id===workerID&&r.status==='active');if(rows.length!==1)return;
 const row=rows[0],pin={pool_id:row.pool_id,runner_id:row.id,worker_id:row.worker_id,tenant_id:tenantID,project_id:projectID,adapter_id:adapter.id,adapter:adapter.adapter,protocol:adapter.protocol};
 return validPoolToolSetupPin(pin)?pin:undefined;
}
export function buildPoolSetupOwnerHref(pin:PoolToolSetupPin):string|undefined{
 if(!validPoolToolSetupPin(pin))return;
 return '?'+new URLSearchParams({view:'settings',sub:'execution-pools',record:pin.pool_id,panel:serializePoolToolSetupPin(pin)});
}
export function buildPoolSetupProjectHref(pin:PoolToolSetupPin):string|undefined{
 if(!validPoolToolSetupPin(pin))return;
 return '?'+new URLSearchParams({project:pin.project_id,view:'settings',sub:'tools',record:'',panel:serializePoolToolSetupPin(pin)});
}
