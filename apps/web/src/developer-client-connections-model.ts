import type {Locale} from './i18n';
import {codexConnectionConfig,projectConnectionEndpoint,type DeveloperConnectionMetadata} from './developer-connection-model';

// Client formats were checked against official documentation on 2026-10-29.
// A guide entry is deliberately not a Runner adapter or a connection result.
export const developerClients=[
 {id:'cursor',name:'Cursor',docs:'https://cursor.com/docs/mcp',target:'.cursor/mcp.json',exportable:true},
 {id:'claude-code',name:'Claude Code',docs:'https://code.claude.com/docs/en/mcp',target:'.mcp.json',exportable:true},
 {id:'qoder',name:'Qoder',docs:'https://docs.qoder.com/user-guide/chat/model-context-protocol',exportable:false},
 {id:'codex',name:'Codex',docs:'https://learn.chatgpt.com/docs/extend/mcp?surface=cli',target:'.codex/config.toml',exportable:true},
 {id:'workbuddy',name:'WorkBuddy',docs:'https://open.workbuddy.cn/en/docs/connector',exportable:false},
 {id:'zed',name:'Zed',docs:'https://zed.dev/docs/ai/mcp',exportable:false},
 {id:'opencode',name:'OpenCode',docs:'https://opencode.ai/docs/mcp-servers/',target:'opencode.json',exportable:true},
 {id:'harness',name:'Harness',docs:'https://github.com/TencentCloud/Octop',exportable:false},
] as const;
export type DeveloperClientID=typeof developerClients[number]['id'];
export type DeveloperClientArtifact={content:string;filename:string;target:string;mime:string};

export function developerClientArtifact(client:DeveloperClientID,metadata:DeveloperConnectionMetadata,locale:Locale):DeveloperClientArtifact|undefined{
 const definition=developerClients.find(item=>item.id===client),endpoint=projectConnectionEndpoint(metadata);
 if(!definition?.exportable||!endpoint)return;
 const name='suma_'+metadata.project_id,headers={'X-Suma-Expected-User':metadata.expected_user_id,'X-Suma-Expected-Organization':metadata.expected_organization_id,'X-Suma-Language':locale};
 if(client==='codex'){
  const content=codexConnectionConfig(metadata,locale,name);
  return content?{content,filename:'suma-codex-'+metadata.project_id+'.toml',target:'.codex/config.toml',mime:'text/plain;charset=utf-8'}:undefined;
 }
 const bearer=client==='cursor'?'Bearer ${env:SUMA_MCP_TOKEN}':client==='claude-code'?'Bearer ${SUMA_MCP_TOKEN}':'Bearer {env:SUMA_MCP_TOKEN}';
 const config=client==='opencode'?{mcp:{[name]:{type:'remote',url:endpoint,oauth:false,headers:{Authorization:bearer,...headers}}}}:{mcpServers:{[name]:{...(client==='claude-code'?{type:'http'}:{}),url:endpoint,headers:{Authorization:bearer,...headers}}}};
 return {content:JSON.stringify(config,null,2),filename:'suma-'+client+'-'+metadata.project_id+'.json',target:definition.target,mime:'application/json;charset=utf-8'};
}
