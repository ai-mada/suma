import {createElement} from 'react';
import {renderToStaticMarkup} from 'react-dom/server';
import {describe,expect,it} from 'vitest';
import {AutomationIngressFacts,AutomationIngressDeliveryRow,AutomationIngressRevisionFacts} from './AutomationIngress';
import type {AutomationIngress,AutomationIngressDelivery} from './ingress-model';
const entry:AutomationIngress={id:'in-1',project_id:'p',rule_id:'r',rule_revision:3,name:'Git检查',provider:'github',events:['push'],repository:'owner/repo',repository_id:'42',ref:'',conclusions:[],version:5,revision:2,state:'paused',activated_by:'user',key_configured:true,key_generation:1,key_fingerprint:'abc',endpoint_path:'/api/v1/hooks/in-1',created_at:'2026-10-24T00:00:00Z',updated_at:'2026-10-24T00:00:00Z',last_error:''};
const delivery:AutomationIngressDelivery={id:'d1',ingress_id:'in-1',ingress_revision:2,rule_id:'r',rule_revision:3,delivery_id:'external-1',provider:'github',body_sha256:'sha',key_generation:1,received_at:'2026-10-24T00:00:00Z',last_received_at:'2026-10-24T00:00:00Z',duplicate_count:2,state:'accepted',reason:'',occurrence_id:'occ',event:{schema_version:1,event:'push',repository:'owner/repo',repository_id:'42',ref:'refs/heads/main',commit:'a'.repeat(40),conclusion:'success',external_id:''}};
describe('ingress displayed evidence',()=>{
 it('shows all-branch semantics, pinned revision and fingerprint without implying sender configuration',()=>{
  const html=renderToStaticMarkup(createElement(AutomationIngressFacts,{entry}));
  for(const text of ['所有分支和标签','r3','第 1 代','abc','/api/v1/hooks/in-1','已暂停','发送端'])expect(html).toContain(text);expect(html).not.toContain('原生接入成功');
 });
 it('keeps accepted delivery, duplicates, source conclusion and actual run evidence distinct',()=>{
  const html=renderToStaticMarkup(createElement(AutomationIngressDeliveryRow,{delivery,project:'p',timezone:'Asia/Shanghai'}));
  expect(html).toContain('已创建触发');expect(html).toContain('重复 2 次');expect(html).toContain('尚未关联运行');expect(html).not.toContain('sub=runs');expect(html).not.toContain('测试通过');
  const run=renderToStaticMarkup(createElement(AutomationIngressDeliveryRow,{delivery:{...delivery,run_id:'real/run',run_state:'failed'},project:'p',timezone:'Asia/Shanghai'}));expect(run).toContain('run=real%2Frun');expect(run).toContain('运行：失败');
 });
 it('renders ignored and blocked as persisted decisions and preserves raw reasons',()=>{
  const html=renderToStaticMarkup(createElement(AutomationIngressDeliveryRow,{delivery:{...delivery,state:'blocked',reason:'activation_authority_revoked',occurrence_id:''},project:'p'}));expect(html).toContain('门禁阻止');expect(html).toContain('activation_authority_revoked');expect(html).not.toContain('已创建触发');
 });
 it('explains a persisted filter mismatch without asserting a specific unmatched field',()=>{
  const html=renderToStaticMarkup(createElement(AutomationIngressDeliveryRow,{delivery:{...delivery,state:'ignored',reason:'filter_mismatch',occurrence_id:'',event:{...delivery.event,event:'monitor.resolved'}},project:'p'}));
  expect(html).toContain('受限条件');expect(html).toContain('当时来源修订');expect(html).toContain('<summary>原始原因</summary>');expect(html).toContain('filter_mismatch');expect(html).not.toContain('未订阅');
 });
 it('uses historical filtering and rule revision instead of the current ingress spec',()=>{
  const html=renderToStaticMarkup(createElement(AutomationIngressRevisionFacts,{revision:{revision:1,rule_revision:1,spec:{...entry,name:'原始过滤',ref:'refs/heads/old'},created_at:entry.created_at}}));expect(html).toContain('原始过滤');expect(html).toContain('refs/heads/old');expect(html).toContain('r1');expect(html).not.toContain('编辑');
 });
});
