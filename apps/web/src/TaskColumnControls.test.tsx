// @vitest-environment jsdom
import {act,useState} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {TaskColumnControls} from './TaskColumnControls';
import {setLocale} from './i18n';
let root:Root,host:HTMLDivElement,changes:(string[]|undefined)[];
beforeEach(async()=>{await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);changes=[];host=document.createElement('div');document.body.append(host);root=createRoot(host);});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();await setLocale('zh-CN',{persist:false});});
function Screen({initial,disabled=false}:{initial?:string[];disabled?:boolean}){const [columns,setColumns]=useState(initial);return <TaskColumnControls columns={columns} disabled={disabled} onChange={value=>{changes.push(value);setColumns(value);}}/>;}
async function render(initial?:string[],disabled=false){await act(async()=>root.render(<Screen key={String(disabled)+JSON.stringify(initial)} initial={initial} disabled={disabled}/>));}
async function open(){const details=host.querySelector('details');expect(details).not.toBeNull();await act(async()=>{details!.open=true;details!.dispatchEvent(new Event('toggle'));});}
async function click(selector:string){const target=host.querySelector<HTMLElement>(selector);expect(target).not.toBeNull();await act(async()=>target!.click());}
it('starts collapsed and offers named keyboard-native reorder controls while fixing title and retaining focus across locale changes',async()=>{
 await render();expect(host.querySelector('details')?.open).toBe(false);await open();expect(host.querySelector<HTMLInputElement>('[data-column-check="title"]')?.disabled).toBe(true);const up=host.querySelector<HTMLButtonElement>('[data-column-up="owner"]')!;up.focus();await click('[data-column-up="owner"]');expect(changes.at(-1)).toEqual(['title','status','owner','priority','updated']);expect(document.activeElement).toBe(host.querySelector('[data-column-up="owner"]'));await click('[data-column-up="owner"]');expect(changes.at(-1)).toEqual(['title','owner','status','priority','updated']);expect(document.activeElement).toBe(host.querySelector('[data-column-check="owner"]'));await act(async()=>setLocale('zh-CN',{persist:false}));expect(host.textContent).toContain('列');expect(document.activeElement).toBe(host.querySelector('[data-column-check="owner"]'));await click('[data-column-check="owner"]');expect(changes.at(-1)).toEqual(['title','status','priority','updated']);await click('[data-columns-default]');expect(changes.at(-1)).toBeUndefined();
});
it('rejects invalid preferences visibly and only restores defaults after an explicit available action',async()=>{
 await render([]);await open();expect(host.querySelector('[role=alert]')).not.toBeNull();expect(changes).toEqual([]);await click('[data-columns-default]');expect(changes).toEqual([undefined]);await render(['title','owner'],true);await open();await click('[data-column-check="owner"]');expect(changes).toHaveLength(1);
});
