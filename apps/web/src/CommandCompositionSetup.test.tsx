// @vitest-environment jsdom
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { CommandCompositionSetup } from './CommandCompositionSetup';
import { setLocale } from './i18n';
import type { CommandCompositionDraft } from './command-composition-model';

let root: Root, host: HTMLDivElement, clipboard: PropertyDescriptor | undefined;
beforeEach(async () => {
  await setLocale('en-US', { persist: false }); vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  host = document.createElement('div'); document.body.append(host); root = createRoot(host);
  clipboard = Object.getOwnPropertyDescriptor(navigator, 'clipboard'); vi.stubGlobal('fetch', vi.fn());
});
afterEach(async () => {
  await act(async () => root.unmount()); host.remove();
  if (clipboard) Object.defineProperty(navigator, 'clipboard', clipboard); else Reflect.deleteProperty(navigator, 'clipboard');
  vi.restoreAllMocks(); vi.unstubAllGlobals();
});
async function settle() { await act(async () => { for (let i = 0; i < 24; i++) await Promise.resolve(); }); }
async function draw(key = 'source-A', disabled = false, onUseCommands?: (draft: CommandCompositionDraft) => void) { await act(async () => root.render(<CommandCompositionSetup key={key} disabled={disabled} onUseCommands={onUseCommands} />)); await settle(); }
async function change(selector: string, value: string) {
  const element = host.querySelector<HTMLInputElement | HTMLSelectElement>(selector)!;
  await act(async () => { Object.getOwnPropertyDescriptor(element instanceof HTMLSelectElement ? HTMLSelectElement.prototype : HTMLInputElement.prototype, 'value')!.set!.call(element, value); element.dispatchEvent(new Event(element instanceof HTMLSelectElement ? 'change' : 'input', { bubbles: true })); }); await settle();
}
async function click(selector: string) { await act(async () => host.querySelector<HTMLElement>(selector)!.click()); await settle(); }
async function fill() {
  await change('[data-command-composition-shell]', 'powershell'); await change('[data-command-composition-output]', 'D:/new/combined.json');
  await change('[data-command-composition-source=source-1]', "D:/existing/O'Brien.json"); await change('[data-command-composition-source=source-2]', 'D:/existing/checks.json');
}

it('T215 keeps composition collapsed, locates field errors and supports bounded keyboard add/remove and a reference-only copy', async () => {
  const copy = vi.fn().mockResolvedValue(undefined); Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: copy } });
  const use = vi.fn(); await draw('source-A', false, use); expect(host.querySelector<HTMLDetailsElement>('[data-command-composition]')!.open).toBe(false);
  expect(host.querySelectorAll('[data-command-composition-source]')).toHaveLength(2);
  await act(async () => { host.querySelector<HTMLDetailsElement>('[data-command-composition]')!.open = true; });
  await click('[data-command-composition-copy]'); expect(document.activeElement).toBe(host.querySelector('[data-command-composition-shell]')); expect(copy).not.toHaveBeenCalled();
  await fill(); await change('[data-command-composition-source=source-2]', 'relative.json');
  expect(host.querySelector('[data-command-composition-source=source-2]')?.getAttribute('aria-invalid')).toBe('true');
  await click('[data-command-composition-copy]'); expect(document.activeElement).toBe(host.querySelector('[data-command-composition-source=source-2]'));
  await change('[data-command-composition-source=source-2]', '/srv/checks.json');
  await click('[data-command-composition-add]'); expect(document.activeElement).toBe(host.querySelector('[data-command-composition-source=source-3]'));
  await click('[data-command-composition-remove=source-3]'); expect(document.activeElement).toBe(host.querySelector('[data-command-composition-source=source-2]'));
  await click('[data-command-composition-copy]'); expect(copy).toHaveBeenCalledOnce(); expect(copy.mock.calls[0][0]).toContain('--compose-commands');
  await click('[data-command-composition-use]'); expect(use).toHaveBeenCalledExactlyOnceWith({ kind: 'commands-only', shell: 'powershell', commands: 'D:/new/combined.json' });
  expect(host.querySelector('[data-command-composition-command]')?.textContent).toContain("O''Brien.json"); expect(host.textContent).toContain('does not grant inventory'); expect(fetch).not.toHaveBeenCalled();
  await change('[data-command-composition-output]', 'D:/new/another.json'); expect(host.querySelector('[data-command-composition-command]')).toBeNull(); expect(host.querySelector('[data-command-composition-use]')).toBeNull(); expect(use).toHaveBeenCalledOnce();
  for (let i = 2; i < 16; i++) await click('[data-command-composition-add]');
  expect(host.querySelectorAll('[data-command-composition-source]')).toHaveLength(16); expect(host.querySelector<HTMLButtonElement>('[data-command-composition-add]')!.disabled).toBe(true);
  await setLocale('zh-CN', { persist: false }); await draw(); expect(host.textContent).toContain('组合已有命令配置');
});

it('T215 rejects late clipboard success after disabled or source ABA and never preserves a prior source draft', async () => {
  const waiting: Array<() => void> = [];
  const copy = vi.fn(() => new Promise<void>(resolve => waiting.push(resolve)));
  const use = vi.fn();
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: copy } });
  await draw('source-A', false, use); await fill(); await click('[data-command-composition-copy]');
  await draw('source-A', true, use); expect(host.querySelector('[data-command-composition-command]')).toBeNull(); expect(host.querySelector('[data-command-composition-use]')).toBeNull();
  for (const element of host.querySelectorAll<HTMLInputElement | HTMLButtonElement | HTMLSelectElement>('input,select,button')) expect(element.disabled).toBe(true);
  await draw('source-A', false, use); await act(async () => waiting.shift()!()); await settle();
  expect(host.querySelector('[data-command-composition-command]')).toBeNull(); expect(host.querySelector('[data-command-composition-copy]')?.textContent).toContain('Copy composition command');
  await click('[data-command-composition-copy]'); await draw('source-B', false, use); await draw('source-A', false, use);
  await act(async () => waiting.shift()!()); await settle();
  expect(host.querySelector<HTMLInputElement>('[data-command-composition-output]')!.value).toBe('');
  expect(host.querySelector<HTMLSelectElement>('[data-command-composition-shell]')!.value).toBe('');
  expect(host.querySelector<HTMLInputElement>('[data-command-composition-source=source-1]')!.value).toBe('');
  expect(host.querySelector<HTMLDetailsElement>('[data-command-composition]')!.open).toBe(false);
  expect(host.querySelector('[data-command-composition-command]')).toBeNull(); expect(host.querySelector('[data-command-composition-use]')).toBeNull(); expect(use).not.toHaveBeenCalled(); expect(fetch).not.toHaveBeenCalled();
});
