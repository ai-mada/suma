import { expect, it } from 'vitest';
import { commandCompositionCommands, commandCompositionIssue, validCommandCompositionFile, validCommandCompositionDraft } from './command-composition-model';

it('T215 builds only closed bounded file-reference stdin with native path checks and literal quoting for both shells', () => {
  const input = { shell: 'powershell', files: ["D:/tools/O'Brien $(Write-Output unexpected).json", '/srv/配置/checks:local.json'], output: 'D:/new/combined.json' };
  const result = commandCompositionCommands(input)!;
  expect(result.request).toEqual({ files: input.files }); expect(Object.keys(result.request)).toEqual(['files']);
  expect(result.request.files).not.toBe(input.files);
  expect(result.prepare).toContain("O''Brien $(Write-Output unexpected).json");
  expect(result.prepare).toContain("suma-runner --compose-commands 'D:/new/combined.json'");
  expect(result.prepare).toContain('try {'); expect(result.prepare).toContain('finally {');
  expect(result.prepare).toContain('$OutputEncoding = $commandCompositionPreviousEncoding');
  expect(result.prepare).not.toMatch(/--allow-exec|--report-tools|--control|--state|Invoke-Expression/);
  const bash = commandCompositionCommands({ ...input, shell: 'bash' })!;
  expect(bash.prepare).toMatch(/^printf '%s' /); expect(bash.prepare).toContain("O'\"'\"'Brien");
  for (const value of [{ ...input, extra: true }, { ...input, shell: { toString: () => 'powershell' } }, { ...input, files: null }, { ...input, files: ['/one'] }, { ...input, files: Array(17).fill('/file') }, { ...input, output: 'relative.json' }]) expect(commandCompositionCommands(value)).toBeUndefined();
  for (const path of ['relative.json', 'C:relative.json', '\\\\server\\share\\file.json', '\\\\?\\C:\\file.json', 'D:/file.json:stream', 'D:/alias./file', 'D:/alias /file', '/srv/../file', '/srv/.', '/srv/file/', '/srv/file\n--allow-exec']) expect(validCommandCompositionFile(path), path).toBe(false);
  expect(validCommandCompositionFile('/srv/valid:name.json')).toBe(true);
  expect(commandCompositionIssue({ ...input, files: ['/first', 'relative'] })).toEqual({ code: 'source_path', field: 'files', row: 2 });
  expect(commandCompositionIssue({ ...input, files: ['/first', '/first'] })).toEqual({ code: 'duplicate_source', field: 'files', row: 2 });
  expect(commandCompositionIssue({ ...input, output: input.files[0] })).toEqual({ code: 'output_source', field: 'output' });
  expect(commandCompositionIssue({ ...input, output: '/srv/combined.conf' })).toEqual({ code: 'output_path', field: 'output' });
  const draft = { kind: 'commands-only', shell: 'powershell', commands: input.output };
  expect(validCommandCompositionDraft(draft)).toBe(true);
  for (const value of [{ ...draft, allow_exec: true }, { ...draft, kind: 'opencode-sdk' }, { ...draft, shell: '' }, { ...draft, commands: 'relative.json' }, { ...draft, commands: '/srv/combined.conf' }, null]) expect(validCommandCompositionDraft(value)).toBe(false);
  const boundedFiles = (bytes: number) => {
    const files = Array.from({ length: 16 }, (_, index) => '/' + index.toString(16).padStart(2, '0') + 'a'.repeat(4093));
    const surplus = Buffer.byteLength(JSON.stringify({ files }), 'utf8') - bytes;
    files[15] = files[15].slice(0, -surplus);
    expect(Buffer.byteLength(JSON.stringify({ files }), 'utf8')).toBe(bytes);
    return files;
  };
  expect(commandCompositionCommands({ ...input, files: boundedFiles(65534) })).toBeDefined();
  expect(commandCompositionIssue({ ...input, files: boundedFiles(65535) })?.code).toBe('too_large');
  expect(commandCompositionCommands({ ...input, shell: 'bash', files: boundedFiles(65536) })).toBeDefined();
});
