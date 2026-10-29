/* Interaction design only. No persistence, model calls, provider requests or business mutations. */
(() => {
  'use strict';
  const S = window.SumaPrototype;
  const {button:b,pill,section,icon,escapeHTML:e} = S;
  const $ = selector => document.querySelector(selector);
  const data = {
    qualityTab:'coverage', configTab:'deployment', issueFilter:'all', selectedIssue:'ISS-032',
    issues:[
      {id:'ISS-032',title:'支付超时后出现重复操作入口',severity:'P1',owner:'林清',status:'待分诊',cycle:1,source:'AC-PAY-01 @ r2 · RUN-061',fix:null,regression:null,accepted:false,history:[]},
      {id:'ISS-035',title:'窄屏审核表单焦点返回错误',severity:'P2',owner:'周弈',status:'修复中',cycle:1,source:'AC-UI-02 @ r1 · 手工复现',fix:'FIX-035-1',regression:null,accepted:false,history:[]},
      {id:'ISS-038',title:'预发布健康探针响应中断',severity:'P1',owner:'陈然',status:'待核对',cycle:1,source:'OBS-011 · 交易 API / 预发布',fix:null,regression:null,accepted:false,history:[]}
    ],
    config:{budget:40,concurrency:2,offline:4}, savedConfig:{budget:40,concurrency:2,offline:4}, configRevision:3, simulatedConflict:false,
    syncState:'offline',syncResolution:null,syncRevision:2,idea:null,ideaDraft:'',ideaHistory:[],
  };
  S.state.delivery = data;
  const tabs=(items,selected,action,label)=>`<div class="toolbar work-tabs"><div class="segmented" role="group" aria-label="${label}">${items.map(([id,title])=>b(title,action,'',`data-id="${id}" aria-pressed="${id===selected}"`)).join('')}</div></div>`;
  function go(route){S.closeModal();S.closeDetail(false);if(location.hash==='#'+route)S.navigate(route);else location.hash=route;}
  function pathStrip(){return `<div class="work-source-strip"><div><span class="work-kicker">DELIVERY CONTEXT</span><strong>星云商业平台 <span>/</span> 支付恢复交付</strong></div><div class="metadata">${pill('REQ-028 · r3','purple')}${pill('候选 c7a29d8','')}<span>示例观察日 10 月 19 日</span></div></div>`;}
  function requirements(){
    const original=S.baseView('requirements');
    return original.replace('<div class="toolbar">',`<div class="work-intake"><div>${icon('spark')}<strong>从想法开始，保留原始问题</strong><small>原文、提议与正式需求分别记录</small></div>${data.idea?b('查看整理记录 · '+data.ideaHistory.length,'work-idea-record'):''}${b('录入想法并预览','work-idea')}</div><div class="toolbar">`);
  }
  function quality(){
    const nav=tabs([['coverage','标准与证据'],['issues','问题与修复'],['release','发布依赖'] ],data.qualityTab,'work-quality-tab','质量与交付视图');
    if(data.qualityTab==='coverage')return S.baseView('quality').replace('<section ',nav+'<section ');
    return S.heading('quality',data.qualityTab==='issues'?'把问题处理到可以验证':'按准确版本，审阅交付依赖','问题、修复、回归、验收与部署分别记录。此处为可操作演练，所有记录只在样稿内存中。',b(data.qualityTab==='issues'?'新增问题样稿':'查看发布前检查',data.qualityTab==='issues'?'work-new-issue':'release-review','primary'))+nav+pathStrip()+(data.qualityTab==='issues'?issuesView():releaseView());
  }
  function issuesView(){
    const list=data.issues.filter(issue=>data.issueFilter==='all'||issue.status===data.issueFilter);
    return `<div class="toolbar"><label>问题状态<select id="work-issue-filter"><option value="all">所有问题</option>${['待分诊','修复中','待核对','待验收','已关闭'].map(s=>`<option ${s===data.issueFilter?'selected':''}>${s}</option>`).join('')}</select></label><span class="spacer"></span><small>${list.length} 项 · 当前筛选范围</small></div><div class="work-issue-grid">${list.length?list.map(issue=>`<article class="work-issue"><div class="work-row">${pill(issue.severity,issue.severity==='P1'?'amber':'blue')}<span class="mono">${issue.id}</span>${pill(issue.status,issue.status==='已关闭'?'green':issue.status==='待核对'?'unknown':'purple')}</div><h2>${e(issue.title)}</h2><p>${e(issue.source)}</p><div class="work-mini-flow"><span>发现</span><i>→</i><span class="${issue.fix?'current':''}">修复</span><i>→</i><span class="${issue.regression?'current':''}">回归</span><i>→</i><span class="${issue.accepted?'current':''}">验收</span></div><div class="work-row"><small>负责人 ${e(issue.owner)} · 修复周期 ${issue.cycle}</small>${b('处理问题','work-issue-open','',`data-id="${issue.id}"`)}</div></article>`).join(''):'<div class="section empty"><h2>当前筛选没有问题</h2><p>可以查看其他状态，或记录新发现。</p>'+b('清除筛选','work-issue-clear')+'</div>'}</div>`;
  }
  function issueDetail(id){
    data.selectedIssue=id;const issue=data.issues.find(i=>i.id===id);
    S.closeDetail(false);
    (document.querySelector(`[data-action="work-issue-open"][data-id="${CSS.escape(id)}"]`)||$('#work-issue-filter'))?.focus({preventScroll:true});
    let next='';
    if(issue.status==='待核对')next=b('查看核对路径','work-issue-reconcile','primary');
    else if(issue.status==='已关闭')next=b('重新打开新周期','work-issue-reopen','primary');
    else if(!issue.fix)next=b('形成修复草稿','work-issue-fix','primary');
    else if(!issue.regression||issue.regression==='failed')next=b('载入示例回归结果','work-issue-test','primary');
    else next=b('审阅并关闭样稿问题','work-issue-review','primary');
    const events=issue.history.map(h=>`<li>${icon(h.kind==='accept'?'check':'doc')}<div>${e(h.text)}<small>样稿周期 ${h.cycle}</small></div></li>`).join('');
    S.showDetail(e(issue.title),`${issue.id} · 周期 ${issue.cycle} · ${issue.status}`,`<div class="detail-section"><dl><dt>原始依据</dt><dd>${e(issue.source)}</dd><dt>责任方</dt><dd>${e(issue.owner)}</dd><dt>修复草稿</dt><dd>${issue.fix||'尚未形成'}</dd><dt>回归结果</dt><dd>${issue.regression==='passed'?'示例独立证据通过':issue.regression==='failed'?'示例独立证据失败':'未载入，不推断通过'}</dd><dt>人工验收</dt><dd>${issue.accepted?'样稿已审阅':'待独立审阅'}</dd></dl></div><div class="callout">所有变更仅是本地演练。正式实现必须绑定准确候选、独立运行与当前权限。</div><div class="detail-section"><h3>处理历史</h3>${events?`<ol class="work-history">${events}</ol>`:'<p>没有本地处理记录。</p>'}</div>`,next);
  }
  function releaseView(){return `<div class="work-release-batches"><section class="section"><div class="section-head"><div><h2>发布集合 RS-014</h2><small>固定成员 3 项 · 示例依赖批次 · 非实际部署状态</small></div>${pill('待完整批准','amber')}</div><div class="section-body"><div class="work-batch"><span class="work-batch-index">01</span><div><h3>交易 API · v1.4.0-rc.2</h3><p>依赖数据库变更计划 DB-008；批准与回退资格尚未确认。</p></div>${b('查看成员门禁','work-release-node','', 'data-node="api"')}</div><div class="work-dependency-line">前置批准及健康观察成立后，才开放下一批次</div><div class="work-batch"><span class="work-batch-index">02</span><div><h3>管理端 + 移动端</h3><p>分别引用准确制品；可并行不代表允许越过成员批准。</p></div>${b('查看下游范围','work-release-node','','data-node="clients"')}</div></div></section><div class="work-two-cols">${section('变更影响',`<div class="section-body"><dl><dt>需求</dt><dd>REQ-028 / AC-PAY-01 r2</dd><dt>候选</dt><dd>c7a29d8 · 任务来源可回查</dd><dt>风险</dt><dd>旧标准证据过期，需要新回归</dd></dl></div>`)}${section('异常后的继续处理',`<div class="section-body"><p class="prose">部分成功保留每个成员结果；未知先核对原操作。回滚需要数据变更、制品与目标环境的独立资格。</p>${b('预览部分成功场景','work-release-partial','spaced')}</div>`)}</div></div>`;}
  function configuration(){
    const nav=tabs([['deployment','部署形态'],['inheritance','配置来源与差异'],['offline','设备与离线']],data.configTab,'work-config-tab','配置工作区');
    if(data.configTab==='deployment')return S.baseView('configuration').replace('<div class="deployment-options">',nav+'<div class="deployment-options">');
    return S.heading('configuration',data.configTab==='inheritance'?'每项配置，都能说明来自哪里':'跨设备工作，有明确的恢复边界','这里演示配置和同步决策，不读取本机秘密或项目数据，不签发实际许可。',b(data.configTab==='inheritance'?'审阅配置差异':'比较离线变更',data.configTab==='inheritance'?'work-config-review':'work-sync-review','primary'))+nav+(data.configTab==='inheritance'?configEditor():offlineView());
  }
  const configFields=[['budget','计划预算','CNY',60,'组织额度上限 ¥60','只能收紧'],['concurrency','计划并发','槽位',3,'项目资源上限 3','只能收紧'],['offline','离线有效期','小时',4,'组织许可上限 4h','只能收紧']];
  function configEditor(){return `<div class="work-inheritance-flow" aria-label="配置解析"><span>平台默认</span><i>→</i><span>组织约束</span><i>→</i><span>项目覆盖</span><i>→</i><span class="current">固定计划</span></div><div class="work-two-cols">${section('拟采用配置',`<form id="work-config-form" class="section-body">${configFields.map(([key,title,unit,max,origin,rule])=>`<div class="work-config-field"><label for="work-config-${key}">${title}<small>${origin} · ${rule}</small></label><div class="work-field-unit"><input id="work-config-${key}" type="number" min="1" max="${max}" step="1" data-config-field="${key}" value="${data.config[key]}" required><span>${unit}</span></div></div>`).join('')}<div class="work-config-field"><div><strong>网络范围</strong><small>组织 / 只可收紧；不接受用户层扩大</small></div>${pill('固定允许范围','')}</div><div class="work-config-field"><div><strong>凭据</strong><small>受控秘密引用；浏览器不持有主密钥</small></div>${pill('不可覆盖','')}</div><p id="work-config-error" role="alert"></p></form><div class="section-foot">来源修订 r${data.configRevision}。用户偏好仅影响语言、密度等界面设置，不覆盖安全策略。</div>`)}${section('采用前需要重验',`<div class="section-body"><ul class="rule-list"><li>${icon('lock')}当前身份、资源范围及策略版本</li><li>${icon('doc')}配置差异、依据修订与预算单位</li><li>${icon('layers')}仅作用于新计划，不改活动运行</li></ul><div class="callout spaced">回退生成新变更，历史账本和已执行事实不重写。</div>${b('模拟来源版本变化','work-config-conflict','spaced')}${b('恢复已存样稿','work-config-reset','quiet spaced')}</div>`)}</div>`;}
  function offlineView(){const modes=[['offline','离线草稿'],['expired','许可过期'],['denied','重连后无权'],['online','已重新授权']];return `<div class="toolbar"><label>恢复场景<select id="work-sync-scenario">${modes.map(([id,name])=>`<option value="${id}" ${id===data.syncState?'selected':''}>${name}</option>`).join('')}</select></label>${pill('本地情景演练，不控制设备','')}</div><div class="work-device-track"><article><span class="work-device-symbol">${icon('doc')}</span><h2>桌面工作区</h2><p>本地正文与独立分支</p>${pill('3 项待上传','amber')}</article><span class="work-link">⇄<small>重新核对权限</small></span><article><span class="work-device-symbol">${icon('shield')}</span><h2>速码控制面</h2><p>正式版本、验收与账本</p>${pill(data.syncState==='denied'?'当前无权':data.syncState==='online'?'允许提交提案':'等待重连',data.syncState==='denied'?'red':'purple')}</article></div><div class="work-two-cols">${section('离线许可边界',`<div class="section-body"><dl><dt>范围</dt><dd>桌面 / CLI 的本地 Runner</dd><dt>期限</dt><dd>${data.syncState==='expired'?'示例已到期，禁止新操作':'示例许可 4 小时，不因重启延长'}</dd><dt>云模型</dt><dd>仍须在线网关，不旁路直连</dd><dt>Web / 移动</dt><dd>资料和草稿；不能离线远程派发</dd></dl></div>`)}${section('待同步变更',`<div class="section-body"><div class="resource"><strong>需求正文 · REQ-028</strong><p>本地与远端同时修改，保留共同基础供比较。</p></div><div class="resource"><strong>代码候选 · candidate-local-01</strong><p>回网上传后独立测试，不能直接成为验收。</p></div><div class="resource"><strong>排期草稿</strong><p>结构化字段采用版本比较，不用正文合并规则覆盖。</p></div>${data.syncResolution?pill('已记录解决提案，等待正式接纳','blue'):pill('原件保留','')}</div>`)}</div>`;}
  function configReview(){
    const form=$('#work-config-form');if(form&&!form.reportValidity())return;
    const invalid=configFields.find(([key,, ,max])=>!Number.isInteger(data.config[key])||data.config[key]<1||data.config[key]>max);
    if(invalid){S.toast('拟采用值必须在当前组织授权上限内。');return;}
    const changed=configFields.filter(([key])=>data.config[key]!==data.savedConfig[key]);
    S.showModal('配置差异与影响',`<p class="prose">来源 r${data.configRevision} · ${changed.length} 项修改 · 仅影响新计划</p>${changed.length?changed.map(([key,title,unit])=>`<div class="diff-row"><div class="diff-side"><small>${title} / 已存</small>${data.savedConfig[key]} ${unit}</div><div class="diff-side new"><small>拟采用</small>${data.config[key]} ${unit}</div></div>`).join(''):'<div class="empty">没有需要采用的差异。</div>'}<div id="work-config-adoption-state" class="callout ${data.simulatedConflict?'amber':''}">${data.simulatedConflict?'模拟检测到来源版本变化：保留草稿，请重新核对来源后审阅。':'真实产品需要当前授权、稳定操作ID与版本检查；此操作只保存样稿。'}</div>`,(data.simulatedConflict?b('重新核对来源','work-config-refresh'):'')+b('采用样稿配置','work-config-apply','primary',changed.length?'':'disabled'));
  }
  function syncReview(){const locked=data.syncState!=='online';S.showModal('离线变更 · 三方比较',`<p class="prose">需求 REQ-028 · 共同基础 r2 · 当前远端 r3</p><div class="work-three-way"><article><small>共同基础</small><p>超时后查询支付结果。</p></article><article><small>本地修改</small><p>查询失败时保留原请求，不重新支付。</p></article><article><small>远端修改</small><p>查询超时转客服核对，并展示处理进度。</p></article></div><label class="spaced">解决提案<select id="work-sync-choice"><option value="merge" ${(data.syncResolution||'merge')==='merge'?'selected':''}>合并两方补充</option><option value="local" ${(data.syncResolution||'merge')==='local'?'selected':''}>保留本地提案</option><option value="remote" ${(data.syncResolution||'merge')==='remote'?'selected':''}>采用远端正文，本地保留副本</option></select></label><div class="callout ${locked?'amber':''} spaced">${locked?data.syncState==='denied'?'当前无权：禁止新的同步接纳。保留本地原件。':'先重连并核对当前权限；不能凭离线旧授权提交。':'已重新授权的示例状态：此处仅记录解决提案，不直接改正式版本。'}</div>`,b('保存解决提案','work-sync-apply','primary',locked?'disabled':''));}
  function ideaForm(){S.showModal('录入想法 · 从原文开始',`<p class="prose">保留你的输入，再给出结构化草稿。这里使用固定整理模板，不调用模型。</p><label class="spaced">想解决什么问题？<textarea id="work-idea-input" rows="5" maxlength="3000" placeholder="谁在什么场景遇到什么问题？希望看到什么变化？">${e(data.ideaDraft)}</textarea></label><p id="work-idea-error" role="alert"></p>`,b('整理为需求草稿','work-idea-preview','primary'));}
  const commands=[
    ['portfolio','组织交付台','组合、项目与待处理'],['requirements','需求与设计','想法、PRD、体验与架构'],['planning','计划与容量','迭代、时间线、排期草稿'],['teams','团队与分工','成员、智能体、专家组与交接'],['execution','执行与调度','队列、资源与固定派发'],['automation','事件与自动化','触发、范围、门禁与结果'],['models','模型与 Harness','四通道、约束匹配与安全交接'],['connections','工具接入','Cursor / Claude / Qoder / Codex / Zed'],['quality','质量与发布','用例、缺陷、版本与依赖'],['assets','资产与治理','知识、记忆、技能、权限与费用'],['configuration','配置与部署','单机、私有化、SaaS与同步'],['architecture','架构地图','模块职责、协议方向与恢复'],['requirements','REQ-028 · 支付异常恢复','正式示例对象 / 需求'],['planning','TASK-108 · 支付异常恢复','正式示例对象 / 任务'],['quality','ISS-032 · 支付重试问题','正式示例对象 / 问题']
  ];
  function commandResults(query=''){const list=commands.filter(item=>item.join(' ').toLowerCase().includes(query.toLowerCase()));return list.length?list.map(([route,title,description])=>`<button class="work-command-result" data-action="work-jump" data-route="${route}" ${title.startsWith('ISS-')?'data-issue="ISS-032"':''}><span>${icon('search')}</span><span><strong>${title}</strong><small>${description}</small></span><span>↵</span></button>`).join(''):'<div class="empty">没有匹配的工作区或示例对象。试试“模型”“团队”或“支付”。</div>';}
  function commandPalette(){if($('#modal').open&&!$('#work-command-query')){S.toast('请先完成或关闭当前对话框，再搜索。');return;}S.showModal('查找工作区与对象',`<label class="spaced">搜索<input id="work-command-query" type="search" autocomplete="off" placeholder="搜索模型、团队、支付…"></label><div id="work-command-results" class="work-command-results">${commandResults()}</div><small>↑ ↓ 移动 · Enter 打开 · Escape 返回</small>`);$('#work-command-query').focus();}
  S.registerViews([{id:'requirements',title:'需求与设计',render:requirements},{id:'quality',title:'质量与发布',render:quality},{id:'configuration',title:'配置与部署',render:configuration}]);
  function undoIdea(){if(!data.idea)return;data.undoneIdeas??=[];data.undoneIdeas.push({record:data.idea,replaced:{...S.state.requirement}});S.state.requirement={...data.idea.before};S.state.requirementDirty=data.idea.dirty;S.state.suggestionApplied=data.idea.suggestionApplied;data.ideaHistory.pop();data.idea=data.ideaHistory.at(-1)||null;S.closeModal();S.closeDetail(false);S.render();S.toast('已恢复替换前需求正文；撤销副本保留在当前会话。');}
  const actions={
    'work-quality-tab':el=>{data.qualityTab=el.dataset.id;S.render();},
    'work-config-tab':el=>{data.configTab=el.dataset.id;S.render();},
    'work-issue-open':el=>issueDetail(el.dataset.id),
    'work-issue-clear':()=>{data.issueFilter='all';S.render();},
    'work-issue-fix':()=>{const issue=data.issues.find(i=>i.id===data.selectedIssue);if(issue.fix)return;issue.fix=`FIX-${issue.id.slice(4)}-${issue.cycle}`;issue.status='修复中';issue.history.push({kind:'draft',cycle:issue.cycle,text:`形成修复草稿 ${issue.fix}；未启动执行`});S.closeDetail(false);S.render();issueDetail(issue.id);},
    'work-issue-test':()=>S.showModal('载入示例回归证据','<p class="prose">选择预设的独立回归结果，用于验证页面门禁；这不会运行测试或产生真实证据。</p><label class="spaced">结果<select id="work-regression-result"><option value="failed">示例失败：原缺陷仍可复现</option><option value="passed">示例通过：准确候选已独立测试</option></select></label>',b('载入样例','work-issue-test-load','primary')),
    'work-issue-test-load':()=>{const issue=data.issues.find(i=>i.id===data.selectedIssue);issue.regression=$('#work-regression-result').value;issue.status=issue.regression==='passed'?'待验收':'修复中';issue.history.push({kind:'test',cycle:issue.cycle,text:`载入示例回归${issue.regression==='passed'?'通过':'失败'}结果（非实际测试）`});S.closeModal();S.render();issueDetail(issue.id);},
    'work-issue-review':()=>S.showModal('审阅准确修复周期','<p class="prose">已载入示例独立通过结果。人工验收与测试结论仍是不同事实；当前操作仅演练审阅。</p><label class="work-check spaced"><input id="work-review-confirm" type="checkbox">已检查原问题、准确候选与回归范围</label><p id="work-review-error" role="alert"></p>',b('关闭样稿问题','work-issue-close','primary')),
    'work-issue-close':()=>{const issue=data.issues.find(i=>i.id===data.selectedIssue);if(!$('#work-review-confirm').checked){$('#work-review-error').textContent='请先确认本次审阅的范围。';return;}if(issue.regression!=='passed'){S.toast('缺少准确周期的通过证据，不能关闭。');return;}issue.accepted=true;issue.status='已关闭';issue.history.push({kind:'accept',cycle:issue.cycle,text:'完成样稿审阅并关闭；不代表真实业务验收'});S.closeModal();S.render();issueDetail(issue.id);},
    'work-issue-reopen':()=>{const issue=data.issues.find(i=>i.id===data.selectedIssue);issue.history.push({kind:'reopen',cycle:issue.cycle,text:`保留周期 ${issue.cycle} 的修复、回归与验收历史`});issue.cycle++;issue.status='待分诊';issue.fix=null;issue.regression=null;issue.accepted=false;S.closeDetail(false);S.render();issueDetail(issue.id);},
    'work-issue-reconcile':()=>S.showModal('先核对原观察与部署','<div class="callout amber spaced">探针中断不说明部署失败。先核对原部署结果、观察窗口和健康证据；此处不能直接重发部署。</div><p class="prose spaced">重新连通后可读取原记录；缺少可信证据时继续保留待核对。</p>'),
    'work-new-issue':()=>S.showModal('记录一个问题样稿','<label class="spaced">问题标题<input id="work-new-issue-title" maxlength="120" required placeholder="描述可复现的异常"></label><label class="spaced">影响范围<select id="work-new-issue-severity"><option>P2</option><option>P1</option><option>P0</option></select></label><p id="work-new-issue-error" role="alert"></p>',b('添加到本地样稿','work-new-issue-save','primary')),
    'work-new-issue-save':()=>{const title=$('#work-new-issue-title').value.trim();if(!title){$('#work-new-issue-error').textContent='请填写能识别问题的标题。';return;}const id='ISS-LOCAL-'+(data.issues.length+1);data.issues.push({id,title,severity:$('#work-new-issue-severity').value,owner:'待分配',status:'待分诊',cycle:1,source:'用户录入 · 本地样稿',fix:null,regression:null,accepted:false,history:[]});data.issueFilter='all';S.closeModal();S.render();S.toast('已添加问题样稿；没有创建正式工单。');},
    'work-release-node':el=>S.showDetail(el.dataset.node==='api'?'交易 API · 批次 01':'客户端应用 · 批次 02','RS-014 · 固定发布集合',`<div class="detail-section"><dl><dt>前置依赖</dt><dd>${el.dataset.node==='api'?'DB-008 数据变更批准':'交易 API 部署与健康观察'}</dd><dt>当前资格</dt><dd>仍有成员批准与证据待核对</dd><dt>回退资格</dt><dd>分别核验数据变更和可用制品</dd></dl></div><div class="callout">图中顺序来自明确依赖；图布局不创建额外执行边。</div>`),
    'work-release-partial':()=>S.showModal('部分成功后的处理','<div class="work-example-events"><p><b>交易 API</b> · 示例已部署，观察健康</p><p><b>管理端</b> · 示例已部署，观察待核对</p><p><b>移动端</b> · 等待前置</p></div><div class="callout amber">保留成功成员，核对未知观察；不一键重做全体部署。恢复与回滚都是新的受控决策。</div>'),
    'work-config-review':configReview,
    'work-config-conflict':()=>{data.simulatedConflict=true;S.toast('已模拟来源修订变化；当前草稿保留，采用时会提示冲突。');},
    'work-config-reset':()=>{data.config={...data.savedConfig};data.simulatedConflict=false;S.render();S.toast('已恢复本地已存配置。');},
    'work-config-refresh':()=>{data.simulatedConflict=false;data.configRevision++;S.closeModal();S.render();S.toast('已载入示例最新来源；草稿保留，请重新审阅差异。');},
    'work-config-apply':()=>{if(data.simulatedConflict){$('#work-config-adoption-state').textContent='来源已变化，未采用。关闭后重新核对来源；你的草稿仍保留。';return;}data.savedConfig={...data.config};data.configRevision++;S.closeModal();S.render();S.toast('样稿配置已保存；真实策略与活动运行未改变。');},
    'work-sync-review':syncReview,
    'work-sync-apply':()=>{if(data.syncState!=='online'){S.toast('当前不能接纳离线提案。');return;}data.syncResolution=$('#work-sync-choice').value;S.closeModal();S.render();S.toast('已保存本地解决提案；双方原件保留，正式接纳仍需版本检查。');},
    'work-idea':ideaForm,
    'work-idea-preview':()=>{const input=$('#work-idea-input').value.trim();if(!input){$('#work-idea-error').textContent='请先写下问题或想法。';return;}data.ideaDraft=input;S.showModal('整理结果 · 原文与建议',`<div class="diff-row"><div class="diff-side"><small>原始想法</small><p>${e(input)}</p></div><div class="diff-side new"><small>结构化草稿模板</small><p>问题：${e(input)}</p><p>目标用户、非目标、验收标准：待补充确认。</p></div></div><div class="callout amber">采用将替换当前需求样稿正文。原始想法和替换前正文保留在本地撤销记录；不会发布或派发。</div>`,b('采用为需求样稿','work-idea-adopt','primary'));},
    'work-idea-adopt':()=>{data.idea={original:data.ideaDraft,before:{...S.state.requirement},dirty:S.state.requirementDirty,suggestionApplied:S.state.suggestionApplied};data.ideaHistory.push(data.idea);S.state.requirement.body='问题：'+data.ideaDraft+'\n\n目标用户：待确认\n非目标：待确认\n验收标准：待补充';data.idea.after={...S.state.requirement};S.state.requirementDirty=true;S.state.suggestionApplied=false;S.closeModal();S.render();S.showDetail('想法已进入需求样稿','本地整理记录，尚未正式采用',`<div class="detail-section"><p>${e(data.idea.original)}</p></div><div class="callout">继续补齐产品、体验、技术和任务章节。撤销恢复替换前正文。</div>`,b('撤销本次整理','work-idea-undo'));},
    'work-idea-record':()=>{if(!data.idea)return;S.showDetail('需求整理记录','本地记录 · '+data.ideaHistory.length+' 次整理',`<div class="detail-section"><h3>最近原始想法</h3><p>${e(data.idea.original)}</p><h3>替换前正文</h3><p>${e(data.idea.before.body)}</p></div><div class="callout">从最近一次逐项撤销。后续有编辑时先比较；更早原件保留在本次会话。</div>`,b('撤销本次整理','work-idea-undo'));},
    'work-idea-undo':()=>{if(!data.idea)return;if(JSON.stringify(S.state.requirement)!==JSON.stringify(data.idea.after)){S.showModal('撤销前比较后续编辑',`<div class="diff-row"><div class="diff-side"><small>当前草稿</small><p>${e(S.state.requirement.body)}</p></div><div class="diff-side new"><small>准备恢复的正文</small><p>${e(data.idea.before.body)}</p></div></div><div class="callout amber">整理后还有编辑。确认恢复会替换当前正文；当前版本将保留在本地撤销副本中。</div>`,b('确认恢复原正文','work-idea-undo-confirm','primary'));return;}undoIdea();},
    'work-idea-undo-confirm':undoIdea,
    'work-jump':el=>{if(el.dataset.issue)data.qualityTab='issues';go(el.dataset.route);},
  };
  Object.entries(actions).forEach(([name,handler])=>S.registerAction(name,handler));
  $('#command-open').addEventListener('click',commandPalette);
  document.addEventListener('input',event=>{
    const target=event.target;
    if(target.id==='work-command-query')$('#work-command-results').innerHTML=commandResults(target.value);
    if(target.id==='work-idea-input')data.ideaDraft=target.value;
    if(target.dataset.configField)data.config[target.dataset.configField]=Number(target.value);
  });
  document.addEventListener('change',event=>{if(event.target.id==='work-issue-filter'){data.issueFilter=event.target.value;S.render();$('#work-issue-filter').focus();}if(event.target.id==='work-sync-scenario'){data.syncState=event.target.value;S.render();$('#work-sync-scenario').focus();}});
  document.addEventListener('keydown',event=>{
    if((event.ctrlKey||event.metaKey)&&event.key.toLowerCase()==='k'){event.preventDefault();commandPalette();}
    if(!$('#work-command-query')||!$('#modal').open)return;
    const results=[...document.querySelectorAll('.work-command-result')],active=results.indexOf(document.activeElement);
    if(event.key==='ArrowDown'){event.preventDefault();results[(active+1)%results.length]?.focus();}
    if(event.key==='ArrowUp'){event.preventDefault();results[active<0?results.length-1:(active-1+results.length)%results.length]?.focus();}
    if(event.key==='Enter'&&document.activeElement===$('#work-command-query')){event.preventDefault();results[0]?.click();}
  });
  $('#help-button').addEventListener('click',()=>{const modal=$('#modal');const link=document.createElement('a');link.href='../../suma-architecture-design.html#s21';link.target='_blank';link.rel='noopener';link.className='work-reference-link';link.textContent='打开架构主文档与场景映射';modal.insertBefore(link,modal.querySelector('.modal-actions'));});
  S.start();
})();
