import {webcrypto} from 'node:crypto';
import {afterEach,expect,it,vi} from 'vitest';
import {validWorkspaceGitStatus,workspaceGitDiff,type WorkspaceGitStatus} from './workspace-git-model';
afterEach(()=>vi.unstubAllGlobals());
const head='a'.repeat(40),stamp='2026-10-27T01:00:00Z';
const file={path:'src/main.ts',previous_path:'',index_status:'M',worktree_status:' ',untracked:false,conflicted:false};
const status:WorkspaceGitStatus={project_id:'p',available:true,reason_code:'ready',branch:'main',head,unborn:false,detached:false,files:[file],total:1,limit:200,truncated:false,restricted_files:0,observed_at:stamp};
const hash=async(value:string)=>Array.from(new Uint8Array(await webcrypto.subtle.digest('SHA-256',new TextEncoder().encode(value))),byte=>byte.toString(16).padStart(2,'0')).join('');

it('rejects ambiguous repository observations and restricted paths instead of rendering partial facts as complete',()=>{
 expect(validWorkspaceGitStatus(status,'p')).toBe(true);
 expect(validWorkspaceGitStatus({...status,unborn:true,head:''},'p')).toBe(true);
 expect(validWorkspaceGitStatus({...status,detached:true,branch:''},'p')).toBe(true);
 for(const changed of [{reason_code:'unknown'},{unborn:true,head},{detached:true},{head:''},{total:201},{truncated:true},{project_id:'other'},{files:[file,file]}])expect(validWorkspaceGitStatus({...status,...changed},'p')).toBe(false);
 for(const path of ['../escape','/absolute','C:/drive','src\\windows.ts','.env.local','src/.GIT/config','nested/credentials.json','key.PEM','secrets/one','src/file.'])expect(validWorkspaceGitStatus({...status,files:[{...file,path}]},'p')).toBe(false);
 const unavailable={...status,available:false,reason_code:'not_repository',branch:'',head:'',files:[],total:0};
 expect(validWorkspaceGitStatus(unavailable,'p')).toBe(true);expect(validWorkspaceGitStatus({...unavailable,files:[file],total:1},'p')).toBe(false);
});

it('binds exact UTF8 comparison sides including real empty content, binary omission and source identity',async()=>{
 vi.stubGlobal('crypto',webcrypto);
 const value={project_id:'p',path:file.path,previous_path:'',source:'index',head,original:'',modified:'new 🧪\r\n',original_sha256:await hash(''),modified_sha256:await hash('new 🧪\r\n'),binary:false,observed_at:stamp};
 expect(await workspaceGitDiff(value,status,file,'index')).toEqual(value);
 for(const changed of [{project_id:'other'},{path:'src/other.ts'},{source:'worktree'},{head:'b'.repeat(40)},{modified:'new 🧪\n'},{original_sha256:''},{modified:'x'.repeat(1048577)}])expect(await workspaceGitDiff({...value,...changed},status,file,'index')).toBeUndefined();
 const binary={...value,binary:true,original:'',modified:'',original_sha256:'',modified_sha256:''};
 expect(await workspaceGitDiff(binary,status,file,'index')).toEqual(binary);
 expect(await workspaceGitDiff({...binary,modified:'must not expose bytes'},status,file,'index')).toBeUndefined();
});
