// @vitest-environment jsdom
import {act,useState} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {setLocale} from './i18n';
import {WorkerArtifactUploadRecovery,useWorkerUploadRecoveryAction} from './WorkerArtifactUploadRecovery';
import type {ExecutionWorker} from './tool-model';
import type {UploadAbandonReceipt,UploadRecoveryView} from './worker-artifact-upload-recovery-model';

const worker:ExecutionWorker={id:'a'.repeat(32),project_id:'project-a',worker_id:'node-a',version:3,status:'active',deployment:false,created_at:'2026-11-04T03:00:00Z',expires_at:'2026-11-05T03:00:00Z',manual_rotation_allowed:true};
const uploadID='b'.repeat(32),manifest='d'.repeat(64);
const path='/release-workers/'+worker.id+'/artifact-uploads/'+uploadID+'/abandon';
const recoveryKey='suma-write-recovery-v1:'+JSON.stringify(['worker-artifact-upload-recovery','org-a','user-a',worker.project_id]);
const first={upload_id:uploadID,run_id:'run-a',node_id:'build-a',attempt:2,fence:'9007199254740999',manifest_sha256:manifest,created_at:'2026-11-04T03:00:00Z',size_bytes:128,attempt_state:'failed',disposition:'eligible'} as const;
const second={...first,upload_id:'c'.repeat(32),run_id:'run-b',node_id:'build-b',attempt_state:'reconciling',disposition:'awaiting_attempt'} as const;
const uploadView=(items:UploadRecoveryView['items']=[first,second],version=worker.version):UploadRecoveryView=>({project_id:worker.project_id,worker_registry_id:worker.id,worker_id:worker.worker_id,worker_version:version,items,next_cursor:''});
const stagedBody=(idempotency_key:string)=>({project_id:worker.project_id,expected_worker_version:3,expected_manifest_sha256:manifest,acknowledge_incomplete:true,idempotency_key});
const receipt=(idempotency_key:string):UploadAbandonReceipt=>({project_id:worker.project_id,worker_registry_id:worker.id,upload_id:uploadID,operation_id:idempotency_key,state:'abandoned'});

let root:Root,host:HTMLElement,reads:string[],posts:{path:string;body:Record<string,unknown>}[];
let listing:()=>Response,send:(body:Record<string,unknown>)=>Promise<Response>;
beforeEach(async()=>{
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);
  await setLocale('en-US',{persist:false});
  sessionStorage.clear();reads=[];posts=[];
  listing=()=>Response.json(uploadView());
  send=async body=>Response.json(receipt(String(body.idempotency_key)));
  vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{
    const url=new URL(input,location.href);
    const endpoint=url.pathname.replace(/^\/api\/v1/,'');
    if(init?.method==='POST'){
      const body=JSON.parse(String(init.body));
      posts.push({path:endpoint,body});
      return send(body);
    }
    reads.push(endpoint+url.search);
    return listing();
  }));
  host=document.createElement('div');document.body.append(host);root=createRoot(host);
});
afterEach(async()=>{
  await act(async()=>root.unmount());
  host.remove();sessionStorage.clear();vi.unstubAllGlobals();
});
function Panel(){
  const action=useWorkerUploadRecoveryAction(worker.project_id,'user-a','org-a');
  const [result,setResult]=useState<UploadAbandonReceipt>();
  return <WorkerArtifactUploadRecovery worker={worker} action={action} disabled={false} restoreDisabled={false} receipt={result} onSaved={setResult}/>;
}
async function render(){await act(async()=>root.render(<Panel/>));}
const button=(selector:string)=>host.querySelector<HTMLButtonElement>(selector);
async function click(selector:string){
  const target=host.querySelector<HTMLElement>(selector);
  expect(target,selector).not.toBeNull();
  await act(async()=>target!.click());
}
function seedOperation(){
  const key='original-upload';
  sessionStorage.setItem(recoveryKey,JSON.stringify({path,method:'POST',body:stagedBody(key),idempotency_key:key}));
}
async function selectUpload(){
  await click('[data-worker-upload-recovery-open]');
  await click('[data-worker-upload-select="'+uploadID+'"]');
  await click('[data-worker-upload-ack]');
}

