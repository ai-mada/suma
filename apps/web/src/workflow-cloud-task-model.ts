const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const positive=(value:unknown)=>typeof value==='number'&&Number.isSafeInteger(value)&&value>0;
const binding=(value:unknown)=>{
 if(!object(value)||Object.keys(value).sort().join(',')!=='connection_id,grant_generation,request'||typeof value.connection_id!=='string'||!value.connection_id.trim()||!positive(value.grant_generation)||!object(value.request))return false;
 return Object.keys(value.request).every(key=>key==='prompt'||key==='name')&&typeof value.request.prompt==='string'&&!!value.request.prompt.trim()&&(value.request.name===undefined||typeof value.request.name==='string');
};

/** Only an exact frozen definition identifies a cloud node. The run's public
 * NodeRun has no config; its display name or a later workflow cannot supply it.
 * The mounted workspace separately validates current authorization and source. */
export function frozenCloudTaskNode(definition:unknown,run:unknown,node:unknown):{nodeID:string;name:string}|undefined{
 if(!object(definition)||!object(run)||!object(node)||typeof node.id!=='string'||!node.id||node.type!=='external_workflow'||definition.id!==run.workflow_id||!positive(definition.version)||definition.version!==run.workflow_version||!Array.isArray(definition.nodes)||!Array.isArray(run.nodes))return;
 const declared=definition.nodes.filter(value=>object(value)&&value.id===node.id),actual=run.nodes.filter(value=>object(value)&&value.id===node.id);
 if(declared.length!==1||actual.length!==1||!object(declared[0])||!object(actual[0])||declared[0].type!=='external_workflow'||actual[0].type!=='external_workflow'||typeof declared[0].name!=='string'||!object(declared[0].config))return;
 const config=declared[0].config;
 if(config.plugin!=='workbuddy-cloud'||config.workflow!=='workbuddy-cloud/v1'||!binding(config.workbuddy_cloud))return;
 return {nodeID:node.id,name:declared[0].name};
}
