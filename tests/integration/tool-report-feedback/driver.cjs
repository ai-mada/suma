'use strict';
// Opt-in integration driver. Its caller creates an owned SQLite fixture and
// passes temporary authentication on stdin. No private value reaches a URL,
// renderer boot envelope, process environment, command argument or saved log.
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict'),crypto=require('node:crypto');
const root=path.resolve(__dirname,'../../..'),hash=value=>crypto.createHash('sha256').update(value).digest('hex');
async function readInput(){let raw='';for await(const chunk of process.stdin){raw+=chunk;if(Buffer.byteLength(raw)>65536)throw Error('Oversized fixture input');}return JSON.parse(raw);}
async function main(){
 const input=await readInput(),origin=input.origin,dir=path.resolve(input.directory),cache=path.join(root,'.cache/tool-report-feedback-204');
 assert.match(origin,/^http:\/\/127\.0\.0\.1:\d+$/);assert.ok(dir.startsWith(cache+path.sep));assert.equal(typeof input.action_key,'string');assert.ok(input.action_key.length>=32);
 const runtime=process.env.SUMA_PLAYWRIGHT_MODULE;if(!runtime)throw Error('Set SUMA_PLAYWRIGHT_MODULE to the installed Playwright module');
 const {_electron}=require(runtime),executable=process.env.SUMA_ELECTRON_EXECUTABLE||path.join(root,'apps/desktop/node_modules/electron/dist/electron.exe');
 const runID=new Date().toISOString().replace(/[^0-9TZ]/g,''),result={stage:'T-TOOL-REPORT-FEEDBACK-204',run_id:runID,scope:'Production remote guide and Quick components against actual HTTP/SQLite; owned report metadata, no vendor CLI/model or tool-profile mutations.',groups:{},images:[],selections:[],api_reads:[],unexpected_requests:[],page_errors:[]};
 fs.mkdirSync(dir,{recursive:true});
 const save=()=>fs.writeFileSync(path.join(dir,'driver-result.json'),JSON.stringify(result,null,2));
 let app,page,nativeExit;
 const action=async name=>{const response=await fetch(origin+'/__fixture/action',{method:'POST',headers:{'Content-Type':'application/json','X-Suma-Fixture-Key':input.action_key},body:JSON.stringify({action:name})});if(!response.ok)throw Error('Fixture action '+name+' failed: '+response.status);return response.json();};
 try{
  const env={};for(const [key,value]of Object.entries(process.env))if(['path','systemroot','windir','comspec','pathext','home','userprofile','homedrive','homepath','appdata','localappdata','programdata','programfiles','programfiles(x86)','programw6432'].includes(key.toLowerCase()))env[key]=value;
  Object.assign(env,{TEMP:path.join(dir,'tmp'),TMP:path.join(dir,'tmp'),SUMA_FEEDBACK204_ORIGIN:origin,SUMA_FEEDBACK204_DIRECTORY:dir});fs.mkdirSync(env.TEMP,{recursive:true});
  app=await _electron.launch({executablePath:executable,args:[path.join(__dirname,'native.cjs')],cwd:root,env,timeout:15000});result.native_pid=app.process().pid;nativeExit=new Promise(resolve=>app.process().once('exit',(code,signal)=>resolve({code,signal})));page=await app.firstWindow();page.setDefaultTimeout(12000);
  // BrowserWindow owns a separate Electron session. Playwright's default
  // context cookie jar is not that partition. Install the temporary cookie in
  // the main process only; it is never evaluated in renderer JavaScript.
  await app.evaluate(async({session},{origin,cookie})=>{await session.fromPartition('tool-report-feedback204').cookies.set({url:origin,name:cookie.name,value:cookie.value,httpOnly:true,sameSite:'strict'});},{origin,cookie:input.cookie});
  page.on('pageerror',error=>result.page_errors.push(error.message));page.on('dialog',dialog=>{result.page_errors.push('Unexpected '+dialog.type());void dialog.dismiss();});
  await page.route('**/*',async route=>{const request=route.request(),url=new URL(request.url()),readOnlyMatch=request.method()==='POST'&&url.pathname==='/api/v1/tool-profiles/matches'&&!url.search,read=request.method()==='GET'&&(url.pathname==='/'||url.pathname==='/fixture.js'||url.pathname==='/fixture.css'||url.pathname==='/__fixture/view'||url.pathname.startsWith('/api/v1/'));if(url.origin===origin&&(read||readOnlyMatch)){if(url.pathname.startsWith('/api/v1/'))result.api_reads.push(url.pathname.slice('/api/v1'.length)+url.search);await route.continue();}else{result.unexpected_requests.push({method:request.method(),path:url.pathname});await route.abort();}});
  const feedback=()=>page.locator('[data-tool-report-feedback]'),status=()=>page.locator('[data-tool-report-status]');
  async function visit(locale,width,theme){await page.setViewportSize({width,height:1000});await page.goto(origin+'/?locale='+locale+'&theme='+theme);await page.waitForLoadState('networkidle');await page.locator('[data-tool-report-fixture]').waitFor();await feedback().waitFor();}
  async function capture(name){const relative=runID+'-'+name+'.png',destination=path.join(dir,relative);await feedback().evaluate(element=>element.scrollIntoView({block:'center'}));await page.screenshot({path:destination,fullPage:false});result.images.push({path:relative,sha256:hash(fs.readFileSync(destination))});}
  async function layout(){const value=await feedback().evaluate(element=>({width:innerWidth,scrollWidth:document.documentElement.scrollWidth,targets:[...element.querySelectorAll('button,input,select,summary')].filter(node=>node.getClientRects().length).map(node=>({tag:node.tagName,height:node.getBoundingClientRect().height}))}));assert.ok(value.scrollWidth<=value.width);assert.ok(value.targets.every(target=>target.height>=43.5));return value;}
  async function group(name,run){result.groups[name]={passed:false};try{await run(result.groups[name]);assert.deepEqual(result.page_errors,[]);assert.deepEqual(result.unexpected_requests,[]);result.groups[name].passed=true;}catch(error){result.groups[name].error=String(error.stack||error);await page.screenshot({path:path.join(dir,runID+'-'+name+'-failure.png')}).catch(()=>{});throw error;}finally{save();}}
  await group('cn_new_report_and_exact_return',async group=>{
   await visit('zh-CN',1366,'light');await page.locator('[data-tool-report-check]').click();await page.waitForFunction(()=>document.querySelector('[data-tool-report-status]')?.getAttribute('data-tool-report-status')==='unreported');
   await page.locator('[data-tool-report-wait]').click();await page.waitForFunction(()=>document.querySelector('[data-tool-report-status]')?.getAttribute('data-tool-report-status')==='waiting');
   const report=await action('report_new');group.report=report;
   await page.waitForFunction(()=>document.querySelector('[data-tool-report-status]')?.getAttribute('data-tool-report-status')==='new');
   group.layout=await layout();await capture('cn-new-inventory');
   const selected=page.locator('[data-tool-report-selection="'+input.command_id+'"]');await selected.focus();await page.keyboard.press('Enter');await page.locator('[data-tool-report-return]').waitFor();
   const selections=await page.evaluate(()=>window.__toolReportFeedback204.snapshot().selections);assert.equal(selections.length,1);assert.equal(selections[0].project,input.project_id);assert.equal(selections[0].workerID,input.worker_id);assert.equal(selections[0].adapterID,input.local_tool);assert.equal(selections[0].commandID,input.command_id);assert.match(selections[0].profileSHA256,/^[a-f0-9]{64}$/);result.selections=selections;
   const quick=page.locator('[data-tool-report-return]');await quick.locator('.tool-quick-node').waitFor();assert.ok((await quick.locator('.tool-quick-node').innerText()).includes(input.worker_id));
   await quick.locator('[data-quick-name]').waitFor();assert.equal(await quick.locator('[data-quick-name]').inputValue(),input.command_id);await page.waitForFunction(()=>{const button=document.querySelector('[data-tool-report-return] [data-quick-register]');return button&&!button.disabled;});
  });
  await group('en_old_ack_cancel_and_revocation',async group=>{
   await action('report_initial');await visit('en-US',390,'dark');await page.locator('[data-tool-report-check]').click();await page.waitForFunction(()=>document.querySelector('[data-tool-report-status]')?.getAttribute('data-tool-report-status')==='current');
   const before=result.api_reads.filter(value=>value.startsWith('/tool-discovery?')).length;
   await page.locator('[data-tool-report-wait]').click();await page.waitForFunction(()=>document.querySelector('[data-tool-report-status]')?.getAttribute('data-tool-report-status')==='waiting');group.replay=await action('replay_initial');
   await page.waitForFunction(()=>document.querySelector('[data-tool-report-status]')?.getAttribute('data-tool-report-status')==='waiting');
   await new Promise(resolve=>{const deadline=Date.now()+8000;const timer=setInterval(()=>{if(result.api_reads.filter(value=>value.startsWith('/tool-discovery?')).length>=before+2){clearInterval(timer);resolve();}else if(Date.now()>deadline){clearInterval(timer);resolve();}},100);});
   assert.ok(result.api_reads.filter(value=>value.startsWith('/tool-discovery?')).length>=before+2,'Observe one real poll after the baseline');assert.equal(await status().getAttribute('data-tool-report-status'),'waiting');await page.locator('[data-tool-report-cancel]').click();
   await page.locator('[data-tool-report-check]').click();await page.waitForFunction(()=>document.querySelector('[data-tool-report-status]')?.getAttribute('data-tool-report-status')==='current');group.layout=await layout();await capture('en-current-inventory');
   await action('revoke_worker');await page.locator('[data-tool-report-check]').click();await page.waitForFunction(()=>document.querySelector('[data-tool-report-status]')?.getAttribute('data-tool-report-status')==='forbidden');assert.equal(await page.locator('[data-tool-report-selection]').count(),0);
  });
 }finally{
  if(app){await app.close();result.native_exit=await nativeExit;}save();
 }
 assert.deepEqual(result.native_exit,{code:0,signal:null});assert.deepEqual(result.page_errors,[]);assert.deepEqual(result.unexpected_requests,[]);
 result.passed=true;save();process.stdout.write(JSON.stringify({stage:result.stage,passed:true,groups:result.groups,images:result.images,native_pid:result.native_pid,native_exit:result.native_exit,api_reads:result.api_reads.length,selections:result.selections.length})+'\n');
}
main().catch(error=>{console.error(String(error.stack||error));process.exitCode=1;});
