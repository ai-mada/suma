// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ToolSetupGuide} from './ToolSetupGuide';
import {setLocale} from './i18n';

let host:HTMLDivElement,root:Root|undefined;
beforeEach(async()=>{(globalThis as Record<string,unknown>).IS_REACT_ACT_ENVIRONMENT=true;await setLocale('en-US',{persist:false});host=document.createElement('div');document.body.append(host);vi.stubGlobal('requestAnimationFrame',(callback:FrameRequestCallback)=>{callback(0);return 1;});});
afterEach(async()=>{if(root)await act(async()=>root!.unmount());root=undefined;host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();});
async function change(element:HTMLInputElement|HTMLSelectElement,value:string){const prototype=element instanceof HTMLSelectElement?HTMLSelectElement.prototype:HTMLInputElement.prototype;await act(async()=>{Object.getOwnPropertyDescriptor(prototype,'value')!.set!.call(element,value);element.dispatchEvent(new Event(element instanceof HTMLSelectElement?'change':'input',{bubbles:true}));});}

it('T215 explicitly adopts composed references once through the original guide while preserving the selected tool, machine and manual edits',async()=>{
 const fetch=vi.fn(async(_input:RequestInfo|URL,_init?:RequestInit)=>Response.json({items:[]}));vi.stubGlobal('fetch',fetch);Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:vi.fn(async()=>{})}});
 root=createRoot(host);const render=async(project?:string)=>act(async()=>root!.render(<ToolSetupGuide expanded project={project} expectedWorkerID="original-node"/>));
 const field=(name:string)=>host.querySelector<HTMLInputElement|HTMLSelectElement>('[name="remote-runner-'+name+'"]')!;
 await render();await change(host.querySelector<HTMLSelectElement>('.tool-cli-setup-select select')!,'harness');
 await change(field('repository'),'D:/original/repository');await change(field('baseCommit'),'a'.repeat(40));
 const fill=async()=>{await change(host.querySelector<HTMLSelectElement>('[data-command-composition-shell]')!,'powershell');await change(host.querySelector<HTMLInputElement>('[data-command-composition-output]')!,'D:/runner/combined.json');const sources=host.querySelectorAll<HTMLInputElement>('[data-command-composition-source]');await change(sources[0],'D:/runner/development.json');await change(sources[1],'D:/runner/checks.json');await act(async()=>host.querySelector<HTMLButtonElement>('[data-command-composition-copy]')!.click());};
 await fill();expect(field('commands').value).toBe('');await act(async()=>host.querySelector<HTMLButtonElement>('[data-command-composition-use]')!.click());
 expect(field('commands').value).toBe('D:/runner/combined.json');expect(field('shell').value).toBe('powershell');expect(field('localTool').value).toBe('dsh-multica');expect(field('worker').value).toBe('original-node');expect(field('repository').value).toBe('D:/original/repository');expect(field('baseCommit').value).toBe('a'.repeat(40));
 await change(field('commands'),'D:/runner/manual-after-adoption.json');await render();expect(field('commands').value).toBe('D:/runner/manual-after-adoption.json');
 // A later selection owns its own intent. Neither a no-tool preset nor an
 // A->B->A re-mount may resurrect the earlier composed output.
 const select=()=>host.querySelector<HTMLSelectElement>('.tool-cli-setup-select select')!;await change(select(),'junit');expect(field('commands').value).toBe('');await change(select(),'harness');expect(field('commands').value).toBe('');
 await fill();await act(async()=>host.querySelector<HTMLButtonElement>('[data-command-composition-use]')!.click());expect(field('commands').value).toBe('D:/runner/combined.json');
 await render('different-project');expect(host.querySelector<HTMLInputElement>('[data-command-composition-output]')!.value).toBe('');expect(field('commands').value).toBe('');expect(host.querySelector('[data-command-composition-use]')).toBeNull();
 expect(fetch.mock.calls.every(([,init])=>!init?.method||init.method==='GET')).toBe(true);
});
