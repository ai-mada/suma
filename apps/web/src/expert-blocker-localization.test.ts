import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {LocalizationCatalog} from '../../../packages/contracts/src';
import {expertBlocker} from './expert-team-model';
import {expertPlanReason} from './expert-team-plan-model';
import {setLocale,setServerCatalog,tr} from './i18n';

// These are the current parameter-free expert-team definitions served by the
// public catalog. API presentation localizes blockers before the view caches
// them, so either language is a valid input for a later local language switch.
const sourceZh='未派发任务的依据已变化，请明确修订计划';
const sourceEn='Expert team pending task source changed; explicitly revise the plan';
const inactiveZh='此团队当前未启用';
const inactiveEn='Expert team is not active';
const catalog:LocalizationCatalog={version:'expert-current-presentation-fixture',template_format:'named-dot-v1',default_locale:'zh-CN',supported_locales:['zh-CN','en-US'],messages:[
 {key:'expert_team.source',parameters:[],texts:{'zh-CN':sourceZh,'en-US':sourceEn}},
 {key:'expert_team.inactive',parameters:[],texts:{'zh-CN':inactiveZh,'en-US':inactiveEn}},
]};

beforeEach(()=>setServerCatalog(catalog));
afterEach(async()=>{setServerCatalog({...catalog,messages:[]});await setLocale('zh-CN',{persist:false});vi.unstubAllGlobals();});

it('switches one cached localized plan blocker zh to en and back without a business request',async()=>{
 const fetch=vi.fn();vi.stubGlobal('fetch',fetch);
 const snapshot={blockers:[sourceZh],task:'用户编写的方案 / User design'};
 await setLocale('zh-CN',{persist:false});expect(expertPlanReason(snapshot.blockers[0])).toBe(sourceZh);
 await setLocale('en-US',{persist:false});expect(expertPlanReason(snapshot.blockers[0])).toBe(sourceEn);
 await setLocale('zh-CN',{persist:false});expect(expertPlanReason(snapshot.blockers[0])).toBe(sourceZh);
 expect(snapshot).toEqual({blockers:[sourceZh],task:'用户编写的方案 / User design'});
 expect(fetch).not.toHaveBeenCalled();
});

it('uses the same exact catalog pipeline for localized configuration blockers',async()=>{
 await setLocale('en-US',{persist:false});expect(expertBlocker(inactiveZh)).toBe(inactiveEn);
 await setLocale('zh-CN',{persist:false});expect(expertBlocker(inactiveZh)).toBe(inactiveZh);
});

it('preserves unknown text and partial product-message matches verbatim',async()=>{
 const unknown=['项目负责人另有说明 / Owner note',sourceZh+'。','Owner note: '+sourceEn,'',JSON.stringify({message:sourceZh})];
 for(const locale of ['en-US','zh-CN'] as const){await setLocale(locale,{persist:false});for(const text of unknown){expect(expertBlocker(text)).toBe(text);expect(expertPlanReason(text)).toBe(text);}}
});

it('retains existing bundled source handling when the server catalog is not loaded',async()=>{
 setServerCatalog({...catalog,messages:[]});await setLocale('zh-CN',{persist:false});
 expect(expertPlanReason(sourceEn)).toBe(tr('expert-team-orchestration.reason.source'));
 expect(expertBlocker(inactiveEn)).toBe(tr('expert-team.blocker.inactive'));
});
