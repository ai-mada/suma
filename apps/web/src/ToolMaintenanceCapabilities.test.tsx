// @vitest-environment jsdom
import {act} from 'react';
import {createRoot} from 'react-dom/client';
import {expect,it,vi} from 'vitest';
import {setLocale} from './i18n';
import {ToolMaintenanceCapabilities} from './ToolMaintenanceCapabilities';
it('T195 presents exact maintenance support and expiry separately from tool health with no reads',async()=>{
 await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);const fetcher=vi.fn();vi.stubGlobal('fetch',fetcher);const host=document.createElement('div'),root=createRoot(host),refresh=vi.fn();document.body.append(host);
 const capability={state:'supported',protocol:'suma-tool-maintenance-v1',instance_id:'durable-instance',checked_at:'2026-11-01T00:00:20Z',observed_at:'2026-11-01T00:00:00Z',expires_at:'2026-11-01T00:01:00Z'} as const;
 try{
  await act(async()=>root.render(<ToolMaintenanceCapabilities maintenance={capability} onRefresh={refresh}/>));expect(host.querySelector('[data-tool-maintenance-state]')?.getAttribute('data-tool-maintenance-state')).toBe('supported');expect(host.querySelector('details')?.open).toBe(false);
  await act(async()=>root.render(<ToolMaintenanceCapabilities maintenance={capability} expired onRefresh={refresh}/>));expect(host.querySelector('[data-tool-maintenance-state]')?.getAttribute('data-tool-maintenance-state')).toBe('expired');
  await act(async()=>host.querySelector<HTMLButtonElement>('[data-tool-maintenance-refresh]')!.click());expect(refresh).toHaveBeenCalledOnce();
  await act(async()=>root.render(<ToolMaintenanceCapabilities maintenance={{...capability,state:'unsupported',protocol:'suma-tool-maintenance-v2'}}/>));expect(host.querySelector('[data-tool-maintenance-state]')?.getAttribute('data-tool-maintenance-state')).toBe('unsupported');
  await act(async()=>root.render(<ToolMaintenanceCapabilities/>));expect(host.querySelector('[data-tool-maintenance-state]')?.getAttribute('data-tool-maintenance-state')).toBe('unknown');expect(fetcher).not.toHaveBeenCalled();
 }finally{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();}
});
