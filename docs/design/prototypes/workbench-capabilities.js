/* Synthetic, memory-only design exploration. No credentials, network or model calls. */
(function () {
  'use strict';
  const P = window.SumaPrototype;
  if (!P) throw new Error('Load the Suma prototype shell before its capability views.');
  const h = P.escapeHTML, b = P.button, pill = P.pill;
  const supplies = {
    subscription: {title:'订阅授权',icon:'lock',short:'OAuth',credential:'个人授权引用 · 项目范围',capability:'沿提供方公开授权能力声明',limit:'额度与硬停止能力待逐项核验；订阅不能默认转换为 API 额度。',status:'四通道设计目标 · OAuth 待实现'},
    byok: {title:'自带密钥',icon:'plug',short:'BYO Key',credential:'组织秘密引用 · 服务端托管',capability:'固定模型部署、参数与网关许可',limit:'浏览器只见引用；费用估算、已知用量与提供方账单分开。',status:'已有网关限定路径 · 统一目录待完善'},
    local: {title:'本地模型',icon:'layers',short:'内网 / 本机',credential:'节点凭证引用 · 不展示秘密',capability:'按部署声明核对工具调用与资源范围',limit:'本地驻留不证明离线可用；模型、工具和授权仍需分别核验。',status:'现有协议路径 · 完整本地供给待验'},
    managed: {title:'平台托管',icon:'grid',short:'平台服务',credential:'平台服务身份 · 项目额度',capability:'统一供给与平台计量的目标通道',limit:'托管服务、商业额度与生产计费仍是设计目标。',status:'设计待办 · 无真实托管服务'}
  };
  // These are invented profiles, not vendor specifications, prices or health claims.
  const models = [
    {id:'sub-a',name:'订阅档案 A',source:'subscription',revision:2,local:false,tools:true,hard:false,context:64,maxOutput:4096,credential:'示例个人授权 / 项目 commerce',purpose:'有人工参与的交互分析',limit:'仅观察输出用量；无法承诺硬输出预算。'},
    {id:'key-b',name:'网关档案 B',source:'byok',revision:4,local:false,tools:true,hard:true,context:128,maxOutput:8192,credential:'示例组织秘密引用 / 项目 commerce',purpose:'固定工具范围的实现任务',limit:'硬限制仅指本样稿的输出 Token 许可；费用与输入用量不在此保证中。'},
    {id:'key-c',name:'网关档案 C',source:'byok',revision:1,local:false,tools:false,hard:true,context:32,maxOutput:4096,credential:'示例组织秘密引用 / 项目 commerce',purpose:'不调用工具的文本分析',limit:'未声明工具调用能力；不能进入需要宿主工具的任务。'},
    {id:'local-d',name:'内网档案 D',source:'local',revision:3,local:true,tools:true,hard:true,context:32,maxOutput:4096,credential:'示例节点引用 / 私有 Runner',purpose:'有数据驻留要求的固定任务',limit:'驻留范围是合成声明；实际网络边界与离线许可没有被验证。'},
    {id:'local-e',name:'本机档案 E',source:'local',revision:1,local:true,tools:false,hard:false,context:16,maxOutput:2048,credential:'示例本机受限引用 / 私有 Runner',purpose:'本机文本草稿',limit:'能力与用量仅作观察，不能充当受控工具执行器。'},
    {id:'hosted-f',name:'托管档案 F',source:'managed',revision:1,local:false,tools:true,hard:true,context:64,maxOutput:4096,credential:'示例平台服务引用 / 项目额度',purpose:'托管服务交互设计',limit:'所有能力均为待实现的样稿声明，不代表真实服务已开放。'}
  ];
  const state = {source:'byok',filters:{local:false,tools:true,hard:false,output:2048},selected:null,saved:null,compare:[],runState:'running',reconcileResult:'unknown',wizard:null,handoff:null,sequence:0,recovery:false};
  const runLabels = {running:'正在执行',stop_requested:'已请求停止，待核对',unknown:'执行结果未知',stopped:'已确认停止',idle:'尚未启动'};
  const harnesses = {implementation:'实现 Harness（示例）',review:'评审 Harness（设计）',analysis:'分析 Harness（设计）'};
  function model(id) { return models.find(item => item.id === id); }
  function signature() { return JSON.stringify(state.filters); }
  function checks(item) {
    return [
      {ok:!state.filters.local || item.local,label:state.filters.local?'需本地驻留':'未限制驻留位置',reason:'不满足本地驻留要求'},
      {ok:!state.filters.tools || item.tools,label:state.filters.tools?'需工具调用':'未要求工具调用',reason:'没有工具调用声明'},
      {ok:!state.filters.hard || item.hard,label:state.filters.hard?'需硬输出预算':'接受观察模式',reason:'只有用量观察，没有硬输出许可'},
      {ok:Number.isInteger(state.filters.output) && state.filters.output>0 && state.filters.output<=item.maxOutput,label:'输出上限 '+state.filters.output+' Token',reason:'输出上限无效或超过此档案声明'}
    ];
  }
  function eligible(item) { return checks(item).every(check => check.ok); }
  function selectionCurrent() { return !!state.selected && state.selected.basis===signature() && eligible(model(state.selected.id)); }
  function rerender(focusId) { P.render(); if(focusId) document.getElementById(focusId)?.focus({preventScroll:true}); }
  function note(text,tone='') { return `<div class="cap-note ${tone?'cap-note-'+tone:''}">${text}</div>`; }
  function facts(entries) { return `<dl class="cap-facts">${entries.map(([label,value])=>`<div><dt>${h(label)}</dt><dd>${value}</dd></div>`).join('')}</dl>`; }
  function reasons(item) { return `<ul class="cap-reasons">${checks(item).map(check=>`<li class="${check.ok?'cap-check-pass':'cap-check-blocked'}">${P.icon(check.ok?'check':'warning')}<span>${h(check.ok?check.label:check.reason)}</span></li>`).join('')}</ul>`; }

  function candidate(item) {
    const allowed=eligible(item),selected=state.selected?.id===item.id,compared=state.compare.includes(item.id);
    return `<article class="cap-candidate ${selected?'cap-candidate-selected':''}" aria-label="${h(item.name)}">
      <div class="cap-candidate-heading"><div><h3>${h(item.name)}</h3><small>合成档案 · 修订 ${item.revision}</small></div>${pill(allowed?'符合当前条件':'条件不满足',allowed?'purple':'amber')}</div>
      <p class="cap-candidate-purpose">${h(item.purpose)}</p>
      <div class="cap-candidate-facts"><span>${h(item.local?'本地驻留':'外部服务')}</span><span>${h(item.tools?'声明工具调用':'未声明工具调用')}</span><span>${h(item.hard?'硬输出许可':'用量观察')}</span><span>示例上下文 ${item.context}K</span></div>
      ${!allowed?`<p class="cap-inline-warning">${h(checks(item).filter(check=>!check.ok).map(check=>check.reason).join('；'))}</p>`:''}
      <div class="cap-candidate-actions"><label class="cap-inline-check"><input id="cap-compare-${item.id}" type="checkbox" data-cap-compare="${item.id}" ${compared?'checked':''} ${!compared&&state.compare.length>=2?'disabled':''}>加入比较</label><div>${b('选择理由','cap-reason','quiet',`data-id="${item.id}"`)}${b(selected?'已选入草稿':'选择此候选','cap-select','',`data-id="${item.id}" ${allowed?'':'disabled'}`)}</div></div>
    </article>`;
  }
  function selectionPanel() {
    const item=state.selected && model(state.selected.id),current=selectionCurrent();
    if(!item) return `<div class="cap-selection-empty">${P.icon('spark')}<h3>先选择一个合格候选</h3><p>选择理由、档案修订与本次条件会一起固定。切换供给通道不会自动替换你的选择。</p></div>`;
    return `<div class="cap-selection-head"><h3>${h(item.name)}</h3>${pill(current?'待保存的选择草稿':'条件已变化','purple')}</div>${facts([
      ['供给来源',h(supplies[item.source].title)],['固定档案',`示例修订 ${state.selected.revision}`],['凭据来源',h(item.credential)],['输出许可',h(String(state.selected.filters.output))+' Token（样稿）']
    ])}${current?reasons(item):note('当前条件与选择时不同。请重新选择合格候选，不能沿用旧理由。','amber')}
      <details class="cap-disclosure"><summary>查看能力与预算边界</summary><p>${h(item.limit)}</p><p>没有真实报价、探测或任务回执；不推断模型健康，也不承诺供应商账单上限。</p></details>
      ${state.saved?`<p class="cap-saved-note">已保存的内存草稿：${h(model(state.saved.id).name)}，档案修订 ${state.saved.revision}。刷新页面后清空。</p>`:''}`;
  }
  function modelsView() {
    const supply=supplies[state.source],list=models.filter(item=>item.source===state.source),fit=list.filter(eligible).length;
    return `<div class="cap-models">${P.heading('models','模型与 Harness','先核对供给和能力，再固定选择；执行器交接与模型切换分别处理。',b('保存选择草稿','cap-save-selection','primary',selectionCurrent()?'':'disabled'))}
      ${note('交互演练 · 所有模型档案、能力和回执都是合成示例。页面仅保存内存草稿，不读取秘密、不连接提供方、不调用模型。')}
      <div class="cap-channel-switch" role="group" aria-label="模型供给通道">${Object.entries(supplies).map(([id,value])=>b(`<span class="cap-channel-icon">${P.icon(value.icon)}</span><span><strong>${h(value.title)}</strong><small>${h(value.short)}</small></span>`,'cap-source','cap-channel',`data-source="${id}" aria-pressed="${state.source===id}"`)).join('')}</div>
      <div class="cap-source-summary">${facts([['凭据来源',h(supply.credential)],['声明能力',h(supply.capability)],['当前范围',h(supply.status)]])}<p>${h(supply.limit)}</p></div>
      <div class="cap-model-layout"><div>${P.section('按任务约束筛选',`<div class="cap-filter-body"><div class="cap-filters">${[['local','需本地驻留'],['tools','需工具调用'],['hard','需硬输出预算']].map(([id,label])=>`<label class="cap-filter"><input id="cap-filter-${id}" type="checkbox" data-cap-filter="${id}" ${state.filters[id]?'checked':''}><span>${label}</span></label>`).join('')}<label class="cap-output-field">单次输出上限（Token）<input id="cap-output" type="number" inputmode="numeric" min="1" max="8192" step="1" value="${h(state.filters.output)}" aria-describedby="cap-budget-help"></label></div><p id="cap-budget-help" class="cap-helper">硬预算在此只指可执行的输出 Token 许可，不等于费用封顶。筛选按明确条件逐项匹配，不使用综合分或静默降级。</p></div>`, '条件立即作用于当前通道；保留不匹配候选的排除理由。')}
      <div class="cap-results-heading"><h2>当前通道候选</h2><span role="status">${fit} / ${list.length} 项符合条件 · 合成数据</span></div>
      ${fit===0?note('当前通道没有合格候选。可以明确调整约束或切换通道；样稿不会替你放宽条件。','amber'):''}
      <div class="cap-candidate-list">${list.map(candidate).join('')}</div>
      <div class="cap-compare-bar"><div><strong>比较候选</strong><p>${state.compare.length?state.compare.map(id=>h(model(id).name)).join(' / '):'跨通道最多保留两项，只比较已声明的事实。'}</p></div>${b('比较 '+state.compare.length+' 项','cap-compare','',state.compare.length===2?'':'disabled')}${state.compare.length?b('清空','cap-clear-compare','quiet'):''}</div>
      </div><aside class="cap-selection">${P.section('本次选择',`<div class="cap-selection-body">${selectionPanel()}</div>`,'草稿依据与选择理由')}</aside></div>
      ${P.section('Harness 安全交接',`<div class="cap-handoff-overview"><div><div class="cap-handoff-title"><h3>实现 Harness · TASK-108</h3>${pill(runLabels[state.runState],['idle','stopped'].includes(state.runState)?'purple':'amber')}</div><p>会话归项目；交接只固定可见摘要与候选引用。目标执行器必须经过新计划预览与授权。</p><label class="cap-state-field">演练原执行状态<select id="cap-run-state">${Object.entries(runLabels).map(([id,label])=>`<option value="${id}" ${state.runState===id?'selected':''}>${label}（模拟）</option>`).join('')}</select></label></div>${b('准备交接','cap-handoff-open','', 'aria-haspopup="dialog"')}</div>${state.handoff?`<div class="cap-handoff-receipt" role="status"><strong>交接草稿已记录 · ${h(harnesses[state.handoff.target])}</strong><p>仅包含明确摘要${state.handoff.candidate?'与示例候选 c7a29d8':''}；没有复制内部会话、凭据或批准。尚未启动目标 Harness。</p>${b('查看固定内容','cap-handoff-review','quiet')}</div>`:''}`,'停止请求、实际停止与结果核对是不同事实。')}
      </div>`;
  }
  function showReasons(item) {
    P.showDetail(item.name+' · 选择依据','合成档案；判断只依据当前四项约束',`<div class="detail-section"><h3>确定性匹配结果</h3>${reasons(item)}</div><div class="detail-section"><h3>声明与来源</h3>${facts([['供给',h(supplies[item.source].title)],['凭据',h(item.credential)],['档案修订',String(item.revision)],['上下文',item.context+'K（示例声明）']])}</div>${note(h(item.limit),'amber')}`,b('返回候选','close-detail'));
  }
  function compareModels() {
    if(state.compare.length!==2) return;
    const pair=state.compare.map(model),rows=[['供给通道',item=>supplies[item.source].title],['驻留位置',item=>item.local?'本地（合成声明）':'外部服务'],['工具调用',item=>item.tools?'已声明（样稿）':'未声明'],['上下文窗口',item=>item.context+'K（合成）'],['输出许可',item=>(item.hard?'硬输出上限 ':'用量观察 ')+item.maxOutput+' Token'],['凭据来源',item=>item.credential],['价格 / 实测延迟',()=> '未提供，不推断'],['限制',item=>item.limit]];
    P.showModal('比较模型声明',`<p class="prose">比较不生成性能排名。当前约束与每项限制决定是否可选；所有参数来自合成档案。</p><div class="cap-comparison" tabindex="0" role="region" aria-label="两个模型的声明比较"><table><thead><tr><th scope="col">比较项</th>${pair.map(item=>`<th scope="col">${h(item.name)}</th>`).join('')}</tr></thead><tbody>${rows.map(([label,value])=>`<tr><th scope="row">${label}</th>${pair.map(item=>`<td>${h(value(item))}</td>`).join('')}</tr>`).join('')}<tr><th scope="row">当前约束</th>${pair.map(item=>`<td>${reasons(item)}${b('选择此候选','cap-select','',`data-id="${item.id}" ${eligible(item)?'':'disabled'}`)}</td>`).join('')}</tr></tbody></table></div>`);
  }
  function handoffSafe() { return ['idle','stopped'].includes(state.runState); }
  function handoffMaterialValid() { const w=state.wizard; return !!w && !!harnesses[w.target] && w.target!=='implementation' && w.summary.trim().length>0 && w.summary.length<=1500 && w.ack; }
  function wizard() {
    const w=state.wizard;if(!w)return;
    const steps=['核对原执行','固定交接材料','审阅新草稿'];let body='';
    if(w.step===0)body=`${note('原执行：'+h(runLabels[state.runState])+'（模拟）',handoffSafe()?'':'amber')}<p class="cap-modal-prose">正在执行时先请求停止；请求成功仍需核对原执行。结果未知时不启动第二个执行器。</p>
      ${!handoffSafe()?`<div class="cap-gate-actions">${b('请求停止（模拟）','cap-stop','',state.runState==='running'?'':'disabled')}<label>示例核对结果<select id="cap-reconcile-result"><option value="unknown" ${state.reconcileResult==='unknown'?'selected':''}>仍然未知</option><option value="running" ${state.reconcileResult==='running'?'selected':''}>仍在运行</option><option value="stopped" ${state.reconcileResult==='stopped'?'selected':''}>原执行已停止</option></select></label>${b('核对原执行（模拟）','cap-reconcile','',state.runState==='running'?'disabled':'')}</div>`:''}
      ${w.stopProof?`<p class="cap-saved-note">${h(w.stopProof)}</p>`:''}${note('本向导不执行停止命令。真实停止必须由原 Runner / 远端服务回执证明；没有回执时保留原操作等待核对。')}`;
    if(w.step===1)body=`<div class="cap-wizard-form"><label>目标 Harness<select id="cap-handoff-target">${Object.entries(harnesses).filter(([id])=>id!=='implementation').map(([id,label])=>`<option value="${id}" ${w.target===id?'selected':''}>${h(label)}</option>`).join('')}</select></label><label>可见的固定摘要<textarea id="cap-handoff-summary" maxlength="1500" rows="5" aria-describedby="cap-summary-help">${h(w.summary)}</textarea></label><small id="cap-summary-help">目标、已确认结论和待办需人工审阅；最多 1500 字符，不填写凭据。</small><label class="cap-inline-check"><input id="cap-handoff-candidate" type="checkbox" ${w.candidate?'checked':''}>附带示例候选 c7a29d8（独立测试未完成）</label><label class="cap-inline-check"><input id="cap-handoff-ack" type="checkbox" ${w.ack?'checked':''}>我已审阅摘要，目标只接收明确列出的材料</label></div>${note('不复制隐藏内部状态、推理过程、凭据、终端连接或原预算授权。候选不携带测试通过、人工验收或发布批准。','amber')}`;
    if(w.step===2)body=`${facts([['原执行',h(runLabels[state.runState])+'（模拟）'],['目标',h(harnesses[w.target])],['候选',w.candidate?'c7a29d8 · 独立测试未完成':'不携带候选'],['下一步','新计划预览与当前授权；尚未启动']])}<div class="cap-fixed-summary"><h3>固定摘要</h3><p>${h(w.summary)}</p></div>${note('记录只生成内存交接草稿。后续实现应复核当前项目、来源修订、工具能力与预算，再沿原 Go DAG 创建新计划。')}`;
    const stepper=`<ol class="cap-wizard-steps" aria-label="交接步骤">${steps.map((title,index)=>`<li ${w.step===index?'aria-current="step"':''}><span>${index+1}</span>${title}</li>`).join('')}</ol>`;
    let actions=w.step?b('上一步','cap-handoff-back'):'';
    actions+=w.step===2?b('记录交接草稿','cap-handoff-save','primary',handoffSafe()&&handoffMaterialValid()?'':'disabled'):b('继续','cap-handoff-next','primary',(w.step===0?handoffSafe():handoffMaterialValid())?'':'disabled');
    P.showModal('Harness 安全交接 · 样稿',stepper+body,actions);
  }
  function refreshWizardValidity() { const button=document.querySelector('[data-action="cap-handoff-next"]');if(button&&state.wizard?.step===1)button.disabled=!handoffMaterialValid(); }

  const nodes = {
    entry:{title:'Web / 桌面 / CLI',icon:'grid',tag:'共同入口',subtitle:'选择、预览与明确采用',duty:'收集用户意图与草稿，展示当前来源、权限和未知结果。',current:'已有多端入口与部分共同领域合同；完整设备与离线闭环待交付。',target:'共享领域命令、草稿恢复和设备范围；浏览器不持有主密钥。',recovery:'网络断开保留原草稿与操作ID；重新授权后恢复原回执，不自动发出新写入。'},
    control:{title:'控制面领域服务',icon:'shield',tag:'限定实现',subtitle:'正式状态与当前授权',duty:'拥有任务、计划、权限、预算引用、验收与审计；跨域命令在边界内协调。',current:'已有服务端授权、版本校验、稳定操作回执与事务；各领域验证范围独立。',target:'完善统一能力声明与配置解析，继续沿明确领域用例扩展。',recovery:'从同事务回执恢复原命令；外部结果未知时进入核对，不重做已提交的业务写入。'},
    dag:{title:'唯一 Go DAG',icon:'branch',tag:'限定实现',subtitle:'固定计划 / attempt / fence',duty:'持久推进依赖、租约、执行路由和当前节点；Harness不拥有第二套调度权。',current:'已有持久唤醒、增量扫描、私有与Pool执行；生产规模和真正多机仍需独立证据。',target:'有界背压、完整资源准入和可解释策略；保留同一Engine。',recovery:'沿原运行、attempt、fence和操作回执核对。收到取消请求不等于已实际停止。'},
    runner:{title:'私有 / Pool Runner',icon:'layers',tag:'限定实现',subtitle:'获授执行与真实回执',duty:'领取固定任务，在隔离工作区运行工具，回写进度、候选、实际停止和用量事实。',current:'已有固定命令、工作区与原回执路径；部分实际Windows运行场景已验。',target:'补齐目标系统、物理多机与受限离线许可，不靠客户端声称运行健康。',recovery:'保留原调用和本地journal；未知副作用先核对，不自动重放非幂等输入。'},
    harness:{title:'Harness / 工具适配',icon:'plug',tag:'分适配器',subtitle:'能力声明与固定宿主工具',duty:'执行明确目标，使用获授工具；向平台提交结构化进度、候选和证据。',current:'原生、Codex、OpenCode、Octop等各有不同限定路径；不能视为统一能力全部具备。',target:'统一manifest、能力探测和安全交接；仅重建固定摘要与公开材料。',recovery:'切换前停止并核对原执行。保留候选与可见摘要，不共享隐藏内部状态。'},
    gateway:{title:'模型网关',icon:'spark',tag:'限定路径',subtitle:'原生 / Octop 的 attempt 级许可',duty:'绑定模型与预算许可，隔离凭据，持久记录请求与已知用量。',current:'原生编码和Octop已有Go网关与剩余输出许可；其他工具继续遵守各自合同。',target:'四通道统一目录、健康与灰度策略；不能提前承诺所有模型都受同一硬预算约束。',recovery:'先记录原请求，再调用提供方；结果未知先核对，不能自动重复POST或静默换提供方。'},
    subscription:{title:'订阅授权',icon:'lock',tag:'设计待办',subtitle:'提供方支持的 OAuth',duty:'引用用户明确授权及其适用范围；不提取或搬运隐藏凭据。',current:'统一订阅供给尚未实现，已有工具自身会话不等于平台OAuth代理。',target:'逐提供方核验授权、额度、取消与撤销语义。',recovery:'过期或撤销后停止新准入；核对原调用，不自动切换到个人或组织密钥。'},
    byok:{title:'自带密钥',icon:'plug',tag:'限定路径',subtitle:'服务端秘密引用',duty:'保持组织/项目/模型范围，浏览器只展示可用的引用与授权来源。',current:'已有模型配置与网关凭据隔离路径；完整四通道目录待完善。',target:'统一引用生命周期、报价来源和提供方验收。',recovery:'拒绝无效或撤销的凭据；已知用量与未知调用分别核对。'},
    local:{title:'本地模型',icon:'layers',tag:'待完善',subtitle:'节点与网络驻留边界',duty:'声明部署位置、协议、资源、模型版本及工具能力。',current:'已有兼容协议路径；本机配置不能证明模型可用或支持离线。',target:'正式安装、硬件资源画像、网络驻留与离线许可验证。',recovery:'节点失联时保留原调用事实；重新核对来源和剩余额度，不自动迁往外部模型。'},
    managed:{title:'平台托管',icon:'grid',tag:'设计待办',subtitle:'平台服务与商业计量',duty:'提供受项目额度和授权约束的模型服务。',current:'不宣称真实托管模型、商业账单或生产容量已可用。',target:'服务可用性、配额、计费和供应商对账分别验证。',recovery:'平台额度未知或服务异常时暂停新准入，原请求回执与费用分别对账。'},
    mcp:{title:'MCP',icon:'plug',tag:'有方向',subtitle:'客户端 → 领域服务 / Harness → 宿主工具',duty:'把工具调用映射到获授的领域入口或执行宿主，角色不能由参数冒充。',current:'已有部分平台MCP与宿主工具路径；客户端配置导出不等于真实连接验证。',target:'完善逐能力权限、来源绑定与真实客户端矩阵。',recovery:'按原工具调用ID核对结果，鉴权失败先重新授权，不以换工具绕过范围。'},
    acp:{title:'ACP',icon:'chat',tag:'区分两向',subtitle:'平台 → 适配器；编辑器 → 平台为目标',duty:'区分外部编辑器调用平台，与平台驱动工具端协议的两个方向。',current:'Octop等已有受控协议路径；完整编辑器入口不能由协议标签推断。',target:'提供受当前权限约束的ACP服务端，并逐版本验证文件、终端和停止能力。',recovery:'断线后依据原会话与回执核对。无法续跑时显式重建上下文，不共享引擎内部状态。'},
    a2a:{title:'A2A',icon:'arrow',tag:'设计待办',subtitle:'平台节点 → 外部 Agent 任务',duty:'连接外部任务身份、原操作和本地DAG节点，回收准确产物引用。',current:'完整A2A适配与跨服务恢复仍待实现。',target:'固定远端任务ID，定义创建、取消、核对、产物和权限边界。',recovery:'远端创建结果未知时按原ID查询；没有查询或幂等能力则转人工核对。'}
  };
  function node(id) { const item=nodes[id];return b(`<span class="cap-node-heading">${P.icon(item.icon)}<strong>${h(item.title)}</strong></span><span class="cap-node-subtitle">${h(item.subtitle)}</span><small>${h(item.tag)}</small>`,'cap-node','cap-architecture-node'+(id==='dag'?' cap-node-authority':''),`data-id="${id}" aria-haspopup="dialog"`); }
  function edge(text) { return `<span class="cap-architecture-edge"><span aria-hidden="true">→</span><small>${h(text)}</small></span>`; }
  function architectureView() {
    return `<div class="cap-architecture">${P.heading('architecture','架构与能力边界','点击节点查看职责、现有范围、设计目标和故障恢复。图中不以“已配置”代表真实可用。',b(state.recovery?'退出恢复演练':'追踪一次未知结果','cap-recovery','primary'))}
      ${note('架构交互样稿 · 依据主文档总体架构、模型四通道与现有合同。四通道统一供给及跨 Harness 交接仍是设计目标，不代表生产已接入。')}
      ${state.recovery?`<section class="cap-recovery-trace" aria-label="未知结果恢复演练"><div><h2>原模型调用没有收到结果</h2><p>只沿原操作核对；不自动新建调用、释放未知占用或更换提供方。</p></div><ol>${[['gateway','原请求已记录'],['runner','保留未知回执'],['dag','暂停后续准入'],['control','受权核对原操作']].map(([id,label])=>`<li>${b(label,'cap-node','',`data-id="${id}"`)}</li>`).join('')}</ol></section>`:''}
      <section class="cap-architecture-map" aria-label="控制面与执行能力的方向图"><div class="cap-map-caption"><strong>正式状态只由控制面与唯一调度内核拥有</strong><span>每个节点均可点击</span></div>
      <div class="cap-architecture-lane"><div class="cap-lane-label">入口与状态</div><div class="cap-lane-nodes">${node('entry')}${edge('明确命令')}${node('control')}${edge('固定计划')}${node('dag')}</div></div>
      <div class="cap-map-bridge">${P.icon('arrow')}<span>Go DAG 下发固定任务；Runner 回写真实回执、attempt 与 fence</span></div>
      <div class="cap-architecture-lane"><div class="cap-lane-label">执行与模型</div><div class="cap-lane-nodes">${node('runner')}${edge('获授宿主工具')}${node('harness')}${edge('受支持的模型路径')}${node('gateway')}</div></div>
      <p class="cap-map-boundary">当前统一 Go 网关已有原生 / Octop 限定路径；Codex、OpenCode等沿各自固定合同管理，不能由此图推断为全部接入。</p>
      <div class="cap-architecture-lane cap-supply-lane"><div class="cap-lane-label">四通道目标</div><div class="cap-supply-nodes">${['subscription','byok','local','managed'].map(node).join('')}</div></div><p class="cap-map-boundary">调用方向：受支持的 Harness / 网关 → 明确选择的供给。授权、能力声明、真实调用和用量账单各自留证。</p>
      </section>
      ${P.section('能力协议有明确方向',`<div class="cap-protocol-grid">${['mcp','acp','a2a'].map(node).join('')}</div>`,'同一个协议名称不能证明调用方向、权限或适配范围。')}
      <div class="cap-architecture-foot">${P.icon('shield')}<p>候选提交、独立测试、人工验收与部署分别拥有证据。任何模型、Harness 或远端协议都不能替代正式验收。</p></div></div>`;
  }
  function showNode(id) {
    const item=nodes[id];if(!item)return;
    P.showDetail(item.title,item.subtitle,`<div class="cap-node-detail">${[['职责',item.duty],['已有限定',item.current],['设计目标',item.target],['故障恢复',item.recovery]].map(([label,text])=>`<section class="detail-section"><h3>${label}</h3><p>${h(text)}</p></section>`).join('')}${note('此页解释设计与已知边界，不提供连接、执行、授权或验收入口。')}</div>`,b('返回架构','close-detail'));
  }

  P.registerViews([{id:'models',title:'模型与 Harness',eyebrow:'模型选择与安全交接',icon:'spark',render:modelsView},{id:'architecture',title:'架构与边界',eyebrow:'状态归属与能力方向',icon:'branch',render:architectureView}]);
  const actions={
    'cap-source':element=>{if(supplies[element.dataset.source]){state.source=element.dataset.source;rerender();}},
    'cap-select':element=>{const item=model(element.dataset.id);if(!item||!eligible(item)){P.toast('此候选不符合当前条件；请查看排除理由。');return;}state.selected={id:item.id,revision:item.revision,filters:{...state.filters},basis:signature()};P.closeModal();rerender();P.toast('候选已加入选择草稿，尚未保存或调用。');},
    'cap-reason':element=>{const item=model(element.dataset.id);if(item)showReasons(item);},
    'cap-save-selection':()=>{if(!selectionCurrent()){P.toast('条件已变化，请重新选择合格候选。');return;}state.saved=structuredClone(state.selected);rerender();P.toast('模型选择已保存到本页内存草稿；没有调用模型或派发。');},
    'cap-compare':compareModels,
    'cap-clear-compare':()=>{state.compare=[];rerender();},
    'cap-handoff-open':()=>{state.wizard={step:0,target:'review',summary:'目标：复核支付异常后的原请求恢复。\n已确认：保留原操作身份，不重复发起支付。\n待办：独立测试与人工验收尚未完成。',candidate:true,ack:false,stopProof:'',id:'example-handoff-'+(++state.sequence)};state.reconcileResult='unknown';wizard();},
    'cap-stop':()=>{if(!state.wizard||state.runState!=='running')return;state.runState='stop_requested';rerender();wizard();P.toast('已记录模拟停止请求，仍需核对实际停止。');},
    'cap-reconcile':()=>{if(!state.wizard||!['stop_requested','unknown'].includes(state.runState))return;state.runState=state.reconcileResult;state.wizard.stopProof=state.runState==='stopped'?'原执行停止已确认（模拟回执）；这不是生产停止证明。':'';rerender();wizard();},
    'cap-handoff-next':()=>{const w=state.wizard;if(!w)return;if(!handoffSafe()){P.toast('原执行尚未确认停止，不能继续交接。');return;}if(w.step===1&&!handoffMaterialValid()){P.toast('请填写摘要并确认交接材料。');return;}if(w.step<2){w.step++;wizard();}},
    'cap-handoff-back':()=>{if(state.wizard?.step){state.wizard.step--;wizard();}},
    'cap-handoff-save':()=>{const w=state.wizard;if(!w||w.step!==2||!handoffSafe()||!handoffMaterialValid()){P.toast('原执行或交接材料需要重新核对。');return;}state.handoff={id:w.id,target:w.target,summary:w.summary,candidate:w.candidate,sourceState:state.runState};state.wizard=null;P.closeModal();rerender();P.toast('固定交接草稿已记录；没有启动新的 Harness。');},
    'cap-handoff-review':()=>{const draft=state.handoff;if(!draft)return;P.showDetail('固定交接草稿',harnesses[draft.target],`<div class="cap-fixed-summary"><h3>显式摘要</h3><p>${h(draft.summary)}</p></div>${facts([['候选引用',draft.candidate?'示例 c7a29d8，独立测试未完成':'无'],['原执行依据',h(runLabels[draft.sourceState])+'（模拟）'],['执行状态','仅有内存草稿；目标未启动']])}${note('没有隐藏内部状态、凭据、预算授权或人工批准。')}`,b('返回','close-detail'));},
    'cap-node':element=>showNode(element.dataset.id),
    'cap-recovery':()=>{state.recovery=!state.recovery;rerender();}
  };
  Object.entries(actions).forEach(([name,handler])=>P.registerAction(name,handler));
  document.addEventListener('change',event=>{
    const target=event.target;if(!(target instanceof HTMLInputElement||target instanceof HTMLSelectElement))return;
    if(target.dataset.capFilter&&Object.hasOwn(state.filters,target.dataset.capFilter)){state.filters[target.dataset.capFilter]=target.checked;rerender(target.id);}
    if(target.id==='cap-output'){state.filters.output=Number(target.value);rerender(target.id);}
    if(target.dataset.capCompare){const id=target.dataset.capCompare;if(!model(id))return;if(target.checked&&!state.compare.includes(id)&&state.compare.length<2)state.compare.push(id);else if(!target.checked)state.compare=state.compare.filter(value=>value!==id);rerender(target.id);}
    if(target.id==='cap-run-state'&&Object.hasOwn(runLabels,target.value)){state.runState=target.value;rerender(target.id);}
    if(target.id==='cap-reconcile-result'&&['unknown','running','stopped'].includes(target.value))state.reconcileResult=target.value;
    if(state.wizard){if(target.id==='cap-handoff-target'&&Object.hasOwn(harnesses,target.value))state.wizard.target=target.value;if(target.id==='cap-handoff-candidate')state.wizard.candidate=target.checked;if(target.id==='cap-handoff-ack')state.wizard.ack=target.checked;refreshWizardValidity();}
  });
  document.addEventListener('input',event=>{const target=event.target;if(target instanceof HTMLTextAreaElement&&target.id==='cap-handoff-summary'&&state.wizard){state.wizard.summary=target.value;refreshWizardValidity();}});
})();
