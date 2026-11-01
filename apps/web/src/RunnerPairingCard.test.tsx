// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,expect,it,vi} from 'vitest';
import type {RunnerPairing} from '../../../packages/contracts/src';
import {RunnerPairingCard,type RunnerPairingCardProps} from './RunnerPairingCard';
import {setLocale} from './i18n';

let root:Root|undefined,host:HTMLDivElement|undefined,clipboard:PropertyDescriptor|undefined;
afterEach(async()=>{if(root)await act(async()=>root!.unmount());host?.remove();if(clipboard)Object.defineProperty(navigator,'clipboard',clipboard);else Reflect.deleteProperty(navigator,'clipboard');vi.unstubAllGlobals();vi.restoreAllMocks();});

it('keeps fingerprint review and explicit approval in front while optional evidence and rerun commands preserve source and pending guards',async()=>{
 await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);const fetch=vi.fn(),writeText=vi.fn(async(_value:string)=>{});vi.stubGlobal('fetch',fetch);clipboard=Object.getOwnPropertyDescriptor(navigator,'clipboard');Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText}});
 const pairing:RunnerPairing={id:'c'.repeat(32),project_id:'p',version:2,state:'proposed',control_url:'https://control.example.test/suma',created_at:'2026-11-01T04:00:00Z',expires_at:'2026-11-01T04:10:00Z',can_approve:true,device_name:'Target build machine',credential_sha256:'f'.repeat(64)};
 const callbacks={onCreate:vi.fn(),onRefresh:vi.fn(),onApprove:vi.fn(),onReject:vi.fn(),onRestore:vi.fn(),onContinue:vi.fn()};
 host=document.createElement('div');document.body.append(host);root=createRoot(host);
 const render=async(patch:Partial<RunnerPairingCardProps>={})=>{await act(async()=>root!.render(<RunnerPairingCard {...callbacks} pairing={pairing} loading={false} {...patch}/>));};
 const node=<T extends HTMLElement>(selector:string)=>host!.querySelector<T>(selector)!;
 await render();
 const evidence=node<HTMLDetailsElement>('[data-runner-pairing-evidence]'),rerun=node<HTMLDetailsElement>('[data-runner-pairing-rerun]');expect(evidence.open).toBe(false);expect(rerun.open).toBe(false);
 expect(node('[data-runner-pairing-device]').textContent).toContain(pairing.device_name);expect(node('[data-runner-pairing-control]').textContent).toBe(pairing.control_url);expect(node('[data-runner-pairing-fingerprint]').textContent).toBe(pairing.credential_sha256);
 for(const selector of ['[data-runner-pairing-device]','[data-runner-pairing-control]','[data-runner-pairing-fingerprint]','[data-runner-pairing-confirm]','[data-runner-pairing-approve]','[data-runner-pairing-reject]','[data-runner-pairing-refresh]'])expect(node(selector).closest('details'),selector).toBeNull();
 expect(node('[data-runner-pairing-shell]').closest('details')).toBe(rerun);expect(node<HTMLInputElement>('[data-runner-pairing-confirm]').checked).toBe(false);expect(node<HTMLButtonElement>('[data-runner-pairing-approve]').disabled).toBe(true);expect(callbacks.onApprove).not.toHaveBeenCalled();
 const summary=rerun.querySelector('summary')!;summary.focus();expect(document.activeElement).toBe(summary);await act(async()=>summary.click());expect(rerun.open).toBe(true);
 await act(async()=>{const select=node<HTMLSelectElement>('[data-runner-pairing-shell]');Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype,'value')!.set!.call(select,'powershell');select.dispatchEvent(new Event('change',{bubbles:true}));});
 const shown=node('[data-runner-pairing-command]').textContent!;await act(async()=>node<HTMLButtonElement>('[data-runner-pairing-copy]').click());expect(writeText).toHaveBeenCalledExactlyOnceWith(shown);expect(shown).toContain('--pair-runner');expect(fetch).not.toHaveBeenCalled();
 await act(async()=>node<HTMLInputElement>('[data-runner-pairing-confirm]').click());expect(node<HTMLButtonElement>('[data-runner-pairing-approve]').disabled).toBe(false);
 const revised={...pairing,version:3,credential_sha256:'e'.repeat(64)};await render({pairing:revised});expect(node<HTMLInputElement>('[data-runner-pairing-confirm]').checked).toBe(false);expect(node<HTMLButtonElement>('[data-runner-pairing-approve]').disabled).toBe(true);
 await act(async()=>node<HTMLInputElement>('[data-runner-pairing-confirm]').click());await render({pairing:revised,loading:true});await render({pairing:revised});expect(node<HTMLInputElement>('[data-runner-pairing-confirm]').checked).toBe(false);
 await act(async()=>node<HTMLInputElement>('[data-runner-pairing-confirm]').click());await render({pairing:{...revised,reason_code:'control_changed',can_approve:false}});expect(node<HTMLInputElement>('[data-runner-pairing-confirm]').checked).toBe(false);expect(node<HTMLButtonElement>('[data-runner-pairing-approve]').disabled).toBe(true);expect(host.textContent).toContain('control URL changed');expect(callbacks.onApprove).not.toHaveBeenCalled();
 await render({pairing:{...revised,reason_code:'approval_required',can_approve:true}});expect(node<HTMLInputElement>('[data-runner-pairing-confirm]').checked).toBe(false);await act(async()=>node<HTMLInputElement>('[data-runner-pairing-confirm]').click());await act(async()=>node<HTMLButtonElement>('[data-runner-pairing-approve]').click());expect(callbacks.onApprove).toHaveBeenCalledTimes(1);
 await render({pairing:revised,pending:true});expect(host.querySelector('[data-runner-pairing-approve]')).toBeNull();expect(host.querySelector('[data-runner-pairing-copy]')).toBeNull();await act(async()=>node<HTMLButtonElement>('[data-runner-pairing-restore]').click());expect(callbacks.onRestore).toHaveBeenCalledTimes(1);expect(callbacks.onApprove).toHaveBeenCalledTimes(1);
 const approved:RunnerPairing={...revised,version:4,state:'approved',can_approve:false,worker_id:'remote-'+pairing.id,credential_expires_at:'2026-11-02T04:00:00Z'};await render({pairing:approved});expect(node('[data-runner-pairing-worker]').textContent).toBe(approved.worker_id);expect(node('[data-runner-pairing-worker]').closest('details')).toBeNull();expect(node<HTMLDetailsElement>('[data-runner-pairing-evidence]').open).toBe(false);await act(async()=>node<HTMLButtonElement>('[data-runner-pairing-continue]').click());expect(callbacks.onContinue).toHaveBeenCalledTimes(1);
 const waiting:RunnerPairing={id:pairing.id,project_id:'p',version:1,state:'waiting',control_url:pairing.control_url,created_at:pairing.created_at,expires_at:pairing.expires_at,can_approve:false};await render({pairing:waiting});expect(node('[data-runner-pairing-shell]').closest('details')).toBeNull();expect(fetch).not.toHaveBeenCalled();
});
