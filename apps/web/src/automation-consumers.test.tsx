import {createElement} from 'react';
import {renderToStaticMarkup} from 'react-dom/server';
import {describe,expect,it} from 'vitest';
import {AutomationRuleFacts,AutomationOccurrenceRow,AutomationUnread,AutomationRevisionFacts} from './AutomationWorkspace';
import type {AutomationRule} from './automation-model';
const rule:AutomationRule={id:'rule-real',project_id:'p',version:8,revision:3,name:'每日校验',workflow_id:'wf-real',workflow_version:2,trigger:'cron',cron:'0 9 * * 1-5',timezone:'Asia/Shanghai',misfire:'catch_up',catch_up_limit:4,overlap:'queue',state:'paused',activated_by:'reviewer-real',next_at:'',last_error:'当前启用人权限已撤销',created_at:'2026-10-24T00:00:00Z',updated_at:'2026-10-24T00:00:00Z'};
describe('automation rendered facts',()=>{
 it('shows persisted pause, exact pinned workflow version, policies and a real dispatch blocker',()=>{
  const html=renderToStaticMarkup(createElement(AutomationRuleFacts,{rule}));
  for(const text of ['已暂停','wf-real','v2','r3','v8','Asia/Shanghai','有界补跑','4','排队等待','当前启用人权限已撤销'])expect(html).toContain(text);
  expect(html).not.toContain('已成功');expect(html).not.toContain('实时连接');
 });
 it('links only a persisted real run and keeps waiting occurrences separate from run completion',()=>{
  const occurrence={id:'occ-real',rule_id:'rule-real',revision:3,source:'manual',scheduled_at:'2026-10-24T00:00:00Z',state:'queued',reason:'前次运行尚未终结',run_id:'',created_at:'2026-10-24T00:00:00Z',updated_at:'2026-10-24T00:00:00Z'};
  const queued=renderToStaticMarkup(createElement(AutomationOccurrenceRow,{occurrence,project:'p'}));
  expect(queued).toContain('等待派发');expect(queued).toContain('前次运行尚未终结');expect(queued).not.toContain('href=');
  const started=renderToStaticMarkup(createElement(AutomationOccurrenceRow,{occurrence:{...occurrence,state:'started',run_id:'run-real'},project:'p'}));
  expect(started).toContain('sub=runs');expect(started).toContain('run=run-real');expect(started).toContain('已绑定运行');expect(started).not.toContain('已成功');
 });
 it('renders the actual joined run state separately from the immutable trigger fact',()=>{
  const html=renderToStaticMarkup(createElement(AutomationOccurrenceRow,{occurrence:{id:'occ',rule_id:'rule-real',revision:1,source:'manual',scheduled_at:'2026-10-24T00:00:00Z',state:'started',reason:'',run_id:'run-real',run_state:'failed',created_at:'2026-10-24T00:00:00Z',updated_at:'2026-10-24T00:00:00Z'} as any,project:'p'}));
  expect(html).toContain('已绑定运行');expect(html).toContain('运行：失败');expect(html).not.toContain('已成功');
 });
 it('uses ingress authority explanations only for webhook occurrences and preserves unknown raw reasons',()=>{
  const occurrence={id:'occ',rule_id:'rule-real',revision:1,source:'webhook',scheduled_at:'2026-10-24T00:00:00Z',state:'blocked',reason:'activation_authority_revoked',run_id:'',created_at:'2026-10-24T00:00:00Z',updated_at:'2026-10-24T00:00:00Z'};
  const webhook=renderToStaticMarkup(createElement(AutomationOccurrenceRow,{occurrence,project:'p'}));expect(webhook).toContain('入口或规则启用人');
  for(const source of ['manual','cron']){const html=renderToStaticMarkup(createElement(AutomationOccurrenceRow,{occurrence:{...occurrence,source},project:'p'}));expect(html).toContain('启用人的项目写入或审批权限已撤销');expect(html).not.toContain('入口或规则启用人');}
  const unknown=renderToStaticMarkup(createElement(AutomationOccurrenceRow,{occurrence:{...occurrence,reason:'future_webhook_reason'},project:'p'}));expect(unknown).toContain('服务端原因');expect(unknown).toContain('future_webhook_reason');
 });
 it('explains server blockers and keeps raw reason codes collapsed for audit',()=>{
  const html=renderToStaticMarkup(createElement(AutomationRuleFacts,{rule:{...rule,last_error:'activation_authority_revoked',state:'enabled',next_at:'2026-10-24T00:00:00Z'}}));
  expect(html).toContain('启用人的项目写入或审批权限已撤销');expect(html).toContain('<summary>原始原因</summary>');expect(html).toContain('activation_authority_revoked');expect(html).toContain('2026/10/24 08:00:00');expect(html).toContain('Asia/Shanghai');expect(html).toContain('UTC 2026-10-24T00:00:00Z');expect(html).toContain('恢复窗口最多 4 次，超出跳过');
 });
 it('renders immutable historical specs without replacing them with the current rule',()=>{
  const html=renderToStaticMarkup(createElement(AutomationRevisionFacts,{revision:{revision:1,spec:{...rule,name:'原始策略',workflow_version:1,cron:'0 8 * * *'},created_at:'2026-10-20T00:00:00Z'}}));
  expect(html).toContain('原始策略');expect(html).toContain('r1');expect(html).toContain('v1');expect(html).toContain('0 8 * * *');expect(html).not.toContain('0 9 * * 1-5');expect(html).not.toContain('编辑策略');
 });
 it('does not turn an unread failed list into an empty successful result',()=>{
  const html=renderToStaticMarkup(createElement(AutomationUnread,{loading:false}));
  expect(html).toContain('尚未读取到规则');expect(html).not.toContain('暂无自动化');expect(html).not.toContain('正在读取');
 });
});
