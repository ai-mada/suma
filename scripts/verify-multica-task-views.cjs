// Owned loopback fixture only. Never target an existing user database.
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict'),crypto=require('node:crypto');
const {chromium,request}=require(process.env.SUMA_PLAYWRIGHT_MODULE||'playwright');
const root=process.cwd(),base='http://127.0.0.1:43956',cache=path.join(root,'.cache/multica-136'),out=path.join(root,'dist/quality/multica-136');
const fixtureFile=path.join(cache,'fixture.json');
const checkList=[],errors=[],external=[];
const check=(name,value)=>{assert.ok(value,name);checkList.push(name);};
(async()=>{
 fs.mkdirSync(out,{recursive:true});
 let fixture=fs.existsSync(fixtureFile)?JSON.parse(fs.readFileSync(fixtureFile,'utf8')):null;
 const api=await request.newContext({baseURL:base,extraHTTPHeaders:{Origin:base}});
 const credentials=fixture?.credentials||{email:'task-view-fixture@example.test',password:crypto.randomBytes(24).toString('hex')};
 const login=await api.post('/api/v1/auth/'+(fixture?'login':'setup'),{data:fixture?credentials:{...credentials,name:'交互验收人员',organization:'速码 · Multica 交互验收样例',project:'交易交付 · 合成样例'}});
 assert.equal(login.status(),200,'Dedicated fixture login/setup must succeed; never reuse unrelated data');
 const session=await login.json();assert.equal(session.organization.name,'速码 · Multica 交互验收样例');
 const project=session.projects[0].id;
 const headers={'X-Suma-Expected-User':session.user.id,'X-Suma-Expected-Organization':session.organization.id};
 const create=async(kind,data)=>{const response=await api.post('/api/v1/'+kind,{headers,data:{project_id:project,...data,idempotency_key:'multica136-'+crypto.randomUUID()}});assert.ok(response.ok(),kind+': '+response.status()+' '+await response.text());return response.json();};
 if(!fixture){
  fixture={credentials,project,marker:'T-MULTICA-137-owned-synthetic',tasks:[]};
  fs.writeFileSync(fixtureFile,JSON.stringify(fixture));
 }
 if(!fixture.owner){fixture.owner=await create('members',{title:'交互验收人员 · 样例',name:'交互验收人员 · 样例',kind:'human',status:'active',data:{role:'developer',user_id:session.user.id}});fs.writeFileSync(fixtureFile,JSON.stringify(fixture));}
 const names=['梳理支付回调验收标准','明确多项目工具授权范围','补充资源队列等待原因','实现模型预算预留核对','完善交接资料与候选引用','检查任务依赖与责任归属','优化移动端任务详情','登记独立测试覆盖要求'];
 while(fixture.tasks.length<125){
  const i=fixture.tasks.length,title=i===0?'分页之外 · 唯一检索 %_!':names[i%names.length]+' · '+String(i).padStart(3,'0');
  fixture.tasks.push(await create('tasks',{title,description:i===0?'此样例最先创建，用于验证全项目检索可以命中默认首100条之外的记录。':'合成验收资料。按正式任务合同记录需求、负责人及业务状态，不代表真实执行或完成。',status:['todo','in_progress','blocked','in_review'][i%4],priority:['high','medium','urgent','low'][i%4],...(i%3?{owner_id:fixture.owner.id}:{}),acceptance_criteria:['样例标准：来源、权限及操作结果均可核对。']}));
  fs.writeFileSync(fixtureFile,JSON.stringify(fixture));
 }
 if(!fixture.second){fixture.second=await create('projects',{title:'第二项目 · 隔离样例',description:'仅验证视图作用域'});fs.writeFileSync(fixtureFile,JSON.stringify(fixture));}
 const query=async(params={})=>{const res=await api.get('/api/v1/task-workspace?'+new URLSearchParams({project_id:project,...params}));assert.equal(res.status(),200);return res.json();};
 const first=await query();check('Actual SQLite total125, first page100',first.total===125&&first.items.length===100&&!!first.next_cursor);
 check('Oldest task is outside default first page',!first.items.some(t=>t.id===fixture.tasks[0].id));
 const tail=await query({q:'%_!'});check('Literal search finds task beyond first100',tail.total===1&&tail.items[0].id===fixture.tasks[0].id);
 const rest=await query({cursor:first.next_cursor});check('Second page25 without duplicate IDs',rest.items.length===25&&new Set([...first.items,...rest.items].map(t=>t.id)).size===125);
 if(process.argv.includes('--seed-only')){console.log(JSON.stringify({checks:checkList,project}));await api.dispose();return;}
 const browser=await chromium.launch({headless:true,executablePath:process.env.SUMA_CHROME_PATH||undefined});
 const context=await browser.newContext({storageState:await api.storageState(),viewport:{width:1440,height:1000},locale:'zh-CN',reducedMotion:'reduce'});
 await context.addInitScript(()=>{if(!localStorage.getItem('suma-locale-v1'))localStorage.setItem('suma-locale-v1','zh-CN');});
 const page=await context.newPage();page.setDefaultTimeout(15000);
 page.on('pageerror',e=>errors.push(e.message));page.on('request',r=>{if(/^https?:/.test(r.url())&&!r.url().startsWith(base+'/'))external.push(r.url());});
 const route='/?'+new URLSearchParams({project,view:'development',sub:'tasks'});
 const ready=()=>page.waitForFunction(()=>document.querySelector('.task-views')?.getAttribute('aria-busy')==='false'&&document.querySelector('[data-task-id]'));
 const shot=async name=>{await page.evaluate(()=>{document.activeElement?.blur();scrollTo(0,0);});await page.screenshot({path:path.join(out,name+'.png'),fullPage:false});};
 try{
  await page.goto(base+route);await ready();await shot('desktop-zh-list');check('Actual App loads100 task rows',await page.locator('[data-task-id]').count()===100);
  await page.locator('.pagination-footer button').click();await page.waitForFunction(()=>document.querySelectorAll('[data-task-id]').length===125);check('Load more retains original page',await page.locator('[data-task-id]').count()===125);
  await page.getByLabel('搜索任务',{exact:true}).fill('%_!');await page.waitForFunction(()=>document.querySelectorAll('[data-task-id]').length===1);check('UI search uses full project',await page.locator('[data-task-id]').getAttribute('data-task-id')===fixture.tasks[0].id);
  await page.getByRole('button',{name:'看板视图',exact:true}).click();check('Board shares query and selected membership',await page.locator('.task-views-card').count()===1);
  await page.getByRole('button',{name:'保存视图',exact:true}).click();await page.getByLabel('视图名称',{exact:true}).fill('分页检索 · 个人');await page.getByRole('dialog').getByRole('button',{name:'保存',exact:true}).click();
  await page.getByRole('button',{name:'清除筛选',exact:true}).first().click();await ready();
  await page.getByLabel('已保存视图',{exact:true}).selectOption({label:'分页检索 · 个人'});await page.waitForFunction(()=>document.querySelectorAll('[data-task-id]').length===1);check('Saved view restores full filter and board',await page.getByLabel('搜索任务',{exact:true}).inputValue()==='%_!'&&await page.locator('.task-views-card').count()===1);
  await page.locator('[data-task-id] .row-title').click();await page.waitForURL(/record=/);await shot('desktop-zh-detail');check('Task detail opens formal object',new URL(page.url()).searchParams.get('record')===fixture.tasks[0].id);await page.goBack();await ready();check('Back restores search',await page.getByLabel('搜索任务',{exact:true}).inputValue()==='%_!');
  await page.reload();await ready();await page.getByLabel('已保存视图',{exact:true}).selectOption({label:'分页检索 · 个人'});check('Named view survives refresh',await page.getByLabel('搜索任务',{exact:true}).inputValue()==='%_!');
  await page.getByRole('button',{name:'删除视图「分页检索 · 个人」',exact:true}).click();check('Delete asks confirmation',await page.getByRole('dialog',{name:'删除保存视图'}).isVisible());await page.getByRole('dialog').getByRole('button',{name:'取消',exact:true}).click();
  const other=await context.newPage();await other.goto(base+route);await other.locator('.task-views').waitFor();await other.getByRole('button',{name:'保存视图',exact:true}).click();await other.getByLabel('视图名称',{exact:true}).fill('其他标签 · 全部');await other.getByRole('dialog').getByRole('button',{name:'保存',exact:true}).click();await page.getByRole('button',{name:'载入最新视图列表',exact:true}).waitFor();check('Cross-tab change keeps active filter',await page.getByLabel('搜索任务',{exact:true}).inputValue()==='%_!');await page.getByRole('button',{name:'载入最新视图列表',exact:true}).click();await other.close();
  await page.goto(base+'/?'+new URLSearchParams({project:fixture.second.id,view:'development',sub:'tasks'}));await page.locator('.task-views').waitFor();check('Named views isolated by project',await page.getByLabel('已保存视图',{exact:true}).locator('option').count()===1);
  await page.goto(base+route);await ready();await page.getByLabel('任务业务状态',{exact:true}).selectOption('blocked');await page.waitForFunction(()=>document.querySelectorAll('[data-task-id]').length===31);check('Business status filter uses complete count',await page.locator('[data-task-id]').count()===31);
  await page.getByRole('button',{name:'看板视图',exact:true}).click();await page.getByLabel('分组',{exact:true}).selectOption('owner');await shot('desktop-zh-light');
  const narrow=await page.evaluate(()=>{localStorage.setItem('suma-theme','dark');return true;});await page.reload();await ready();await shot('desktop-zh-dark');check('Dark theme applied',narrow&&await page.locator('html').getAttribute('data-theme')==='dark');
  await page.setViewportSize({width:390,height:844});await shot('mobile-zh-dark');const mobileCard=await page.locator('.task-views-card .row-title').first().boundingBox();check('Mobile first task title remains in first screen',!!mobileCard&&mobileCard.y<844);await page.locator('.task-views-controls-toggle').click();check('Mobile filters expand explicitly',await page.getByLabel('搜索任务',{exact:true}).isVisible());await page.locator('.task-views-controls-toggle').click();check('Mobile page has no horizontal overflow',await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1));
  await page.evaluate(()=>{localStorage.setItem('suma-locale-v1','en-US');localStorage.setItem('suma-theme','light');});await page.reload();await ready();await shot('mobile-en-light');check('English renders localized task controls',await page.getByLabel('Search tasks',{exact:true}).count()===1);check('English mobile page has no horizontal overflow',await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1));
  await page.setViewportSize({width:1440,height:1000});await shot('desktop-en-light');
  const mineResponse=await api.get('/api/v1/my-work?'+new URLSearchParams({project_id:project}));assert.equal(mineResponse.status(),200);const mine=await mineResponse.json();check('Integrated personal scope uses canonical resource',mine.resource_id===fixture.owner.id&&mine.total===83);
  await page.keyboard.press('Control+k');await page.getByRole('dialog').getByRole('combobox',{name:'Search pages and project content',exact:true}).fill('My work');await page.locator('.command-palette-option').filter({has:page.getByText('My work',{exact:true})}).first().click();await page.locator('.my-work-list').waitFor();check('Command palette opens retained personal workspace',new URL(page.url()).searchParams.get('sub')==='my-work');await shot('integrated-my-work');
  await page.keyboard.press('Control+k');await page.getByRole('dialog').getByRole('combobox',{name:'Search pages and project content',exact:true}).fill('Tasks');await shot('integrated-command-palette');await page.locator('.command-palette-option').filter({has:page.getByText('Tasks',{exact:true})}).first().click();await ready();check('Command navigation opens task views without stale filters',new URL(page.url()).searchParams.get('sub')==='tasks'&&await page.getByLabel('Search tasks',{exact:true}).inputValue()===''&&await page.locator('[data-task-id]').count()===100);
  check('No browser runtime errors',errors.length===0);check('No external browser requests',external.length===0);
  const summary={stage:'T-MULTICA-137',date:new Date().toISOString(),kind:'actual App + Go HTTP + SQLite; synthetic owned records',checks:checkList,errors,external,screenshots:fs.readdirSync(out).filter(n=>n.endsWith('.png')&&n!=='failure.png')};
  fs.writeFileSync(path.join(out,'browser-summary.json'),JSON.stringify(summary,null,2)+'\n');console.log(JSON.stringify(summary));
 }catch(error){await page.screenshot({path:path.join(out,'failure.png'),fullPage:false});console.error(await page.locator('body').innerText().then(t=>t.slice(0,4500)));throw error;}finally{await browser.close();await api.dispose();}
})().catch(e=>{console.error(e);process.exitCode=1;});
