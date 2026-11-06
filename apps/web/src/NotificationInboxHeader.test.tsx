// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {App} from './App';
import {ApiViewProvider} from './api-context';
import {NotificationInboxHeader,notifyOrganizationInboxChanged} from './NotificationInboxHeader';
import {setLocale} from './i18n';
import type {Session} from './api';

const summaryPath='/api/v1/notification-inbox/organization/unread-summary';
const organizationInboxPath='/api/v1/notification-inbox/organization?filter=inbox&limit=20';
const session:Session={
 user:{id:'user-a',name:'User A',email:'user-a@example.test',role:'member'},
 organization:{id:'org-a',name:'Organization A'},
 projects:[],
};
const englishLabel='Open organization inbox';
const unread=(count:number)=>({organization_id:'org-a',user_id:'user-a',unread:count});
let host:HTMLDivElement,root:Root;

beforeEach(async()=>{
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);
 localStorage.clear();
 sessionStorage.clear();
 await setLocale('en-US',{persist:false});
 host=document.createElement('div');
 document.body.append(host);
 root=createRoot(host);
});
afterEach(async()=>{
 await act(async()=>root.unmount());
 host.remove();
 vi.unstubAllGlobals();
 await setLocale('zh-CN',{persist:false});
});

function headerButton(){
 const button=host.querySelector<HTMLButtonElement>(`button[aria-label^="${englishLabel}"]`);
 expect(button).not.toBeNull();
 return button!;
}
async function renderHeader(refresh:number,scope:Session=session){
 await act(async()=>root.render(<ApiViewProvider session={scope}>
  <NotificationInboxHeader userID={scope.user.id} organizationID={scope.organization.id} refresh={refresh} onOpen={()=>{}}/>
 </ApiViewProvider>));
}

it('opens the organization inbox from the header without a project or any write',async()=>{
 // Preload the real lazy route so its module resolves within the click's act.
 await import('./NotificationInbox');
 history.replaceState(null,'','/?view=overview');
 const fetcher=vi.fn(async(input:string,_init:RequestInit)=>{
  if(input===summaryPath)return Response.json(unread(103));
  if(input===organizationInboxPath)return Response.json({
   organization_id:'org-a',user_id:'user-a',limit:20,items:[],total:0,
   counts:{inbox:0,unread:0,archived:0},next_cursor:'',as_of:'2026-11-06T03:00:00Z',scope_revision:'a'.repeat(64),
  });
  if(input==='/api/v1/auth/session')return Response.json(session);
  if(input==='/api/v1/account/preferences')return Response.json({user_id:'user-a',locale:'',version:0,updated_at:''});
  if(input==='/api/v1/organization/preferences')return Response.json({organization_id:'org-a',locale:'en-US',version:1,updated_at:'2026-11-06T03:00:00Z'});
  return Response.json({items:[],total:0});
 });
 vi.stubGlobal('fetch',fetcher);
 await act(async()=>root.render(<App/>));
 const button=headerButton();
 expect(button.closest('.topbar')).not.toBeNull();
 expect(button.getAttribute('aria-label')).toBe('Open organization inbox, 103 unread');
 expect(button.textContent).toContain('99+');
 const summary=fetcher.mock.calls.find(([path])=>path===summaryPath)?.[1];
 expect(summary).toMatchObject({method:'GET'});
 expect(new Headers(summary?.headers).get('X-Suma-Expected-User')).toBe('user-a');
 expect(new Headers(summary?.headers).get('X-Suma-Expected-Organization')).toBe('org-a');
 await act(async()=>button.click());
 expect(fetcher.mock.calls.some(([path])=>path===organizationInboxPath)).toBe(true);
 const destination=new URLSearchParams(location.search);
 expect(destination.get('view')).toBe('development');
 expect(destination.get('sub')).toBe('inbox');
 expect(destination.get('inbox_scope')).toBe('organization');
 expect(destination.has('project')).toBe(false);
 expect(headerButton().getAttribute('aria-current')).toBe('page');
 expect(fetcher.mock.calls.every(([,init])=>init.method==='GET'&&!init.body)).toBe(true);
});

it('clears an earlier badge on malformed or unauthorized summary and reloads only for its own inbox change',async()=>{
 let response=Response.json(unread(4));
 const fetcher=vi.fn(async()=>response.clone());
 vi.stubGlobal('fetch',fetcher);
 await renderHeader(0);
 expect(headerButton().getAttribute('aria-label')).toBe('Open organization inbox, 4 unread');

 response=Response.json({...unread(8),unread:-1});
 await renderHeader(1);
 expect(headerButton().getAttribute('aria-label')).toBe(englishLabel);
 response=Response.json({...unread(8),organization_id:'org-b'});
 await renderHeader(2);
 expect(headerButton().getAttribute('aria-label')).toBe(englishLabel);

 response=Response.json(unread(3));
 const before=fetcher.mock.calls.length;
 await act(async()=>notifyOrganizationInboxChanged('user-b','org-a'));
 expect(fetcher).toHaveBeenCalledTimes(before);
 await act(async()=>notifyOrganizationInboxChanged('user-a','org-a'));
 expect(headerButton().getAttribute('aria-label')).toBe('Open organization inbox, 3 unread');

 response=Response.json({error:{code:'forbidden',message:'Current access revoked'}},{status:403});
 await renderHeader(3);
 expect(headerButton().getAttribute('aria-label')).toBe(englishLabel);
 expect(headerButton().disabled).toBe(false);
});

it('keeps a late unread response out of the new identity and localizes its current count',async()=>{
 let finishPrevious!:(response:Response)=>void;
 const previousResponse=new Promise<Response>(resolve=>{finishPrevious=resolve;});
 const fetcher=vi.fn(async(_path:string,init:RequestInit)=>{
  const headers=new Headers(init.headers);
  if(headers.get('X-Suma-Expected-Organization')==='org-a')return previousResponse;
  return Response.json({organization_id:'org-b',user_id:'user-b',unread:2});
 });
 vi.stubGlobal('fetch',fetcher);
 await renderHeader(0);
 expect(fetcher).toHaveBeenCalledTimes(1);
 expect(headerButton().getAttribute('aria-label')).toBe(englishLabel);
 await renderHeader(0,{...session,user:{...session.user,id:'user-b'},organization:{id:'org-b',name:'Organization B'}});
 expect(headerButton().getAttribute('aria-label')).toBe('Open organization inbox, 2 unread');
 await act(async()=>finishPrevious(Response.json(unread(17))));
 expect(headerButton().getAttribute('aria-label')).toBe('Open organization inbox, 2 unread');
 await act(async()=>{await setLocale('zh-CN',{persist:false});});
 expect(host.querySelector('button[aria-label="打开组织收件箱，2 条未读"]')).not.toBeNull();
});
