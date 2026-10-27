// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {webcrypto} from 'node:crypto';
import {afterEach,expect,it,vi} from 'vitest';
import {WorkspaceGitPanel} from './WorkspaceGitPanel';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';

vi.mock('@monaco-editor/react',async()=>{const {createElement}=await import('react');return {DiffEditor:({original,modified,options}:{original:string;modified:string;options:{readOnly:boolean}})=>createElement('section',{'data-git-diff':true,'data-readonly':String(options.readOnly)},createElement('pre',{'data-before':true},original),createElement('pre',{'data-after':true},modified))};});
let root:Root|undefined,host:HTMLDivElement|undefined,preview:HTMLDivElement|undefined;
afterEach(async()=>{if(root)await act(async()=>root!.unmount());root=undefined;host?.remove();preview?.remove();vi.unstubAllGlobals();vi.restoreAllMocks();});
const hash=async(value:string)=>Array.from(new Uint8Array(await webcrypto.subtle.digest('SHA-256',new TextEncoder().encode(value))),byte=>byte.toString(16).padStart(2,'0')).join('');
const observed='2026-10-27T01:00:00Z',head='a'.repeat(40);
const file={path:'src/main.ts',previous_path:'',index_status:'M',worktree_status:'M',untracked:false,conflicted:false};
const status=(project:string)=>({project_id:project,available:true,reason_code:'ready',branch:'main',head,unborn:false,detached:false,files:[file],total:1,limit:200,truncated:false,restricted_files:0,observed_at:observed});
const diff=async(project:string,source='worktree')=>({project_id:project,path:file.path,previous_path:'',source,head,original:'before\r\n',modified:source==='index'?'staged 🧪\r\n':'working 🧪\r\n',original_sha256:await hash('before\r\n'),modified_sha256:await hash(source==='index'?'staged 🧪\r\n':'working 🧪\r\n'),binary:false,observed_at:observed});
async function mount(project='p'){
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.stubGlobal('crypto',webcrypto);await setLocale('en-US',{persist:false});
 host=document.createElement('div');preview=document.createElement('div');document.body.append(host,preview);root=createRoot(host);
 const render=(nextProject:string,actor='A')=>root!.render(<ApiViewProvider session={{user:{id:'user-'+actor,name:'Editor',email:'editor@example.test'},organization:{id:'org-'+actor,name:'Org'}}}><WorkspaceGitPanel project={nextProject} active previewTarget={preview!} onOpenFile={vi.fn()}/></ApiViewProvider>);
 await act(async()=>render(project));return render;
}
async function settle(ready:()=>boolean){for(let i=0;i<40;i++){await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});if(ready())return;}expect(ready()).toBe(true);}
const button=(text:string)=>[...document.querySelectorAll('button')].find(item=>item.textContent===text||item.getAttribute('aria-label')===text)!;

it('reads exact worktree and staged comparisons, clears changed/invalid text, and never sends a write',async()=>{
 const requests:RequestInit[]=[];let changed=false,wrong=false;
 vi.stubGlobal('fetch',vi.fn(async(input:string,init:RequestInit)=>{
  requests.push(init);const url=new URL(String(input),location.href);
  if(url.pathname==='/api/v1/workspace/git')return Response.json(status('p'));
  if(changed)return Response.json({error:{code:'workspace_git_changed',message:'Observation changed'}},{status:409});
  const value=await diff('p',url.searchParams.get('source')!);return Response.json(wrong?{...value,modified_sha256:'0'.repeat(64)}:value);
 }));
 await mount();await settle(()=>!!button('src/main.ts'));
 await act(async()=>button('src/main.ts').click());await settle(()=>!!preview!.querySelector('[data-git-diff]'));
 expect(preview!.querySelector('[data-readonly]')?.getAttribute('data-readonly')).toBe('true');
 expect(preview!.querySelector('[data-after]')?.textContent).toBe('working 🧪\r\n');
 await act(async()=>button('Staged').click());await settle(()=>preview!.querySelector('[data-after]')?.textContent==='staged 🧪\r\n');
 changed=true;await act(async()=>button('Working tree').click());await settle(()=>preview!.textContent!.includes('changed since it was read'));
 expect(preview!.querySelector('[data-git-diff]')).toBeNull();
 changed=false;wrong=true;await act(async()=>button('Refresh changes').click());await settle(()=>preview!.textContent!.includes('does not match'));
 expect(preview!.querySelector('[data-git-diff]')).toBeNull();
 expect(requests.every(request=>request.method==='GET'&&!request.body)).toBe(true);
});

it('drops late project/account observations and clears displayed code after current authorization refusal',async()=>{
 let resolveOld:(response:Response)=>void=()=>{},rejectCurrent=false;
 vi.stubGlobal('fetch',vi.fn(async(input:string)=>{
  const url=new URL(String(input),location.href),project=url.searchParams.get('project_id')!;
  if(url.pathname==='/api/v1/workspace/git')return rejectCurrent?Response.json({error:{code:'forbidden',message:'No current access'}},{status:403}):Response.json(status(project));
  if(project==='p')return new Promise<Response>(resolve=>{resolveOld=resolve;});
  return Response.json(await diff(project));
 }));
 const render=await mount();await settle(()=>!!button('src/main.ts'));
 await act(async()=>button('src/main.ts').click());
 await act(async()=>render('other','B'));await settle(()=>!!button('src/main.ts'));
 await act(async()=>button('src/main.ts').click());await settle(()=>!!preview!.querySelector('[data-git-diff]'));
 await act(async()=>resolveOld(Response.json({...await diff('p'),modified:'private old project'})));
 expect(preview!.textContent).not.toContain('private old project');
 await act(async()=>render('other','C'));
 expect(preview!.querySelector('[data-git-diff]')).toBeNull();
 await settle(()=>!!button('src/main.ts'));await act(async()=>button('src/main.ts').click());await settle(()=>!!preview!.querySelector('[data-git-diff]'));
 rejectCurrent=true;await act(async()=>button('Refresh changes').click());await settle(()=>host!.textContent!.includes('No current access'));
 expect(preview!.querySelector('[data-git-diff]')).toBeNull();expect(button('src/main.ts')).toBeUndefined();
});
