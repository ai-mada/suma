'use strict';
// Production tool/task components and real authenticated HTTP/SQLite. This
// journey deliberately stops at the immutable preview; it never clicks Start.
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict'),crypto=require('node:crypto');
const root=path.resolve(__dirname,'../../..'),base=path.join(root,'.cache/tool-task-handoff-213');
const sha=b=>crypto.createHash('sha256').update(b).digest('hex');
async function input(){let raw='';for await(const chunk of process.stdin){raw+=chunk;if(Buffer.byteLength(raw)>65536)throw Error('Oversized private fixture input');}return JSON.parse(raw);}
async function main(){
  const owned=await input(),origin=owned.origin,dir=path.resolve(owned.directory);
  assert.equal(owned.protocol,'suma-tool-task-handoff-actual-v1');assert.match(origin,/^http:\/\/127\.0\.0\.1:\d+$/);assert.ok(dir.startsWith(base+path.sep));
  const result={stage:'T-TOOL-TASK-HANDOFF-213',passed:false,checks:[],images:[],api:[],page_errors:[],unexpected_dialogs:[],scope:'Real HTTP/SQLite and production handoff/preview; owned inventory, no execution or acceptance'};
  let app,exit,page,closing=false,releasePending=()=>{};
  const checkpoint=name=>{result.progress=name;fs.writeFileSync(path.join(dir,'progress.json'),JSON.stringify({progress:name,at:new Date().toISOString(),api:result.api,native_pid:result.native_pid})+'\n');};
  try{
    const env={};for(const [key,value]of Object.entries(process.env))if(['path','systemroot','windir','comspec','pathext','home','userprofile','homedrive','homepath','appdata','localappdata','programdata','programfiles','programfiles(x86)','programw6432'].includes(key.toLowerCase()))env[key]=value;
    const partition='suma-fixture-tool-task213';Object.assign(env,{TEMP:path.join(dir,'browser-tmp'),TMP:path.join(dir,'browser-tmp'),SUMA_FIXTURE_ORIGIN:origin,SUMA_FIXTURE_DIRECTORY:dir,SUMA_FIXTURE_PARTITION:partition});fs.mkdirSync(env.TEMP,{recursive:true});
    const {_electron}=require(process.env.SUMA_PLAYWRIGHT_MODULE);
    app=await _electron.launch({executablePath:path.join(root,'apps/desktop/node_modules/electron/dist/electron.exe'),args:[path.join(__dirname,'../support/native.cjs')],cwd:root,env,timeout:15000});
    result.native_pid=app.process().pid;exit=new Promise(resolve=>app.process().once('exit',(code,signal)=>resolve({code,signal})));
    fs.writeFileSync(path.join(dir,'native-start.json'),JSON.stringify({pid:result.native_pid,origin,started_at:new Date().toISOString()})+'\n');
    await app.evaluate(async({session},{origin,partition,cookie})=>session.fromPartition(partition).cookies.set({url:origin,name:cookie.name,value:cookie.value,httpOnly:true,sameSite:'strict'}),{origin,partition,cookie:owned.sessionCookie});
    page=await app.firstWindow();page.setDefaultTimeout(15000);
    page.on('pageerror',error=>result.page_errors.push(error.message));
    // Unexpected native unload prompts are failures. Observe and close them
    // explicitly so Electron/Playwright's default dismissal cannot abort the
    // driver before it writes its owned process and failure evidence.
    page.on('dialog',dialog=>{
      if(closing){result.teardown_dialogs=(result.teardown_dialogs||0)+1;result.passed=false;void dialog.accept().catch(error=>{result.dialog_handler_error=String(error);});}
      else{result.unexpected_dialogs.push(dialog.type());void dialog.dismiss().catch(error=>{result.dialog_handler_error=String(error);});}
    });
    page.on('response',response=>{const url=new URL(response.url());if(url.pathname.startsWith('/api/v1/'))result.api.push({method:response.request().method(),path:url.pathname,status:response.status()});});
    const screenshot=async name=>{await page.screenshot({path:path.join(dir,name)});result.images.push({path:name,sha256:sha(fs.readFileSync(path.join(dir,name)))});};
    await page.setViewportSize({width:1366,height:1000});await page.goto(origin+'/?locale=zh-CN');
    await page.locator('[data-tool-use-for-task]').click();
    assert.equal(await page.locator('[role="dialog"]').count(),1);
    await page.locator('[data-handoff-task]').selectOption(owned.task_id);
    await page.locator('[data-handoff-action="continue"]').click();
    await page.locator('.tool-dispatch').waitFor();assert.equal(await page.locator('[role="dialog"]').count(),1);
    result.checks.push('exact_tool_current_task_recheck_opens_one_original_dispatch_dialog');
    const workflow=page.locator('.tool-dispatch select').filter({has:page.locator('option[value^="'+owned.workflow_id+'@"]')});
    const workflowValue=await workflow.locator('option').evaluateAll((options,id)=>options.map(o=>o.value).find(v=>v.startsWith(id+'@')),owned.workflow_id);assert.ok(workflowValue);
    await workflow.selectOption(workflowValue);
    await page.locator('[data-dispatch-handoff-adopt]').click();
    await page.locator('[data-dispatch-handoff-status="adopted"]').waitFor();
    assert.equal(await page.locator('[data-dispatch-manual-tool]').inputValue(),owned.tool_id);
    await page.locator('[data-dispatch-handoff]').scrollIntoViewIfNeeded();await screenshot('cn-exact-tool-adopted.png');
    result.checks.push('explicit_exact_profile_adoption_uses_original_selector_without_automatic_write');
    assert.equal(result.api.filter(r=>r.method!=='GET').length,0);
    const preview=page.locator('.tool-dispatch form > .dialog-actions button.primary');
    // Pause only resolution of the original native fetch promise after the
    // actual HTTP response. CDP response fulfillment is intentionally avoided:
    // Electron reported status0 for that test transport despite a committed
    // 201. No request/header/body/response bytes are replaced by this gate.
    await page.evaluate(()=>{
      const nativeFetch=window.fetch.bind(window);let once=true;
      window.fetch=async(input,init)=>{
        const response=await nativeFetch(input,init);
        if(once&&init?.method==='POST'&&new URL(String(input),location.href).pathname==='/api/v1/task-dispatch-plans'){
          once=false;window.__taskPreviewHeld=response.status;
          await new Promise(resolve=>{window.__releaseTaskPreview=resolve;});
        }
        return response;
      };
    });
    releasePending=()=>page.evaluate(()=>window.__releaseTaskPreview?.()).catch(()=>{});
    checkpoint('click_preview');
    await preview.click({noWaitAfter:true});
    await page.waitForFunction(()=>window.__taskPreviewHeld===201);
    checkpoint('refresh_parent');
    const refreshObserved=page.waitForResponse(response=>new URL(response.url()).pathname==='/api/v1/tool-profiles/'+owned.tool_id);
    await page.evaluate(()=>window.dispatchEvent(new Event('suma:fixture-refresh')));await refreshObserved;
    assert.equal(await page.locator('.tool-dispatch').count(),1);assert.equal(await page.locator('[role="dialog"]').count(),1);
    assert.equal(await preview.isDisabled(),true);
    result.checks.push('parent_detail_refresh_preserves_original_pending_preview_dialog');
    checkpoint('release_preview');
    await Promise.all([page.waitForURL(url=>url.searchParams.get('panel')?.startsWith('dispatch:')===true),releasePending()]);
    await page.locator('.tool-dispatch .tool-start-confirm').waitFor();
    const previewRoute=new URL(page.url());assert.equal(previewRoute.searchParams.get('record'),owned.task_id);assert.equal(previewRoute.searchParams.get('sub'),'tasks');
    result.plan_id=previewRoute.searchParams.get('panel').slice(9);
    assert.equal(result.api.filter(r=>r.method==='POST'&&r.path==='/api/v1/task-dispatch-plans').length,1);
    assert.equal(result.api.filter(r=>r.path.endsWith('/start')).length,0);
    result.checks.push('one_explicit_preview_is_refreshable_on_original_task_plan_route_and_does_not_start');
    previewRoute.searchParams.set('locale','en-US');previewRoute.searchParams.set('theme','dark');
    await page.setViewportSize({width:390,height:1000});await page.goto(previewRoute.href);
    await page.locator('.tool-dispatch .tool-start-confirm').waitFor();
    await page.locator('.tool-dispatch').evaluate(el=>el.closest('[role="dialog"]')?.scrollTo(0,0));
    result.geometry=await page.evaluate(()=>({width:innerWidth,scrollWidth:document.documentElement.scrollWidth}));assert.ok(result.geometry.scrollWidth<=390);
    await screenshot('en-fixed-plan.png');
    assert.equal(result.page_errors.length,0);assert.equal(result.unexpected_dialogs.length,0);assert.ok(result.api.every(r=>r.status>=200&&r.status<300));
    result.checks.push('english_narrow_fixed_plan_read_keeps_start_explicit');result.passed=true;
  }catch(error){
    result.error=error.stack||String(error);
    if(page){try{result.ui=await page.evaluate(()=>({url:location.href,text:document.querySelector('.tool-dispatch')?.textContent?.slice(0,14000)}));await page.screenshot({path:path.join(dir,'failed-ui.png')});}catch(observation){result.observation_error=String(observation);}}
    fs.writeFileSync(path.join(dir,'failure-before-close.json'),JSON.stringify(result,null,2)+'\n');
  }
  finally{
    closing=true;
    await releasePending();
    if(app&&!result.passed){
      // The failed fixture may intentionally retain an unknown write guard.
      // Only this owned test window ignores unload during teardown; the
      // product guard and its original durable request are left unchanged.
      result.failed_fixture_teardown_unload_override=true;
      await app.evaluate(({BrowserWindow})=>{for(const window of BrowserWindow.getAllWindows())window.webContents.on('will-prevent-unload',event=>event.preventDefault());}).catch(()=>{});
    }
    if(app){try{
      result.native_exit=await Promise.race([(async()=>{await app.close();return await exit;})(),new Promise((_,reject)=>setTimeout(()=>reject(Error('Native close not confirmed')),15000))]);
    }catch(error){
      result.passed=false;result.close_error=String(error);
      // Only the launched fixture may be stopped after its bounded teardown
      // fails. Never resolve or clear the original product operation to exit.
      result.forced_owned_fixture_exit=true;await app.evaluate(({app})=>app.exit(1)).catch(()=>{});
      result.native_exit=await Promise.race([exit,new Promise(resolve=>setTimeout(()=>resolve({code:null,signal:'unconfirmed'}),3000))]);
    }}
    if(result.native_exit?.code!==0||result.native_exit?.signal!==null)result.passed=false;
    fs.writeFileSync(path.join(dir,'result.json'),JSON.stringify(result,null,2)+'\n');
  }
  process.stdout.write(JSON.stringify({passed:result.passed})+'\n');if(!result.passed)process.exitCode=1;
}
main().catch(error=>{process.stderr.write(String(error)+'\n');process.exitCode=1;});
