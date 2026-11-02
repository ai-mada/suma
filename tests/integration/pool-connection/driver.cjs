'use strict';
// One real HTTP/SQLite/system-vault check, then two explicitly requested modes
// of the original PoolWorker library. No foreground CLI start is claimed.
const fs=require('node:fs'),path=require('node:path'),cp=require('node:child_process');
const crypto=require('node:crypto'),assert=require('node:assert/strict');
const root=path.resolve(__dirname,'../../..'),base=path.join(root,'.cache/pool-connection-212');
const sha=raw=>crypto.createHash('sha256').update(raw).digest('hex');
async function input(){let raw='';for await(const chunk of process.stdin){raw+=chunk;if(Buffer.byteLength(raw)>65536)throw Error('Oversized private fixture input');}return JSON.parse(raw);}

async function main(){
  const owned=await input(),origin=owned.origin,dir=path.resolve(owned.directory);
  assert.equal(owned.protocol,'suma-pool-connection-actual-v1');
  assert.match(origin,/^http:\/\/127\.0\.0\.1:\d+$/);assert.ok(dir.startsWith(base+path.sep));
  assert.match(owned.pool_id,/^[a-f0-9]{32}$/);assert.equal(owned.worker_id,'pool-'+owned.runner_id);
  assert.equal(owned.action_key.length,64);
  const result={stage:'T-POOL-CONNECTION-212',scope:'Production connection/Tools components, actual HTTP/SQLite and system-backed identity. One compiled check; explicit fixture actions use the original Worker library. No vendor/model/code acceptance.',
    compiled_check_calls:0,groups:{},checks:[],images:[],api:[],page_errors:[],passed:false};
  const action=async name=>{
    const response=await fetch(origin+'/__fixture/action',{method:'POST',headers:{'Content-Type':'application/json','X-Suma-Fixture-Key':owned.action_key},body:JSON.stringify({action:name})});
    assert.equal(response.status,200,'Owned fixture '+name);return response.json();
  };
  let app,exit,page;
  try{
    const env={};
    for(const [key,value] of Object.entries(process.env))
      if(['path','systemroot','windir','comspec','pathext','home','userprofile','homedrive','homepath','appdata','localappdata','programdata','programfiles','programfiles(x86)','programw6432'].includes(key.toLowerCase()))env[key]=value;
    const partition='suma-fixture-pool-connection212';
    Object.assign(env,{TEMP:path.join(dir,'browser-tmp'),TMP:path.join(dir,'browser-tmp'),SUMA_FIXTURE_ORIGIN:origin,SUMA_FIXTURE_DIRECTORY:dir,SUMA_FIXTURE_PARTITION:partition});
    fs.mkdirSync(env.TEMP,{recursive:true});
    const {_electron}=require(process.env.SUMA_PLAYWRIGHT_MODULE);
    app=await _electron.launch({executablePath:path.join(root,'apps/desktop/node_modules/electron/dist/electron.exe'),
      args:[path.join(__dirname,'../support/native.cjs')],cwd:root,env,timeout:15000});
    result.native_pid=app.process().pid;
    exit=new Promise(resolve=>app.process().once('exit',(code,signal)=>resolve({code,signal})));
    await app.evaluate(async({session},{origin,partition,cookie})=>{
      await session.fromPartition(partition).cookies.set({url:origin,name:cookie.name,value:cookie.value,httpOnly:true,sameSite:'strict'});
    },{origin,partition,cookie:owned.cookie});
    page=await app.firstWindow();page.setDefaultTimeout(15000);
    await page.addInitScript(()=>Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:async text=>{window.__poolConnectionCopied=text;}}}));
    page.on('pageerror',error=>result.page_errors.push(error.message));
    page.on('response',response=>{const url=new URL(response.url());if(url.pathname.startsWith('/api/v1/'))result.api.push({method:response.request().method(),path:url.pathname,status:response.status()});});
    const field=name=>page.locator('[data-pool-connection-field="'+name+'"]');
    const screenshot=async name=>{await page.screenshot({path:path.join(dir,name),fullPage:false});result.images.push({path:name,sha256:sha(fs.readFileSync(path.join(dir,name)))});};
    async function open(locale,width,theme){
      await page.setViewportSize({width,height:1000});await page.goto(origin+'/?locale='+locale+'&theme='+theme);
      await page.locator('[data-pool-config-connection]>summary').click();
      await field('identity').fill(owned.identity_file);
      assert.equal(await field('state').inputValue(),owned.pool_config_file+'.runner-state');
      assert.equal(await field('mode').inputValue(),'maintenance');
    }
    async function refreshCurrent(){
      for(let attempt=0;attempt<30;attempt++){
        await page.locator('[data-pool-connection-refresh]').click();
        await page.waitForFunction(()=>!document.querySelector('[data-pool-connection-refresh]')?.disabled);
        if(await page.locator('[data-pool-connection-readiness][data-source-current="true"]').count())return;
        await page.waitForTimeout(200);
      }
      throw Error('Original authenticated inventory did not become current');
    }
    async function copied(kind){
      await page.locator('[data-pool-connection-copy="'+kind+'"]').click();
      const command=await page.locator('[data-pool-connection-command="'+kind+'"]').textContent();
      assert.equal(await page.evaluate(()=>window.__poolConnectionCopied),command);
      return command;
    }

    const before=await action('facts');
    assert.equal(before.historical_last_seen_at,'');assert.equal(before.journal_exists,false);assert.equal(before.run_count,0);
    await open('zh-CN',1366,'light');
    assert.equal(await page.locator('[data-pool-connection-copy="start"]').isDisabled(),true);
    const check=await copied('check');
    assert.ok(check.includes('--check-pool-connection')&&!check.includes('--allow-exec')&&!check.includes('--state'));
    const script=path.join(dir,'copied-check.ps1');
    fs.writeFileSync(script,check+'\nexit $LASTEXITCODE\n');
    const pwsh=process.env.SUMA_PWSH;assert.ok(path.isAbsolute(pwsh||''));
    const cliEnv={...env,PATH:path.dirname(owned.runner_executable)+path.delimiter+(env.PATH||env.Path||''),SUMA_POOL_RUNNER_TOKEN:'owned-env-token-must-not-be-read',SUMA_RUNNER_TOKEN:'owned-project-token-must-not-be-read'};
    result.compiled_check_calls++;
    const checked=cp.spawnSync(pwsh,['-NoProfile','-NonInteractive','-File',script],{cwd:dir,env:cliEnv,encoding:'utf8',windowsHide:true,timeout:20000});
    fs.writeFileSync(path.join(dir,'check.stdout.txt'),checked.stdout||'');fs.writeFileSync(path.join(dir,'check.stderr.txt'),checked.stderr||'');
    assert.equal(checked.status,0,checked.stderr);const report=JSON.parse(checked.stdout.trim());
    assert.equal(report.protocol,'suma-pool-connection-check-v1');assert.equal(report.pool_id,owned.pool_id);
    assert.equal(report.runner_id,owned.runner_id);assert.equal(report.worker_id,owned.worker_id);
    assert.equal(report.pool_active,true);assert.equal(report.configured_slots,1);assert.equal(report.current_max_slots,1);
    assert.equal(report.scopes.length,2);assert.ok(report.scopes.every(scope=>scope.granted));
    assert.equal(report.configuration_file_sha256,sha(fs.readFileSync(owned.pool_config_file)));
    result.check_report=report;
    const afterCheck=await action('facts');
    assert.equal(afterCheck.historical_last_seen_at,'');assert.equal(afterCheck.journal_exists,false);assert.equal(afterCheck.worker_starts,0);assert.equal(afterCheck.run_count,0);
    result.checks.push('copied_compiled_check_authenticates_without_worker_journal_last_seen_or_task');

    await page.locator('[data-pool-connection-refresh]').click();
    await page.locator('[data-pool-connection-readiness]').waitFor();
    assert.equal(await page.locator('[data-pool-connection-readiness]').getAttribute('data-source-current'),'false');
    assert.equal(await page.locator('[data-pool-connection-tools]').count(),1,'Authorized project navigation is independent of inventory availability');
    await field('discovery').check();
    const maintenance=await copied('start');
    assert.ok(maintenance.includes('--maintenance-only')&&maintenance.includes('--allow-installed-tool-discovery')&&!maintenance.includes('--allow-exec'));
    result.maintenance_command_sha256=sha(maintenance);await action('start_maintenance');
    await refreshCurrent();
    const maintained=await action('facts');assert.equal(maintained.source_current,true);assert.equal(maintained.run_count,0);assert.equal(maintained.profile_count,0);
    const factsBox=page.locator('[data-pool-connection-readiness]');await factsBox.evaluate(element=>element.scrollIntoView({block:'end'}));
    await screenshot('cn-current-pool-report.png');
    result.groups.cn={passed:true,locale:'zh-CN',maintenance_source_current:true,run_count:0,profile_count:0};
    result.checks.push('explicit_maintenance_original_worker_reports_current_bound_inventory_without_tasks');

    const link=page.locator('[data-pool-connection-tools]'),href=await link.getAttribute('href');
    const target=new URL(href,origin);assert.equal(target.searchParams.get('project'),owned.project);
    assert.equal(target.searchParams.get('panel'),'pool-tool:'+owned.pool_id+':'+owned.runner_id);
    await link.click();
    await page.locator('.tool-quick-connect').waitFor();
    await page.locator('[data-quick-tool]').selectOption('codex-jsonl');
    assert.ok((await page.locator('[role="dialog"]').innerText()).includes(owned.worker_id));
    const navigated=await action('facts');assert.equal(navigated.profile_count,0);assert.equal(navigated.run_count,0);
    result.checks.push('authorized_exact_project_pool_runner_locator_opens_original_quick_without_scan_register_enable');

    await open('en-US',390,'dark');await field('mode').selectOption('tasks');
    const taskStart=await copied('start');assert.ok(taskStart.includes('--allow-exec')&&!taskStart.includes('--maintenance-only'));
    result.tasks_command_sha256=sha(taskStart);await action('start_tasks');
    let executed;
    for(let attempt=0;attempt<80;attempt++){
      executed=await action('facts');if(executed.task_state==='succeeded')break;
      if(executed.task_state==='failed')throw Error('Owned original Pool command failed');
      await page.waitForTimeout(150);
    }
    assert.equal(executed.task_state,'succeeded');assert.equal(executed.attempt_count,1);assert.equal(executed.run_count,1);
    assert.equal(executed.worker_id,owned.worker_id);assert.ok(executed.output.includes('owned-pool212-verification'));
    assert.equal(executed.profile_count,0);result.execution=executed;
    await refreshCurrent();
    const geometry=await page.locator('[data-pool-config-connection]').evaluate(node=>({width:innerWidth,scrollWidth:document.documentElement.scrollWidth,
      controls:[...node.querySelectorAll('button,input:not([type=checkbox]),select,summary,a.button')].filter(element=>element.getClientRects().length).map(element=>element.getBoundingClientRect().height)}));
    assert.ok(geometry.scrollWidth<=geometry.width+1);assert.ok(geometry.controls.every(height=>height>=43.5));
    await page.locator('[data-pool-connection-readiness]').evaluate(element=>element.scrollIntoView({block:'end'}));await screenshot('en-current-pool-report.png');
    result.groups.en={passed:true,locale:'en-US',viewport:{width:390,height:1000},geometry};
    result.checks.push('explicit_task_mode_original_worker_executes_one_real_readonly_project_command_and_http_result');
    await action('stop_worker');const stopped=await action('facts');assert.equal(stopped.worker_starts,2);assert.equal(stopped.worker_stops,2);
    result.final_facts=stopped;assert.deepEqual(result.page_errors,[]);
    assert.equal(result.api.filter(row=>row.method!=='GET'&&!row.path.startsWith('/api/v1/pool-runner/')).length,0);
    result.passed=true;
  }finally{
    if(app){await app.close();result.native_exit=await exit;}
    fs.writeFileSync(path.join(dir,'result.json'),JSON.stringify(result,null,2)+'\n');
  }
  assert.deepEqual(result.native_exit,{code:0,signal:null});
  console.log(JSON.stringify({passed:result.passed,compiled_check_calls:result.compiled_check_calls,groups:Object.keys(result.groups),images:result.images,native_exit:result.native_exit}));
}
main().catch(error=>{console.error(error.stack||String(error));process.exitCode=1;});
