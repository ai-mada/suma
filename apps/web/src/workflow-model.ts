import type {Workflow,Run} from './api';
export function declaredEdges(w:Workflow):Workflow['edges']{
 const edges=new Map<string,Workflow['edges'][number]>();
 for(const edge of w.edges||[])edges.set(edge.from+'\0'+edge.to,edge);
 for(const node of w.nodes)for(const from of node.depends_on||[]){const key=from+'\0'+node.id;if(!edges.has(key))edges.set(key,{from,to:node.id});}
 return [...edges.values()];
}

export function dependencyError(nodeIDs:string[],edges:Workflow['edges']):string|undefined{
 const ids=new Set(nodeIDs),next=new Map(nodeIDs.map(id=>[id,[] as string[]]));
 for(const edge of edges){if(!ids.has(edge.from)||!ids.has(edge.to))return '依赖节点不存在。';if(edge.from===edge.to)return '节点不能依赖自身。';next.get(edge.from)!.push(edge.to);}
 const state=new Map<string,number>();
 function visit(id:string):boolean{if(state.get(id)===1)return true;if(state.get(id)===2)return false;state.set(id,1);for(const target of next.get(id)||[])if(visit(target))return true;state.set(id,2);return false;}
 if(nodeIDs.some(visit))return '该依赖会形成环路，未应用本次修改。';
 return undefined;
}

export function updateDependency(nodeIDs:string[],edges:Workflow['edges'],from:string,to:string,enabled:boolean):{edges:Workflow['edges'];error?:string}{
 const candidate=enabled?(edges.some(e=>e.from===from&&e.to===to)?[...edges]:[...edges,{from,to}]):edges.filter(e=>!(e.from===from&&e.to===to));
 const error=dependencyError(nodeIDs,candidate);
 return error?{edges,error}:{edges:candidate};
}

export function canCompensate(_workflow:Workflow,run:Run|null):boolean{
 if(!run||!['succeeded','failed','cancelled','skipped'].includes(run.state)||run.compensation_run_id)return false;
 return run.nodes.some(node=>node.compensatable===true&&node.state==='succeeded'&&node.attempt>0);
}
export function canApproveNode(node:{type:string;state:string}){return node.type==='approval'&&node.state==='waiting';}
export function canRetryNode(node:{state:string;idempotent?:boolean}){return node.state==='failed'&&node.idempotent===true;}
export function latestProgress(events:{sequence:number;type:string;node_id:string;data?:{attempt?:number;fence?:number;message?:unknown}}[],node:{id:string;attempt:number;fence?:number;state:string}){if(['pending','ready','waiting'].includes(node.state))return;const latest=events.filter(event=>event.type==='node_progress'&&event.node_id===node.id&&event.data?.attempt===node.attempt&&(!node.fence||event.data.fence===node.fence)&&typeof event.data.message==='string').sort((a,b)=>b.sequence-a.sequence)[0];return latest?.data?.message as string|undefined;}

