'use strict';
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict'),crypto=require('node:crypto');
const root=path.resolve(__dirname,'../../..'),sha=data=>crypto.createHash('sha256').update(data).digest('hex');
async function stdin(){let raw='';for await(const chunk of process.stdin){raw+=chunk;if(Buffer.byteLength(raw)>65536)throw Error('Oversized private fixture input');}return JSON.parse(raw);}
async function main(){
 const input=await stdin(),origin=input.origin,dir=path.resolve(input.directory),base=path.join(root,'.cache/remote-tool-scan-205'),partition='suma-fixture-remote-tools205';
 assert.match(origin,/^http:\/\/127\.0\.0\.1:\d+$/);assert.ok(dir.startsWith(base+path.sep));assert.equal(input.local_tool,'codebuddy-cli');assert.ok(input.action_key.length>=32);
 if(!process.env.SUMA_PLAYWRIGHT_MODULE)throw Error('Set the installed SUMA_PLAYWRIGHT_MODULE');
 const {_electron}=require(process.env.SUMA_PLAYWRIGHT_MODULE),runID=new Date().toISOString().replace(/[^0-9TZ]/g,''),result={stage:'T-REMOTE-TOOL-SCAN-205',run_id:runID,origin,scope:'Production scan/guide components, real service/SQLite and actual Runner with owned Node version/help package; no vendor/model or task execution.',groups:{},images:[],api:[],unexpected:[],page_errors:[]};
 const save=()=>fs.writeFileSync(path.join(dir,'driver-result.json'),JSON.stringify(result,null,2));
 const action=async name=>{const response=await fetch(origin+'/__fixture/action',{method:'POST',headers:{'Content-Type':'application/json','X-Suma-Fixture-Key':input.action_key},body:JSON.stringify({action:name})});assert.equal(response.status,200,'Owned fixture action '+name);return response.json();};
 let app,page,exit;
 try{
  const env={};for(const [key,value]of Object.entries(process.env))if(['path','systemroot','windir','comspec','pathext','home','userprofile','homedrive','homepath','appdata','localappdata','programdata','programfiles','programfiles(x86)','programw6432'].includes(key.toLowerCase()))env[key]=value;
  Object.assign(env,{TEMP:path.join(dir,'browser-tmp'),TMP:path.join(dir,'browser-tmp'),SUMA_FIXTURE_ORIGIN:origin,SUMA_FIXTURE_DIRECTORY:dir,SUMA_FIXTURE_PARTITION:partition});fs.mkdirSync(env.TEMP,{recursive:true});
  app=await _electron.launch({executablePath:process.env.SUMA_ELECTRON_EXECUTABLE||path.join(root,'apps/desktop/node_modules/electron/dist/electron.exe'),args:[path.join(__dirname,'../support/native.cjs')],cwd:root,env,timeout:15000});result.native_pid=app.process().pid;exit=new Promise(resolve=>app.process().once('exit',(code,signal)=>resolve({code,signal})));page=await app.firstWindow();page.setDefaultTimeout(15000);
  await app.evaluate(async({session},{origin,partition,cookie})=>{await session.fromPartition(partition).cookies.set({url:origin,name:cookie.name,value:cookie.value,httpOnly:true,sameSite:'strict'});},{origin,partition,cookie:input.cookie});
  page.on('pageerror',error=>result.page_errors.push(error.message));page.on('dialog',dialog=>{result.page_errors.push('Unexpected '+dialog.type());void dialog.dismiss();});
  await page.route('**/*',async route=>{const request=route.request(),url=new URL(request.url()),read=request.method()==='GET'&&(url.pathname==='/'||url.pathname==='/fixture.js'||url.pathname==='/fixture.css'||url.pathname==='/__fixture/view'||url.pathname.startsWith('/api/v1/')),create=request.method()==='POST'&&url.pathname==='/api/v1/tool-installation-scans'&&!url.search;if(url.origin===origin&&(read||create)){if(url.pathname.startsWith('/api/v1/'))result.api.push({method:request.method(),path:url.pathname});await route.continue();}else{result.unexpected.push({method:request.method(),path:url.pathname});await route.abort();}});
  const panel=()=>page.locator('[data-remote-tool-scan]'),status=()=>page.locator('[data-remote-tool-scan-status]');
  async function phase(value,timeout=15000){await page.waitForFunction(value=>document.querySelector('[data-remote-tool-scan-status]')?.getAttribute('data-remote-tool-scan-status')===value,value,{timeout});}
  async function visit(locale,width,theme){await page.setViewportSize({width,height:1050});await page.goto(origin+'/?locale='+locale+'&theme='+theme);await page.waitForLoadState('networkidle');await panel().waitFor();}
  async function capture(name){const file=runID+'-'+name+'.png';await panel().evaluate(element=>element.scrollIntoView({block:'center'}));await page.screenshot({path:path.join(dir,file),fullPage:false});result.images.push({path:file,sha256:sha(fs.readFileSync(path.join(dir,file)))});}
  async function layout(){const value=await panel().evaluate(element=>({width:innerWidth,scrollWidth:document.documentElement.scrollWidth,targets:[...element.querySelectorAll('button,input,select,summary')].filter(node=>node.getClientRects().length).map(node=>({tag:node.tagName,height:node.getBoundingClientRect().height}))}));assert.ok(value.scrollWidth<=value.width);assert.ok(value.targets.every(target=>target.height>=43.5));return value;}
  async function group(name,run){result.groups[name]={passed:false};try{await run(result.groups[name]);assert.deepEqual(result.page_errors,[]);assert.deepEqual(result.unexpected,[]);result.groups[name].passed=true;}catch(error){result.groups[name].error=String(error.stack||error);result.groups[name].body=await panel().innerText().catch(()=>undefined);await page.screenshot({path:path.join(dir,runID+'-'+name+'-failure.png')}).catch(()=>{});throw error;}finally{save();}}
  await group('cn_recover_and_prepare',async group=>{
   await visit('zh-CN',1366,'light');await phase('ready');await page.locator('[data-remote-tool-scan-create]').click();await phase('write_unknown');await page.locator('[data-remote-tool-scan-recover]').click();await phase('completed',25000);
   const row=page.locator('[data-remote-tool-scan-row="codebuddy-cli"]');await row.waitFor();group.layout=await layout();await capture('cn-installed');
   const picker=page.locator('[data-remote-tool-scan-request]');result.scan_id=await picker.inputValue();assert.ok(result.scan_id);
   await page.locator('[data-remote-tool-scan-prepare="codebuddy-cli"]').focus();await page.keyboard.press('Enter');await page.locator('[data-remote-scan-guide]').waitFor();
   const prepares=await page.evaluate(()=>window.__remoteToolScan205.snapshot().prepares);assert.deepEqual(prepares,[{adapterID:input.local_tool,workerID:input.worker_id}]);result.prepares=prepares;
   await page.waitForFunction(()=>document.querySelector('.tool-cli-setup-select select')?.value==='codebuddy');assert.equal(await page.locator('[name="remote-runner-worker"]').inputValue(),input.worker_id);assert.equal(await page.locator('[data-remote-runner-allow-installed-discovery]').isChecked(),false);
  });
  await group('en_history_and_revocation',async group=>{
   await visit('en-US',390,'dark');const picker=page.locator('[data-remote-tool-scan-request]');await picker.waitFor();await picker.selectOption(result.scan_id);await phase('completed');group.layout=await layout();await capture('en-history');
   await action('revoke_worker');await page.locator('[data-remote-tool-scan-refresh]').click();await page.waitForFunction(()=>document.querySelector('[data-remote-tool-scan]')?.getAttribute('data-remote-tool-scan-capability')==='unknown');
   // The current project reader retains historical audit access. Revoking the
   // machine removes current support and continuation, rather than rewriting
   // its immutable completed observation into an invented authorization error.
   assert.ok(await page.locator('[data-remote-tool-scan-row="codebuddy-cli"]').count());assert.equal(await page.locator('[data-remote-tool-scan-create]').isDisabled(),true);
   const prepare=page.locator('[data-remote-tool-scan-prepare]');if(await prepare.count())assert.equal(await prepare.first().isDisabled(),true);
  });
 }finally{if(app){await app.close();result.native_exit=await exit;}save();}
 assert.deepEqual(result.native_exit,{code:0,signal:null});assert.deepEqual(result.page_errors,[]);assert.deepEqual(result.unexpected,[]);result.passed=true;save();process.stdout.write(JSON.stringify({stage:result.stage,passed:true,groups:result.groups,images:result.images,native_pid:result.native_pid,native_exit:result.native_exit,scan_id:result.scan_id,prepares:result.prepares})+'\n');
}
main().catch(error=>{console.error(String(error.stack||error));process.exitCode=1;});
