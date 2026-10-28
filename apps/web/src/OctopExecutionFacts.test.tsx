// @vitest-environment jsdom
import {createElement} from 'react';
import {renderToStaticMarkup} from 'react-dom/server';
import {afterEach,expect,it} from 'vitest';
import {ExecutionModelFacts} from './ExecutionModelFacts';
import {OctopExecutionFacts} from './OctopExecutionFacts';
import {modelBinding,runtimeCapability,octopEvidence} from './model-execution.test-support';
import {setLocale} from './i18n';
afterEach(async()=>{await setLocale('zh-CN',{persist:false});});
it('T118 renders shared model and gateway facts with separate stop and ledger boundaries in both languages',async()=>{
 const evidence=octopEvidence();evidence.octop_execution={...evidence.octop_execution!,process_stopped:false,cancel_requested:true};
 await setLocale('en-US',{persist:false});const html=renderToStaticMarkup(createElement(ExecutionModelFacts,{binding:modelBinding,evidence,state:'reconciling',project:'project-a'}));expect(html).toContain('data-octop-run-state="unknown"');expect(html).toContain('end_turn');expect(html).toContain('Stop not confirmed');expect(html).toContain('Cancellation requested');expect(html).toContain('data-model-gateway-state="complete"');expect(html).toContain('returned-model');expect(html).toContain('do not prove ledger settlement');expect(html).toContain('settings&amp;sub=usage');expect(html).not.toContain('observe-only');const content=document.createElement('div');content.innerHTML=html;expect([...content.querySelectorAll('details')].every(details=>!details.open)).toBe(true);expect(content.textContent).toContain('original-session');
 await setLocale('zh-CN',{persist:false});const chinese=renderToStaticMarkup(createElement(ExecutionModelFacts,{binding:modelBinding,evidence,state:'reconciling'}));expect(chinese).toContain('停止尚未确认');expect(chinese).toContain('已请求取消');expect(chinese).toContain('模型请求已核对');
});
it('T118 never manufactures a run receipt from configured runtime versions or missing proof',async()=>{
 await setLocale('en-US',{persist:false});const configured=renderToStaticMarkup(createElement(OctopExecutionFacts,{capability:runtimeCapability}));expect(configured).not.toContain('data-octop-run-state');expect(configured).toContain('shared model gateway');const missing=renderToStaticMarkup(createElement(ExecutionModelFacts,{binding:modelBinding,state:'succeeded',evidence:{...octopEvidence(),octop_execution:undefined,model_execution:undefined}}));expect(missing).toContain('data-octop-run-state="unrecorded"');expect(missing).toContain('data-model-gateway-state="unrecorded"');
});
