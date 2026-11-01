import {expect,it} from 'vitest';
import {continuationBinding,continuationCandidate,continuationOptions,continuationSource} from './development-continuation.test-support';
import {developmentContinuationOptionsPath,readDevelopmentContinuationOptions,sameDevelopmentContinuationSource,validDevelopmentContinuationBinding,validDevelopmentContinuationSource} from './development-continuation-model';

const expected={project:'project-a',task:'task-a',taskVersion:7,limit:20};
it('validates explicit source intent and a sealed same-task binding without treating its digest as current authority',()=>{
 expect(validDevelopmentContinuationSource(continuationSource)).toBe(true);expect(validDevelopmentContinuationBinding(continuationBinding,expected.project,expected.task,7)).toBe(true);
 expect(validDevelopmentContinuationSource({...continuationSource,transfer_id:'chosen-by-client'})).toBe(false);
 for(const patch of [{attempt:0},{fence:0},{expected_run_version:0},{run_id:'other/run'},{credential:'not-an-intent'}])expect(validDevelopmentContinuationSource({...continuationSource,...patch})).toBe(false);
 for(const patch of [{task_id:'other-task'},{task_version:8},{source_task_version:8},{source_task_run_id:'other-run'},{binding_sha256:'invalid'},{token:'private'},{source:{...continuationCandidate,state:'prepared'}},{source:{...continuationCandidate,head_commit:continuationCandidate.bundle_base_commit}},{source:{...continuationCandidate,source:{...continuationCandidate.source,project_id:'other-project'}}}])expect(validDevelopmentContinuationBinding({...continuationBinding,...patch},expected.project,expected.task,7)).toBe(false);
 expect(validDevelopmentContinuationBinding({...continuationBinding,binding_sha256:'f'.repeat(64)},expected.project,expected.task,7)).toBe(true);
});

it('keeps options on the exact task and terminal run while preserving empty reasons and honest page completeness',()=>{
 expect(readDevelopmentContinuationOptions(continuationOptions,expected)).toEqual(continuationOptions);
 const empty={...continuationOptions,source_task_run_id:'',source_run_version:0,source_run_state:'',items:[],total:0,reason:'no_run'};
 expect(readDevelopmentContinuationOptions(empty,expected)).toEqual(empty);
 expect(readDevelopmentContinuationOptions({...continuationOptions,source_run_state:'running',items:[],total:0,reason:'run_not_terminal'},expected).items).toEqual([]);
 for(const reason of ['task_accepted','no_sealed_candidate'])expect(readDevelopmentContinuationOptions({...continuationOptions,items:[],total:0,reason},expected).reason).toBe(reason);
 expect(()=>readDevelopmentContinuationOptions({...empty,reason:'unexpected_reason'},expected)).toThrow();
 const next={...continuationOptions,total:2,complete:false,next_cursor:'opaque-page'};expect(readDevelopmentContinuationOptions(next,expected).next_cursor).toBe('opaque-page');
 for(const patch of [{project_id:'other-project'},{task_version:8},{source_task_run_id:'different-run'},{source_run_version:9},{source_run_state:'running'},{total:0},{limit:100},{complete:false},{next_cursor:'opaque-page'},{items:[...continuationOptions.items,...continuationOptions.items]},{extra:'unknown'},{items:[{...continuationOptions.items[0],selection:{...continuationSource,fence:10}}]},{items:[{...continuationOptions.items[0],candidate:{...continuationCandidate,source:{...continuationCandidate.source,node_id:'other-node'}}}]}])expect(()=>readDevelopmentContinuationOptions({...continuationOptions,...patch},expected)).toThrow();
 expect(()=>readDevelopmentContinuationOptions({...empty,source_run_version:1},expected)).toThrow();
});

it('encodes an opaque bounded seek cursor and compares the complete source identity instead of a display label',()=>{
 expect(developmentContinuationOptionsPath({project:'project+a',task:'task:a',limit:20,before:'opaque/a+b='})).toBe('/tasks/task%3Aa/development-continuation-options?project_id=project%2Ba&limit=20&before=opaque%2Fa%2Bb%3D');
 for(const patch of [{limit:0},{limit:101},{before:'x'.repeat(4097)},{project:'bad/project'}])expect(()=>developmentContinuationOptionsPath({...expected,...patch})).toThrow();
 expect(sameDevelopmentContinuationSource(continuationSource,{...continuationSource})).toBe(true);
 for(const key of ['attempt','fence','expected_run_version'] as const)expect(sameDevelopmentContinuationSource(continuationSource,{...continuationSource,[key]:continuationSource[key]+1})).toBe(false);
 expect(sameDevelopmentContinuationSource(continuationSource,{...continuationSource,node_id:'another-node'})).toBe(false);
});
