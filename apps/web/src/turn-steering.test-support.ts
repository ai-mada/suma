import type {SteeringContext,SteeringInstruction,SteeringMessage,SteeringSession} from '../../../packages/contracts/src';
import type {TurnSteeringScope} from './turn-steering-model';

// Public protocol shapes only. Fixture hashes do not establish native delivery
// or authority; the control service computes the actual message/params digests.
export const turnSteeringScope:TurnSteeringScope={project:'project-a',task:'task-a',userID:'user-a',organizationID:'org-a'};
export const turnSteeringMessage:SteeringMessage={id:'message-a',sequence:12,author_id:'user-a',body:'Please keep the existing public API and focus on the parser.',body_sha256:'4'.repeat(64)};
export const turnSteeringSession:SteeringSession={id:'session-a',project_id:'project-a',task_id:'task-a',run_id:'run-a',node_id:'develop',attempt:2,fence:9,worker_id:'worker-a',dispatch_id:'dispatch-a',profile_sha256:'1'.repeat(64),source:{protocol:'codex-turn-steer-v1',thread_id:'thread-a',turn_id:'turn-a',binding_sha256:'2'.repeat(64)},version:3,state:'active',opened_at:'2026-11-01T08:00:00Z',expires_at:'2026-11-01T08:15:00Z',allowed_actions:['send']};
export const turnSteeringInstruction:SteeringInstruction={id:'instruction-a',session_id:'session-a',project_id:'project-a',task_id:'task-a',run_id:'run-a',node_id:'develop',attempt:2,fence:9,version:1,message:turnSteeringMessage,actor_id:'user-a',created_at:'2026-11-01T08:01:00Z',expires_at:'2026-11-01T08:15:00Z'};
export const turnSteeringContext:SteeringContext={task_id:'task-a',task_version:7,message:turnSteeringMessage,targets:[turnSteeringSession],instructions:[],can_send:true};
