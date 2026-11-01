// @vitest-environment jsdom
import {act} from 'react';
import {createRoot} from 'react-dom/client';
import {expect,it,vi} from 'vitest';
import type {ToolProfile} from './tool-model';
import {ToolReadiness} from './ToolReadiness';
import {setLocale} from './i18n';

it('T194 keeps an empty filter distinct from missing connections and uses only secondary context-preserving next actions',async()=>{
 await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);const fetch=vi.fn(),onWorker=vi.fn(),onRegister=vi.fn(),onGuide=vi.fn(),onSelect=vi.fn(),onClearFilters=vi.fn();vi.stubGlobal('fetch',fetch);
 const host=document.createElement('div');document.body.append(host);const root=createRoot(host),base={items:[] as ToolProfile[],total:0,uncertain:false,identitiesKnown:true,identityCount:0,disabled:false,onWorker,onRegister,onGuide,onSelect,onClearFilters};
 const render=async(extra:Partial<Parameters<typeof ToolReadiness>[0]>)=>act(async()=>root.render(<ToolReadiness {...base} {...extra}/>));
 try{
  await render({filtered:true});expect(host.textContent).toContain('No matching connections');expect(host.querySelector('.tool-readiness-main')?.textContent).not.toContain('Connect a development tool');const clear=host.querySelector<HTMLButtonElement>('[data-tool-readiness-clear-filters]')!;expect(clear).not.toBeNull();expect(host.querySelector('.primary')).toBeNull();await act(async()=>clear.click());expect(onClearFilters).toHaveBeenCalledTimes(1);expect(onRegister).not.toHaveBeenCalled();expect(onWorker).not.toHaveBeenCalled();
  await render({filtered:true,disabled:true});expect(host.querySelector<HTMLButtonElement>('[data-tool-readiness-clear-filters]')!.disabled).toBe(true);
  await render({filtered:true,onClearFilters:undefined});expect(host.querySelector('[data-tool-readiness-clear-filters]')).toBeNull();expect(host.querySelector('.tool-readiness-main button')).toBeNull();
  const paused={id:'paused-original',state:'paused',available:false} as ToolProfile;await render({filtered:true,items:[paused],total:9,identityCount:1});expect(host.querySelector('[data-tool-readiness-scope]')?.textContent).toContain('Current filter');expect(host.textContent).toContain('1 / 9');expect(host.querySelector('.primary')).toBeNull();await act(async()=>host.querySelector<HTMLButtonElement>('[data-tool-readiness-next]')!.click());expect(onSelect).toHaveBeenCalledExactlyOnceWith('paused-original');
  await render({filtered:true,uncertain:true});expect(host.textContent).toContain('Status unconfirmed');expect(host.textContent).not.toContain('No matching connections');expect(host.querySelector('[data-tool-readiness-clear-filters]')).toBeNull();
  await render({});expect(host.querySelector('.primary')).toBeNull();await act(async()=>host.querySelector<HTMLButtonElement>('[data-tool-readiness-next]')!.click());expect(onWorker).toHaveBeenCalledTimes(1);expect(onRegister).not.toHaveBeenCalled();
  await render({identityCount:1});await act(async()=>host.querySelector<HTMLButtonElement>('[data-tool-readiness-next]')!.click());expect(onRegister).toHaveBeenCalledTimes(1);expect(host.querySelector('.primary')).toBeNull();expect(host.querySelector('details')?.open).toBe(false);
  await render({filtered:true});await act(async()=>setLocale('zh-CN',{persist:false}));expect(host.textContent).toContain('没有符合条件的接入');expect(host.querySelector('[data-tool-readiness-clear-filters]')?.textContent).toContain('清除筛选');expect(fetch).not.toHaveBeenCalled();
 }finally{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();}
});
