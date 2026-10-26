import type {CandidateConflictRepairEvidence} from '../../../packages/contracts/src';
import type {WorkspaceEvidence} from './api';
import {compositionID,compositionInteger} from './candidate-composition-model';

const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const commit=(value:unknown):value is string=>typeof value==='string'&&/^(?:[a-f0-9]{40}|[a-f0-9]{64})$/.test(value);
const commits=(value:unknown,max=16):value is string[]=>Array.isArray(value)&&value.length<=max&&value.every(commit);
const hash=(value:unknown)=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
/** Versioned display validation; completed repair code must match this exact workspace. */
export function validConflictRepairEvidence(value:unknown,evidence:WorkspaceEvidence):value is CandidateConflictRepairEvidence{
 if(!object(value)||value.protocol!=='suma.candidate-conflict-repair.v1'||!compositionID(value.plan_id)||!compositionInteger(value.plan_version)||value.plan_version<1||!compositionID(value.composition_id)||value.plan_id===value.composition_id||!object(value.original_assignment)||!compositionID(value.original_assignment.run_id)||typeof value.original_assignment.node_id!=='string'||!value.original_assignment.node_id||!compositionInteger(value.original_assignment.attempt)||value.original_assignment.attempt<1||!compositionInteger(value.original_assignment.fence)||value.original_assignment.fence<1||typeof value.original_assignment.worker_id!=='string'||!hash(value.original_evidence_sha256)||!hash(value.sources_sha256)||!compositionID(value.operation_id)||!['preparing','conflicted','resolved','completed','unknown'].includes(String(value.state))||!commits(value.parents)||!commits(value.source_heads)||!commits(value.merge_commits)||!Array.isArray(value.steps)||value.steps.length>15)return false;
 if(!value.steps.every(step=>object(step)&&compositionInteger(step.source_index)&&step.source_index>0&&step.source_index<16&&commit(step.left_commit)&&commit(step.right_commit)&&commit(step.seed_tree)&&commit(step.result_commit)&&commit(step.result_tree)&&commits(step.parents)&&(step.conflict_sha256===undefined||hash(step.conflict_sha256))))return false;
 return value.state!=='completed'||value.source_heads.length>=2&&value.steps.length===value.source_heads.length-1&&commit(value.result_commit)&&value.result_commit===evidence.head_commit&&commit(value.result_tree);
}
