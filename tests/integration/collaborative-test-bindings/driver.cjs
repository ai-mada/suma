'use strict';
const fs=require('node:fs'),path=require('node:path'),http=require('node:http'),net=require('node:net'),crypto=require('node:crypto'),assert=require('node:assert/strict');
const data=require('./fixtures.cjs'),{closeOwnedNative}=require('../support/close-native.cjs');
const root=path.resolve(__dirname,'../../..'),base=path.join(root,'.cache/collaborative-test-bindings-223/browser'),out=path.join(base,'out');
const directory=path.join(base,'visual',new Date().toISOString().replace(/[^0-9TZ]/g,''));fs.mkdirSync(directory,{recursive:true});
const hash=value=>crypto.createHash('sha256').update(value).digest('hex');
const result={stage:'T223',scope:'Production collaboration entry components and selected Doctor over synthetic public HTTP; not a real database, Worker, model or full App',passed:false,groups:{},requests:[],unexpected:[],images:[],page_errors:[],console_errors:[],console_warnings:[]};
const finalOnly=process.argv.includes('--final-only'),remaining=process.argv.includes('--remaining')||finalOnly;
if(remaining){const prior='.cache/collaborative-test-bindings-223/browser/visual/20261103T005240052Z/result.json';result.reused_discussion={path:prior,sha256:hash(fs.readFileSync(path.join(root,prior))),scope:'Only original completed CN discussion group; prior overall run failed in unfinished team revision fixture without explicitly selecting its row.'};}
if(finalOnly){const prior='.cache/collaborative-test-bindings-223/browser/visual/20261103T005414130Z/result.json';result.reused_team={path:prior,sha256:hash(fs.readFileSync(path.join(root,prior))),scope:'Only completed CN expert create/explicit-definition-revision; its overall run failed on incomplete synthetic recommendation coverage reason.'};}
const save=()=>fs.writeFileSync(path.join(directory,'result.json'),JSON.stringify(result,null,2)+'\n');
let discussion,team,recommended=data.recommendation;
const selection={node_id:'verify',tool_profile_id:'tester',expected_tool_version:3};
function closed(port){return new Promise(resolve=>{const socket=net.connect({host:'127.0.0.1',port});socket.on('connect',()=>{socket.destroy();resolve(false);});socket.on('error',error=>resolve(error.code==='ECONNREFUSED'));socket.setTimeout(1000,()=>{socket.destroy();resolve(false);});});}
async function main(){let server,app,exit,page;
 try{
  server=http.createServer((request,response)=>{
   const url=new URL(request.url,'http://127.0.0.1'),route=url.pathname.replace(/^\/api\/v1/,''),item={method:request.method,path:url.pathname,query:url.search};result.requests.push(item);
   const send=(value,status=200)=>{item.status=status;response.writeHead(status,{'Content-Type':'application/json','Cache-Control':'no-store'});response.end(JSON.stringify(value));};
   if(request.method!=='GET'){
    let raw='';request.on('data',chunk=>{raw+=chunk;if(raw.length>65536)request.destroy();});request.on('end',()=>{
     try{
      const body=JSON.parse(raw);item.body=body;
      assert.equal(request.headers['x-suma-expected-user'],data.session.user.id);assert.equal(request.headers['x-suma-expected-organization'],data.session.organization.id);
      if(route==='/task-discussion-development-plans'){assert.deepEqual(body.test_bindings,[selection]);discussion=data.discussionPlan(body);return send(discussion,201);}
      if(route==='/dispatch-recommendations/recommendation-one/adopt'){assert.deepEqual(body.test_bindings,[selection]);assert.equal(Object.keys(body.choice).length,5);const value=data.adoption(body);recommended=value.recommendation;return send(value);}
      if(route==='/expert-team-plans'){assert.deepEqual(body.items[0].test_bindings,[selection]);team=data.teamPlan(body);return send(team,201);}
      if(route==='/expert-team-plans/team-preview/commands'&&body.action==='revise'){assert.deepEqual(body.items[0].test_bindings,[]);team=data.teamPlan(body,2,2);return send(team);}
      throw Error('Unexpected synthetic mutation '+route);
     }catch(error){result.unexpected.push(String(error));send({error:{code:'synthetic_contract_failure',message:String(error)}},400);}
    });return;
   }
   if(url.pathname==='/__fixture/view')return send({...data,recommendationScope:{project_id:data.project,task_id:data.recommendationTask.id,expected_task_version:2,workflow_id:'flow-a',workflow_version:3,agent_node_id:'develop',knowledge:[]},profiles:[data.developer,data.tester]});
   if(url.pathname==='/favicon.ico'){response.writeHead(204);response.end();return;}
   if(route==='/workflows')return send({items:[data.workflow]});
   if(route==='/tool-profiles')return send({items:[data.developer,data.tester],total:2,next_cursor:''});
   if(route==='/tool-profiles/developer')return send(data.developer);
   if(route==='/tool-profiles/tester')return send(data.tester);
   if(route==='/tool-profiles/doctor-profile/diagnostics')return send(data.doctorReport());
   if(route==='/tasks/task-discussion/dispatch-options')return send({project_id:data.project,task_id:data.discussionTask.id,task_version:7,workflow_id:'flow-a',workflow_version:3,agent_node_id:'develop',observed_at:new Date().toISOString(),context:{task:data.discussionTask},execution_route:data.reply.source.execution_route,items:[{tool:data.developer,eligible:true,reason_code:'eligible',reason:'',model_state:'fixed_by_tool',cost_state:'unknown',eligible_basis:'current_observation_and_compatibility'}],total:1,next_cursor:'',recommended_tool_profile_id:'developer'});
   if(route==='/tasks/task-discussion/development-continuation-options')return send({project_id:data.project,task_id:data.discussionTask.id,task_version:7,items:[],total:0,next_cursor:''});
   if(route==='/task-discussion-development-plans/discussion-preview'&&discussion)return send(discussion);
   if(route==='/dispatch-recommendations/recommendation-one')return send(recommended);
   if(route==='/dispatch-recommendations')return send({items:[recommended],total:1,next_cursor:''});
   if(route==='/runs/analysis-run')return send({id:'analysis-run',project_id:data.project,nodes:[{id:'recommend',tool_profile_id:recommended.analysis_tool_profile_id,tool_profile_sha256:recommended.analysis_tool_profile_sha256}]});
   if(route==='/expert-teams')return send({items:[data.configuration],total:1,next_cursor:''});
   if(route==='/expert-teams/team-one')return send(data.configuration);
   if(route==='/tasks/task-team')return send(data.teamTask);
   if(route==='/expert-team-plans/team-preview'&&team)return send(team);
   if(route==='/expert-team-plans')return send({items:team?[team]:[],total:team?1:0,next_cursor:''});
   if(route==='/model-execution/options')return send({project_id:data.project,tool_profile_id:'developer',tool_profile_version:2,tool_profile_sha256:data.developer.profile.profile_sha256,observed_at:new Date().toISOString(),supported:false,reason:'Fixed CLI configuration',codex_supported:false,codex_reason:'',parameter_limits:{max_steps:128,max_output_tokens:131072},items:[]});
   const staticFile={'/':'index.html','/fixture.js':'fixture.js','/fixture.css':'fixture.css'}[url.pathname];
   if(staticFile){response.writeHead(200,{'Content-Type':staticFile.endsWith('.js')?'text/javascript':staticFile.endsWith('.css')?'text/css':'text/html'});fs.createReadStream(path.join(out,staticFile)).pipe(response);return;}
   result.unexpected.push('GET '+route);send({error:{code:'fixture_unexpected_read',message:route}},404);
  });
  await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));const port=server.address().port,origin='http://127.0.0.1:'+port;result.port=port;result.origin=origin;save();
  const env={};for(const key of ['SystemRoot','WINDIR','ComSpec','PATHEXT'])if(process.env[key])env[key]=process.env[key];
  const home=path.join(directory,'home');fs.mkdirSync(home);const folder=name=>{const file=path.join(home,name);fs.mkdirSync(file);return file;};
  Object.assign(env,{PATH:path.join(process.env.SystemRoot||'C:/Windows','System32'),HOME:home,USERPROFILE:home,APPDATA:folder('appdata'),LOCALAPPDATA:folder('localappdata'),XDG_CONFIG_HOME:folder('xdg-config'),XDG_DATA_HOME:folder('xdg-data'),XDG_STATE_HOME:folder('xdg-state'),TEMP:folder('tmp'),TMP:folder('temporary'),SUMA_FIXTURE_ORIGIN:origin,SUMA_FIXTURE_DIRECTORY:directory,SUMA_FIXTURE_PARTITION:'suma-fixture-collaborative223'});
  const {_electron}=require(process.env.SUMA_PLAYWRIGHT_MODULE);app=await _electron.launch({executablePath:path.join(root,'apps/desktop/node_modules/electron/dist/electron.exe'),args:[path.join(__dirname,'../support/native.cjs')],cwd:root,env,timeout:15000});
  result.native_pid=app.process().pid;exit=new Promise(resolve=>app.process().once('exit',(code,signal)=>resolve({code,signal})));save();page=await app.firstWindow();page.setDefaultTimeout(12000);
  page.on('pageerror',error=>result.page_errors.push(error.message));page.on('console',message=>{if(message.type()==='error')result.console_errors.push(message.text());if(message.type()==='warning')result.console_warnings.push(message.text());});
  async function visit(scene,locale,width,theme){await page.setViewportSize({width,height:960});await page.goto(origin+'/?'+new URLSearchParams({scene,locale,theme}));await page.locator('[data-fixture-scene="'+scene+'"]').waitFor();}
  async function expand(locator){await locator.locator(':scope > summary').click();}
  async function geometry(container){return container.evaluate(node=>{const controls=[...node.querySelectorAll('button,select,textarea')].filter(value=>value.getClientRects().length);return {width:innerWidth,scroll:document.documentElement.scrollWidth,minimum:Math.min(...controls.map(value=>value.getBoundingClientRect().height))};});}
  async function screenshot(name,target){if(target)await target.scrollIntoViewIfNeeded();await page.screenshot({path:path.join(directory,name),fullPage:false});result.images.push({path:name,sha256:hash(fs.readFileSync(path.join(directory,name)))});}
  async function group(name,run){const facts=result.groups[name]={passed:false};try{await run(facts);facts.passed=true;}catch(error){facts.error=String(error.stack||error);await page.screenshot({path:path.join(directory,name+'-failure.png')}).catch(()=>{});throw error;}finally{save();}}
  if(!remaining)await group('cn_discussion_explicit_tests_fixed_preview',async facts=>{
   await visit('discussion','zh-CN',1366,'light');await page.locator('[data-development-toggle]').click();await page.locator('.discussion-development-fields select').first().selectOption('flow-a@3');await page.locator('[data-development-tool="developer"]').click();
   await expand(page.locator('[data-dispatch-test-bindings]'));await page.locator('[data-dispatch-test-select="verify"]').selectOption('tester');await page.locator('[data-dispatch-test-status="ready"]').waitFor();
   await page.locator('[data-development-criteria]').fill('Keep the independent test gate');await page.locator('.discussion-development-consent input').check();await page.locator('[data-development-create]').click();
   const review=page.locator('details').filter({has:page.locator('[data-dispatch-fixed-test="verify"]')}).first();await expand(review);await page.locator('[data-dispatch-fixed-test="verify"]').waitFor({state:'visible'});assert.ok((await page.locator('[data-dispatch-fixed-test="verify"]').textContent()).includes('worker-test'));
   facts.geometry=await geometry(page.locator('.discussion-development'));assert.ok(facts.geometry.scroll<=facts.geometry.width);assert.ok(facts.geometry.minimum>=43.5);await screenshot('cn-discussion-tests.png',page.locator('[data-dispatch-fixed-test="verify"]'));facts.preview_only=true;
  });
  if(!finalOnly)await group('cn_expert_team_explicit_create_and_definition_revision',async facts=>{
   await visit('team','zh-CN',1366,'light');await page.locator('[name=dispatch_team]').selectOption('team-one');await page.locator('[name=dispatch_role_task-team]').selectOption('developer');
   await expand(page.locator('[data-expert-test-task="task-team"]'));await page.locator('[data-expert-test-mode="task-team"]').selectOption('adjust');
   await expand(page.locator('[data-expert-test-adjust="task-team"] [data-dispatch-test-bindings]'));await page.locator('[data-dispatch-test-select="verify"]').selectOption('tester');await page.locator('[data-dispatch-test-status="ready"]').waitFor();await page.locator('[data-team-preview]').click();
   await page.locator('[data-team-command="revise"]').waitFor();await page.locator('[data-team-command="revise"]').click();await page.locator('[name=team_revision_select_task-team]').check();await expand(page.locator('[data-expert-test-task="task-team"]'));await page.locator('[data-expert-test-mode="task-team"]').selectOption('definition');
   await page.locator('[name=team_plan_reason]').fill('Explicitly restore the published independent-test definition');await page.locator('[name=team_plan_ack]').check();await page.locator('[data-team-confirm-command]').click();
   await page.waitForFunction(()=>!document.querySelector('[data-team-confirm-command]'));facts.geometry=await geometry(page.getByRole('dialog'));assert.ok(facts.geometry.scroll<=facts.geometry.width);await screenshot('cn-team-test-revision.png',page.locator('.expert-plan-panel'));facts.empty_array_is_explicit=true;facts.started_runs=0;
  });
  await group('en_390_recommendation_adopts_exact_tests_and_keeps_choice_closed',async facts=>{
   await visit('recommendation','en-US',390,'dark');await page.locator('[data-recommendation-toggle]').click();await page.locator('[data-recommendation-adoption-reason]').fill('Use the reviewed independent JUnit configuration');await page.locator('[data-recommendation-adopt]').click();
   await page.locator('[data-fixture-adopted] [data-dispatch-fixed-test="verify"]').waitFor();assert.ok((await page.locator('[data-fixture-adopted] [data-dispatch-fixed-test="verify"]').textContent()).includes('worker-test'));
   facts.geometry=await geometry(page.locator('[data-fixture-scene]'));assert.ok(facts.geometry.scroll<=facts.geometry.width);assert.ok(facts.geometry.minimum>=43.5);await screenshot('en-recommended-tests.png',page.locator('[data-fixture-adopted]'));facts.original_choice_fields=5;
  });
  await group('selected_doctor_visible_read_then_live_pause',async facts=>{
   await page.clock.install();await visit('doctor','en-US',390,'dark');await page.locator('[data-tool-doctor-state="ready"]').waitFor();
   const count=()=>result.requests.filter(item=>item.path==='/api/v1/tool-profiles/doctor-profile/diagnostics').length;
   const before=count();await page.clock.fastForward(16000);await page.waitForFunction(()=>document.querySelector('[data-tool-doctor-state="ready"]'));
   assert.equal(count(),before+1);await page.locator('[data-fixture-doctor-live]').uncheck();await page.locator('[data-tool-doctor-auto="paused"]').waitFor();const paused=count();await page.clock.fastForward(30000);assert.equal(count(),paused);
   facts.reads_before=before;facts.reads_after_visible=paused;facts.reads_while_paused=count()-paused;facts.no_probe_or_write=true;await screenshot('en-doctor-live-pause.png',page.locator('.tool-profile-doctor'));
  });
  assert.deepEqual(result.unexpected,[]);assert.deepEqual(result.page_errors,[]);assert.deepEqual(result.console_errors,[]);
  assert.ok(result.requests.filter(item=>item.method!=='GET').every(item=>['/api/v1/task-discussion-development-plans','/api/v1/expert-team-plans','/api/v1/expert-team-plans/team-preview/commands','/api/v1/dispatch-recommendations/recommendation-one/adopt'].includes(item.path)));result.checks_passed=true;
 }finally{
  if(app)Object.assign(result,await closeOwnedNative(app,exit));
  if(server){await new Promise(resolve=>server.close(resolve));result.port_closed=await closed(result.port);}save();
 }
 assert.equal(result.forced,false);assert.deepEqual(result.native_exit,{code:0,signal:null});assert.equal(result.port_closed,true);result.passed=true;save();console.log(JSON.stringify({passed:true,directory,images:result.images,native_exit:result.native_exit}));
}
main().catch(error=>{result.error=String(error.stack||error);save();console.error(error.stack||String(error));process.exitCode=1;});
