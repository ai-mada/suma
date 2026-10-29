import type {Locale} from './i18n';

import type {ApplicationToolDefinition as ToolContract,DeveloperConnectionMetadata as ConnectionContract} from '../../../packages/contracts/src';

// Consume generated wire types; the view omits schemas because it only renders
// declarations and never invokes tools or handles secrets.
export type ApplicationToolDeclaration=Pick<ToolContract,'name'|'mcp_name'|'description'|'description_key'|'read_only'|'credential_scopes'>;
export type DeveloperConnectionMetadata=Omit<ConnectionContract,'tools'>&{tools:ApplicationToolDeclaration[]};

const object=(value:unknown):value is Record<string,unknown>=>value!==null&&typeof value==='object'&&!Array.isArray(value);
const text=(value:unknown):value is string=>typeof value==='string'&&value.length>0&&!/[\u0000-\u001f\u007f]/.test(value);

export function connectionMetadata(value:unknown,project:string):DeveloperConnectionMetadata|undefined{
 if(!object(value)||value.project_id!==project||!text(value.endpoint_path)||!['configured','listener_loopback','unconfigured'].includes(String(value.endpoint_source))||!text(value.transport)||typeof value.stateless!=='boolean'||!text(value.sdk_name)||!text(value.sdk_version)||value.token_environment!=='SUMA_MCP_TOKEN'||!text(value.expected_user_id)||!text(value.expected_organization_id)||value.client_connection_verified!==false||!Array.isArray(value.tools))return;
 if(value.endpoint_url!==undefined&&value.endpoint_url!==''&&!text(value.endpoint_url))return;
 if(value.tools.some(tool=>!object(tool)||!text(tool.name)||!text(tool.mcp_name)||typeof tool.description!=='string'||(tool.description_key!==undefined&&!text(tool.description_key))||typeof tool.read_only!=='boolean'||!Array.isArray(tool.credential_scopes)||tool.credential_scopes.some(scope=>scope!=='agent'&&scope!=='api')))return;
 return value as DeveloperConnectionMetadata;
}

// All client exports consume the same trusted, exact project endpoint. Never
// turn editable browser input, query credentials or a normalized path into it.
export function projectConnectionEndpoint(metadata:DeveloperConnectionMetadata):string|undefined{
 if(metadata.endpoint_source==='unconfigured'||!metadata.endpoint_url||metadata.transport!=='streamable_http'||metadata.stateless!==true||!/^[-A-Za-z0-9_]{1,200}$/.test(metadata.project_id)||metadata.endpoint_path!=='/api/v1/mcp/projects/'+metadata.project_id||!text(metadata.endpoint_url))return;
 try{
  const url=new URL(metadata.endpoint_url),loopback=['localhost','127.0.0.1','[::1]'].includes(url.hostname),rawOrigin=metadata.endpoint_url.endsWith(metadata.endpoint_path)?metadata.endpoint_url.slice(0,-metadata.endpoint_path.length):'';
  if(url.protocol!=='https:'&&!(url.protocol==='http:'&&loopback)||url.username||url.password||url.search||url.hash||url.pathname!==metadata.endpoint_path||!/^https?:\/\/[^/?#\\]+$/.test(rawOrigin)||metadata.endpoint_source==='listener_loopback'&&!loopback)return;
  return metadata.endpoint_url;
 }catch{return;}
}

export function codexConnectionConfig(metadata:DeveloperConnectionMetadata,locale:Locale,serverName='suma'):string|undefined{
 const endpoint=projectConnectionEndpoint(metadata);
 if(!endpoint||!/^[-A-Za-z0-9_]{1,220}$/.test(serverName))return;
 return '[mcp_servers.'+serverName+']\nurl = '+JSON.stringify(endpoint)+'\nbearer_token_env_var = "SUMA_MCP_TOKEN"\nhttp_headers = { "X-Suma-Expected-User" = '+JSON.stringify(metadata.expected_user_id)+', "X-Suma-Expected-Organization" = '+JSON.stringify(metadata.expected_organization_id)+', "X-Suma-Language" = '+JSON.stringify(locale)+' }';
}

// Octop's custom MCP connector accepts literal HTTP headers. Unlike the Codex
// client it does not expand a bearer-token environment reference here. Keep a
// visibly non-secret placeholder; the connection view never receives a token.
export function octopConnectionConfig(metadata:DeveloperConnectionMetadata,locale:Locale):string|undefined{
 if(!projectConnectionEndpoint(metadata))return;
 // Project-specific keys avoid silently replacing another project's connector
 // when the administrator merges this fragment into Octop's servers object.
 return JSON.stringify({servers:{['suma_'+metadata.project_id]:{transport:'streamable_http',url:metadata.endpoint_url,headers:{Authorization:'Bearer REPLACE_WITH_PROJECT_API_TOKEN','X-Suma-Expected-User':metadata.expected_user_id,'X-Suma-Expected-Organization':metadata.expected_organization_id,'X-Suma-Language':locale},enabled:true,display_name:'suma',shared:false,default_open:false}}},null,2);
}
