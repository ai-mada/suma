import {describe,it,expect} from 'vitest';
import {renderToStaticMarkup} from 'react-dom/server';
import {RecordLineage} from './RecordLineage';

describe('record evidence navigation',()=>{
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
