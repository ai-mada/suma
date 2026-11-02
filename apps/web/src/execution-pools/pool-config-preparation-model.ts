import { wellFormed } from '../i18n/catalog';
import { runnerSetupLiteral } from '../cursor-setup-guide-model';
/** Private local references only. The native CLI reads and validates their
 * contents; none of these paths is persisted or sent to the control plane. */
export type PoolConfigPreparationScope={
  tenant_id: string;
  project_id: string;
  workspace: string;
  commands_file: string;
  repository: string;
  base_commit: string;
  worktrees: string;
  model_deployments_file: string;
  external_config_file: string;
};
export type PoolConfigPreparationRequest={
  pool_id: string;
  runner_id: string;
  slots: number;
  scopes: PoolConfigPreparationScope[];
};
export type PoolConfigPreparationInput=PoolConfigPreparationRequest&{
  shell: ''|'powershell'|'bash';
  output: string;
};
export type PoolConfigPreparationIssue={
  code: 'target'|'slots'|'scope_count'|'scope_id'|'duplicate'|'path'|'baseline'|'too_large';
  row?: number;
  field?: keyof PoolConfigPreparationScope|'shell'|'output'|'slots';
};
export const emptyPoolConfigScope=(): PoolConfigPreparationScope => ({ tenant_id: '', project_id: '', workspace: '', commands_file: '', repository: '', base_commit: '', worktrees: '', model_deployments_file: '', external_config_file: '' });
const scopeFields=['tenant_id', 'project_id', 'workspace', 'commands_file', 'repository', 'base_commit', 'worktrees', 'model_deployments_file', 'external_config_file'] as const;
const object=(value: unknown): value is Record<string, unknown> => !!value&&typeof value==='object'&&!Array.isArray(value)&&[Object.prototype, null].includes(Object.getPrototypeOf(value));
const closed=(value: Record<string, unknown>, keys: readonly string[]) => keys.every(key => Object.hasOwn(value, key))&&Reflect.ownKeys(value).every(key => typeof key==='string'&&keys.includes(key));
const integer=(value: unknown, min: number, max: number) => Number.isSafeInteger(value)&&(value as number)>=min&&(value as number)<=max;
export const validPoolConfigID=(value: unknown): value is string => typeof value==='string'&&/^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$/.test(value);
const text=(value: unknown): value is string => typeof value==='string'&&!!value&&value.trim()===value&&wellFormed(value)&&new TextEncoder().encode(value).length<=4096&&!/[\p{Cc}\u2028\u2029]/u.test(value);
export function validPoolConfigPath(value: unknown): value is string {
  if(!text(value))
    return false;
  // A target shell is not an OS detector: PowerShell may run on Linux. Only
  // recognizable drive paths get Windows alias/ADS rules; POSIX ':' is literal.
  if(/^[A-Za-z]:[\\/]/.test(value))
    return value.slice(3).split(/[\\/]/).every(part => part!=='.'&&part!=='..'&&!/[. ]$/.test(part)&&!part.includes(':'));
  return /^\/(?!\/)/.test(value)&&!value.split('/').some(part => part==='.'||part==='..');
}
/** One bounded validation path shared by the UI's concise diagnostic and the
 * command builder. It cannot attest file existence, a Grant or tool health. */
export function poolConfigPreparationIssue(value: unknown, maxSlots: number): PoolConfigPreparationIssue|undefined {
  if(!object(value)||!closed(value, ['shell', 'output', 'pool_id', 'runner_id', 'slots', 'scopes'])||!validPoolConfigID(value.pool_id)||!validPoolConfigID(value.runner_id))
    return { code: 'target' };
  if(value.shell!=='powershell'&&value.shell!=='bash')
    return { code: 'target', field: 'shell' };
  if(!validPoolConfigPath(value.output)||/[\\/]$/.test(value.output))
    return { code: 'target', field: 'output' };
  if(!integer(maxSlots, 1, 128)||!integer(value.slots, 1, Math.min(maxSlots, 128)))
    return { code: 'slots', field: 'slots' };
  if(!Array.isArray(value.scopes)||value.scopes.length<1||value.scopes.length>256||Reflect.ownKeys(value.scopes).length!==value.scopes.length+1)
    return { code: 'scope_count' };
  const seen=new Set<string>();
  for(let index=0; index<value.scopes.length; index++) {
    const row=value.scopes[index], issue=(code: PoolConfigPreparationIssue['code'], field?: PoolConfigPreparationIssue['field']) => ({ code, row: index+1, ...(field? { field }:{}) });
    if(!object(row)||!closed(row, scopeFields)||!scopeFields.every(field => typeof row[field]==='string'))
      return issue('scope_id');
    if(!validPoolConfigID(row.tenant_id)||!validPoolConfigID(row.project_id))
      return issue('scope_id', !validPoolConfigID(row.tenant_id)? 'tenant_id':'project_id');
    const key=JSON.stringify([row.tenant_id, row.project_id]);
    if(seen.has(key))
      return issue('duplicate', 'project_id');
    seen.add(key);
    if(!validPoolConfigPath(row.workspace))
      return issue('path', 'workspace');
    const invalidPath=(['commands_file', 'repository', 'worktrees', 'model_deployments_file', 'external_config_file'] as const).find(field => row[field]!==''&&!validPoolConfigPath(row[field]));
    if(invalidPath)
      return issue('path', invalidPath);
    if((row.repository==='')!==(row.base_commit==='')||row.base_commit!==''&&!/^(?:[a-f0-9]{40}|[a-f0-9]{64})$/.test(row.base_commit as string))
      return issue('baseline', row.repository===''? 'repository':'base_commit');
  }
  // PowerShell's single pipeline string adds a line ending (at most CRLF).
  // Bash printf adds none. Budget actual transport bytes before copying.
  const request={ pool_id: value.pool_id, runner_id: value.runner_id, slots: value.slots, scopes: value.scopes };
  if(new TextEncoder().encode(JSON.stringify(request)).length+(value.shell==='powershell'? 2:0)>65536)
    return { code: 'too_large' };
  return;
}
export function poolConfigPreparationCommands(value: unknown, maxSlots: number): {
  request: PoolConfigPreparationRequest;
  prepare: string;
}|undefined {
  if(poolConfigPreparationIssue(value, maxSlots))
    return;
  const input=value as PoolConfigPreparationInput, request: PoolConfigPreparationRequest={ pool_id: input.pool_id, runner_id: input.runner_id, slots: input.slots, scopes: input.scopes.map(scope => ({ ...scope })) };
  const json=runnerSetupLiteral(JSON.stringify(request), input.shell as 'powershell'|'bash'), runner='suma-runner --prepare-pool-config '+runnerSetupLiteral(input.output, input.shell as 'powershell'|'bash');
  const prepare=input.shell==='powershell'? '& {\n  $poolConfigPreparationPreviousEncoding = $OutputEncoding\n  try {\n    $OutputEncoding = [System.Text.UTF8Encoding]::new($false)\n    '+json+' | '+runner+'\n  } finally {\n    $OutputEncoding = $poolConfigPreparationPreviousEncoding\n  }\n}':"printf '%s' "+json+' | '+runner;
  return { request, prepare };
}
