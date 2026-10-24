import {describe,it,expect} from 'vitest';
import {renderToStaticMarkup} from 'react-dom/server';
import {RecordLineage,RecordRequirementReference} from './RecordLineage';
import {parseWorkbenchRoute} from './navigation';

describe('record evidence navigation',()=>{
 it('opens an exact linked requirement independently of loaded rows in the current project route',()=>{
  const item={id:'task-1',project_id:'project-a',version:7,requirement_id:'requirement-outside-loaded-window'};
  const html=renderToStaticMarkup(<RecordRequirementReference project="project-a" item={item}/>);
  expect(html).toContain('查看关联需求');expect(html).toContain(item.requirement_id);
  const href=html.match(/href="([^"]+)"/)?.[1].replaceAll('&amp;','&');expect(href).toBeDefined();
  expect(parseWorkbenchRoute(href!)).toMatchObject({project:'project-a',view:'requirements',record:item.requirement_id,run:'',node:''});
  expect(renderToStaticMarkup(<RecordRequirementReference project="other-project" item={item}/>)).toBe('');
  expect(renderToStaticMarkup(<RecordRequirementReference project="project-a" item={{...item,requirement_id:'https://example.invalid'}}/>)).toBe('');
 });
 it('keeps the current project and exact persisted issue, operation and run',()=>{
  const html=renderToStaticMarkup(<RecordLineage project="project-a" item={{id:'requirement',project_id:'project-a',version:1,data:{source_issue_id:'issue-1',source:{release_operation_id:'deploy-1'}},run_id:'run-1'}}/>);
  expect(html).toContain('查看来源问题');
  expect(html).toContain('project=project-a');
  expect(html).toContain('record=issue-1');
  expect(html).toContain('record=deploy-1');
  expect(html).toContain('run=run-1');
 });
 it('does not turn unrelated project or arbitrary URLs into navigation',()=>{
  expect(renderToStaticMarkup(<RecordLineage project="project-a" item={{id:'x',project_id:'other',version:1,source_issue_id:'issue'}}/>)).toBe('');
  expect(renderToStaticMarkup(<RecordLineage project="project-a" item={{id:'x',version:1,source_issue_id:'https://example.invalid',data:{release_operation_id:4}}}/>)).toBe('');
 });
 it('reads normalized records without inventing object names',()=>{
  const html=renderToStaticMarkup(<RecordLineage project="project-a" item={{id:'x',version:1,source_issue_id:'issue-exact',release_operation_id:'deploy-exact'}}/>);
  expect(html).toContain('issue-exact');
  expect(html).toContain('deploy-exact');
  expect(html).not.toContain('已解决');
 });
});
