import {afterEach,beforeEach,expect,it} from 'vitest';
import {readFileSync} from 'node:fs';
import ts from 'typescript';
import {i18n,setLocale} from './index';
import {literalText,formatLiteralValues} from './literal';
import {flowHandleLabel} from '../flow-localization';

const raw='$t(flow.zoom_in) $& 用户';
// The independent guard's same eight adversarial name/peer scenarios. Numbers
// are already formatted strings here, as required by the caller-owned contract.
const fixtures=[
 {key:'development.workflow.graph_node_aria',params:{name:'{{id}} {{state}} '+raw,id:'node-1',state:'Pending'},en:'Node {{id}} {{state}} '+raw+' (node-1): Pending',zh:'节点 {{id}} {{state}} '+raw+' (node-1)：Pending'},
 {key:'development.workflow.upstream_node',params:{name:'{{id}} '+raw,id:'node-1'},en:'Upstream: {{id}} '+raw+' (node-1)',zh:'上游：{{id}} '+raw+' (node-1)'},
 {key:'development.candidate.source_option',params:{name:'{{id}} {{type}} '+raw,id:'source',type:'Command'},en:'{{id}} {{type}} '+raw+' · source (Command)',zh:'{{id}} {{type}} '+raw+' · source (Command)'},
 {key:'development.candidate.build_option',params:{name:'{{id}} {{command}} '+raw,id:'build',command:'build-command'},en:'{{id}} {{command}} '+raw+' · build · Registered build · build-command',zh:'{{id}} {{command}} '+raw+' · build · 已登记构建 · build-command'},
 {key:'collaboration.capacity_resource_week',params:{name:'{{week}} '+raw,week:'2026-10-26'},en:'{{week}} '+raw+' · Week starting 2026-10-26',zh:'{{week}} '+raw+' · 2026-10-26 开始的一周'},
 {key:'development.dispatch.tool_option',params:{name:'{{worker}} {{version}} {{status}} '+raw,worker:'worker-1',version:'3',status:'Eligible'},en:'{{worker}} {{version}} {{status}} '+raw+' · worker-1 · v3 · Eligible',zh:'{{worker}} {{version}} {{status}} '+raw+' · worker-1 · v3 · Eligible'},
 {key:'automation.fixed_rule_basis',params:{name:'{{revision}} '+raw,revision:'7'},en:'Fixed rule: {{revision}} '+raw+' r7',zh:'固定规则：{{revision}} '+raw+' r7'},
 {key:'release.prepare.environment_option',params:{name:'{{archived}} {{pending}} '+raw,archived:' (Archived)',pending:' (Pending)'},en:'{{archived}} {{pending}} '+raw+' (Archived) (Pending)',zh:'{{archived}} {{pending}} '+raw+' (Archived) (Pending)'},
] as const;
// Deliberately exercise runtime validation separately from the compile-time API.
const runtime=formatLiteralValues as (key:unknown,values:unknown)=>{text:string;formatted:boolean};
const original=new Map<string,unknown>();
beforeEach(async()=>{await setLocale('en-US',{persist:false});for(const locale of ['en-US','zh-CN'])for(const key of [...fixtures.map(row=>row.key),'flow.handle.input','flow.handle.output'])original.set(locale+':'+key,i18n.getResource(locale,'translation',key));});
afterEach(()=>{for(const [entry,value] of original){const split=entry.indexOf(':');i18n.addResource(entry.slice(0,split),'translation',entry.slice(split+1),value as string);}original.clear();});

