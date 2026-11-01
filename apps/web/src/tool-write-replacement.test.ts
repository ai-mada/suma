import {expect,it} from 'vitest';
import type {ApiCaller} from './api';
import {ApiError} from './api';
import {ToolWriteRecovery} from './tool-write';
import {toolProfileReplacementPath,validToolProfileReplacementReceipt} from './tool-profile-replacement-model';

it('T200 restores only the original atomic handoff body and key and retains an uncertain or mismatched receipt',async()=>{
 const profile={command_id:'codex-next',adapter:'codex',protocol:'codex-jsonl-v1',profile_sha256:'c'.repeat(64),executable_sha256:'b'.repeat(64),tool_version:'1.0.0',read_only:false};
 const old={id:'original',project_id:'project',worker_id:'worker-a',name:'Previous Codex',version:4,state:'enabled',profile:{...profile,command_id:'codex-before',profile_sha256:'a'.repeat(64)},enabled_by:'owner',available:false,reason:'',created_at:'2026-11-01T00:00:00Z',updated_at:'2026-11-01T00:00:00Z'};
 const target={...old,id:'target',worker_id:'worker-b',name:'Next Codex',version:1,state:'paused',profile,enabled_by:''};
 const reply={id:'relation',project_id:'project',original:old,replacement:target,disposition:'created',pause_original:false,created_at:'2026-11-02T00:00:00.000000000Z',created_by:'owner'};
 const body={project_id:'project',expected_version:4,worker_id:target.worker_id,name:target.name,profile,pause_original:false};
 const values=new Map<string,string>(),store={getItem:(key:string)=>values.get(key)??null,setItem:(key:string,value:string)=>{values.set(key,value);},removeItem:(key:string)=>{values.delete(key);}};
 const calls:unknown[][]=[];let phase=0;
 const send:ApiCaller=async(...args)=>{calls.push(structuredClone(args));if(phase===0)throw new ApiError(0,'network','unknown-ack');return (phase===1?{...reply,replacement:{...target,worker_id:'wrong-worker'}}:reply) as never;};
 const validate:ConstructorParameters<typeof ToolWriteRecovery>[3]=(value,request)=>validToolProfileReplacementReceipt(value,request,'owner');
 const first=new ToolWriteRecovery('org:owner:project:profiles',store,send,validate);
 await expect(first.run(toolProfileReplacementPath(old.id),body)).rejects.toThrow('unknown-ack');
 const originalRaw=[...values.values()][0];expect(originalRaw).toBeTruthy();expect(first.pending?.body).toEqual(body);
 phase=1;const restored=new ToolWriteRecovery('org:owner:project:profiles',store,send,validate);
 await expect(restored.restore()).rejects.toThrow();expect([...values.values()][0]).toBe(originalRaw);expect(restored.pending).toEqual(first.pending);expect(calls[1]).toEqual(calls[0]);
 const count=calls.length;await expect(restored.restore(()=>false)).rejects.toThrow();expect(calls).toHaveLength(count);expect([...values.values()][0]).toBe(originalRaw);
 phase=2;await expect(restored.restore()).resolves.toEqual(reply);expect(calls[2]).toEqual(calls[0]);expect(restored.pending).toBeUndefined();expect(values.size).toBe(0);
 const acceptedCalls=calls.length;
 for(const invalid of [{...body,reuse:{profile_id:'other',expected_version:1},name:''},{...body,pause_original:undefined},{...body,profile:{...profile,args:['do-not-execute']}}])await expect(new ToolWriteRecovery('invalid',store,send,validate).run(toolProfileReplacementPath(old.id),invalid)).rejects.toThrow();
 await expect(new ToolWriteRecovery('wrong-method',store,send,validate).run(toolProfileReplacementPath(old.id),body,'PATCH')).rejects.toThrow();expect(calls).toHaveLength(acceptedCalls);expect(values.size).toBe(0);
});