// The editor's current edges are authoritative. Old depends_on fields must not
// resurrect a removed dependency in either candidate options or publish payloads.
export function workflowWithEdges(definition:Workflow,edges:Workflow['edges']):Workflow{return {...definition,nodes:definition.nodes.map(node=>({...node,depends_on:[]})),edges:edges.map(edge=>({...edge}))};}
export function candidateSources(workflow:Workflow,nodeID:string):Workflow['nodes']{if(!['test','command','agent'].includes(workflow.nodes.find(node=>node.id===nodeID)?.type||''))return [];const edges=declaredEdges(workflow);if(dependencyError(workflow.nodes.map(node=>node.id),edges))return [];const parents=new Map<string,string[]>();for(const edge of edges)parents.set(edge.to,[...(parents.get(edge.to)||[]),edge.from]);const ancestors=new Set<string>(),queue=[...(parents.get(nodeID)||[])];while(queue.length){const id=queue.shift()!;if(ancestors.has(id))continue;ancestors.add(id);queue.push(...(parents.get(id)||[]));}return workflow.nodes.filter(node=>node.id!==nodeID&&ancestors.has(node.id)&&['command','agent'].includes(node.type));}
export function candidateSourceConfig(workflow:Workflow,nodeID:string,source:string):Record<string,unknown>{const node=workflow.nodes.find(item=>item.id===nodeID);if(!node||!['test','command','agent'].includes(node.type))throw new Error('只有测试、command 或 agent 构建节点可选择候选来源。');const config={...node.config};if(!source){delete config.candidate_from;return config;}if(!candidateSources(workflow,nodeID).some(item=>item.id===source))throw new Error('候选来源必须是当前流程中合法的 command 或 agent 静态上游。');return {...config,candidate_from:source};}
export function candidateSourceError(workflow:Workflow,testID:string):string|undefined{const node=workflow.nodes.find(item=>item.id===testID);if(!node?.config||!Object.hasOwn(node.config,'candidate_from'))return;const source=node.config.candidate_from;if(typeof source!=='string'||!source)return '候选来源必须是单个非空上游节点 ID。';if(!candidateSources(workflow,testID).some(item=>item.id===source))return '候选来源已不在当前合法上游中，请重新选择或清空。';}
export type RegisteredBuildWorker={project_id:string;status:string;expires_at:string;profiles:{id:string;kind:string;sha256:string}[]};
// A registered profile is configuration evidence, not a successful build proof.
export function artifactSources(workflow:Workflow,nodeID:string,workers:RegisteredBuildWorker[],project:string,now=Date.now()):Workflow['nodes']{if(workflow.nodes.find(node=>node.id===nodeID)?.type!=='test')return [];const profiles=new Set(workers.filter(worker=>worker.project_id===project&&worker.status==='active'&&Date.parse(worker.expires_at)>now).flatMap(worker=>worker.profiles.filter(profile=>profile.kind==='build'&&/^[a-f0-9]{64}$/.test(profile.sha256)).map(profile=>profile.id)));return candidateSources(workflow,nodeID).filter(node=>node.command?.length===1&&profiles.has(node.command[0]));}
export function artifactSourceConfig(workflow:Workflow,nodeID:string,source:string,workers:RegisteredBuildWorker[],project:string):Record<string,unknown>{const node=workflow.nodes.find(item=>item.id===nodeID);if(node?.type!=='test')throw new Error('只有测试节点可读取上游制品。');const config={...node.config};if(!source){delete config.artifact_from;return config;}if(!artifactSources(workflow,nodeID,workers,project).some(item=>item.id===source))throw new Error('制品来源必须是具有当前已登记构建命令的合法静态上游。');return {...config,artifact_from:source};}
export function artifactSourceError(workflow:Workflow,nodeID:string,workers:RegisteredBuildWorker[],project:string):string|undefined{const node=workflow.nodes.find(item=>item.id===nodeID);if(!node?.config||!Object.hasOwn(node.config,'artifact_from'))return;const source=node.config.artifact_from;if(typeof source!=='string'||!source)return '制品来源必须是单个非空上游节点 ID。';if(!artifactSources(workflow,nodeID,workers,project).some(item=>item.id===source))return '制品来源已不是合法上游，或其构建命令登记已失效。请重新选择或清空。';}
export type CandidatePin={attempt:number;commit:string;source_node:string;source_attempt:number;source_fence:number;evidence_sha256:string};
export function latestCandidatePin(events:{sequence:number;type:string;node_id:string;data?:Record<string,unknown>}[],node:{id:string;attempt:number;state:string}):CandidatePin|undefined{if(['pending','ready','waiting'].includes(node.state))return;const event=events.filter(item=>item.type==='candidate_pinned'&&item.node_id===node.id&&item.data?.attempt===node.attempt).sort((a,b)=>b.sequence-a.sequence)[0],data=event?.data;if(!data||typeof data.commit!=='string'||typeof data.source_node!=='string'||typeof data.source_attempt!=='number'||typeof data.source_fence!=='number'||typeof data.evidence_sha256!=='string')return;return data as CandidatePin;}