for(const locale of ['en-US','zh-CN'] as const)for(const row of fixtures)it(`${locale}: ${row.key} keeps all inserted values literal`,async()=>{
 await setLocale(locale,{persist:false});const before=JSON.stringify(row.params);
 expect(runtime(row.key,row.params)).toEqual({text:locale==='en-US'?row.en:row.zh,formatted:true});expect(JSON.stringify(row.params)).toBe(before);
});
it('keeps empty suffixes legal without adding meaning or formatting caller-owned values',async()=>{
 for(const locale of ['en-US','zh-CN']){await setLocale(locale,{persist:false});expect(literalText('release.prepare.environment_option',{name:'{{archived}} {{pending}} '+raw,archived:'',pending:''})).toBe('{{archived}} {{pending}} '+raw);}
});
it('scans only the controlled template, independent of variable order and repeated placeholders',async()=>{
 for(const locale of ['en-US','zh-CN']){await setLocale(locale,{persist:false});i18n.addResource(locale,'translation','flow.handle.input','{{id}} / {{name}} / {{id}}');
 expect(literalText('flow.handle.input',{id:'{{name}} $& $$',name:'{{id}} $t(flow.zoom_in) " <img>'})).toBe('{{name}} $& $$ / {{id}} $t(flow.zoom_in) " <img> / {{name}} $& $$');}
});
it.each([undefined,null,[],42,{},'','{{name}}','{{name}} {{id}} {{future}}','{{name}} {{id}} {{','{{name, uppercase}} {{id}}','{{name}} {{id}}\uD800'])('falls back honestly for a missing or incompatible resource %j',async value=>{
 for(const locale of ['en-US','zh-CN']){await setLocale(locale,{persist:false});i18n.addResource(locale,'translation','flow.handle.input',value as string);
 expect(formatLiteralValues('flow.handle.input',{name:'{{id}} raw name',id:'raw-id'})).toEqual({text:'{{id}} raw name · raw-id',formatted:false});
 expect(flowHandleLabel('input','{{id}} raw name','raw-id')).toBe((locale==='en-US'?'Connection handle':'连线端点')+': {{id}} raw name (raw-id)');}
});
it('recovers the full label immediately when the approved resource returns',async()=>{
 for(const locale of ['en-US','zh-CN']){await setLocale(locale,{persist:false});const generic=locale==='en-US'?'Connection handle':'连线端点';
 const template=i18n.getResource(locale,'translation','flow.handle.input');i18n.addResource(locale,'translation','flow.handle.input',null as never);
 expect(flowHandleLabel('input','{{id}} raw','id')).toBe(generic+': {{id}} raw (id)');
 i18n.addResource(locale,'translation','flow.handle.input',template);
 expect(flowHandleLabel('input','{{id}} raw','id')).toBe(locale==='en-US'?'Input connection (target) for node {{id}} raw (id)':'节点 {{id}} raw（id）的输入连接点（目标）');
 expect(flowHandleLabel('unknown' as never,'{{id}} raw','id')).toBe(generic+': {{id}} raw (id)');}
});
it.each(['display.tool.dispatchable','native.endpoint.rejected','automation.ingress_revision_title','future.key','constructor','__proto__',42,null])('does not read unapproved template keys %j',async key=>{
 for(const locale of ['en-US','zh-CN']){await setLocale(locale,{persist:false});expect(runtime(key,{name:'raw {{id}}',id:'raw-id'})).toEqual({text:'raw {{id}} · raw-id',formatted:false});}
});
it.each([
 {name:'raw',id:'id',extra:'extra'}, {name:'raw'}, {name:'raw',id:1}, {name:'raw',id:null},
 ['raw','id'],Object.assign(Object.create({inherited:'text'}),{name:'raw',id:'id'}),
])('refuses a wrong parameter shape while retaining available raw values %j',async params=>{
 for(const locale of ['en-US','zh-CN']){await setLocale(locale,{persist:false});const result=runtime('flow.handle.input',params);expect(result.formatted).toBe(false);expect(result.text).toContain('raw');}
});
it('requires own scalar data fields without evaluating accessors or mutating the object',()=>{
 let invoked=false;const values={name:'raw',get id(){invoked=true;return 'id';}};
 expect(runtime('flow.handle.input',values)).toEqual({text:'raw',formatted:false});expect(invoked).toBe(false);
 const symbols={name:'raw',id:'id',[Symbol('extra')]:'extra'};expect(runtime('flow.handle.input',symbols).formatted).toBe(false);
});

it('keeps exactly the approved eight consumer keys on the literal API, without scanning other translations',()=>{
 const owners:Record<string,string[]>={
  'Workflow.tsx':['development.workflow.graph_node_aria','development.workflow.upstream_node'],
  'CandidateSource.tsx':['development.candidate.source_option','development.candidate.build_option'],
  'Planning.tsx':['collaboration.capacity_resource_week'],
  'TaskDispatch.tsx':['development.dispatch.tool_option'],
  'AutomationIngress.tsx':['automation.fixed_rule_basis'],
  'ReleaseForms.tsx':['release.prepare.environment_option'],
 };
 for(const [file,keys] of Object.entries(owners)){
  const source=ts.createSourceFile(file,readFileSync(new URL('../'+file,import.meta.url),'utf8'),ts.ScriptTarget.Latest,true,ts.ScriptKind.TSX),calls=new Map<string,string[]>();
  const visit=(node:ts.Node)=>{if(ts.isCallExpression(node)&&node.arguments[0]&&ts.isStringLiteral(node.arguments[0])&&keys.includes(node.arguments[0].text)){const key=node.arguments[0].text;calls.set(key,[...(calls.get(key)||[]),node.expression.getText(source)]);}ts.forEachChild(node,visit);};visit(source);
  for(const key of keys)expect(calls.get(key),file+': '+key).toEqual(['literalText']);
  expect(source.statements.some(statement=>ts.isImportDeclaration(statement)&&statement.moduleSpecifier.getText(source)==="'./i18n/literal'"),file).toBe(true);
 }
});

// The public typed boundary rejects unknown keys, missing/extra fields and
// unformatted numbers. Runtime tests above deliberately bypass these checks.
if(false){
 // @ts-expect-error reserved diagnostic keys are not approved UI templates
 literalText('display.tool.dispatchable',{name:'raw'});
 // @ts-expect-error name/id is the exact Handle shape
 literalText('flow.handle.input',{name:'raw'});
 // @ts-expect-error extra fields are not approved
 literalText('flow.handle.input',{name:'raw',id:'id',extra:'text'});
 // @ts-expect-error numbers remain the caller's formatting responsibility
 literalText('development.dispatch.tool_option',{name:'raw',worker:'worker',version:3,status:'Eligible'});
}
