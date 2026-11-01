// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ACPQuickConnectSelection} from './ACPQuickConnectSelection';
import type {ACPQuickSelectionState} from './acp-quick-connect-selection-model';
import {setLocale} from './i18n';

const nativeID='native-installation-a',firstID='provider:model/a',secondID='provider:model/b';
function fresh(patch:Partial<ACPQuickSelectionState>={}):ACPQuickSelectionState{return {installations:[{id:nativeID,label:'Qoder native',version:'Reviewed fixture version',kind:'native'},{id:'wrapper-a',label:'Qoder wrapper',version:'Observed wrapper version',kind:'wrapper_only'},{id:'unverified-a',label:'Unverified entry',kind:'unverified'}],installationID:nativeID,sourceKey:'current-scope-and-installation',catalogueSourceKey:'current-scope-and-installation',catalogue:{source:'acp_session_config',protocol_version:1,config_id:'model-config',current_value:secondID,models:[{id:firstID,name:'Shared display name',configuration_sha256:'a'.repeat(64)},{id:secondID,name:'Shared display name',configuration_sha256:'b'.repeat(64)}]},expiresAt:'2026-11-01T10:01:00Z',phase:'ready',modelID:'',...patch};}
let root:Root,host:HTMLDivElement;
const actions={onInspect:vi.fn(),onInstallationChange:vi.fn(),onDiscover:vi.fn(),onModelChange:vi.fn(),onRecover:vi.fn()};
beforeEach(async()=>{await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.stubGlobal('fetch',vi.fn(async()=>{throw new Error('Presentational selection must not perform HTTP');}));vi.useFakeTimers({toFake:['Date']});vi.setSystemTime('2026-11-01T10:00:00Z');Object.values(actions).forEach(action=>action.mockClear());host=document.createElement('div');document.body.append(host);root=createRoot(host);});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.useRealTimers();vi.unstubAllGlobals();vi.restoreAllMocks();});
async function render(state:ACPQuickSelectionState,extra:{disabled?:boolean;detecting?:boolean;canDiscover?:boolean}={}){await act(async()=>root.render(<ACPQuickConnectSelection toolName="Qoder" installationGuideURL="https://docs.qoder.com/cli/installation" state={state} canDiscover={true} {...actions} {...extra}/>));}
function element<T extends HTMLElement=HTMLButtonElement>(selector:string){const node=host.querySelector<T>(selector);expect(node,selector).not.toBeNull();return node!;}
async function click(selector:string){await act(async()=>element(selector).click());}
async function select(selector:string,value:string){const node=element<HTMLSelectElement>(selector);await act(async()=>{Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype,'value')!.set!.call(node,value);node.dispatchEvent(new Event('change',{bubbles:true}));});}

it('uses exact installation and model IDs and never turns rendering or the current model into an automatic action',async()=>{
 const idle=fresh({installationID:'',phase:'idle',catalogue:undefined,catalogueSourceKey:undefined,expiresAt:undefined});await render(idle);
 Object.values(actions).forEach(action=>expect(action).not.toHaveBeenCalled());expect(element<HTMLOptionElement>('option[value="wrapper-a"]').disabled).toBe(true);expect(element<HTMLOptionElement>('option[value="unverified-a"]').disabled).toBe(true);
 await click('[data-acp-installations-inspect]');expect(actions.onInspect).toHaveBeenCalledOnce();await select('[data-acp-installation-select]',nativeID);expect(actions.onInstallationChange).toHaveBeenCalledExactlyOnceWith(nativeID);expect(actions.onDiscover).not.toHaveBeenCalled();
 await render({...idle,installationID:nativeID});await click('[data-acp-models-discover]');expect(actions.onDiscover).toHaveBeenCalledOnce();
 const observed=fresh();await render(observed);const models=element<HTMLSelectElement>('[data-acp-model-select]');expect(models.value).toBe('');expect(actions.onModelChange).not.toHaveBeenCalled();
 expect([...models.options].filter(option=>option.value).map(option=>option.value)).toEqual([firstID,secondID]);expect([...models.options].filter(option=>option.value).every(option=>option.textContent?.includes('Shared display name'))).toBe(true);
 await select('[data-acp-model-select]',secondID);expect(actions.onModelChange).toHaveBeenCalledExactlyOnceWith(secondID);await render({...observed,modelID:secondID});expect(element('[data-acp-selected-model]').textContent).toContain(secondID);expect(element('[data-acp-selection-ready]').getAttribute('data-acp-selection-ready')).toBe('true');expect(fetch).not.toHaveBeenCalled();
});

it('invalidates source and expiry changes and distinguishes failed, unsupported and empty observations without adopting a stale model',async()=>{
 const selected=fresh({modelID:firstID});await render(selected);expect(element('[data-acp-selection-ready]').getAttribute('data-acp-selection-ready')).toBe('true');
 const variants:{status:string;state:ACPQuickSelectionState}[]=[
  {status:'source_changed',state:{...selected,sourceKey:'another-scope'}},
  {status:'expired',state:{...selected,expiresAt:'2026-11-01T09:59:59Z'}},
  {status:'failed',state:{...selected,phase:'failed'}},
  {status:'unsupported',state:{...selected,phase:'unsupported'}},
  {status:'empty',state:{...selected,catalogue:{...selected.catalogue!,models:[]}}},
 ];
 for(const variant of variants){await render(variant.state);expect(element('[data-acp-models-state]').getAttribute('data-acp-models-state')).toBe(variant.status);expect(element('[data-acp-selection-ready]').getAttribute('data-acp-selection-ready')).toBe('false');expect(host.querySelector('[data-acp-selected-model]')).toBeNull();const models=host.querySelector<HTMLSelectElement>('[data-acp-model-select]');expect(!models||models.disabled||[...models.options].every(option=>!option.value)).toBe(true);}
 Object.values(actions).forEach(action=>expect(action).not.toHaveBeenCalled());expect(fetch).not.toHaveBeenCalled();
});

it('freezes an unknown observation for original recovery and honors disabled without issuing any other callback',async()=>{
 const unknown=fresh({phase:'unknown',modelID:firstID});await render(unknown);Object.values(actions).forEach(action=>expect(action).not.toHaveBeenCalled());expect(element<HTMLSelectElement>('[data-acp-installation-select]').disabled).toBe(true);expect(element<HTMLButtonElement>('[data-acp-installations-inspect]').disabled).toBe(true);expect(host.querySelector('[data-acp-models-discover]')).toBeNull();expect(host.querySelector('[data-acp-model-select]')).toBeNull();expect(host.querySelector('[data-acp-selected-model]')).toBeNull();
 await click('[data-acp-installations-inspect]');await click('[data-acp-observation-recover]');expect(actions.onRecover).toHaveBeenCalledOnce();
 await render(unknown,{disabled:true});expect(element<HTMLButtonElement>('[data-acp-observation-recover]').disabled).toBe(true);await click('[data-acp-observation-recover]');expect(actions.onRecover).toHaveBeenCalledOnce();for(const action of [actions.onInspect,actions.onInstallationChange,actions.onDiscover,actions.onModelChange])expect(action).not.toHaveBeenCalled();expect(fetch).not.toHaveBeenCalled();
});
