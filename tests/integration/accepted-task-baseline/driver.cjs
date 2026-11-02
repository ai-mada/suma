'use strict';
// UI automation over the original live HTTP/SQLite fixture. The private
// HttpOnly cookie is injected only by its owned loopback bootstrap response.
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto'),assert=require('node:assert/strict');
const {closeOwnedNative}=require('../support/close-native.cjs');
const root=path.resolve(__dirname,'../../..'),base=path.join(root,'.cache/accepted-task-baseline-222/actual');
const directory=path.resolve(process.argv[2]||'');
const relative=path.relative(base,directory);assert.ok(relative&&!relative.startsWith('..')&&!path.isAbsolute(relative),'Explicit owned actual directory required');
const ready=JSON.parse(fs.readFileSync(path.join(directory,'ready.json'),'utf8'));
const alreadyBound=process.argv.includes('--bound');
assert.match(ready.origin,/^http:\/\/127\.0\.0\.1:\d+$/);assert.equal(path.resolve(ready.directory),directory);
const hash=bytes=>crypto.createHash('sha256').update(bytes).digest('hex');
const result={stage:'T-ACCEPTED-TASK-BASELINE-222',scope:'Production baseline panel and actual HTTP/SQLite; original runtime proceeds after an explicit UI checkpoint, not vendor/model acceptance',groups:{},api:[],images:[],page_errors:[],console_errors:[],console_warnings:[],passed:false};
const save=()=>fs.writeFileSync(path.join(directory,'ui-result.json'),JSON.stringify(result,null,2)+'\n');
async function main(){let app,exit,page;
 try{
  const env={};for(const key of ['SystemRoot','WINDIR','ComSpec','PATHEXT'])if(process.env[key])env[key]=process.env[key];
  const home=path.join(directory,'browser-home');fs.mkdirSync(home,{recursive:true});
  const folder=name=>{const p=path.join(home,name);fs.mkdirSync(p,{recursive:true});return p;};
  Object.assign(env,{PATH:path.join(process.env.SystemRoot||'C:/Windows','System32'),HOME:home,USERPROFILE:home,APPDATA:folder('appdata'),LOCALAPPDATA:folder('localappdata'),XDG_CONFIG_HOME:folder('xdg-config'),XDG_DATA_HOME:folder('xdg-data'),XDG_STATE_HOME:folder('xdg-state'),TEMP:folder('tmp'),TMP:folder('tmp'),SUMA_FIXTURE_ORIGIN:ready.origin,SUMA_FIXTURE_DIRECTORY:directory,SUMA_FIXTURE_PARTITION:'suma-fixture-accepted-baseline222'});
  const {_electron}=require(process.env.SUMA_PLAYWRIGHT_MODULE);
  app=await _electron.launch({executablePath:path.join(root,'apps/desktop/node_modules/electron/dist/electron.exe'),args:[path.join(__dirname,'../support/native.cjs')],cwd:root,env,timeout:15000});
  result.native_pid=app.process().pid;exit=new Promise(resolve=>app.process().once('exit',(code,signal)=>resolve({code,signal})));save();
  page=await app.firstWindow();page.setDefaultTimeout(15000);
  page.on('pageerror',error=>result.page_errors.push(error.message));page.on('console',message=>{if(message.type()==='error')result.console_errors.push(message.text());else if(message.type()==='warning')result.console_warnings.push(message.text());});
  page.on('request',request=>{const url=new URL(request.url());if(url.origin===ready.origin&&url.pathname.startsWith('/api/'))result.api.push({method:request.method(),path:url.pathname,...(request.method()==='POST'&&url.pathname.endsWith('/code-baseline')?{body:request.postDataJSON()}:{} )});});
  const panel=()=>page.locator('.composition-task-baseline');
  async function visit(locale,width,theme,target=ready.target_task_id){await page.setViewportSize({width,height:width===390?900:960});await page.goto(ready.origin+'/?'+new URLSearchParams({locale,theme,target}));await page.locator('[data-fixture-target="'+target+'"]').waitFor();await panel().waitFor();}
  async function image(name){await panel().scrollIntoViewIfNeeded();await page.screenshot({path:path.join(directory,name),fullPage:false});result.images.push({path:name,sha256:hash(fs.readFileSync(path.join(directory,name)))});}
  async function geometry(){const value=await panel().evaluate(node=>({width:innerWidth,scroll:document.documentElement.scrollWidth,controls:[...node.querySelectorAll('button,input,select,textarea')].filter(control=>control.getClientRects().length).map(control=>({tag:control.tagName,type:control.getAttribute('type'),height:control.getBoundingClientRect().height}))}));assert.ok(value.scroll<=value.width);assert.ok(value.controls.filter(control=>control.type!=='checkbox').every(control=>control.height>=43.5));return value;}
  async function group(name,run){result.groups[name]={passed:false};try{await run(result.groups[name]);result.groups[name].passed=true;}catch(error){result.groups[name].error=String(error.stack||error);await page.screenshot({path:path.join(directory,name+'-failure.png')}).catch(()=>{});throw error;}finally{save();}}
  const sourceValue=JSON.stringify([ready.source_task_id,ready.source_node_id]);
  await group('cn_explicit_ordinary_accepted_source_double_cas_binding',async facts=>{
   if(alreadyBound){
    await visit('zh-CN',1366,'light');await page.waitForFunction(commit=>[...document.querySelectorAll('.composition-facts code')].some(node=>node.textContent===commit),ready.source_commit);
    const proof=await page.evaluate(async url=>{const response=await fetch(url);return {status:response.status,value:response.ok?await response.json():null};},'/api/v1/tasks/'+ready.target_task_id+'/code-baseline?project_id='+ready.project_id);
    assert.equal(proof.status,200);assert.equal(proof.value.binding.source_task_id,ready.source_task_id);assert.equal(proof.value.binding.source_task_version,ready.source_task_version);assert.equal(proof.value.binding.source.head_commit,ready.source_commit);result.binding=proof.value.binding;facts.continuation='Prior native session already performed the unique real POST; this session only re-reads its current persisted binding using renderer same-origin credentials.';return;
   }
   await visit('zh-CN',1366,'light');await page.locator('[data-task-baseline-origin]').selectOption('accepted_task');
   await page.waitForFunction(value=>[...document.querySelectorAll('[data-task-baseline-source] option')].some(option=>option.value===value&&!option.disabled),sourceValue);
   await page.locator('[data-task-baseline-source]').selectOption(sourceValue);await page.locator('[data-task-baseline-candidate]').waitFor();assert.ok((await page.locator('[data-task-baseline-candidate]').textContent()).includes(ready.source_commit));
   await page.locator('[name=composition_bind_reason]').fill('Isolated fixture explicitly hands accepted ordinary agent M to unstarted task B.');await page.locator('[name=composition_bind_ack]').check();
   facts.geometry=await geometry();await image('cn-accepted-ordinary-baseline.png');
   await page.locator('[data-composition-bind-submit]').click();await page.locator('.composition-success').waitFor();
   const response=await page.evaluate(async url=>{const value=await fetch(url);return {status:value.status,value:value.ok?await value.json():null};},'/api/v1/tasks/'+ready.target_task_id+'/code-baseline?project_id='+ready.project_id);assert.equal(response.status,200);const value=response.value;
   assert.equal(value.binding.source_task_id,ready.source_task_id);assert.equal(value.binding.source_task_version,ready.source_task_version);assert.equal(value.binding.source.source.node_id,ready.source_node_id);assert.equal(value.binding.source.head_commit,ready.source_commit);assert.match(value.binding.binding_sha256,/^[a-f0-9]{64}$/);result.binding=value.binding;
   const writes=result.api.filter(item=>item.method==='POST'&&item.path.endsWith('/code-baseline'));assert.equal(writes.length,1);assert.equal(writes[0].body.source_task_id,ready.source_task_id);assert.equal(writes[0].body.source_node_id,ready.source_node_id);assert.equal(writes[0].body.expected_source_task_version,ready.source_task_version);assert.equal(writes[0].body.expected_binding_version,0);assert.ok(writes[0].body.expected_task_version>0);assert.equal(writes[0].body.acknowledge,true);facts.original_double_cas_and_exact_source=true;
  });
  await group('en_390_dark_refresh_preserves_exact_current_baseline_without_rebinding',async facts=>{
   await visit('en-US',390,'dark');await page.waitForFunction(commit=>[...document.querySelectorAll('.composition-facts code')].some(node=>node.textContent===commit),ready.source_commit);
   const details=panel().locator('details').filter({has:page.locator('pre.composition-raw')}).first();await details.locator(':scope > summary').click();const binding=JSON.parse(await details.locator('pre').textContent());assert.equal(binding.source_task_id,ready.source_task_id);assert.equal(binding.source.source.node_id,ready.source_node_id);assert.equal(binding.source.head_commit,ready.source_commit);await details.locator(':scope > summary').click();facts.geometry=await geometry();await image('en-mobile-accepted-baseline.png');facts.same_binding_no_new_write=true;
  });
  // This checkpoint is fixture coordination only. The original service reads
  // and verifies the actual persisted binding before starting any Worker.
  fs.writeFileSync(path.join(directory,'continue.json'),JSON.stringify({project_id:ready.project_id,target_task_id:ready.target_task_id,source_task_id:ready.source_task_id,source_commit:ready.source_commit})+'\n');
  const returnedFile=path.join(directory,'source-returned.json');const deadline=Date.now()+120000;
  while(!fs.existsSync(returnedFile)){if(Date.now()>deadline)throw Error('Original follow-up runtime did not reach returned-source checkpoint');await new Promise(resolve=>setTimeout(resolve,100));}
  const returned=JSON.parse(fs.readFileSync(returnedFile,'utf8'));assert.equal(returned.candidate_M,ready.source_commit);assert.notEqual(returned.candidate_N,ready.source_commit);assert.ok(returned.rejected_new_action_status>=400&&returned.rejected_new_action_status<500);result.runtime_checkpoint=returned;
  await group('returned_source_blocks_fresh_target_and_does_not_rewrite_completed_facts',async facts=>{
   await visit('zh-CN',1366,'light',ready.third_task_id);await page.locator('[data-task-baseline-origin]').selectOption('accepted_task');await page.locator('[data-task-baseline-source-refresh]').click();
   await page.waitForFunction(value=>{const choice=[...document.querySelectorAll('[data-task-baseline-source] option')].find(option=>option.value===value);return !choice||choice.disabled;},sourceValue);
   assert.equal(await page.locator('[name=composition_bind_ack]').isDisabled(),true);assert.equal(await page.locator('[data-composition-bind-submit]').isDisabled(),true);assert.equal(result.api.filter(item=>item.method==='POST'&&item.path.endsWith('/code-baseline')).length,alreadyBound?0:1);facts.current_returned_source_not_eligible=true;facts.current_version_api_rejection=returned.rejected_new_action_status;
  });
  assert.deepEqual(result.page_errors,[]);assert.deepEqual(result.console_errors,[]);result.checks_passed=true;
 }finally{if(app)Object.assign(result,await closeOwnedNative(app,exit));save();}
 assert.equal(result.forced,false);assert.deepEqual(result.native_exit,{code:0,signal:null});result.passed=true;save();fs.writeFileSync(path.join(directory,'close.json'),JSON.stringify({ui_passed:true,target_task_id:ready.target_task_id})+'\n');console.log(JSON.stringify({passed:true,directory,groups:Object.keys(result.groups),native_exit:result.native_exit,images:result.images}));
}
main().catch(error=>{console.error(error.stack||String(error));process.exitCode=1;});
