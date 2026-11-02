import { wellFormed } from '../i18n/catalog';
import { uniqueJSONMembers } from '../unique-json-members';
import type { PoolConfigPreparationInput,PoolConfigPreparationRequest,PoolConfigPreparationScope } from './pool-config-preparation-model';
const fieldsByCode={
  input_invalid: ['input'],
  output_invalid: ['output'],
  output_unavailable: ['output'],
  output_exists: ['output'],
  output_unconfirmed: ['output'],
  directory_unavailable: ['workspace','repository','worktrees'],
  workspace_conflict: ['workspace'],
  source_unavailable: ['commands_file','model_deployments_file','external_config_file'],
  source_invalid: ['commands_file','model_deployments_file','external_config_file'],
  inputs_changed: ['input'],
  configuration_invalid: ['input'],
  cancelled: ['input'],
} as const;
export type PoolConfigDiagnosticCode=keyof typeof fieldsByCode;
export type PoolConfigDiagnosticField='input'|'output'|'workspace'|'repository'|'worktrees'|'commands_file'|'model_deployments_file'|'external_config_file';
export type PoolConfigDiagnostic={
  protocol: 'suma-pool-config-diagnostic-v1';
  code: PoolConfigDiagnosticCode;
  scope_index: number;
  field: PoolConfigDiagnosticField;
  input_sha256: string;
  output_sha256: string;
  message: string;
};
export type PoolConfigDiagnosticAnalysis={
  state: 'matched';
  binding: 'input_and_output'|'output_only';
  report: PoolConfigDiagnostic;
}|{
  state: 'invalid'|'mismatch'|'crypto_unavailable';
};
const object=(value: unknown): value is Record<string,unknown> => !!value&&typeof value==='object'&&!Array.isArray(value)&&[Object.prototype,null].includes(Object.getPrototypeOf(value));
const exact=(value: Record<string,unknown>,keys: readonly string[]) => keys.every(key => Object.hasOwn(value,key))&&Reflect.ownKeys(value).every(key => typeof key==='string'&&keys.includes(key));
const bytes=(value: string) => new TextEncoder().encode(value).length;
const hash=(value: unknown): value is string => typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
const keys=['protocol','code','scope_index','field','input_sha256','output_sha256','message'];
const rowFields=['tenant_id','project_id','workspace','commands_file','repository','base_commit','worktrees','model_deployments_file','external_config_file'];
/** Imported CLI text is historical, untrusted metadata. Its localized message
 * is checked for shape but is never used as UI text, HTML or a correction. */
export function readPoolConfigDiagnostic(raw: unknown): PoolConfigDiagnostic|undefined {
  if(typeof raw!=='string'||!wellFormed(raw)||bytes(raw)>4096)
    return;
  const line=raw.trim();
  if(!line||/[\r\n]/.test(line))
    return;
  try {
    const value: unknown=JSON.parse(line);
    if(!object(value)||!exact(value,keys)||!uniqueJSONMembers(line)||value.protocol!=='suma-pool-config-diagnostic-v1'||typeof value.code!=='string'||!Object.hasOwn(fieldsByCode,value.code)||!Number.isInteger(value.scope_index)||(value.scope_index as number)<0||(value.scope_index as number)>256||typeof value.field!=='string'||!(fieldsByCode[value.code as PoolConfigDiagnosticCode] as readonly string[]).includes(value.field)||typeof value.input_sha256!=='string'||value.input_sha256!==''&&!hash(value.input_sha256)||!hash(value.output_sha256)||typeof value.message!=='string'||!wellFormed(value.message)||bytes(value.message)>1024)
      return;
    const global=value.field==='input'||value.field==='output';
    if(global!==(value.scope_index===0)||!global&&!hash(value.input_sha256))
      return;
    return value as PoolConfigDiagnostic;
  }
  catch {
    return;
  }
}
/** Mirrors the original preparation builder's exact four/nine-key JSON order.
 * Invalid edited values may be diagnosed, but they cannot revive an old script.
 * Do not use Go re-encoding, canonical JSON, path normalization or hash retries. */
function currentRequest(input: PoolConfigPreparationInput): string|undefined {
  if(!object(input)||!exact(input,['shell','output','pool_id','runner_id','slots','scopes'])||typeof input.pool_id!=='string'||typeof input.runner_id!=='string'||typeof input.slots!=='number'||!Number.isFinite(input.slots)||!Array.isArray(input.scopes)||input.scopes.length<1||input.scopes.length>256||Reflect.ownKeys(input.scopes).length!==input.scopes.length+1)
    return;
  if(!input.scopes.every(row => object(row)&&exact(row,rowFields)&&rowFields.every(field => typeof row[field as keyof PoolConfigPreparationScope]==='string'&&wellFormed(row[field as keyof PoolConfigPreparationScope]))))
    return;
  const request: PoolConfigPreparationRequest={ pool_id: input.pool_id,runner_id: input.runner_id,slots: input.slots,scopes: input.scopes.map(row => ({ ...row })) };
  const compact=JSON.stringify(request);
  return bytes(compact)<=65536? compact:undefined;
}
async function sha256(raw: string): Promise<string> {
  if(!globalThis.crypto?.subtle)
    throw Error('crypto_unavailable');
  const result=await globalThis.crypto.subtle.digest('SHA-256',new TextEncoder().encode(raw));
  return Array.from(new Uint8Array(result),byte => byte.toString(16).padStart(2,'0')).join('');
}
export async function analyzePoolConfigDiagnostic(raw: unknown,input: PoolConfigPreparationInput): Promise<PoolConfigDiagnosticAnalysis> {
  const report=readPoolConfigDiagnostic(raw);
  if(!report)
    return { state: 'invalid' };
  if(typeof input?.output!=='string'||!wellFormed(input.output)||bytes(input.output)>4096)
    return { state: 'mismatch' };
  try {
    if(await sha256(input.output)!==report.output_sha256)
      return { state: 'mismatch' };
    // Before a complete stdin was available, only the literal destination is
    // bound. This never permits row focus or a claim that input was checked.
    if(report.input_sha256==='')
      return { state: 'matched',binding: 'output_only',report };
    const compact=currentRequest(input);
    if(compact===undefined||report.scope_index>input.scopes.length||await sha256(compact)!==report.input_sha256)
      return { state: 'mismatch' };
    return { state: 'matched',binding: 'input_and_output',report };
  }
  catch {
    return { state: 'crypto_unavailable' };
  }
}
