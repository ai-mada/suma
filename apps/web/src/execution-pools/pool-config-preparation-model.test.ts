import { expect, it } from 'vitest';
import { emptyPoolConfigScope, poolConfigPreparationCommands, poolConfigPreparationIssue, type PoolConfigPreparationInput } from './pool-config-preparation-model';
const input: PoolConfigPreparationInput={ shell: 'powershell', pool_id: 'pool-one', runner_id: 'runner-one', output: "D:/configs/O'Brien $(Write-Output unexpected).json", slots: 1, scopes: [{ ...emptyPoolConfigScope(), tenant_id: 'tenant-one', project_id: 'project-one', workspace: "D:/work/O'Brien $(Write-Output unexpected)", commands_file: '' }] };
const quote=(value: string, shell: 'powershell'|'bash') => "'"+(shell==='powershell'? value.replaceAll("'", "''"):value.replaceAll("'", "'\"'\"'"))+"'";
it('T209 prepares only the exact bounded local Pool input with empty file references and literal shell quoting', () => {
  for(const shell of ['powershell', 'bash'] as const) {
    const value={ ...input, shell }, result=poolConfigPreparationCommands(value, 2)!;
    expect(result).toBeDefined();
    expect(Object.keys(result.request)).toEqual(['pool_id', 'runner_id', 'slots', 'scopes']);
    expect(Object.keys(result.request.scopes[0])).toHaveLength(9);
    expect(result.request.scopes[0].commands_file).toBe('');
    expect(result.prepare).toContain(quote(JSON.stringify(result.request), shell)+' | suma-runner --prepare-pool-config '+quote(value.output, shell));
    expect(result.prepare).not.toMatch(/--(?:control|project|allow-exec|describe-tools|pool-token|maintenance-only)\b/);
    if(shell==='powershell') {
      expect(result.prepare).toContain('[System.Text.UTF8Encoding]::new($false)');
      expect(result.prepare).toContain('finally');
      expect(result.prepare).toContain('$OutputEncoding = $poolConfigPreparationPreviousEncoding');
    }
    else
      expect(result.prepare.startsWith("printf '%s' ")).toBe(true);
  }
  const posix={ ...input, shell: 'powershell' as const, output: '/srv/new-pool.json', scopes: [{ ...input.scopes[0], workspace: '/srv/work:copy' }] };
  expect(poolConfigPreparationCommands(posix, 2)).toBeDefined();
  expect(input.scopes[0].commands_file).toBe('');
});
it('T209 rejects private payload injection, duplicate scopes, unsafe target paths and actual stdin byte overflow', () => {
  for(const patch of [{ shell: '' }, { output: 'relative.json' }, { output: 'D:/configs/../new.json' }, { output: 'D:/configs./new.json' }, { output: 'D:/configs/new.json:extra' }, { output: '\\\\server\\share\\new.json' }, { output: '\\\\?\\D:\\new.json' }, { slots: 0 }, { slots: 3 }, { slots: 1.5 }, { scopes: [] }, { scopes: [...input.scopes, ...input.scopes] }, { control_url: 'https://example.test' }, { token: 'not-accepted' }])
    expect(poolConfigPreparationCommands({ ...input, ...patch }, 2), JSON.stringify(patch)).toBeUndefined();
  for(const patch of [{ workspace: 'D:/parent. /work' }, { workspace: 'D:/work:stream/file' }, { workspace: 'D:/work\nextra' }, { repository: '/srv/repo' }, { base_commit: 'a'.repeat(40) }, { repository: '/srv/repo', base_commit: 'A'.repeat(40) }, { commands_file: 'relative.json' }, { commands_file: null }, { external_config_file: {} }, { commands: {} }])
    expect(poolConfigPreparationCommands({ ...input, scopes: [{ ...input.scopes[0], ...patch }] }, 2), JSON.stringify(patch)).toBeUndefined();
  const missing={ ...input.scopes[0] } as Record<string, unknown>;
  delete missing.external_config_file;
  expect(poolConfigPreparationCommands({ ...input, scopes: [missing] }, 2)).toBeUndefined();
  const sized=(bytes: number, shell: 'powershell'|'bash') => {
    const value={ ...input, shell, scopes: Array.from({ length: 32 }, (_, index) => ({ ...emptyPoolConfigScope(), tenant_id: 'tenant', project_id: 'project-'+index, workspace: '/w/' })) }; const request=() => ({ pool_id: value.pool_id, runner_id: value.runner_id, slots: value.slots, scopes: value.scopes }); let remaining=bytes-new TextEncoder().encode(JSON.stringify(request())).length; for(const scope of value.scopes) {
      const count=Math.min(4093, remaining);
      scope.workspace+='x'.repeat(count);
      remaining-=count;
    } expect(remaining).toBe(0); expect(new TextEncoder().encode(JSON.stringify(request())).length).toBe(bytes); return value;
  };
  expect(poolConfigPreparationCommands(sized(65534, 'powershell'), 2)).toBeDefined();
  expect(poolConfigPreparationIssue(sized(65535, 'powershell'), 2)?.code).toBe('too_large');
  expect(poolConfigPreparationCommands(sized(65536, 'bash'), 2)).toBeDefined();
  expect(poolConfigPreparationIssue(sized(65537, 'bash'), 2)?.code).toBe('too_large');
});
