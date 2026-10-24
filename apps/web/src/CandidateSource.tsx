import type {Workflow} from './api';
import {query} from './api';
import {Field} from './ui';
import {useReleaseRead} from './release-hooks';
import {artifactSourceConfig,artifactSourceError,artifactSources,candidateSourceConfig,candidateSourceError,candidateSources,type CandidatePin,type RegisteredBuildWorker} from './workflow-model';

export function CandidateSource({workflow,nodeID,onChange}:{workflow:Workflow;nodeID:string;onChange:(config:Record<string,unknown>)=>void}){
 const node=workflow.nodes.find(item=>item.id===nodeID);if(!node||!['test','command','agent'].includes(node.type))return null;
 const sources=candidateSources(workflow,nodeID),source=typeof node.config?.candidate_from==='string'?node.config.candidate_from:'',error=candidateSourceError(workflow,nodeID),test=node.type==='test';
 return <><Field label={test?'测试候选来源':'构建候选来源'} hint={'固定所选上游当前成功尝试的干净提交进行'+(test?'测试':'不可变构建')+'。本地 Runner 需配置 repository；多个分支不会自动聚合。'+(!test?'构建须使用管理员已配置 ArtifactOutput 的命令，否则 Runner 拒绝。':'')}><select value={source} onChange={event=>onChange(candidateSourceConfig(workflow,nodeID,event.target.value))}><option value="">不使用上游候选（管理员基准）</option>{source&&!sources.some(item=>item.id===source)&&<option value={source} disabled>当前来源无效：{source}</option>}{sources.map(item=><option key={item.id} value={item.id}>{item.name} · {item.id} ({item.type})</option>)}</select></Field>{error?<div className="warning" role="alert"><p>{error}</p><button type="button" onClick={()=>onChange(candidateSourceConfig(workflow,nodeID,''))}>清除无效候选配置</button></div>:!sources.length?<p className="muted">先连接本流程的 command 或 agent 上游，再选择其候选提交。</p>:null}</>;
}

type ArtifactSourceProps={workflow:Workflow;nodeID:string;project:string;onChange:(config:Record<string,unknown>)=>void};
export function ArtifactSource(props:ArtifactSourceProps){
 const isTest=props.workflow.nodes.find(node=>node.id===props.nodeID)?.type==='test',registry=useReleaseRead<{items:RegisteredBuildWorker[]}>((isTest?'/release-workers'+query(props.project):''));
 if(!isTest)return null;
 return <ArtifactSourceField {...props} workers={registry.data?.items||[]} loading={registry.loading} error={registry.error} onRetry={()=>void registry.read()}/>;
}
export function ArtifactSourceField({workflow,nodeID,project,workers,loading,error,onRetry,onChange}:ArtifactSourceProps&{workers:RegisteredBuildWorker[];loading:boolean;error:string;onRetry:()=>void}){
 const node=workflow.nodes.find(item=>item.id===nodeID);if(node?.type!=='test')return null;
 const sources=artifactSources(workflow,nodeID,workers,project),source=typeof node.config?.artifact_from==='string'?node.config.artifact_from:'',invalid=!loading&&!error?artifactSourceError(workflow,nodeID,workers,project):undefined;
 return <><Field label="包测试制品来源" hint="读取所选构建上游实际封存的同一包，并固定其源提交。测试命令须由管理员配置 ArtifactInput；登记命令不代表构建已成功。"><select disabled={loading||!!error} value={source} onChange={event=>onChange(artifactSourceConfig(workflow,nodeID,event.target.value,workers,project))}><option value="">不读取上游制品</option>{source&&!sources.some(item=>item.id===source)&&<option value={source} disabled>当前来源尚未确认：{source}</option>}{sources.map(item=><option key={item.id} value={item.id}>{item.name} · {item.id} · 已登记构建 · {item.command![0]}</option>)}</select></Field>{loading?<p className="muted" role="status">正在读取真实构建命令登记…</p>:error?<div className="warning" role="alert"><p>{error}</p><button type="button" onClick={onRetry}>重新读取构建登记</button></div>:invalid?<div className="warning" role="alert"><p>{invalid}</p></div>:!sources.length?<p className="muted">尚无已登记构建命令的静态上游。请先由管理员登记发布执行节点的 build 命令，并在上游节点的命令参数填写相同标识。</p>:null}{Object.hasOwn(node.config||{},'artifact_from')&&<button type="button" onClick={()=>onChange(artifactSourceConfig(workflow,nodeID,'',workers,project))}>清除制品来源</button>}<p className="muted">同时选择候选来源时，两种来源必须指向一致提交；实际执行仍由服务端验证当前构建证明。</p></>;
}

export function CandidateProvenance({proof}:{proof:CandidatePin}){return <section className="workspace-evidence"><h4>本次尝试已固定候选</h4><dl><dt>来源节点</dt><dd>{proof.source_node}</dd><dt>来源尝试</dt><dd>{proof.source_attempt}</dd><dt>来源租约</dt><dd>{proof.source_fence}</dd><dt>固定提交</dt><dd><code>{proof.commit}</code></dd><dt>来源证据</dt><dd><code>{proof.evidence_sha256}</code></dd></dl><p className="muted">来自当前尝试的 candidate_pinned 事件。</p></section>;}
