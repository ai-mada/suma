import {describe,expect,it} from 'vitest';
import {knowledgeListPath,knowledgeSourceInputs,publishedKnowledgeReference,knowledgeSourceLabel,type KnowledgeSummary} from './knowledge-model';
import {parseWorkbenchRoute,sourceDestination,sourceHref} from './navigation';
import {setLocale} from './i18n';
const published:KnowledgeSummary={id:'knowledge-a',project_id:'project-a',kind:'memory',state:'published',version:3,title:'Raw 用户内容',summary:'summary',latest_revision:3,published_revision:2,published_sha256:'a'.repeat(64),published_title:'Published title',published_summary:'Published summary',has_draft:true,published_eligible:true,eligibility:'current',created_at:'',updated_at:''};
describe('published knowledge selection',()=>{
 it('pins the exact published revision rather than the newer draft',()=>{expect(publishedKnowledgeReference(published,'project-a')).toEqual({id:'knowledge-a',revision:2,sha256:'a'.repeat(64)});});
 it.each([{project_id:'foreign'},{eligibility:'source_changed'},{published_eligible:false},{state:'retired'},{published_sha256:'not-a-hash'}])('refuses unconfirmed or out of project context %j',patch=>{expect(()=>publishedKnowledgeReference({...published,...patch} as KnowledgeSummary,'project-a')).toThrow();});
 it('preserves an exact quote but removes display-only source fields',()=>{expect(knowledgeSourceInputs([{kind:'documents',id:'doc',version:2,title:'name',quote:'  raw\n{{text}}  ',sha256:'b'.repeat(64)}])).toEqual([{kind:'documents',id:'doc',version:2,quote:'  raw\n{{text}}  '}]);});
});
describe('source navigation',()=>{
 it('routes issue results into quality and preserves the exact source record',()=>{expect(sourceDestination('project-a',{type:'issues',id:'issue-a'})).toMatchObject({view:'quality',sub:'issues',record:'issue-a'});});
 it('opens canonical locators and rejects external or unknown source routes',()=>{expect(sourceHref('project-a',{locator:{type:'documents',id:'doc-a',project_id:'project-a'}})).toContain('sub=documents');expect(sourceHref('project-a',{locator:{type:'documents',id:'doc-a',project_id:'foreign'}})).toBeUndefined();expect(sourceHref('project-a',{type:'https://outside',id:'doc-a'})).toBeUndefined();});
 it('keeps only known knowledge filters in refreshable routes',()=>{expect(parseWorkbenchRoute('?view=knowledge&sub=project-knowledge&knowledge_kind=skill').knowledge_kind).toBe('skill');expect(parseWorkbenchRoute('?knowledge_kind=external').knowledge_kind).toBeUndefined();expect(knowledgeListPath('project-a',{kind:'memory',state:'published',q:'known text'})).toContain('state=published');});
});

it('uses complete known source labels in both supported languages without inventing task entity keys',async()=>{
 for(const locale of ['zh-CN','en-US'] as const){await setLocale(locale,{persist:false});for(const kind of ['documents','requirements','tasks','issues'])expect(knowledgeSourceLabel(kind)).not.toMatch(/^(common|shell)\./);expect(knowledgeSourceLabel('tasks')).toBe(locale==='zh-CN'?'任务':'Tasks');}
 await setLocale('zh-CN',{persist:false});
});
