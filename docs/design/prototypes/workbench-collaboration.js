/* Clickable design sample. All records and decisions are synthetic, memory-only. */
(function () {
  'use strict';
  const P = window.SumaPrototype;
  if (!P) throw new Error('Collaboration prototype requires the workbench shell API.');
  const {icon, button, pill, section, heading, escapeHTML: esc} = P;
  const copy = value => structuredClone(value);
  const projects = [{id:'commerce',name:'星云商业平台'},{id:'insight',name:'洞察数据中台'},{id:'service',name:'客户服务助手'}];
  const identity = {human:{name:'人类',icon:'doc',hint:'对目标负责、作出正式审阅决定',unit:'小时'},agent:{name:'Agent',icon:'spark',hint:'在既定模型、工具与预算内执行',unit:'并发槽位'},expert:{name:'专家',icon:'layers',hint:'提供专业分析与协作计划',unit:'审阅窗口'},tool:{name:'工具',icon:'plug',hint:'提供受限能力，不承担业务责任',unit:'工具槽位'}};
  const members = [
    {id:'lin',name:'林清',kind:'human',role:'产品与交付负责人',capacity:32,context:'工作时间已设置；正式审批仍由当前授权确认。'},
    {id:'zhou',name:'周弈',kind:'human',role:'后端开发',capacity:32,context:'本周可用 32 小时；仅对已有执行估时汇总。'},
    {id:'chen',name:'陈然',kind:'human',role:'测试与质量',capacity:24,context:'本周可用 24 小时；审核工作尚未单独估时。'},
    {id:'native',name:'开发 Agent',kind:'agent',role:'原生代码执行',capacity:2,context:'示例并发上限 2；模型授权、费用额度与任务门禁分别确认。'},
    {id:'architect',name:'架构专家组',kind:'expert',role:'方案与边界审阅',capacity:null,context:'尚未约定可用审阅窗口，不能按空闲资源推荐。'},
    {id:'runner',name:'项目构建工具',kind:'tool',role:'独立检查与打包',capacity:2,context:'示例工具槽位上限 2；登记能力不代表当前可用性。'}
  ];
  const taskSeed = [
    {id:'TASK-104',project:'commerce',title:'订单权限与数据范围',owner:'lin',executor:'native',reviewer:'chen',tool:'runner',hours:null,week:'current',status:'执行中',active:true},
    {id:'TASK-108',project:'commerce',title:'支付异常恢复',owner:'lin',executor:'zhou',reviewer:'chen',tool:'runner',hours:24,week:'current',status:'待分配确认',active:false},
    {id:'TASK-112',project:'commerce',title:'订单审核交互',owner:'lin',executor:'zhou',reviewer:'chen',tool:'',hours:20,week:'current',status:'待分配确认',active:false},
    {id:'TASK-118',project:'commerce',title:'支付回归与异常用例',owner:'chen',executor:'',reviewer:'lin',tool:'runner',hours:16,week:'next',status:'等待交接',active:false},
    {id:'TASK-160',project:'insight',title:'数据边界与审计方案',owner:'lin',executor:'architect',reviewer:'chen',tool:'',hours:null,week:'current',status:'容量未设置',active:false},
    {id:'TASK-170',project:'service',title:'服务答复来源核验',owner:'chen',executor:'',reviewer:'lin',tool:'',hours:null,week:'next',status:'待澄清',active:false}
  ];
  const ruleSeed = {template:'repair',trigger:'execution.failed',project:'commerce',severity:'warning',action:'repair_draft',approval:'required',maxAttempts:1};
  const events = [
    {id:'EVT-201',at:'10:08:12',project:'commerce',kind:'execution.failed',severity:'critical',title:'支付恢复检查报告失败',source:'TASK-108 / attempt 1',operation:'OP-PAY-01',dedupe:'commerce:pay-check:1',failure:'payment-timeout',effect:'known',detail:'示例检查报告：支付结果读取超时。任务验收状态没有因此改变。'},
    {id:'EVT-202',at:'10:08:15',project:'commerce',kind:'execution.failed',severity:'critical',title:'同一事件再次送达',source:'TASK-108 / attempt 1',operation:'OP-PAY-01',dedupe:'commerce:pay-check:1',failure:'payment-timeout',effect:'known',detail:'与 EVT-201 有相同项目、来源与幂等键，应返回原处理结果。'},
    {id:'EVT-203',at:'10:13:42',project:'commerce',kind:'execution.failed',severity:'critical',title:'第二次报告相同失败',source:'TASK-108 / attempt 2',operation:'OP-PAY-02',dedupe:'commerce:pay-check:2',failure:'payment-timeout',effect:'known',detail:'第二个示例 attempt 仍报告 payment-timeout；连续相同失败达到两次，应停止自动尝试。'},
    {id:'EVT-204',at:'10:16:20',project:'commerce',kind:'external.unknown',severity:'critical',title:'远端提交回执未收到',source:'TASK-118 / 原提交',operation:'OP-REMOTE-44',dedupe:'commerce:remote:44',failure:'',effect:'unknown',detail:'无法判断提供方是否已创建任务。只能按原操作查询，不能自动重试创建。'},
    {id:'EVT-205',at:'10:20:03',project:'insight',kind:'artifact.sealed',severity:'info',title:'分析服务候选制品已封存',source:'APP-INSIGHT / candidate 7',operation:'OP-ART-07',dedupe:'insight:artifact:7',failure:'',effect:'known',detail:'仅说明示例候选制品已封存，不代表测试、审批或部署完成。'},
    {id:'EVT-206',at:'10:23:07',project:'commerce',kind:'artifact.sealed',severity:'info',title:'交易 API 候选制品已封存',source:'APP-PAY / candidate 3',operation:'OP-ART-03',dedupe:'commerce:artifact:3',failure:'',effect:'known',detail:'可形成发布准备预览，仍需精确版本、独立检查和逐项审批。'}
  ];
  const s = P.state.collaboration = {view:'assignments',project:'all',member:'all',tasks:copy(taskSeed),saved:copy(taskSeed),revision:1,wizard:'reassign',handoffTarget:'native',handoffNote:'补齐支付失败与未知效果的回归用例；独立测试与正式验收仍由原流程完成。',handoffs:[],rule:copy(ruleSeed),savedRule:copy(ruleSeed),ruleRevision:1,previewed:false,decisions:[],seen:{},failures:{},attempts:{},reconciliation:{},eventFilter:'all'};
  const memberName = id => members.find(item=>item.id===id)?.name || '未分配';
  const projectName = id => projects.find(item=>item.id===id)?.name || '全部示例项目';
  const names = {owner:'负责人',executor:'执行者',reviewer:'审阅者',week:'计划周',current:'本周',next:'下周'};
  const kindNames = {'execution.failed':'执行失败','artifact.sealed':'制品封存','external.unknown':'外部效果未知'};
  const actionNames = {repair_draft:'形成修复任务草稿',release_preview:'形成发布准备预览',reconcile:'仅查询原操作'};
  const severities = {info:0,warning:1,critical:2};
  const selected = (a,b) => a===b?' selected':'';
  const attrs = (key,value) => `data-${key}="${esc(value)}"`;
  function fieldSelect(label, field, value, options, extra='') {
    return `<label class="collab-field"><span>${esc(label)}</span><select data-collab-field="${field}" aria-label="${esc(label)}" ${extra}>${options.map(([id,text])=>`<option value="${esc(id)}"${selected(id,value)}>${esc(text)}</option>`).join('')}</select></label>`;
  }
  function taskChanges() {
    return s.tasks.flatMap(task=>{const saved=s.saved.find(item=>item.id===task.id);return ['owner','executor','reviewer','week'].filter(key=>task[key]!==saved[key]).map(key=>({task,key,before:saved[key],after:task[key]}));});
  }
  function assignmentConflicts() {return s.tasks.filter(task=>task.executor&&task.executor===task.reviewer);}
  function assignmentConflictNotice() {const conflicts=assignmentConflicts();return conflicts.length?`<div class="callout amber collab-spaced" role="alert"><strong>独立审阅冲突 · 暂不能确认保存</strong><ul class="collab-bullets">${conflicts.map(task=>`<li>${esc(task.id+' · '+task.title)}：${esc(memberName(task.executor))}同时担任执行者和审阅者。</li>`).join('')}</ul><p>请更换审阅者；当前修改继续保留在草稿中，不会开始执行。</p></div>`:'';}
  function renderKeepingFocus(element) {
    const {collabField,task,role,event}=element.dataset;P.render();
    const matches=[...document.querySelectorAll('[data-collab-field]')].filter(node=>node.dataset.collabField===collabField&&node.dataset.task===task&&node.dataset.role===role&&node.dataset.event===event&&!node.disabled);
    (matches.find(node=>node.getClientRects().length)||matches[0])?.focus({preventScroll:true});
  }
  function shownTasks() {return s.tasks.filter(task=>(s.project==='all'||task.project===s.project)&&(s.member==='all'||[task.owner,task.executor,task.reviewer,task.tool].includes(s.member)));}
  function roleField(task,key) {
    const eligible=members.filter(member=>key==='executor'?member.kind!=='tool':member.kind==='human');
    return `<select data-collab-field="assignment" data-task="${task.id}" data-role="${key}" aria-label="${esc(task.title+' · '+names[key])}" ${task.active&&key==='executor'?'disabled':''}><option value="">未分配</option>${eligible.map(member=>`<option value="${member.id}"${selected(member.id,task[key])}>${esc(member.name)} · ${identity[member.kind].name}</option>`).join('')}</select>${task.active&&key==='executor'?'<small>已开始执行，当前执行者固定</small>':''}`;
  }
  function teamFilters() {
    return `<div class="toolbar collab-filters">${fieldSelect('项目范围','project',s.project,[['all','全部示例项目'],...projects.map(item=>[item.id,item.name])])}${fieldSelect('成员 / 资源','member',s.member,[['all','全部身份'],...members.map(item=>[item.id,`${item.name} · ${identity[item.kind].name}`])])}<span class="spacer"></span><div class="segmented" aria-label="团队工作视图">${button('分工表','collab-team-view','',`data-view="assignments" aria-pressed="${s.view==='assignments'}"`)}${button('负载视图','collab-team-view','',`data-view="load" aria-pressed="${s.view==='load'}"`)}</div></div>`;
  }
  function assignmentTable() {
    const tasks=shownTasks();
    if(!tasks.length)return empty('当前筛选没有任务','该成员在选定项目暂无分工；这不代表成员空闲，也不会扩大项目访问范围。',button('清除筛选','collab-reset-filters'));
    const rows=tasks.map(task=>`<tr><td>${button(esc(task.title),'collab-task-detail','row-link',attrs('task',task.id))}<small>${task.id} · ${projectName(task.project)}</small></td><td>${roleField(task,'owner')}</td><td>${roleField(task,'executor')}</td><td>${roleField(task,'reviewer')}</td><td>${pill(task.status,task.active?'purple':task.executor?'':'amber')}<small>${names[task.week]} · ${task.hours===null?'估时未设置':task.hours+'h 执行估时'}</small></td></tr>`).join('');
    const cards=tasks.map(task=>`<article class="collab-assignment-card"><div>${button(esc(task.title),'collab-task-detail','row-link',attrs('task',task.id))}${pill(task.status,task.active?'purple':'')}</div><small>${task.id} · ${projectName(task.project)} · ${names[task.week]}</small><div class="collab-role-fields">${['owner','executor','reviewer'].map(key=>`<label><span>${names[key]}</span>${roleField(task,key)}</label>`).join('')}</div></article>`).join('');
    return `<div class="table-scroll collab-assignment-table"><table><thead><tr><th>任务 / 项目</th><th>负责人</th><th>执行者</th><th>审阅者</th><th>当前依据</th></tr></thead><tbody>${rows}</tbody></table></div><div class="collab-assignment-cards">${cards}</div>`;
  }
  function loadFor(member,tasks) {
    const owned=tasks.filter(task=>task.week==='current'&&(member.kind==='tool'?task.tool===member.id:task.executor===member.id));
    const unknown=member.capacity===null||(member.kind==='human'&&owned.some(task=>task.hours===null));
    const used=member.kind==='human'?owned.reduce((sum,task)=>sum+(task.hours||0),0):owned.length;
    return {owned,used,unknown,over:!unknown&&used>member.capacity};
  }
  function loadView() {
    const tasks=shownTasks(),visible=members.filter(member=>s.member==='all'||s.member===member.id);
    if(!tasks.length)return empty('当前筛选没有可汇总的分工','容量和实际占用是不同事实；没有任务不能证明资源可立即执行。',button('清除筛选','collab-reset-filters'));
    return `<div class="collab-load-list">${visible.map(member=>{const load=loadFor(member,tasks),type=identity[member.kind],ratio=load.unknown?0:Math.min(100,load.used/member.capacity*100);return `<article class="collab-load-row"><div class="collab-person">${icon(type.icon)}<div>${button(esc(member.name),'collab-member-detail','row-link',attrs('member',member.id))}<small>${type.name} · ${member.role}</small></div></div><div><div class="collab-load-heading"><strong>${load.unknown?'未设置容量':`${load.used} / ${member.capacity} ${type.unit}`}</strong>${pill(load.unknown?'未知':load.over?'计划超配':'计划范围内',load.unknown?'unknown':load.over?'amber':'blue')}</div><div class="collab-load-track ${load.over?'collab-over':''} ${load.unknown?'collab-unset':''}" aria-hidden="true"><span style="width:${ratio}%"></span></div><small>${member.kind==='human'?'执行估时，不含未估算的审阅工作':member.kind==='tool'?'任务工具需求，不代表工具在线或可用':member.kind==='agent'?'任务并发需求；不是费用额度':'专业协作窗口尚待约定'}</small></div><div class="collab-load-tasks">${load.owned.length?load.owned.map(task=>button(task.id,'collab-task-detail','row-link',attrs('task',task.id))).join(''):'<small>暂无本周执行分配</small>'}</div></article>`;}).join('')}</div><div class="section-foot">筛选范围内 · 本周示例计划。人工小时、Agent 槽位、专家窗口和工具槽位分别显示，不能相加。</div>`;
  }
  function empty(title,description,action='') {return `<div class="empty collab-empty">${icon('search')}<h3>${esc(title)}</h3><p>${esc(description)}</p>${action}</div>`;}
  function draftBar() {
    const changes=taskChanges();
    return `<div class="draft-bar collab-draft-bar" aria-live="polite"><div><strong>${changes.length?`${changes.length} 项分工调整尚未保存`:`分工样稿 · 修订 ${s.revision}`}</strong><small>${changes.length?'分工和负载已显示同一份草稿；修改负责人不启动执行，也不授予访问。':'仅保存在当前页面内存；刷新页面将恢复示例。'}</small>${assignmentConflicts().length?pill('独立审阅冲突 · 请查看差异','red'):''}</div><span class="spacer"></span>${changes.length?button('查看差异','collab-assignment-diff')+button('撤销全部','collab-assignment-undo')+button('保存分工草稿','collab-assignment-save','primary'):button('查看交接卡','collab-handoff')}</div>`;
  }
  function teams() {
    const conflicts=loadFor(members.find(item=>item.id==='zhou'),s.tasks).over;
    return `<div class="collab-root">${heading('teams','团队与分工','从职责到执行条件：明确由谁负责、谁来执行、谁来审阅。所有内容为本地示例。',button(`${icon('layers')}分配向导`,'collab-allocation','primary'))}<div class="collab-identity-grid">${Object.entries(identity).map(([kind,type])=>`<div class="collab-identity ${kind==='agent'?'collab-agent':''}">${icon(type.icon)}<div><strong>${type.name}</strong><p>${type.hint}</p></div></div>`).join('')}</div>${teamFilters()}<div class="collab-team-layout"><div>${section(s.view==='load'?'本周计划负载':'任务职责与分工',s.view==='load'?loadView():assignmentTable(),`${projectName(s.project)} · ${shownTasks().length} 项匹配任务 · 筛选与草稿在两个视图间共享`)}${draftBar()}</div><aside class="collab-team-aside">${section(conflicts?'需要协调的资源冲突':'分配调整已进入草稿',`<div class="section-body"><div class="collab-conflict-value">${pill(conflicts?'周弈 · 本周超配':'等待确认',conflicts?'amber':'blue')}<strong>${loadFor(members[1],s.tasks).used}<small> / 32h</small></strong></div><p class="prose">${conflicts?'原分配的两项执行估时共 44h，超过周弈 32h 可用容量。':'当前草稿中周弈的已知本周执行估时已低于容量；还需核对审阅工作与接手条件。'}</p><p class="collab-note">建议将审核交互交给有余量的成员，或移至下周。向导只形成分工草稿。</p>${button('解释与调整','collab-allocation','full-width')}</div>`,'合成示例，不是实时人员排班')}${section('交接需要固定依据',`<div class="section-body"><div class="collab-handoff-people"><span>陈然</span>${icon('arrow')}<span>${memberName(s.handoffTarget)}</span></div><h3>支付回归与异常用例</h3><p class="collab-note">TASK-118 · 原需求 REQ-028 · 用例计划 TP-12。交接目标、已有证据与未知项一并确认。</p>${pill(s.handoffs.length?'已保存本地交接草稿':'等待交接',s.handoffs.length?'blue':'amber')}${button('打开交接卡','collab-handoff','full-width collab-spaced')}</div>`)}${section('尚未确认的条件',`<div class="section-body collab-unknown-list"><p>${pill('专家窗口未知','unknown')}架构专家组没有可用容量，不按零负载推荐。</p><p>${pill('工具可用性未验证','unknown')}已登记槽位不代表有可运行的 Worker。</p><p>${icon('lock')}团队身份与任务分工不改变项目访问权限。</p></div>`)}</aside></div></div>`;
  }
  function describeChange(change) {return change.key==='week'?names[change.before]+' → '+names[change.after]:memberName(change.before)+' → '+memberName(change.after);}
  function diffBody() {const changes=taskChanges();return `<p class="prose">核对这 ${changes.length} 项本地调整。保存不会发起真实任务，也不会变更项目访问、模型预算或审批权限。</p>${assignmentConflictNotice()}<div class="collab-diff-list">${changes.map(change=>`<div><strong>${esc(change.task.title)}</strong><small>${change.task.id} · ${names[change.key]}</small><p>${esc(describeChange(change))}</p></div>`).join('')}</div>`;}
  function assignmentDiff(save) {
    if(!taskChanges().length){P.toast('暂无需要保存的分工调整。');return;}
    P.showModal(save?'保存分工草稿':'分工差异',diffBody(),button('继续编辑','collab-close')+(save?button('确认保存到本地样稿','collab-assignment-confirm','primary',assignmentConflicts().length?'disabled':''):button('保存草稿','collab-assignment-save','primary',assignmentConflicts().length?'disabled':'')));
  }
  function allocation(preview=false) {
    const target=s.tasks.find(item=>item.id==='TASK-112');
    P.showModal(preview?'预览分配调整':'协调本周资源冲突',`<div class="collab-step-label">${preview?'2 / 2 · 预览与采用':'1 / 2 · 选择调整方式'}</div><div class="callout amber">周弈：44h 计划 / 32h 可用。超出 12h；当前任务尚未开始，可形成新的分工草稿。</div>${preview?`<div class="diff-row"><div class="diff-side"><small>当前草稿</small><p>${esc(memberName(target.executor))} · ${names[target.week]}</p></div><div class="diff-side new"><small>拟调整 TASK-112</small><p>${s.wizard==='reassign'?'陈然执行 · 林清审阅 · 本周':'周弈执行 · 陈然审阅 · 下周'}</p></div></div><p class="prose">${s.wizard==='reassign'?'周弈本周执行估时降至 24h；陈然增加 20h / 24h，并由林清审阅以避免自审。审阅工作尚未独立估时，需人工确认。':'周弈本周执行估时降至 24h；下周增加 20h。未修改里程碑或其他任务日期。'}</p>`:`${fieldSelect('调整方案','allocation',s.wizard,[['reassign','由陈然执行订单审核交互'],['defer','将订单审核交互移至下周']])}<dl class="collab-spaced"><dt>责任边界</dt><dd>负责人林清不变；若陈然接手执行，则改由林清审阅。</dd><dt>执行边界</dt><dd>尚未开始，采用仅修改草稿。</dd><dt>来源范围</dt><dd>当前已授权示例项目；不申请额外资源权限。</dd></dl>`}<p class="collab-note">容量调整不是执行许可；正式派发仍需工具、模型、预算与独立审阅条件。</p>`,button(preview?'返回选择':'取消',preview?'collab-allocation':'collab-close')+button(preview?'采用到分工草稿':'预览调整',preview?'collab-allocation-adopt':'collab-allocation-preview','primary'));
  }
  function handoff() {
    P.showDetail('交接卡 · TASK-118','本地示例草稿 · 保存与开始执行是不同动作',`<section class="detail-section"><h3>交接依据</h3><dl><dt>任务</dt><dd>支付回归与异常用例</dd><dt>原需求</dt><dd>REQ-028 · 修订 3</dd><dt>用例计划</dt><dd>TP-12 · 固定候选验收边界</dd><dt>当前状态</dt><dd>等待交接；没有由此创建执行</dd></dl></section><section class="detail-section">${fieldSelect('拟接手执行者','handoff-target',s.handoffTarget,members.filter(member=>member.kind!=='tool').map(member=>[member.id,member.name+' · '+identity[member.kind].name]))}<label class="collab-field collab-spaced"><span>交接目标与未知项</span><textarea rows="5" data-collab-field="handoff-note" maxlength="600">${esc(s.handoffNote)}</textarea></label></section><section class="detail-section"><h3>开始前仍需确认</h3><ul class="collab-bullets"><li>明确原候选与测试范围；补齐异常用例。</li><li>检查拟接手者的工具、模型与项目范围。</li><li>回执不确定时先核对原交接，不重复派发。</li></ul>${s.handoffs.length?pill('本地交接草稿已保存','blue'):pill('尚未保存','unknown')}</section>`,button('保存本地交接草稿','collab-handoff-save','primary'));
  }
  function taskDetail(id) {
    const task=s.tasks.find(item=>item.id===id);if(!task)return;
    P.showDetail(task.title,`${task.id} · ${projectName(task.project)} · 样稿数据`,`<section class="detail-section"><dl><dt>负责人</dt><dd>${memberName(task.owner)}</dd><dt>执行者</dt><dd>${memberName(task.executor)}</dd><dt>审阅者</dt><dd>${memberName(task.reviewer)}</dd><dt>关联工具</dt><dd>${task.tool?memberName(task.tool):'未指定'}</dd><dt>执行估时</dt><dd>${task.hours===null?'未设置':task.hours+' 小时'}</dd><dt>计划周</dt><dd>${names[task.week]}</dd></dl></section><section class="detail-section"><h3>职责与许可分开</h3><p>负责人协调目标与范围；执行者接收明确任务；审阅者核对独立证据。分工保存本身不代表审批通过或执行开始。</p></section>`,button('查看交接方式','collab-handoff'));
  }
  function memberDetail(id) {
    const member=members.find(item=>item.id===id);if(!member)return;const load=loadFor(member,shownTasks());
    P.showDetail(member.name,identity[member.kind].name+' · '+member.role,`<section class="detail-section"><dl><dt>身份类型</dt><dd>${identity[member.kind].name}</dd><dt>本周计划</dt><dd>${load.unknown?'容量未设置':`${load.used} / ${member.capacity} ${identity[member.kind].unit}`}</dd><dt>数据范围</dt><dd>${projectName(s.project)} · 当前成员筛选</dd></dl></section><section class="detail-section"><h3>使用边界</h3><p>${esc(member.context)}</p><p class="collab-note">这里只显示示例职责与计划。执行授权、外部可用性和正式人审资格仍需分别确认。</p></section>`);
  }
  function matchEvent(event) {return event.project===s.rule.project&&event.kind===s.rule.trigger&&severities[event.severity]>=severities[s.rule.severity];}
  function ruleErrors() {
    const errors=[];if(!Number.isInteger(Number(s.rule.maxAttempts))||Number(s.rule.maxAttempts)<1||Number(s.rule.maxAttempts)>3)errors.push('最大尝试次数需为 1–3 的整数。');
    if(s.rule.trigger==='external.unknown'&&s.rule.action!=='reconcile')errors.push('外部效果未知只能查询原操作；不能采用创建或发布动作。');
    if(s.rule.action==='release_preview'&&s.rule.approval!=='required')errors.push('发布准备预览须保留人工审批。');
    return errors;
  }
  function ruleChanged() {return JSON.stringify(s.rule)!==JSON.stringify(s.savedRule);}
  function ruleFacts(rule) {return `<dl class="collab-rule-facts"><dt>项目</dt><dd>${projectName(rule.project)}</dd><dt>触发</dt><dd>${kindNames[rule.trigger]}</dd><dt>门槛</dt><dd>${{info:'信息及以上',warning:'警告及以上',critical:'仅严重'}[rule.severity]}</dd><dt>动作</dt><dd>${actionNames[rule.action]}</dd><dt>审批</dt><dd>${rule.approval==='required'?'人工确认后才可继续':'仅形成草稿，不执行'}</dd><dt>最大尝试</dt><dd>${rule.maxAttempts} 次；同失败两次仍停止</dd></dl>`;}
  function ruleFields() {
    return `<div class="collab-rule-steps"><div><span class="collab-step-number">01</span>${fieldSelect('触发事件','rule-trigger',s.rule.trigger,Object.entries(kindNames))}</div><div><span class="collab-step-number">02</span>${fieldSelect('条件 · 最低严重程度','rule-severity',s.rule.severity,[['info','信息及以上'],['warning','警告及以上'],['critical','仅严重']])}</div><div><span class="collab-step-number">03</span>${fieldSelect('作用范围','rule-project',s.rule.project,projects.map(project=>[project.id,project.name]))}</div><div><span class="collab-step-number">04</span>${fieldSelect('拟议动作','rule-action',s.rule.action,Object.entries(actionNames))}</div><div><span class="collab-step-number">05</span>${fieldSelect('审批方式','rule-approval',s.rule.approval,[['required','人工确认后才可继续'],['draft_only','仅形成草稿，不执行']])}</div><div><span class="collab-step-number">06</span><label class="collab-field"><span>结果与恢复 · 最大尝试</span><input type="number" min="1" max="3" step="1" value="${esc(s.rule.maxAttempts)}" data-collab-field="rule-maxAttempts" aria-label="最大尝试次数"></label><small>相同失败连续 2 次仍立即停止</small></div></div>`;
  }
  function matchingEvents() {
    const matching=events.filter(matchEvent),errors=ruleErrors();
    return `<div class="collab-rule-preview" aria-live="polite">${errors.length?`<div class="callout amber"><strong>规则尚不能模拟采用</strong><ul class="collab-bullets">${errors.map(error=>`<li>${esc(error)}</li>`).join('')}</ul></div>`:`<div class="collab-preview-summary"><strong>${matching.length} 个示例事件匹配</strong>${pill('按当前草稿即时计算','blue')}</div>`}${matching.length?`<div class="collab-match-list">${matching.map(event=>`<button type="button" data-action="collab-event-detail" data-event="${event.id}"><span>${event.id}<small>${event.at} · ${projectName(event.project)}</small></span><strong>${esc(event.title)}</strong>${icon('arrow')}</button>`).join('')}</div>`:empty('没有示例事件匹配','调整触发事件、严重程度或项目范围后，可继续预览。零匹配不代表生产环境没有事件。')}${button('模拟匹配事件序列','collab-simulate-sequence','primary collab-spaced',errors.length||!matching.length?'disabled':'')}</div>`;
  }
  function eventRows() {
    const list=events.filter(event=>s.eventFilter==='all'||event.effect===s.eventFilter||event.kind===s.eventFilter);
    return `<div class="collab-event-list">${list.map(event=>{const result=[...s.decisions].reverse().find(item=>item.event===event.id);return `<article class="collab-event-row"><span class="collab-event-time">${event.at}<small>${event.id}</small></span><div>${button(esc(event.title),'collab-event-detail','row-link',attrs('event',event.id))}<small>${projectName(event.project)} · ${event.source}</small><span class="collab-event-meta">${pill(kindNames[event.kind],event.effect==='unknown'?'unknown':'')}${pill(matchEvent(event)?'匹配当前规则':'不匹配',matchEvent(event)?'purple':'')}${result?pill(result.label,result.tone):''}</span></div>${button('模拟决策','collab-simulate-event','',attrs('event',event.id))}</article>`;}).join('')}</div>`;
  }
  function automation() {
    return `<div class="collab-root">${heading('automation','事件与自动化','先解释为什么匹配，再展示动作边界与恢复路径。这里只演示决策，不创建真实运行。',button(`${icon('branch')}查看模拟记录`,'collab-simulation-log'))}<div class="collab-automation-layout"><div>${section('从模板形成规则草稿',`<div class="section-body"><div class="collab-template-row">${[['repair','失败回流','执行失败 → 修复草稿'],['release','发布准备','制品封存 → 审批预览'],['reconcile','未知效果核对','回执未知 → 查询原操作']].map(([id,title,description])=>`<button type="button" class="collab-template" data-action="collab-template" data-template="${id}" aria-pressed="${s.rule.template===id}"><strong>${title}</strong><small>${description}</small></button>`).join('')}</div>${ruleFields()}<div class="collab-rule-footer"><span>${ruleChanged()?pill('规则草稿已修改','amber'):pill('本地样稿修订 '+s.ruleRevision,'blue')}</span>${button('撤销规则修改','collab-rule-undo','',ruleChanged()?'':'disabled')}${button('预览并保存规则','collab-rule-save','',ruleErrors().length?'disabled':'')}</div></div>`,'触发 → 条件 → 范围 → 动作 → 审批 → 结果；不会跨项目扩大范围')}${section('匹配预览',matchingEvents(),'真实匹配逻辑作用于固定示例事件；不代表已启用规则')}${section('事件序列',eventRows(),'接收、去重、决策与效果分别保留',fieldSelect('事件筛选','event-filter',s.eventFilter,[['all','全部示例事件'],['execution.failed','执行失败'],['unknown','未知效果'],['artifact.sealed','制品封存']]))}</div><aside class="collab-automation-aside">${section('始终保留的门禁',`<div class="section-body collab-gates"><div>${icon('lock')}<strong>同一事件只处理一次</strong><p>项目、来源与稳定幂等键共同定位原决策；重复送达返回原结果。</p></div><div>${icon('warning')}<strong>同一失败连续两次停止</strong><p>上限即使设为 3，也不会继续相同失败的第三次自动尝试。</p></div><div>${icon('search')}<strong>未知效果先核对</strong><p>没有“再创建一次”按钮，只按原操作查询。没有回执仍保持未知。</p></div></div>`)}${section('本地决策记录',simulationSummary(),'刷新页面会重置；并非运行日志',button('重置模拟','collab-simulation-reset'))}${section('试一条恢复路径',`<div class="section-body"><h3>远端提交回执丢失</h3><p class="collab-note">OP-REMOTE-44 可能已经产生外部效果。切换到核对模板，观察为何不能自动重试。</p>${button('查看未知事件','collab-event-detail','full-width','data-event="EVT-204"')}</div>`)}</aside></div></div>`;
  }
  function simulationSummary() {
    return s.decisions.length?`<div class="collab-decision-list">${s.decisions.slice(-4).reverse().map(item=>`<button type="button" data-action="collab-decision-detail" data-decision="${item.id}"><span>${item.event}<small>${item.operation}</small></span>${pill(item.label,item.tone)}</button>`).join('')}</div><div class="section-foot">累计 ${s.decisions.length} 次本地模拟 · 未发出请求、未启动执行</div>`:empty('尚未模拟决策','调整规则后，选择单个事件或完整序列。');
  }
  function evaluate(event) {
    const result={id:'SIM-'+String(s.decisions.length+1).padStart(3,'0'),event:event.id,operation:event.operation,label:'不匹配',tone:'',steps:[]};
    const add=(title,detail)=>result.steps.push({title,detail});
    add('接收示例事件',`${event.id} · ${kindNames[event.kind]} · ${projectName(event.project)}`);
    if(ruleErrors().length){result.label='规则待修正';result.tone='amber';add('规则校验未通过',ruleErrors().join(' '));}
    else if(!matchEvent(event)){add('范围或条件不匹配','当前规则不采用这个事件；不创建动作或改变业务状态。');}
    else if(s.seen[event.dedupe]){result.label='重复已去重';result.tone='blue';add('发现原幂等键',`${event.dedupe} 已对应 ${s.seen[event.dedupe]}，返回原模拟决策，不增加尝试。`);}
    else {
      s.seen[event.dedupe]=result.id;
      add('条件匹配',`${projectName(event.project)} · ${kindNames[s.rule.trigger]} · 严重程度达到当前门槛。`);
      add('幂等键已定位',event.dedupe+'；本次仅在内存中登记。');
      if(event.effect==='unknown'){
        result.label='仅可核对';result.tone='unknown';add('外部效果未知',`原操作 ${event.operation} 没有确定回执；禁止创建重试，只允许查询原操作。`);
      } else {
        const previous=s.failures[event.source.split(' / ')[0]],sameFailure=event.failure&&previous?.fingerprint===event.failure;
        const streak=event.failure?(sameFailure?previous.count+1:1):0;
        if(event.failure)s.failures[event.source.split(' / ')[0]]={fingerprint:event.failure,count:streak};
        const attemptKey=event.project+':'+event.source.split(' / ')[0],attempts=s.attempts[attemptKey]||0;
        if(streak>=2){result.label='相同失败已停止';result.tone='red';add('连续失败门禁',`${event.failure} 连续出现 ${streak} 次；停止后续自动尝试，转交人工检查。`);}
        else if(attempts>=Number(s.rule.maxAttempts)){result.label='尝试上限已停止';result.tone='amber';add('次数上限',`已形成 ${attempts} 次示例尝试决策，达到当前规则上限。`);}
        else {s.attempts[attemptKey]=attempts+1;result.label=s.rule.approval==='required'?'模拟待审批':'模拟草稿已形成';result.tone=s.rule.approval==='required'?'amber':'blue';add('动作预览',actionNames[s.rule.action]+'；仅形成当前模拟记录，不运行任务。');add('审批与业务边界',s.rule.approval==='required'?'等待具备当前权限的人明确确认；没有自动审批。':'仅形成草稿；没有创建执行、通过测试或正式验收。');}
      }
    }
    add('结果边界','这是一条本地模拟决策。没有服务请求、模型调用、真实派发、部署或消息发送。');s.decisions.push(result);return result;
  }
  function decisionDetail(result) {
    if(!result)return;
    P.showDetail(result.id+' · '+result.label,`${result.event} · ${result.operation} · 本地模拟`,`${pill(result.label,result.tone)}<ol class="collab-decision-steps">${result.steps.map(step=>`<li><strong>${esc(step.title)}</strong><p>${esc(step.detail)}</p></li>`).join('')}</ol>`,result.label==='仅可核对'?button('模拟查询原操作','collab-reconcile','',attrs('event',result.event)):button('查看事件原件','collab-event-detail','',attrs('event',result.event)));
  }
  function eventDetail(id) {
    const event=events.find(item=>item.id===id);if(!event)return;
    const result=[...s.decisions].reverse().find(item=>item.event===id);
    P.showDetail(event.title,`${event.id} · ${event.at} · 固定示例事件`,`<section class="detail-section"><dl><dt>事件类型</dt><dd>${kindNames[event.kind]}</dd><dt>项目范围</dt><dd>${projectName(event.project)}</dd><dt>来源</dt><dd>${event.source}</dd><dt>原操作</dt><dd><code>${event.operation}</code></dd><dt>幂等键</dt><dd><code>${event.dedupe}</code></dd><dt>外部效果</dt><dd>${pill(event.effect==='unknown'?'未知，先核对':'本事件已知',event.effect==='unknown'?'unknown':'blue')}</dd></dl></section><section class="detail-section"><h3>示例内容</h3><p>${esc(event.detail)}</p></section><section class="detail-section"><h3>当前规则结果</h3>${pill(matchEvent(event)?'匹配':'不匹配',matchEvent(event)?'purple':'')}<p class="collab-note">${result?'最新模拟：'+result.label:'尚未对本事件模拟决策。'}</p>${event.effect==='unknown'?'<div class="callout amber">效果未知时，不能通过改参数绕过核对后再次创建。</div>':''}</section>`,button('模拟当前决策','collab-simulate-event','',attrs('event',id))+(event.effect==='unknown'?button('模拟查询原操作','collab-reconcile','primary',attrs('event',id)):''));
  }
  function reconcile(id) {
    const event=events.find(item=>item.id===id);if(!event||event.effect!=='unknown')return;
    P.showModal('核对原操作 · '+event.operation,`<p class="prose">这里只演示查询后的处理路径，不访问提供方。原操作身份保持不变，不产生第二次创建。</p>${fieldSelect('选择模拟查询结果','reconcile-result',s.reconciliation[id]||'unknown',[['unknown','仍无确定回执'],['found','示例回执：原效果已发生'],['not_started','示例回执：确认未开始']],attrs('event',id))}<div class="callout amber collab-spaced">未知不能被当作失败重试；即使示例回执为“未开始”，真实重试仍需当前授权与原操作依据。</div>`,button('关闭','collab-close')+button('查看核对决策','collab-reconcile-preview','primary',attrs('event',id)));
  }
  function reconciliationPreview(id) {
    const event=events.find(item=>item.id===id);if(!event)return;const choice=s.reconciliation[id]||'unknown';
    P.showModal('核对决策 · 仅本地演示',`<div class="collab-reconcile-result">${pill(choice==='unknown'?'保持未知':choice==='found'?'示例：已有原效果':'示例：原操作未开始',choice==='unknown'?'unknown':'blue')}<h3>${choice==='unknown'?'继续保留原操作，等待证据':choice==='found'?'沿用原结果，不重复创建':'具备评估重试的依据，尚未重试'}</h3><p>${choice==='unknown'?'没有新增回执，不改变效果状态，也没有自动重试。':choice==='found'?'示例回执关联原操作；正式业务仍须核对当前来源与证据。':'需要再次确认当前授权、工具与预算；样稿到此结束，不发出执行。'}</p><code>${event.operation}</code></div>`,button('完成查看','collab-close','primary'));
  }
  function template(id) {
    s.rule=id==='release'?{template:id,trigger:'artifact.sealed',project:'commerce',severity:'info',action:'release_preview',approval:'required',maxAttempts:1}:id==='reconcile'?{template:id,trigger:'external.unknown',project:'commerce',severity:'warning',action:'reconcile',approval:'required',maxAttempts:1}:copy(ruleSeed);
    s.previewed=false;P.render();P.toast('模板已形成规则草稿，未保存或启用。');
  }
  const actions={
    'collab-team-view':el=>{s.view=el.dataset.view;P.render();},
    'collab-reset-filters':()=>{s.project='all';s.member='all';P.render();},
    'collab-task-detail':el=>taskDetail(el.dataset.task),
    'collab-member-detail':el=>memberDetail(el.dataset.member),
    'collab-assignment-diff':()=>assignmentDiff(false),
    'collab-assignment-save':()=>assignmentDiff(true),
    'collab-assignment-confirm':()=>{if(assignmentConflicts().length){P.toast('请先解决执行者兼任审阅者的冲突；草稿已保留。');return;}if(!taskChanges().length)return;s.saved=copy(s.tasks);s.revision++;P.closeModal();P.render();P.toast('分工草稿已保存到页面内存；没有启动执行或授予访问。');},
    'collab-assignment-undo':()=>{s.tasks=copy(s.saved);P.render();P.toast('已撤销尚未保存的分工调整。');},
    'collab-allocation':()=>allocation(),
    'collab-allocation-preview':()=>allocation(true),
    'collab-allocation-adopt':()=>{const target=s.tasks.find(task=>task.id==='TASK-112');if(target.active){P.toast('该任务已开始，不能更换执行依据。');return;}if(s.wizard==='reassign'){target.executor='chen';target.reviewer='lin';}else target.week='next';P.closeModal();P.render();P.toast('调整已加入分工草稿，尚未保存。');},
    'collab-handoff':()=>handoff(),
    'collab-handoff-save':()=>{if(!s.handoffNote.trim()){P.toast('请填写交接目标与未知项。');return;}s.handoffs=[{task:'TASK-118',to:s.handoffTarget,note:s.handoffNote,state:'draft'}];P.render();handoff();P.toast('交接草稿已保存在页面内存；没有派发、接手或发送消息。');},
    'collab-close':()=>P.closeModal(),
    'collab-template':el=>template(el.dataset.template),
    'collab-rule-undo':()=>{s.rule=copy(s.savedRule);P.render();},
    'collab-rule-save':()=>{if(ruleErrors().length){P.toast(ruleErrors()[0]);return;}P.showModal('保存规则草稿',`<p class="prose">核对完整范围与参数；当前匹配 ${events.filter(matchEvent).length} 个固定示例事件。</p><div class="diff-row"><div class="diff-side"><small>原样稿修订 ${s.ruleRevision}</small>${ruleFacts(s.savedRule)}</div><div class="diff-side new"><small>当前草稿</small>${ruleFacts(s.rule)}</div></div><div class="callout">仅保存在页面内存，不启用真实规则。审批、幂等、同失败两次停止和未知核对门禁始终保留。</div>`,button('继续编辑','collab-close')+button('确认保存本地草稿','collab-rule-confirm','primary'));},
    'collab-rule-confirm':()=>{if(ruleErrors().length)return;s.savedRule=copy(s.rule);s.ruleRevision++;P.closeModal();P.render();P.toast('规则草稿已保存在页面内存；未启用真实自动化。');},
    'collab-event-detail':el=>eventDetail(el.dataset.event),
    'collab-simulate-event':el=>{const event=events.find(item=>item.id===el.dataset.event);if(!event)return;const result=evaluate(event);P.render();decisionDetail(result);},
    'collab-simulate-sequence':()=>{if(ruleErrors().length)return;events.filter(matchEvent).forEach(evaluate);P.render();P.showModal('事件序列模拟完成',simulationSummary()+'<p class="collab-note">按固定示例顺序进行本地决策；没有执行动作、测试或审批。再次模拟将依据原幂等键去重。</p>',button('返回规则','collab-close','primary'));},
    'collab-simulation-log':()=>P.showDetail('本地模拟决策记录','仅本次页面会话 · 不是真实运行日志',s.decisions.length?`<div class="collab-decision-list">${[...s.decisions].reverse().map(item=>button(`${esc(item.id+' · '+item.event)} ${pill(item.label,item.tone)}`,'collab-decision-detail','',attrs('decision',item.id))).join('')}</div>`:empty('尚无模拟记录','从单个事件或匹配序列开始。')),
    'collab-decision-detail':el=>{P.closeModal();decisionDetail(s.decisions.find(item=>item.id===el.dataset.decision));},
    'collab-simulation-reset':()=>{s.decisions=[];s.seen={};s.failures={};s.attempts={};s.reconciliation={};P.render();P.toast('只重置本地模拟记录，规则草稿保留。');},
    'collab-reconcile':el=>reconcile(el.dataset.event),
    'collab-reconcile-preview':el=>reconciliationPreview(el.dataset.event)
  };
  Object.entries(actions).forEach(([name,handler])=>P.registerAction(name,handler));
  document.addEventListener('change',event=>{
    const el=event.target;if(!(el instanceof HTMLElement)||!el.matches('[data-collab-field]'))return;const field=el.dataset.collabField,value=el.value;
    if(field==='project'||field==='member'){s[field]=value;renderKeepingFocus(el);}
    else if(field==='assignment'){const task=s.tasks.find(item=>item.id===el.dataset.task),role=el.dataset.role,member=members.find(item=>item.id===value);if(!task||!['owner','executor','reviewer'].includes(role)||task.active&&role==='executor')return;if(value&&(!member||(role==='executor'?member.kind==='tool':member.kind!=='human')))return;task[role]=value;renderKeepingFocus(el);}
    else if(field==='allocation')s.wizard=value;
    else if(field==='handoff-target')s.handoffTarget=value;
    else if(field==='handoff-note')s.handoffNote=value;
    else if(field==='event-filter'){s.eventFilter=value;renderKeepingFocus(el);}
    else if(field==='reconcile-result')s.reconciliation[el.dataset.event]=value;
    else if(field.startsWith('rule-')){const key=field.slice(5);s.rule[key]=key==='maxAttempts'?Number(value):value;s.previewed=false;renderKeepingFocus(el);}
  });
  document.addEventListener('input',event=>{const el=event.target;if(el instanceof HTMLElement&&el.dataset.collabField==='handoff-note')s.handoffNote=el.value;});
  P.registerViews([{id:'teams',title:'团队与分工',eyebrow:'COLLABORATION',icon:'layers',render:teams},{id:'automation',title:'事件与自动化',eyebrow:'AUTOMATION',icon:'branch',render:automation}]);
}());
