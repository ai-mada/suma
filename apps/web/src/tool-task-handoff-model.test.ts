import {describe,it,expect} from 'vitest';
import type {ToolProfile} from './tool-model';
import {normalizeResponse} from './api';
import {createToolTaskHandoffPin,readToolTaskHandoffProfile,readToolTaskHandoffTask,validToolTaskHandoffPin} from './tool-task-handoff-model';

const definition={command_id:'develop',adapter:'codex',protocol:'codex-jsonl-v1',profile_sha256:'a'.repeat(64),executable_sha256:'b'.repeat(64),tool_version:'1.0.0',read_only:false};
const tool:ToolProfile={id:'tool-exact',project_id:'project',worker_id:'pool-runner',name:'Codex development',version:7,state:'enabled',profile:definition,enabled_by:'owner',created_at:'2026-11-02T00:00:00Z',updated_at:'2026-11-02T00:00:00Z',available:true,reason:'',observation:{profile:definition,available:true,reason:''}};

describe('tool task handoff intent',()=>{
  it('keeps only the exact public target and rejects changed, unobserved or non-writable configurations',()=>{
    const pin=createToolTaskHandoffPin(tool,'project');expect(pin).toEqual({project_id:'project',id:'tool-exact',version:7,worker_id:'pool-runner',profile_sha256:'a'.repeat(64),name:'Codex development'});
    expect(readToolTaskHandoffProfile(structuredClone(tool),pin!)).toEqual(tool);
    for(const changed of [{...tool,version:8},{...tool,worker_id:'other'},{...tool,state:'paused'},{...tool,state:'revoked'},{...tool,available:false},{...tool,observation:undefined},{...tool,observation:{...tool.observation!,available:false}},{...tool,profile:{...definition,profile_sha256:'c'.repeat(64)}},{...tool,profile:{...definition,read_only:true}},{...tool,profile:{...definition,protocol:'text-v1',adapter:'command'}},{...tool,project_id:'foreign'}]){
      expect(readToolTaskHandoffProfile(changed,pin!),JSON.stringify(changed)).toBeUndefined();
    }
    expect(createToolTaskHandoffPin(tool,'foreign')).toBeUndefined();
    expect(validToolTaskHandoffPin({...pin,token:'forbidden'})).toBe(false);
    expect(validToolTaskHandoffPin({...pin,id:'../other'})).toBe(false);
  });
  it('uses the freshly reread exact task version without accepting foreign or malformed records',()=>{
    // Both task-workspace and exact useApi reads use this current normalization.
    const fresh=normalizeResponse({id:'task-a',project_id:'project',version:4,title:'Current task',data:{},acceptance_criteria:['Independent tests required']});
    expect(readToolTaskHandoffTask(fresh,'project','task-a')).toEqual(fresh);
    expect(readToolTaskHandoffTask({...fresh,acceptance_criteria:null},'project','task-a')).toBeDefined();
    for(const bad of [{...fresh,id:'other'},{...fresh,project_id:'foreign'},{...fresh,version:NaN},{...fresh,version:0},{...fresh,title:{}},{...fresh,acceptance_criteria:{}},{...fresh,acceptance_criteria:[{}]}])expect(readToolTaskHandoffTask(bad,'project','task-a')).toBeUndefined();
  });
});