it('reads the current node on demand and confirms one exact eligible upload',async()=>{
  await render();
  expect(reads).toEqual([]);
  await click('[data-worker-upload-recovery-open]');
  expect(reads).toEqual(['/release-workers/'+worker.id+'/artifact-uploads?project_id=project-a']);
  expect(host.textContent).toContain(first.fence);
  expect(host.textContent).toContain('Awaiting the original attempt');
  expect(host.querySelectorAll('[data-worker-upload-select]')).toHaveLength(1);
  await click('[data-worker-upload-select="'+uploadID+'"]');
  expect(document.activeElement?.textContent).toBe('Confirm closeout');
  expect(button('[data-worker-upload-abandon]')?.disabled).toBe(true);
  expect(posts).toEqual([]);
  await click('[data-worker-upload-ack]');
  listing=()=>Response.json(uploadView([second]));
  await click('[data-worker-upload-abandon]');
  expect(posts).toEqual([{path,body:stagedBody(String(posts[0].body.idempotency_key))}]);
  expect(host.querySelector('[data-worker-upload-receipt]')).not.toBeNull();
  expect(host.querySelector('[data-worker-upload-row="'+uploadID+'"]')).toBeNull();
  expect(sessionStorage.getItem(recoveryKey)).toBeNull();
});

it('withholds stale node records and offers a fresh read',async()=>{
  listing=()=>Response.json(uploadView([first],worker.version+1));
  await render();
  await click('[data-worker-upload-recovery-open]');
  expect(host.querySelector('[data-worker-upload-row]')).toBeNull();
  expect(host.textContent).toContain('could not be verified');
  expect(posts).toEqual([]);
  listing=()=>Response.json(uploadView());
  await click('[data-worker-upload-refresh]');
  expect(host.querySelector('[data-worker-upload-row]')).not.toBeNull();
});

it('restores an unknown original without selecting a new target or rotating credentials',async()=>{
  send=async()=>{throw Error('response lost');};
  await render();
  await selectUpload();
  await click('[data-worker-upload-abandon]');
  expect(button('[data-worker-upload-restore]')).not.toBeNull();
  let mayLeave=true;
  await act(async()=>{mayLeave=window.dispatchEvent(new Event('suma:navigate',{cancelable:true}));});
  expect(mayLeave).toBe(false);
  const original=posts[0];
  expect(JSON.parse(sessionStorage.getItem(recoveryKey)!)).toMatchObject(original);
  await act(async()=>root.unmount());root=createRoot(host);
  await render();
  send=async body=>Response.json(receipt(String(body.idempotency_key)));
  await click('[data-worker-upload-restore]');
  expect(posts).toEqual([original,original]);
  expect(host.querySelector('[data-worker-upload-receipt]')).not.toBeNull();
  expect(window.dispatchEvent(new Event('suma:navigate',{cancelable:true}))).toBe(true);
});

it.each([
  [409,'version_conflict',false],
  [409,'artifact_upload_attempt_unresolved',false],
  [403,'forbidden',true],
] as const)('handles restored HTTP %s %s according to exact commit proof',async(status,code,retains)=>{
  seedOperation();
  send=async()=>Response.json({error:{code,message:'Original operation was refused'}},{status});
  await render();
  await click('[data-worker-upload-restore]');
  expect(posts).toEqual([{path,body:stagedBody('original-upload')}]);
  expect(sessionStorage.getItem(recoveryKey)!==null).toBe(retains);
  expect(host.querySelector('[data-worker-upload-receipt]')).toBeNull();
  expect(host.querySelector('[data-worker-upload-restore]')!==null).toBe(retains);
});
