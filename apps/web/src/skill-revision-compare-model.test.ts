import {expect,it} from 'vitest';
import type {SkillCatalogDetail} from '../../../packages/contracts/src';
import {canAdoptComparedCurrent,comparisonRevisions,validRevisionPage} from './skill-revision-compare-model';
const origin={project_id:'source-project',knowledge_id:'skill-one',knowledge_version:4,revision:2,sha256:'a'.repeat(64)};
function entry(version=3):SkillCatalogDetail{return {id:'catalog-one',version,state:'published',title:'Skill',summary:'Summary',domain:'Engineering',scenarios:['Review'],tags:[],origin:{...origin},source_current:true,eligibility:'current',created_by:'owner',created_at:'2026-10-30T00:00:00Z',reviewed_by:'owner',updated_at:'2026-10-30T00:00:00Z',reason:'Approved',can_manage:true,can_adopt:true,allowed_actions:['adopt'],content:'Current approved body'};}
const historical=(version=2):SkillCatalogDetail=>({...entry(version),origin:{...origin,revision:1,sha256:'b'.repeat(64)},content:'Earlier body',can_adopt:false,allowed_actions:[]});
it('rejects wrong, future, duplicated or mutated fixed revisions as one page',()=>{
 const current=entry(),old=historical(),page={items:[{...current,can_adopt:false,allowed_actions:[]},old],total:2,next_cursor:''};
 expect(validRevisionPage(page,current)).toBe(true);
 for(const items of [[{...old,id:'foreign'}],[{...old,version:4}],[old,old],[{...old,origin:{...old.origin,knowledge_id:'foreign'}}],[{...page.items[0],content:'Changed same-version body'}],[{...old,content:'x'.repeat(16001)}],[{...old,can_adopt:true}]])expect(validRevisionPage({...page,items},current)).toBe(false);
 expect(validRevisionPage(null,current)).toBe(false);
});
it('compares immutable text without replacing the current item with a historical projection',()=>{
 const current=entry(),old=historical(),historyCurrent={...current,can_adopt:false,allowed_actions:[]};
 const values=comparisonRevisions(current,[historyCurrent,old]);
 expect(values.map(value=>value.version)).toEqual([3,2]);expect(values[0]).toBe(current);expect(values[1].content).toBe('Earlier body');
});
it('allows only the effective current version through the existing explicit adoption gate',()=>{
 const current=entry(),old=historical();expect(canAdoptComparedCurrent(current,old,current)).toBe(true);expect(canAdoptComparedCurrent(current,old,historical(1))).toBe(false);expect(canAdoptComparedCurrent(current,current,current)).toBe(false);
 for(const patch of [{can_adopt:false},{source_current:false},{eligibility:'source_changed'},{state:'retired'},{allowed_actions:[]}])expect(canAdoptComparedCurrent({...current,...patch},old,current)).toBe(false);
 expect(canAdoptComparedCurrent(current,old,{...current,origin:{...current.origin,sha256:'c'.repeat(64)}})).toBe(false);
});
