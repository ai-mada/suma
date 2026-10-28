import {useEffect,useMemo,useState} from 'react';
import {Background,Controls,MarkerType,ReactFlow,type Node} from '@xyflow/react';
import ELK from 'elkjs/lib/elk.bundled.js';
import type {ApplicationDependencyGraph} from '../../../packages/contracts/src';
import {flowAriaLabelConfig} from './flow-localization';
import {tr,useLocale} from './i18n';
import {Loading} from './ui';
import '@xyflow/react/dist/style.css';

const elk=new ELK();
/** This canvas is a read-only projection. Its equivalent relation list remains
 * available even if layout fails or an edge points outside the loaded graph. */
export function ProjectApplicationGraph({graph,selected,onSelect}:{graph:ApplicationDependencyGraph;selected:string;onSelect:(id:string)=>void}){
 useLocale();const [positions,setPositions]=useState<{sha:string;nodes:Node[]}>(),[failed,setFailed]=useState(false);
 useEffect(()=>{let current=true;setFailed(false);const ids=new Set(graph.nodes.map(node=>node.application_id));
  void elk.layout({id:'applications',layoutOptions:{'elk.algorithm':'layered','elk.direction':'RIGHT','elk.spacing.nodeNode':'30','elk.layered.spacing.nodeNodeBetweenLayers':'70'},children:graph.nodes.map(node=>({id:node.application_id,width:190,height:74})),edges:graph.edges.filter(edge=>ids.has(edge.source_application_id)&&ids.has(edge.target_application_id)).map((edge,index)=>({id:'dependency-'+index,sources:[edge.source_application_id],targets:[edge.target_application_id]}))}).then(layout=>{if(current)setPositions({sha:graph.sha256,nodes:graph.nodes.map(node=>{const item=layout.children?.find(child=>child.id===node.application_id);return {id:node.application_id,position:{x:item?.x||0,y:item?.y||0},data:{label:node.name},style:{width:190,minHeight:74,background:'var(--surface)',color:'var(--text)',border:'1px solid var(--border)',fontSize:12}};})});}).catch(()=>{if(current)setFailed(true);});
  return()=>{current=false;};
 },[graph.sha256]);
 const nodes=useMemo(()=>positions?.sha===graph.sha256?positions.nodes.map(node=>({...node,selected:node.id===selected})):[],[positions,graph.sha256,selected]);
 const ids=new Set(nodes.map(node=>node.id)),edges=graph.edges.flatMap((edge,index)=>ids.has(edge.source_application_id)&&ids.has(edge.target_application_id)?[{id:'dependency-'+index,source:edge.source_application_id,target:edge.target_application_id,markerEnd:{type:MarkerType.ArrowClosed,color:'var(--muted)'},style:{stroke:'var(--muted)'},label:edge.target_version_id.slice(0,8)}]:[]);
 if(failed)return <p className="pa-error">{tr('project-applications.graph_unavailable')}</p>;
 if(!positions||positions.sha!==graph.sha256)return <Loading/>;
 return <div className="pa-graph" role="region" aria-label={tr('project-applications.graph')}><ReactFlow key={graph.sha256} nodes={nodes} edges={edges} nodesDraggable={false} nodesConnectable={false} edgesReconnectable={false} deleteKeyCode={null} ariaLabelConfig={flowAriaLabelConfig(false)} onNodeClick={(_,node)=>onSelect(node.id)} fitView fitViewOptions={{padding:.2,maxZoom:1}} minZoom={.15} maxZoom={1.6}><Background color="var(--grid)" gap={24}/><Controls showInteractive={false}/></ReactFlow></div>;
}
