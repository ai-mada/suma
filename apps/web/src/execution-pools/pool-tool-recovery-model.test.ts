import {expect,it} from 'vitest';
import {createPoolToolRecoveryPin,parsePoolToolRecoveryPin,serializePoolToolRecoveryPin,readPoolRecoveryTool,buildPoolRecoveryProjectHref,buildPoolRecoveryOwnerHref} from './pool-tool-recovery-model';
import type {ToolProfile} from '../tool-model';

it('T218 bounds public source locators and preserves an exact offline profile without granting current eligibility',()=>{
 const tool:ToolProfile={id:'profile-a',project_id:'project-a',worker_id:'pool-runner-a',name:'Offline tool',version:7,state:'enabled',enabled_by:'u',created_at:'',updated_at:'',available:false,reason:'inventory_stale',profile:{command_id:'build',adapter:'command',protocol:'text-v1',profile_sha256:'a'.repeat(64),executable_sha256:'b'.repeat(64),tool_version:'1.0.0',read_only:false}};
 const row={id:'runner-a',pool_id:'pool-a',worker_id:tool.worker_id,status:'active',name:'Runner',credential_expires_at:'2027-01-01T00:00:00Z'};
 const page={as_of:'2026-11-03T00:00:00Z',items:[row]};
 const pin=createPoolToolRecoveryPin(tool,'tenant-a',page)!;
 expect(pin).toBeDefined();expect(Object.keys(pin)).toHaveLength(10);
 expect(parsePoolToolRecoveryPin(serializePoolToolRecoveryPin(pin))).toEqual(pin);
 const owner=new URL(buildPoolRecoveryOwnerHref(pin)!,'https://suma.test/'),project=new URL(buildPoolRecoveryProjectHref(pin)!,'https://suma.test/');
 expect(owner.searchParams.get('record')).toBe('pool-a');expect(owner.searchParams.has('project')).toBe(false);
 expect(project.searchParams.get('project')).toBe('project-a');expect(project.searchParams.get('record')).toBe(tool.id);
 expect(parsePoolToolRecoveryPin(project.searchParams.get('panel')!)).toEqual(pin);
 expect(readPoolRecoveryTool(tool,pin)).toBe(tool);
 expect(readPoolRecoveryTool({...tool,version:8},pin)).toBeUndefined();
 expect(readPoolRecoveryTool({...tool,profile:{...tool.profile,profile_sha256:'c'.repeat(64)}},pin)).toBeUndefined();
 expect(createPoolToolRecoveryPin(tool,'tenant-a',{...page,items:[row,row]})).toBeUndefined();
 expect(createPoolToolRecoveryPin(tool,'tenant-a',{...page,items:[{...row,worker_id:'pool-decoy'}]})).toBeUndefined();
 expect(createPoolToolRecoveryPin({...tool,state:'revoked'},'tenant-a',page)).toBeUndefined();
 expect(parsePoolToolRecoveryPin(serializePoolToolRecoveryPin(pin)+':extra')).toBeUndefined();
 expect(parsePoolToolRecoveryPin(serializePoolToolRecoveryPin(pin).replace(':7:',':07:'))).toBeUndefined();
 expect(()=>serializePoolToolRecoveryPin({...pin,worker_id:'ordinary-worker'})).toThrow();
 expect(()=>serializePoolToolRecoveryPin({...pin,secret:'must-not-enter-url'} as typeof pin)).toThrow();
});
