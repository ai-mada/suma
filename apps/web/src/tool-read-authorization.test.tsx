// @vitest-environment jsdom
import {act} from 'react';
import {createRoot} from 'react-dom/client';
import {expect,it,vi} from 'vitest';
import {useToolRead} from './tool-hooks';
import {ApiViewProvider} from './api-context';
import type {Session} from './api';

it('retains a tool read on a temporary outage and removes its protected data on an explicit authorization refusal',async()=>{
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);let status=200,read:()=>Promise<boolean>=async()=>false;
 vi.stubGlobal('fetch',vi.fn(async()=>Response.json(status===200?{name:'Protected original node'}:{error:{code:'refused',message:'Refused'}},{status})));
 const host=document.createElement('div');document.body.append(host);const root=createRoot(host);
 function View(){const state=useToolRead<{name:string}>('/release-workers?project_id=p');read=state.read;return <p>{state.data?.name}</p>;}
 try{
  await act(async()=>root.render(<View/>));expect(host.textContent).toContain('Protected original node');
  await act(async()=>{expect(await read()).toBe(true);});
  status=503;await act(async()=>{expect(await read()).toBe(false);});expect(host.textContent).toContain('Protected original node');
  status=403;await act(async()=>{expect(await read()).toBe(false);});expect(host.textContent).not.toContain('Protected original node');
  const session=(id:string):Session=>({user:{id,name:id,email:id+'@example.test'},organization:{id:'org',name:'Org'},projects:[]});
  const pending:Array<(response:Response)=>void>=[];vi.mocked(fetch).mockImplementation(async()=>new Promise(resolve=>pending.push(resolve)));
  await act(async()=>root.render(<ApiViewProvider session={session('first')}><View/></ApiViewProvider>));
  await act(async()=>root.render(<ApiViewProvider session={session('second')}><View/></ApiViewProvider>));
  await act(async()=>pending[0](Response.json({name:'Late protected first actor'})));expect(host.textContent).not.toContain('Late protected first actor');
  await act(async()=>pending[1](Response.json({name:'Current second actor'})));expect(host.textContent).toContain('Current second actor');
 }finally{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();}
});
