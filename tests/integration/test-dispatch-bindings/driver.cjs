'use strict';
// Real original task dispatch UI against the owned HTTP/SQLite host. Session
// cookie is private stdin/main-process data; no credential enters the renderer.
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict'),crypto=require('node:crypto');
const root=path.resolve(__dirname,'../../..'),base=path.join(root,'.cache/test-dispatch-bindings-216');
const{closeOwnedNative}=require('../support/close-native.cjs');
const sha=bytes=>crypto.createHash('sha256').update(bytes).digest('hex');
async function main(){
 let raw='';for await(const chunk of process.stdin){raw+=chunk;if(Buffer.byteLength(raw)>65536)throw Error('Private fixture settings exceed limit');}
 const owned=JSON.parse(raw);assert.equal(owned.protocol,'suma-test-dispatch-bindings-actual-v1');assert.match(owned.origin,/^http:\/\/127\.0\.0\.1:\d+$/);
 const relative=path.relative(base,owned.directory);assert.ok(relative&&!relative.startsWith('..')&&!path.isAbsolute(relative));
 const dir=owned.directory,result={stage:'T-TEST-DISPATCH-BINDINGS-216',passed:false,scope:'Original UI and HTTP preview; original Workers execute only after this driver exits',checks:[],images:[],api:[],page_errors:[]};
 let app,page,exit,release;
 try{
  const env={};for(const[key,value]of Object.entries(process.env))if(['path','systemroot','windir','comspec','pathext','home','userprofile','homedrive','homepath','appdata','localappdata','programdata','programfiles','programfiles(x86)','programw6432'].includes(key.toLowerCase()))env[key]=value;
  const tmp=path.join(dir,'browser-tmp');fs.mkdirSync(tmp,{recursive:true});Object.assign(env,{TEMP:tmp,TMP:tmp,SUMA_FIXTURE_ORIGIN:owned.origin,SUMA_FIXTURE_DIRECTORY:dir,SUMA_FIXTURE_PARTITION:'suma-fixture-test-dispatch216'});
  const {_electron}=require(process.env.SUMA_PLAYWRIGHT_MODULE);app=await _electron.launch({executablePath:path.join(root,'apps/desktop/node_modules/electron/dist/electron.exe'),args:[path.join(__dirname,'../support/native.cjs')],cwd:root,env,timeout:15000});
  result.native_pid=app.process().pid;exit=new Promise(resolve=>app.process().once('exit',(code,signal)=>resolve({code,signal})));fs.writeFileSync(path.join(dir,'native-start.json'),JSON.stringify({pid:result.native_pid,origin:owned.origin})+'\n');
  await app.evaluate(async({session},{origin,cookie})=>{await session.fromPartition('suma-fixture-test-dispatch216').cookies.set({url:origin,name:cookie.name,value:cookie.value,httpOnly:true,sameSite:'strict'});},{origin:owned.origin,cookie:owned.sessionCookie});
  page=await app.firstWindow();page.setDefaultTimeout(15000);page.on('pageerror',error=>result.page_errors.push(error.message));page.on('request',request=>{const url=new URL(request.url());if(url.origin===owned.origin&&url.pathname.startsWith('/api/'))result.api.push({method:request.method(),path:url.pathname});});
  const capture=async name=>{await page.screenshot({path:path.join(dir,name)});result.images.push({path:name,sha256:sha(fs.readFileSync(path.join(dir,name)))});};
  await page.setViewportSize({width:1366,height:1000});await page.goto(owned.origin+'/?locale=zh-CN');await page.locator('.tool-dispatch fieldset select').first().selectOption(owned.workflow_id+'@'+owned.workflow_version);
  await page.locator('[data-dispatch-manual-tool]').selectOption(owned.developer_profile_id);await page.locator('[data-dispatch-test-bindings] > summary').click();
  const tester=page.locator('select[data-dispatch-test-select="test"]');await tester.selectOption(owned.test_profile_id);await page.locator('[data-dispatch-test-node="test"] [data-dispatch-test-status="ready"]').waitFor();
  result.checks.push('original_task_dispatch_selects_developer_and_current_exact_tester_without_default_substitution');
  // Delay only promise resolution, leaving native request/body/headers/status
  // and response bytes unchanged, to inspect the original durable pending form.
  await page.evaluate(()=>{const original=window.fetch.bind(window);let once=true;window.fetch=async(input,init)=>{const response=await original(input,init);if(once&&init?.method==='POST'&&new URL(String(input),location.href).pathname==='/api/v1/task-dispatch-plans'){once=false;window.__heldPreviewStatus=response.status;window.__previewRequest=JSON.parse(init.body);await new Promise(resolve=>{window.__releasePreview=resolve;});}return response;};});
  release=()=>page.evaluate(()=>window.__releasePreview?.()).catch(()=>{});
  await page.locator('[data-dispatch-prepare]').click({noWaitAfter:true});await page.waitForFunction(()=>window.__heldPreviewStatus===201);assert.equal(await tester.isDisabled(),true);
  result.preview_request=await page.evaluate(()=>window.__previewRequest);assert.equal(result.preview_request.tool_profile_id,owned.developer_profile_id);assert.equal(result.preview_request.test_bindings.length,1);assert.equal(result.preview_request.test_bindings[0].node_id,'test');assert.equal(result.preview_request.test_bindings[0].tool_profile_id,owned.test_profile_id);assert.ok(result.preview_request.test_bindings[0].expected_tool_version>0);
  await page.locator('[data-dispatch-test-bindings]').scrollIntoViewIfNeeded();await capture('cn-test-dispatch-selection.png');result.checks.push('pending_preview_keeps_exact_test_selection_and_locks_edits_without_changing_body');
  await release();release=undefined;await page.waitForFunction(()=>document.querySelector('[data-fixture-plan-id]')?.getAttribute('data-fixture-plan-id')!=='new');
  result.plan_id=await page.locator('[data-fixture-plan-id]').getAttribute('data-fixture-plan-id');assert.ok(result.plan_id);await page.locator('[data-dispatch-fixed-test="test"]').waitFor();assert.ok((await page.locator('[data-dispatch-fixed-test="test"]').innerText()).includes(owned.test_profile_id));result.checks.push('original_preview_renders_its_frozen_test_tool_and_worker');
  await page.setViewportSize({width:390,height:1000});await page.goto(owned.origin+'/?'+new URLSearchParams({locale:'en-US',theme:'dark',plan:result.plan_id}));await page.locator('[data-dispatch-fixed-test="test"]').waitFor();await page.locator('[data-dispatch-fixed-test="test"]').scrollIntoViewIfNeeded();await capture('en-test-dispatch-fixed.png');
  result.geometry=await page.evaluate(()=>{const rect=document.querySelector('[data-dispatch-fixed-test]').getBoundingClientRect();return{width:innerWidth,scrollWidth:document.documentElement.scrollWidth,facts:{left:rect.left,right:rect.right,width:rect.width}};});assert.ok(result.geometry.scrollWidth<=390);assert.ok(result.geometry.facts.left>=0&&result.geometry.facts.right<=390);
  assert.equal(result.api.filter(item=>item.method==='POST'&&item.path==='/api/v1/task-dispatch-plans').length,1);assert.equal(result.api.filter(item=>item.path.endsWith('/start')).length,0);assert.equal(result.page_errors.length,0);result.checks.push('english_narrow_fixed_plan_refresh_reads_same_binding_without_second_preview_or_start');result.passed=true;
 }catch(error){result.error=error.stack||String(error);if(page)await page.screenshot({path:path.join(dir,'failed-ui.png')}).catch(()=>{});}
 finally{
  if(release)await release();if(app){try{const closed=await closeOwnedNative(app,exit);result.native_exit=closed.native_exit;result.teardown=closed;if(closed.forced||closed.close_error)result.passed=false;}catch(error){result.passed=false;result.close_error=String(error);}}
  if(result.native_exit?.code!==0||result.native_exit?.signal!==null)result.passed=false;fs.writeFileSync(path.join(dir,'ui-result.json'),JSON.stringify(result,null,2)+'\n');
 }
 console.log(JSON.stringify({passed:result.passed,plan_id:result.plan_id,checks:result.checks.length}));if(!result.passed)process.exitCode=1;
}
main().catch(()=>{console.error('Owned task dispatch UI verification could not complete.');process.exitCode=1;});
