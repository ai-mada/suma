'use strict';
// Representative production components with explicitly synthetic read-only
// HTTP data. This is no database, backend authorization or real Review approval.
const fs=require('node:fs'),path=require('node:path'),http=require('node:http'),crypto=require('node:crypto'),assert=require('node:assert/strict');
const root=path.resolve(__dirname,'../../..'),base=path.join(root,'.cache/task-delivery-navigation-214'),out=path.join(base,'web/out');
const sha=value=>crypto.createHash('sha256').update(value).digest('hex');
async function main(){
  const dir=path.join(base,'visual',new Date().toISOString().replace(/[-:.]/g,''));fs.mkdirSync(dir,{recursive:true});
  const session={user:{id:'user-a',name:'Reviewer',email:'review@example.test',role:'owner'},organization:{id:'org-a',name:'Organization'},projects:[]};
  const summary={total:1,waiting:0,running:0,succeeded:1,failed:0,skipped:0,reconciling:0,other:0,limit:20,complete:true,items:[{id:'verify',name:'Independent checks',type:'test',state:'succeeded',attempt:1}]};
  const delivery={project_id:'project-a',task_id:'task-a',task_version:3,task_status:'in_progress',run_basis_state:'current',formal_acceptance:false,run:{run_id:'run-a',version:4,state:'succeeded',nodes:summary,tests:summary},expert_plans:{scope:'current_run',total:1,limit:20,complete:true,items:[{plan_id:'plan-a',version:2,revision:1,team_id:'team-a',team_name:'Engineering',state:'active',item_state:'awaiting_review'}]},issues:{scope:'current_run',total:1,limit:20,complete:true,items:[{issue_id:'issue-a',title:'Review finding',issue_version:1,issue_status:'open',source:'review_rejected',source_review_id:'review-a',requirement_id:'req-a',requirement_version:2}]}};
  const result={stage:'T-TASK-DELIVERY-NAVIGATION-214',passed:false,scope:'Synthetic read-only HTTP with original production panel and locator. Original App Review form checked separately.',api:[],checks:[],images:[],page_errors:[],post_count:0};
  const server=http.createServer((request,response)=>{
    const url=new URL(request.url,'http://fixture.test');response.setHeader('Cache-Control','no-store');
    if(request.method!=='GET'){result.post_count++;response.writeHead(405);return response.end();}
    if(url.pathname==='/__fixture/session'){response.setHeader('Content-Type','application/json');return response.end(JSON.stringify(session));}
    if(url.pathname==='/api/v1/tasks/task-a/delivery'&&url.searchParams.get('project_id')==='project-a'){result.api.push({method:'GET',path:url.pathname});response.setHeader('Content-Type','application/json');return response.end(JSON.stringify(delivery));}
    const name=url.pathname==='/'?'index.html':url.pathname.slice(1);
    if(!['index.html','fixture.js','fixture.css'].includes(name)){response.writeHead(404);return response.end();}
    response.setHeader('Content-Type',name.endsWith('.js')?'text/javascript':name.endsWith('.css')?'text/css':'text/html');response.end(fs.readFileSync(path.join(out,name)));
  });
  await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));const origin='http://127.0.0.1:'+server.address().port;result.origin=origin;
  let app,page,exit;
  try{
    const env={};for(const [key,value]of Object.entries(process.env))if(['path','systemroot','windir','comspec','pathext','home','userprofile','homedrive','homepath','appdata','localappdata','programdata','programfiles','programfiles(x86)','programw6432'].includes(key.toLowerCase()))env[key]=value;
    const tmp=path.join(dir,'browser-tmp');fs.mkdirSync(tmp,{recursive:true});Object.assign(env,{TEMP:tmp,TMP:tmp,SUMA_FIXTURE_ORIGIN:origin,SUMA_FIXTURE_DIRECTORY:dir,SUMA_FIXTURE_PARTITION:'suma-fixture-delivery-navigation214'});
    const {_electron}=require(process.env.SUMA_PLAYWRIGHT_MODULE);app=await _electron.launch({executablePath:path.join(root,'apps/desktop/node_modules/electron/dist/electron.exe'),args:[path.join(__dirname,'../support/native.cjs')],cwd:root,env,timeout:15000});
    result.native_pid=app.process().pid;exit=new Promise(resolve=>app.process().once('exit',(code,signal)=>resolve({code,signal})));fs.writeFileSync(path.join(dir,'native-start.json'),JSON.stringify({pid:result.native_pid,origin})+'\n');
    page=await app.firstWindow();page.setDefaultTimeout(12000);page.on('pageerror',error=>result.page_errors.push(error.message));
    const capture=async name=>{await page.screenshot({path:path.join(dir,name)});result.images.push({path:name,sha256:sha(fs.readFileSync(path.join(dir,name)))});};
    async function open(locale,width,theme){await page.setViewportSize({width,height:1000});await page.goto(origin+'/?locale='+locale+'&theme='+theme);await page.locator('[data-task-delivery-toggle]').click();await page.locator('[data-task-delivery-plan]').waitFor();}
    await open('zh-CN',1366,'light');await page.locator('[data-task-delivery-requirement]').scrollIntoViewIfNeeded();await capture('cn-delivery-actions.png');
    for(const [selector,expected]of [['plan','expert-team:plan-a'],['issue','issue-a'],['requirement','req-a']]){
      const href=await page.locator('[data-task-delivery-'+selector+']').getAttribute('href');const target=new URL(href,origin);assert.equal(target.searchParams.get('project'),'project-a');assert.equal(selector==='plan'?target.searchParams.get('panel'):target.searchParams.get('record'),expected);
    }
    result.checks.push('native_exact_project_team_issue_and_requirement_hrefs_present_without_callbacks');
    await page.locator('[data-task-delivery-review]').click();await page.locator('[data-task-acceptance-locator][data-state="opened"]').waitFor();assert.equal(await page.locator('[data-fixture-opened]').getAttribute('data-fixture-opened'),'1');
    assert.equal(new URL(page.url()).searchParams.get('panel'),'task-acceptance:3:run-a');result.checks.push('review_native_link_consumes_exact_version_run_once_without_post');
    await open('en-US',390,'dark');await page.locator('[data-task-delivery-requirement]').scrollIntoViewIfNeeded();
    result.geometry=await page.evaluate(()=>({width:innerWidth,scrollWidth:document.documentElement.scrollWidth,controls:[...document.querySelectorAll('.task-delivery-reference')].map(element=>element.getBoundingClientRect().height)}));assert.ok(result.geometry.scrollWidth<=390);assert.ok(result.geometry.controls.every(height=>height>=44));await capture('en-delivery-actions.png');
    const issue=page.locator('[data-task-delivery-issue]');await issue.click();await page.locator('[data-fixture-destination]').waitFor();assert.equal(new URL(page.url()).searchParams.get('record'),'issue-a');result.checks.push('english_narrow_native_issue_navigation_preserves_exact_record');
    await page.goto(origin+'/?'+new URLSearchParams({locale:'en-US',panel:'task-acceptance:4:run-a'}));await page.locator('[data-task-acceptance-locator][data-state="basis_changed"]').waitFor();assert.equal(await page.locator('[data-fixture-opened]').getAttribute('data-fixture-opened'),'0');result.checks.push('mismatched_review_locator_never_calls_entry');
    assert.equal(result.page_errors.length,0);assert.equal(result.post_count,0);result.passed=true;
  }catch(error){result.error=error.stack||String(error);if(page)await page.screenshot({path:path.join(dir,'failed-ui.png')}).catch(()=>{});}
  finally{
    if(app){let timer;try{result.native_exit=await Promise.race([(async()=>{await app.close();return await exit;})(),new Promise((_,reject)=>{timer=setTimeout(()=>reject(Error('Owned native close timed out')),15000);})]);}catch(error){result.passed=false;result.close_error=String(error);}finally{clearTimeout(timer);}}
    await new Promise(resolve=>server.close(resolve));
    if(result.native_exit?.code!==0||result.native_exit?.signal!==null)result.passed=false;
    fs.writeFileSync(path.join(dir,'result.json'),JSON.stringify(result,null,2)+'\n');
  }
  console.log(JSON.stringify({passed:result.passed,directory:dir,checks:result.checks.length}));if(!result.passed)process.exitCode=1;
}
main().catch(error=>{console.error(String(error));process.exitCode=1;});
