import { createHash,webcrypto } from 'node:crypto';
import { afterEach,expect,it,vi } from 'vitest';
import { readPoolConfigDiagnostic,analyzePoolConfigDiagnostic } from './pool-config-diagnostic-model';
import { emptyPoolConfigScope,type PoolConfigPreparationInput } from './pool-config-preparation-model';
const input: PoolConfigPreparationInput={ shell: 'bash',output: "/srv/O'Brien<&中文>/new.json",pool_id: 'pool-one',runner_id: 'runner-one',slots: 1,scopes: [{ ...emptyPoolConfigScope(),tenant_id: 'tenant',project_id: 'one',workspace: "/srv/O'Brien<&中文>/one" },{ ...emptyPoolConfigScope(),tenant_id: 'tenant',project_id: 'two',workspace: '/srv/two',commands_file: '/srv/two.json' }] };
const sha=(value: string) => createHash('sha256').update(value).digest('hex');
const request=(value=input) => JSON.stringify({ pool_id: value.pool_id,runner_id: value.runner_id,slots: value.slots,scopes: value.scopes });
const report=() => ({ protocol: 'suma-pool-config-diagnostic-v1',code: 'source_invalid',scope_index: 2,field: 'commands_file',input_sha256: sha(request()),output_sha256: sha(input.output),message: 'Untrusted imported localized message' });
afterEach(() => vi.unstubAllGlobals());
it('T211 reads only a bounded unique seven-field diagnostic with the exact finite code/row/field matrix',() => {
  const raw=JSON.stringify(report());
  expect(readPoolConfigDiagnostic(raw+'\r\n')?.code).toBe('source_invalid');
  for(const patch of [{ protocol: 'other' },{ code: 'unknown' },{ scope_index: 0 },{ scope_index: 257 },{ scope_index: 1.5 },{ scope_index: null },{ field: 'repository' },{ field: '__proto__' },{ input_sha256: '' },{ input_sha256: 'A'.repeat(64) },{ output_sha256: null },{ message: 'x'.repeat(1025) },{ message: '\ud800' },{ extra: true }])
    expect(readPoolConfigDiagnostic(JSON.stringify({ ...report(),...patch })),JSON.stringify(patch)).toBeUndefined();
  const missing={ ...report() } as Record<string,unknown>;
  delete missing.scope_index;
  expect(readPoolConfigDiagnostic(JSON.stringify(missing))).toBeUndefined();
  expect(readPoolConfigDiagnostic(raw.replace('"code":','"code":"input_invalid","co\\u0064e":'))).toBeUndefined();
  expect(readPoolConfigDiagnostic(raw+'{}')).toBeUndefined();
  expect(readPoolConfigDiagnostic('{\n'+raw.slice(1))).toBeUndefined();
  expect(readPoolConfigDiagnostic(raw+' '.repeat(4096))).toBeUndefined();
  expect(readPoolConfigDiagnostic(JSON.stringify({ ...report(),message: { nested: 'not text' } }))).toBeUndefined();
  for(const patch of [{ code: 'output_unconfirmed',scope_index: 0,field: 'output',input_sha256: '' },{ code: 'input_invalid',scope_index: 0,field: 'input',input_sha256: '' },{ code: 'directory_unavailable',scope_index: 1,field: 'worktrees' },{ code: 'workspace_conflict',scope_index: 1,field: 'workspace' }])
    expect(readPoolConfigDiagnostic(JSON.stringify({ ...report(),...patch }))).toBeDefined();
});
it('T211 binds the exact current compact input and literal output, preserving limited global reports without canonical or alternate digests',async () => {
  vi.stubGlobal('crypto',webcrypto);
  const raw=JSON.stringify(report());
  expect(await analyzePoolConfigDiagnostic(raw,input)).toMatchObject({ state: 'matched',binding: 'input_and_output',report: { scope_index: 2,field: 'commands_file' } });
  for(const changed of [{ ...input,output: input.output+'/../different.json' },{ ...input,slots: 0 },{ ...input,scopes: input.scopes.slice(0,1) },{ ...input,scopes: input.scopes.map((row,index) => index===1? { ...row,commands_file: '/srv/edited.json' }:row) }])
    expect(await analyzePoolConfigDiagnostic(raw,changed)).toEqual({ state: 'mismatch' });
  const encoded=request().replace(/[<&]/g,value => value==='<'? '\\u003c':'\\u0026');
  expect(encoded).not.toBe(request());
  expect(await analyzePoolConfigDiagnostic(JSON.stringify({ ...report(),input_sha256: sha(encoded) }),input)).toEqual({ state: 'mismatch' });
  for(const field of ['input','output']) {
    const imported=JSON.stringify({ ...report(),code: field==='input'? 'input_invalid':'output_exists',scope_index: 0,field,input_sha256: '' });
    expect(await analyzePoolConfigDiagnostic(imported,input)).toMatchObject({ state: 'matched',binding: 'output_only' });
    expect(await analyzePoolConfigDiagnostic(imported,{ ...input,output: '/another-output' })).toEqual({ state: 'mismatch' });
  }
  expect(await analyzePoolConfigDiagnostic(JSON.stringify({ ...report(),code: 'cancelled',scope_index: 0,field: 'input',input_sha256: 'a'.repeat(64) }),input)).toEqual({ state: 'mismatch' });
  vi.stubGlobal('crypto',undefined);
  expect(await analyzePoolConfigDiagnostic(raw,input)).toEqual({ state: 'crypto_unavailable' });
});
