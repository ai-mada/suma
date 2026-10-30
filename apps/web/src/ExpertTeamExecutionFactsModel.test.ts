import {expect,it} from 'vitest';
import {readTeamExecution,executionNodeHref,executionFocus,type TeamExecutionFact} from './ExpertTeamExecutionFactsModel';
const node=(node_id:string,state='running',attempt=1)=>({node_id,name:node_id,type:'agent',state,reason:'',attempt,worker_id:'runner-a',ready_at:''});
const fact=():TeamExecutionFact=>({run_id:'run-a',observed_at:'2026-10-30T00:00:00Z',nodes_total:1,nodes:[node('code')],truncated:false});
it('binds bounded execution facts to the exact run and refuses ambiguous or malformed snapshots',()=>{
 expect(readTeamExecution(fact(),'run-a')).toEqual(fact());
 for(const value of [{...fact(),run_id:'other'},{...fact(),observed_at:'not-a-date'},{...fact(),nodes_total:0},{...fact(),truncated:true},{...fact(),nodes:[node('code'),node('code')],nodes_total:2},{...fact(),nodes:Array.from({length:25},(_,i)=>node('n'+i)),nodes_total:25},{...fact(),nodes:[node('code','running',-1)]}])expect(readTeamExecution(value,'run-a')).toBeUndefined();
});
it('retains unknown state and literal reason as evidence, while showing the bounded range accurately',()=>{
 const value={...fact(),nodes_total:30,truncated:true,nodes:[{...node('odd','future_state',0),reason:'<script>untrusted evidence</script>'},...Array.from({length:23},(_,i)=>node('n'+i))]};
 expect(readTeamExecution(value,'run-a')).toEqual(value);
 expect(executionFocus(value.nodes)[0].node_id).toBe('odd');
 expect(readTeamExecution({...value,nodes:[]},'run-a')).toBeUndefined();
});
it('preserves the server order of current facts without inventing another priority or sequential completion',()=>{
 const nodes=[node('done','succeeded'),node('ready','ready',0),node('active'),node('failed','failed'),node('unknown','future_state')];
 expect(executionFocus(nodes).map(n=>n.node_id)).toEqual(['done','ready','active']);
 expect(nodes[0].node_id).toBe('done');
});
it('links the observed attempt only when one exists',()=>{
 const actual=new URL(executionNodeHref('project-a','run-a',node('code','running',2)),'http://localhost');
 expect(Object.fromEntries(actual.searchParams)).toMatchObject({project:'project-a',view:'development',sub:'runs',run:'run-a',node:'code',attempt:'2'});
 expect(new URL(executionNodeHref('project-a','run-a',node('code','ready',0)),'http://localhost').searchParams.has('attempt')).toBe(false);
});
