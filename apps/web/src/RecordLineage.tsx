import {tr,useLocale} from "./i18n";
import {ArrowUpRight,Link} from 'lucide-react';
import type {RecordItem} from './api';

const identifier=(value:unknown)=>typeof value==='string'&&/^[A-Za-z0-9_-]{1,150}$/.test(value)?value:'';
const object=(value:unknown):Record<string,unknown>=>value&&typeof value==='object'&&!Array.isArray(value)?value as Record<string,unknown>:{};

export function RecordRequirementReference({item,project,onOpen}:{item:RecordItem;project:string;onOpen?:(id:string)=>void}){useLocale();
 if(item.project_id&&item.project_id!==project)return null;
 const requirement=identifier(item.requirement_id);
 if(!requirement)return null;
 return <a style={{display:'inline-flex',flexWrap:'wrap',alignItems:'center',gap:'4px 6px',maxWidth:'100%',minWidth:0,minHeight:'36px',padding:'2px 0',verticalAlign:'top',overflowWrap:'anywhere'}} href={'?'+new URLSearchParams({project,view:'requirements',record:requirement})} onClick={event=>{if(!onOpen||event.defaultPrevented||event.button!==0||event.metaKey||event.ctrlKey||event.shiftKey||event.altKey)return;event.preventDefault();onOpen(requirement);}}><span style={{display:'inline-flex',alignItems:'center',gap:'4px'}}>{tr("records.lineage.requirement")}<ArrowUpRight size={13}/></span><code style={{flex:'1 1 100%',minWidth:0,maxWidth:'100%',overflowWrap:'anywhere',wordBreak:'break-all'}}>{requirement}</code></a>;
}

export function RecordLineage({item,project}:{item:RecordItem;project:string}){useLocale();
 if(item.project_id&&item.project_id!==project)return null;
 const data=object(item.data),source=object(item.source??data.source);
 const issue=identifier(item.source_issue_id??data.source_issue_id);
 const deployment=identifier(item.release_operation_id??data.release_operation_id??source.release_operation_id);
 const run=identifier(item.run_id??data.run_id??source.run_id);
 const links:{name:string;id:string;route:Record<string,string>}[]=[];
 if(issue)links.push({name:tr("records.lineage.issue"),id:issue,route:{view:'quality',sub:'issues',record:issue}});
 if(deployment)links.push({name:tr("records.lineage.deployment"),id:deployment,route:{view:'operations',sub:'deployments',record:deployment}});
 if(run)links.push({name:tr("records.lineage.run"),id:run,route:{view:'development',sub:'runs',run}});
 if(!links.length)return null;
 return <section className="detail-section" aria-label={tr("records.lineage.title")}><h3><Link size={15}/>{tr("records.lineage.title")}</h3>{links.map(link=><p key={link.id}><a href={'?'+new URLSearchParams({project,...link.route})}>{link.name}<ArrowUpRight size={14}/></a><small className="muted"> · {link.id.slice(0,16)}{link.id.length>16?'…':''}</small></p>)}</section>;
}
