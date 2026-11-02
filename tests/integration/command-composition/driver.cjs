'use strict';
// Production component -> generated PowerShell command -> compiled original
// CLI -> new local file. No control service, model, credentials or task process.
const fs=require('node:fs'),path=require('node:path'),http=require('node:http'),cp=require('node:child_process'),crypto=require('node:crypto'),assert=require('node:assert/strict');
const root=path.resolve(__dirname,'../../..'),base=path.join(root,'.cache/command-composition-215'),out=path.join(base,'web/out');
const sha=bytes=>crypto.createHash('sha256').update(bytes).digest('hex');
async function main(){
 const dir=path.join(base,'visual',new Date().toISOString().replace(/[-:.]/g,''));fs.mkdirSync(dir,{recursive:true});
 const machine=path.join(dir,'local files');fs.mkdirSync(machine);
 const files=[path.join(machine,"开发 source's.json"),path.join(machine,'tests.json')],destination=path.join(machine,'combined.json');
 const coder={adapter:'suma',protocol:'suma-jsonl-v1',executable:'node',args:[path.join(machine,'not-launched.mjs')],input:'agent-jsonl',read_only:false,model_execution_protocol:'suma-model-execution-v1',native_workspace:{checks:['fixed-check'],runtime_manifest:path.join(machine,'not-read.manifest.json')}};
 fs.writeFileSync(files[0],JSON.stringify({'native-coder':coder})+'\n');
 fs.writeFileSync(files[1],JSON.stringify({'fixed-check':{adapter:'command',protocol:'text-v1',executable:'not-launched-215',args:[],read_only:false,test_evidence:true,test_report:true}})+'\n');
 const before=files.map(file=>sha(fs.readFileSync(file)));
 const result={stage:'T-COMMAND-COMPOSITION-215',scope:'Original component, generated shell and compiled offline CLI; no vendor or task execution',passed:false,checks:[],images:[],api:[],page_errors:[],cli:[]};
 const server=http.createServer((request,response)=>{
  const url=new URL(request.url,'http://fixture.test');response.setHeader('Cache-Control','no-store');
  if(url.pathname.startsWith('/api/'))result.api.push({method:request.method,path:url.pathname});
  const file=url.pathname==='/'?'index.html':url.pathname.slice(1);
  if(request.method!=='GET'||!['index.html','fixture.js','fixture.css'].includes(file)){response.writeHead(404);return response.end();}
  response.setHeader('Content-Type',file.endsWith('.js')?'text/javascript':file.endsWith('.css')?'text/css':'text/html');response.end(fs.readFileSync(path.join(out,file)));
 });
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));const origin='http://127.0.0.1:'+server.address().port;result.origin=origin;
 let app,page,exit;
 try{
  const env={};for(const[key,value]of Object.entries(process.env))if(['path','systemroot','windir','comspec','pathext','home','userprofile','homedrive','homepath','appdata','localappdata','programdata','programfiles','programfiles(x86)','programw6432'].includes(key.toLowerCase()))env[key]=value;
  const tmp=path.join(dir,'process-tmp');fs.mkdirSync(tmp);Object.assign(env,{TEMP:tmp,TMP:tmp,SUMA_FIXTURE_ORIGIN:origin,SUMA_FIXTURE_DIRECTORY:dir,SUMA_FIXTURE_PARTITION:'suma-fixture-command-composition215'});
  const {_electron}=require(process.env.SUMA_PLAYWRIGHT_MODULE);app=await _electron.launch({executablePath:path.join(root,'apps/desktop/node_modules/electron/dist/electron.exe'),args:[path.join(__dirname,'../support/native.cjs')],cwd:root,env,timeout:15000});
  result.native_pid=app.process().pid;exit=new Promise(resolve=>app.process().once('exit',(code,signal)=>resolve({code,signal})));fs.writeFileSync(path.join(dir,'native-start.json'),JSON.stringify({pid:result.native_pid,origin})+'\n');
  page=await app.firstWindow();page.setDefaultTimeout(12000);page.on('pageerror',error=>result.page_errors.push(error.message));
  const capture=async name=>{await page.screenshot({path:path.join(dir,name)});result.images.push({path:name,sha256:sha(fs.readFileSync(path.join(dir,name)))});};
  async function open(locale,width,theme){await page.setViewportSize({width,height:1000});await page.goto(origin+'/?locale='+locale+'&theme='+theme);await page.locator('[data-command-composition] > summary').click();}
  async function fill(){await page.locator('[data-command-composition-shell]').selectOption('powershell');const sources=page.locator('[data-command-composition-source]');await sources.nth(0).fill(files[0]);await sources.nth(1).fill(files[1]);await page.locator('[data-command-composition-output]').fill(destination);}
  await open('zh-CN',1366,'light');await page.locator('[data-command-composition-copy]').click();await page.locator('[data-command-composition-error]').waitFor();assert.equal(await page.locator('[data-command-composition-shell]').evaluate(element=>element===document.activeElement),true);await fill();
  await page.evaluate(()=>{window.__copied=[];Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:async text=>window.__copied.push(text)}});});
  await page.locator('[data-command-composition-copy]').click();const command=await page.locator('[data-command-composition-command]').innerText();assert.deepEqual(await page.evaluate(()=>window.__copied),[command]);await capture('cn-command-composition.png');
  result.checks.push('production_form_generates_and_copies_exact_powershell_command_with_unicode_and_quote_paths');
  const binaryDirectory=path.join(base,'bin'),cliEnv={...env,Path:binaryDirectory+';'+(env.Path||env.PATH||'')};delete cliEnv.PATH;
  // An interactive PowerShell block deliberately does not exit the user's
  // terminal. The harness propagates the native exit code to its child shell.
  const run=()=>cp.spawnSync(process.env.SUMA_PWSH_PATH,['-NoLogo','-NoProfile','-NonInteractive','-Command',command+'\nexit $LASTEXITCODE'],{cwd:root,env:cliEnv,encoding:'utf8',windowsHide:true,timeout:15000,maxBuffer:1<<20});
  const first=run();result.cli.push({exit_code:first.status,signal:first.signal,stdout:first.stdout,stderr:first.stderr});assert.equal(first.status,0,first.stderr);assert.equal(first.error,undefined);
  const receipt=JSON.parse(first.stdout),written=fs.readFileSync(destination),composed=JSON.parse(written);assert.equal(receipt.configuration_sha256,sha(written));assert.equal(receipt.source_count,2);assert.equal(receipt.command_count,2);assert.deepEqual(composed['native-coder'].native_workspace.checks,['fixed-check']);assert.equal(composed['fixed-check'].test_report,true);assert.deepEqual(files.map(file=>sha(fs.readFileSync(file))),before);
  result.checks.push('actual_compiled_cli_combines_cross_file_checks_and_preserves_exact_source_bytes_without_launching_tools');
  const second=run();result.cli.push({exit_code:second.status,signal:second.signal,stdout:second.stdout,stderr:second.stderr});assert.equal(second.status,1);assert.equal(JSON.parse(second.stderr).code,'output_exists');assert.equal(sha(fs.readFileSync(destination)),sha(written));
  result.checks.push('same_generated_command_refuses_existing_destination_without_overwrite');
  await open('en-US',390,'dark');await fill();await page.evaluate(()=>{Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:async()=>{}}});});await page.locator('[data-command-composition-copy]').click();await page.locator('[data-command-composition-command]').waitFor();await capture('en-command-composition.png');result.geometry=await page.evaluate(()=>({width:innerWidth,scrollWidth:document.documentElement.scrollWidth,controls:[...document.querySelectorAll('[data-command-composition] input,[data-command-composition] select,[data-command-composition] button')].map(element=>element.getBoundingClientRect().height)}));assert.ok(result.geometry.scrollWidth<=390);assert.ok(result.geometry.controls.every(height=>height>=44));assert.equal(result.api.length,0);assert.equal(result.page_errors.length,0);result.checks.push('english_narrow_dark_controls_fit_and_no_api_requests');
  result.configuration_sha256=sha(written);result.source_hashes=before;result.passed=true;
 }catch(error){result.error=error.stack||String(error);if(page)await page.screenshot({path:path.join(dir,'failed-ui.png')}).catch(()=>{});}
 finally{
  if(app){let timer;try{result.native_exit=await Promise.race([(async()=>{await app.close();return await exit;})(),new Promise((_,reject)=>{timer=setTimeout(()=>reject(Error('Owned native close timed out')),15000);})]);}catch(error){result.passed=false;result.close_error=String(error);}finally{clearTimeout(timer);}}
  await new Promise(resolve=>server.close(resolve));if(result.native_exit?.code!==0||result.native_exit?.signal!==null)result.passed=false;
  fs.writeFileSync(path.join(dir,'result.json'),JSON.stringify(result,null,2)+'\n');
 }
 console.log(JSON.stringify({passed:result.passed,directory:dir,checks:result.checks.length}));if(!result.passed)process.exitCode=1;
}
main().catch(error=>{console.error(String(error));process.exitCode=1;});
