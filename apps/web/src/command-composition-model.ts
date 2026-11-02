import { runnerSetupLiteral } from './cursor-setup-guide-model';
import { validPoolConfigPath } from './execution-pools/pool-config-preparation-model';

export type CommandCompositionInput = { shell: '' | 'powershell' | 'bash'; files: string[]; output: string };
export type CommandCompositionDraft = { kind: 'commands-only'; shell: 'powershell' | 'bash'; commands: string };
export type CommandCompositionIssue = {
  code: 'shell' | 'count' | 'source_path' | 'duplicate_source' | 'output_path' | 'output_source' | 'too_large';
  field: 'shell' | 'files' | 'output';
  row?: number;
};
const object = (value: unknown): value is Record<string, unknown> =>
  !!value && typeof value === 'object' && !Array.isArray(value) &&
  [Object.prototype, null].includes(Object.getPrototypeOf(value));

/** Reuse the original native-reference boundary. A target shell is not an OS
 * detector; only the native composer can inspect actual files and links. */
export function validCommandCompositionFile(value: unknown): value is string {
  return validPoolConfigPath(value) && !/[\\/]$/.test(value);
}
// Guided outputs must be accepted by the original JSON connection flow.
export function validCommandCompositionOutput(value: unknown): value is string {
  return validCommandCompositionFile(value) && /\.json$/i.test(value);
}
/** Path adoption carries no adapter, installation or successful-write proof. */
export function validCommandCompositionDraft(value: unknown): value is CommandCompositionDraft {
  return object(value) && Reflect.ownKeys(value).length === 3 &&
    ['kind', 'shell', 'commands'].every(key => Object.hasOwn(value, key)) &&
    value.kind === 'commands-only' && (value.shell === 'powershell' || value.shell === 'bash') &&
    validCommandCompositionOutput(value.commands);
}
export function commandCompositionIssue(value: unknown): CommandCompositionIssue | undefined {
  if (!object(value) || Reflect.ownKeys(value).length !== 3 ||
      !['shell', 'files', 'output'].every(key => Object.hasOwn(value, key)) ||
      value.shell !== 'powershell' && value.shell !== 'bash') return { code: 'shell', field: 'shell' };
  if (!Array.isArray(value.files) || value.files.length < 2 || value.files.length > 16)
    return { code: 'count', field: 'files' };
  const sources = new Set<string>();
  for (const [index, file] of value.files.entries()) {
    if (!validCommandCompositionFile(file)) return { code: 'source_path', field: 'files', row: index + 1 };
    if (sources.has(file)) return { code: 'duplicate_source', field: 'files', row: index + 1 };
    sources.add(file);
  }
  if (!validCommandCompositionOutput(value.output)) return { code: 'output_path', field: 'output' };
  if (sources.has(value.output)) return { code: 'output_source', field: 'output' };
  // PowerShell adds CRLF to its single JSON string; account for the actual
  // framing consumed by the native bounded-stdin decoder.
  const bytes = new TextEncoder().encode(JSON.stringify({ files: value.files })).length;
  if (bytes + (value.shell === 'powershell' ? 2 : 0) > 65536) return { code: 'too_large', field: 'files' };
}

/** Only file references enter this closed request. Shell quoting reuses the
 * original builder; creating or copying a command does not execute it. */
export function commandCompositionCommands(value: unknown): { request: { files: string[] }; prepare: string } | undefined {
  if (commandCompositionIssue(value)) return;
  const input = value as CommandCompositionInput, shell = input.shell as 'powershell' | 'bash';
  const request = { files: [...input.files] }, json = runnerSetupLiteral(JSON.stringify(request), shell);
  const runner = 'suma-runner --compose-commands ' + runnerSetupLiteral(input.output, shell);
  const prepare = shell === 'powershell'
    ? '& {\n  $commandCompositionPreviousEncoding = $OutputEncoding\n  try {\n    $OutputEncoding = [System.Text.UTF8Encoding]::new($false)\n    ' + json + ' | ' + runner + '\n  } finally {\n    $OutputEncoding = $commandCompositionPreviousEncoding\n  }\n}'
    : "printf '%s' " + json + ' | ' + runner;
  return { request, prepare };
}
