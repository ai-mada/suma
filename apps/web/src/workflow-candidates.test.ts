import {createElement} from 'react';
import {renderToStaticMarkup} from 'react-dom/server';
import {describe,expect,it} from 'vitest';
import type {Workflow} from './api';
import {ArtifactSourceField,CandidateSource,CandidateProvenance} from './CandidateSource';
import {WorkspaceEvidence} from './WorkspaceEvidence';
import {artifactSources,artifactSourceConfig,artifactSourceError,candidateSources,candidateSourceConfig,candidateSourceError,workflowWithEdges,latestCandidatePin,latestProgress} from './workflow-model';

const workflow=():Workflow=>({id:'real-flow',name:'候选验证',version:3,nodes:[
 {id:'code',name:'生成代码',type:'command',config:{repository:'admin-config'}},
 {id:'agent',name:'修改代码',type:'agent'},
 {id:'approval',name:'审阅',type:'approval'},
 {id:'script',name:'其他脚本',type:'script'},
 {id:'other',name:'旁支代码',type:'command'},
 {id:'test',name:'候选测试',type:'test',config:{timeout_label:'preserve-me',compensation:{type:'command',command:['cleanup']}}},
],edges:[{from:'code',to:'agent'},{from:'agent',to:'approval'},{from:'approval',to:'test'},{from:'script',to:'test'}]});
describe('workflow candidate consumers',()=>{
 it('offers only this workflow’s static command/agent ancestors, including transitive dependencies',()=>{
  const w=workflow();expect(candidateSources(w,'test').map(node=>node.id)).toEqual(['code','agent']);
  expect(candidateSources(w,'approval')).toEqual([]);expect(candidateSources(w,'missing')).toEqual([]);
  expect(candidateSources({...w,edges:[...w.edges,{from:'test',to:'code'}]},'test')).toEqual([]);
 });
 it('uses the same current-edge payload for candidate selection and publication, dropping removed legacy dependencies',()=>{
  const w=workflow();w.nodes.find(node=>node.id==='test')!.depends_on=['other'];
  expect(candidateSources(w,'test').map(node=>node.id)).toContain('other');
  const current=workflowWithEdges(w,w.edges.filter(edge=>edge.to!=='test'));
  expect(candidateSources(current,'test')).toEqual([]);expect(current.nodes.find(node=>node.id==='test')!.depends_on).toEqual([]);
  expect(()=>candidateSourceConfig(current,'test','other')).toThrow('合法');expect(w.nodes.find(node=>node.id==='test')!.depends_on).toEqual(['other']);
 });
 it('writes one candidate_from field into the actual published node config and preserves advanced JSON',()=>{
  const w=workflow(),test=w.nodes.find(node=>node.id==='test')!,config=candidateSourceConfig(w,'test','agent');
  const published=workflowWithEdges({...w,version:w.version+1,nodes:w.nodes.map(node=>node.id==='test'?{...node,config}:node)},w.edges);
  const consumer=JSON.parse(JSON.stringify({project_id:'project-real',workflow:published}));
  expect(consumer.workflow.nodes.find((node:any)=>node.id==='test').config).toEqual({...test.config,candidate_from:'agent'});
  expect(test.config).not.toHaveProperty('candidate_from');expect(candidateSourceError(published,'test')).toBeUndefined();
 });
 it('clears the property instead of writing an empty string, and refuses arbitrary sibling/type/cross-run inputs',()=>{
  const w=workflow();w.nodes.find(node=>node.id==='test')!.config!.candidate_from='agent';
  const cleared=candidateSourceConfig(w,'test','');expect(cleared).not.toHaveProperty('candidate_from');expect(cleared.timeout_label).toBe('preserve-me');
  for(const source of ['other','approval','script','missing','child-run:code'])expect(()=>candidateSourceConfig(w,'test',source)).toThrow('合法');
  expect(()=>candidateSourceConfig(w,'code','agent')).toThrow('合法');
 });
 it('keeps advanced invalid input visible for correction without adding it as a selectable legal candidate',()=>{
  const w=workflow();w.nodes.find(node=>node.id==='test')!.config!.candidate_from=['code','agent'];
  expect(candidateSourceError(w,'test')).toContain('单个');
  const html=renderToStaticMarkup(createElement(CandidateSource,{workflow:w,nodeID:'test',onChange:()=>{}}));
  expect(html).toContain('清除无效候选配置');expect(html).toContain('本地 Runner 需配置 repository');expect(html).toContain('多个分支不会自动聚合');
  expect(html).toContain('value="code"');expect(html).toContain('value="agent"');expect(html).not.toContain('value="other"');expect(html).not.toContain('value="approval"');
  expect(renderToStaticMarkup(createElement(CandidateSource,{workflow:w,nodeID:'approval',onChange:()=>{}}))).toBe('');
 });
 it('lets an immutable command or agent build fix one legal code ancestor, preserving its advanced build config',()=>{
  const w=workflow(),build=w.nodes.find(node=>node.id==='agent')!;build.command=['site-build'];build.config={build_tag:'keep'};
  expect(candidateSources(w,'agent').map(node=>node.id)).toEqual(['code']);
  expect(candidateSourceConfig(w,'agent','code')).toEqual({build_tag:'keep',candidate_from:'code'});
  const html=renderToStaticMarkup(createElement(CandidateSource,{workflow:w,nodeID:'agent',onChange:()=>{}}));
  expect(html).toContain('构建候选来源');expect(html).toContain('ArtifactOutput');expect(html).toContain('value="code"');expect(html).not.toContain('value="other"');
  w.nodes.find(node=>node.id==='agent')!.type='command';expect(candidateSourceConfig(w,'agent','code')).toMatchObject({candidate_from:'code'});
 });
 it('offers only static build ancestors backed by real active project profile registrations, not arbitrary commands',()=>{
  const w=workflow();w.nodes.find(node=>node.id==='code')!.command=['site-build'];w.nodes.find(node=>node.id==='agent')!.command=['source-only'];
  const workers=[{project_id:'p',status:'active',expires_at:'2099-01-01T00:00:00Z',profiles:[{id:'site-build',kind:'build',sha256:'a'.repeat(64)},{id:'source-only',kind:'package_test',sha256:'b'.repeat(64)}]}];
  expect(artifactSources(w,'test',workers,'p').map(node=>node.id)).toEqual(['code']);
  expect(artifactSources(w,'test',workers,'other-project')).toEqual([]);
  expect(artifactSources(w,'test',[{...workers[0],status:'revoked'}],'p')).toEqual([]);
  expect(artifactSources(w,'test',[{...workers[0],expires_at:'2020-01-01T00:00:00Z'}],'p')).toEqual([]);
  const config=artifactSourceConfig(w,'test','code',workers,'p');expect(config).toMatchObject({artifact_from:'code',timeout_label:'preserve-me'});
  expect(()=>artifactSourceConfig(w,'test','agent',workers,'p')).toThrow('登记');expect(w.nodes.find(node=>node.id==='test')!.config).not.toHaveProperty('artifact_from');
  w.nodes.find(node=>node.id==='test')!.config=config;expect(artifactSourceConfig(w,'test','',[], 'p')).not.toHaveProperty('artifact_from');expect(artifactSourceError(w,'test',[], 'p')).toContain('失效');
  const html=renderToStaticMarkup(createElement(ArtifactSourceField,{workflow:w,nodeID:'test',project:'p',workers,loading:false,error:'',onRetry:()=>{},onChange:()=>{}}));
  expect(html).toContain('包测试制品来源');expect(html).toContain('value="code"');expect(html).not.toContain('value="agent"');expect(html).toContain('不代表构建已成功');expect(html).toContain('site-build');
  const unavailable=renderToStaticMarkup(createElement(ArtifactSourceField,{workflow:w,nodeID:'test',project:'p',workers:[],loading:false,error:'无法读取登记',onRetry:()=>{},onChange:()=>{}}));expect(unavailable).toContain('无法读取登记');expect(unavailable).toContain('重新读取');expect(unavailable).not.toContain('已登记构建 ·');
 });
 it('shows pinned source provenance only for the currently selected test attempt',()=>{
  const events=[{sequence:1,type:'candidate_pinned',node_id:'test',data:{attempt:1,commit:'old-head',source_node:'code',source_attempt:1,source_fence:1,evidence_sha256:'old-proof'}},{sequence:2,type:'candidate_pinned',node_id:'test',data:{attempt:2,commit:'tested-head',source_node:'agent',source_attempt:3,source_fence:9,evidence_sha256:'source-proof'}}];
  const proof=latestCandidatePin(events,{id:'test',attempt:2,state:'running'})!;expect(proof.commit).toBe('tested-head');expect(proof.source_node).toBe('agent');
  expect(latestCandidatePin(events,{id:'test',attempt:3,state:'running'})).toBeUndefined();expect(latestCandidatePin(events,{id:'other',attempt:2,state:'running'})).toBeUndefined();
  const html=renderToStaticMarkup(createElement(CandidateProvenance,{proof}));for(const value of ['tested-head','agent','来源尝试','来源租约','source-proof','candidate_pinned'])expect(html).toContain(value);expect(html).not.toContain('old-head');
 });
 it.each(['pending','ready','waiting'])('hides prior attempt output and candidate proof while %s without deleting history',state=>{
  const events=[{sequence:1,type:'node_progress',node_id:'test',data:{attempt:1,fence:7,message:'prior attempt output'}},{sequence:2,type:'candidate_pinned',node_id:'test',data:{attempt:1,commit:'prior-head',source_node:'agent',source_attempt:1,source_fence:6,evidence_sha256:'prior-proof'}}];
  const history=JSON.stringify(events),node={id:'test',attempt:1,fence:7,state};
  expect(latestProgress(events,node)).toBeUndefined();expect(latestCandidatePin(events,node)).toBeUndefined();
  expect(JSON.stringify(events)).toBe(history);
  expect(latestProgress(events,{...node,state:'failed'})).toBe('prior attempt output');expect(latestCandidatePin(events,{...node,state:'failed'})?.commit).toBe('prior-head');
  expect(latestProgress(events,{...node,attempt:2,fence:8,state:'running'})).toBeUndefined();expect(latestCandidatePin(events,{...node,attempt:2,state:'running'})).toBeUndefined();
 });
 it('labels actual workspace source fields separately from the administrator base and candidate HEAD',()=>{
  const html=renderToStaticMarkup(createElement(WorkspaceEvidence,{evidence:{admin_base_commit:'admin-base',base_commit:'tested-head',head_commit:'tested-head',candidate_from:'agent',candidate_attempt:3,candidate_fence:9,candidate_evidence_sha256:'source-proof'}}));
  for(const value of ['管理员基准','admin-base','候选来源节点','agent','来源尝试','来源租约','来源证据摘要','source-proof'])expect(html).toContain(value);
  expect(html).toContain('未在此回执中记录');
 });
});
