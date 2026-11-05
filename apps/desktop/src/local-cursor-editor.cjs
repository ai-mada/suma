'use strict';

const fs = require('node:fs');
const path = require('node:path').win32;
const { execFile: executeFile, spawn: spawnProcess } = require('node:child_process');
const { canonicalLocalPath } = require('./workspace-grants.cjs');

// Exact machine installer record. A missing or invalid record falls back to
// the conventional native installation roots without consulting PATH wrappers.
const installerKey = 'HKLM\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\{D7D7D7D7-7D7D-7D7D-7D7D-7D7D7D7D7D7D}}_is1';
const editorEnvironmentNames = new Set([
  'appdata', 'localappdata', 'userprofile', 'homedrive', 'homepath', 'systemdrive',
  'systemroot', 'windir', 'programdata', 'programfiles', 'programfiles(x86)', 'programw6432',
  'temp', 'tmp', 'path', 'pathext', 'comspec', 'username', 'computername',
  'processor_architecture', 'number_of_processors', 'lang', 'language', 'lc_all',
]);
const failure = code => ({ status: 'failed', code });

function ordinaryPath(value) {
  if (typeof value !== 'string' || !/^[A-Za-z]:[\\/]/.test(value) ||
      /[<>:"|?*]/.test(value.slice(2)) ||
      value.slice(3).split(/[\\/]/).some(part => /[. ]$/.test(part))) return undefined;
  try {
    const normalized = canonicalLocalPath(value, 'win32');
    return normalized === path.parse(normalized).root ? normalized : normalized.replace(/\\+$/, '');
  } catch { return undefined; }
}

function localFact(value, directory) {
  const file = ordinaryPath(value);
  if (!file) return undefined;
  try {
    const ancestors = [];
    for (let current = file; ; current = path.dirname(current)) {
      ancestors.push(current);
      if (current === path.dirname(current)) break;
    }
    const root = path.parse(file).root;
    if (canonicalLocalPath(fs.realpathSync(root), 'win32').toLowerCase() !== root.toLowerCase()) return undefined;
    let identity;
    // Check parents first so a junction cannot redirect inspection of a leaf.
    for (const current of ancestors.reverse()) {
      const stat = fs.lstatSync(current, { bigint: true });
      if (stat.isSymbolicLink() || ((directory || current !== file) ? !stat.isDirectory() : !stat.isFile())) return undefined;
      if (current === file) identity = stat;
    }
    const real = canonicalLocalPath(fs.realpathSync(file), 'win32');
    return real.toLowerCase() === file.toLowerCase() ? { file, identity } : undefined;
  } catch { return undefined; }
}

function unchanged(original, directory) {
  const next = localFact(original.file, directory)?.identity;
  const before = original.identity;
  return !!next && before.dev === next.dev && before.ino === next.ino &&
    (directory || (before.size === next.size && before.mtimeNs === next.mtimeNs));
}

function inWorkspace(workspace, candidate) {
  const relative = path.relative(workspace, candidate);
  return relative === '' || (!path.isAbsolute(relative) && relative !== '..' && !relative.startsWith('..\\'));
}

function installLocation(output) {
  if (typeof output !== 'string' || output.includes('\uFFFD')) return undefined;
  const values = new Map();
  for (const line of output.split(/\r?\n/)) {
    const match = /^\s*(DisplayName|Publisher|InstallLocation)\s+REG_SZ\s+(.+?)\s*$/.exec(line);
    if (!match) continue;
    if (values.has(match[1])) return undefined;
    values.set(match[1], match[2]);
  }
  if (values.get('DisplayName') !== 'Cursor' || values.get('Publisher') !== 'Anysphere') return undefined;
  const directory = ordinaryPath(values.get('InstallLocation'));
  return directory && path.basename(directory).toLowerCase() === 'cursor' ? directory : undefined;
}

function machineInstall({ env, roots, workspace, execFile }) {
  const systemRoot = ordinaryPath(roots.systemroot);
  const executable = systemRoot && path.join(systemRoot, 'System32', 'reg.exe');
  if (!executable || inWorkspace(workspace, executable)) return undefined;
  const registry = localFact(executable, false);
  if (!registry) return undefined;
  return new Promise(resolve => {
    try {
      execFile(registry.file, ['query', installerKey], {
        cwd: path.dirname(registry.file), env, shell: false, windowsHide: true,
        timeout: 1800, maxBuffer: 32768, encoding: 'utf8',
      }, (error, stdout) => resolve(error ? undefined : installLocation(stdout)));
    } catch { resolve(undefined); }
  });
}

function nativeEditor(directories, workspace) {
  for (const directory of directories) {
    if (!directory) continue;
    const executable = path.join(directory, 'Cursor.exe');
    if (inWorkspace(workspace, executable)) continue;
    const editor = localFact(executable, false);
    if (editor) return editor;
  }
  return undefined;
}

function requestWindow(spawn, editor, workspace, env) {
  return new Promise(resolve => {
    try {
      const child = spawn(editor, ['--classic', '--new-window', workspace], {
        cwd: path.dirname(editor), env, shell: false, windowsHide: false, detached: true, stdio: 'ignore',
      });
      child.once('error', () => resolve(failure('editor_launch_failed')));
      child.once('spawn', () => {
        child.unref();
        resolve({ status: 'requested' });
      });
    } catch { resolve(failure('editor_launch_failed')); }
  });
}

/** Launches the native editor for an already-authorized workspace. The caller
 * owns its session/binding checks and supplies a fresh guard before spawning.
 */
function createLocalCursorEditor({
  environment = process.env, platform = process.platform, execFile = executeFile, spawn = spawnProcess,
} = {}) {
  const env = {};
  const roots = {};
  for (const [name, value] of Object.entries(environment)) {
    const lower = name.toLowerCase();
    if (editorEnvironmentNames.has(lower) && typeof value === 'string') {
      env[name] = value;
      roots[lower] = value;
    }
  }
  return async (repositoryPath, authorizeLaunch) => {
    if (platform !== 'win32') return failure('editor_unavailable');
    const workspace = localFact(repositoryPath, true);
    if (!workspace) return failure('editor_path_invalid');
    const registered = await machineInstall({ env, roots, workspace: workspace.file, execFile });
    const localAppData = ordinaryPath(roots.localappdata);
    const programRoots = [roots.programw6432, roots.programfiles, roots['programfiles(x86)']]
      .map(ordinaryPath).filter(Boolean);
    const editor = nativeEditor([
      registered, localAppData && path.join(localAppData, 'Programs', 'cursor'),
      ...programRoots.map(root => path.join(root, 'cursor')),
    ], workspace.file);
    if (!editor) return failure('editor_unavailable');
    try {
      if (typeof authorizeLaunch !== 'function' || await authorizeLaunch() !== true) return failure('access_changed');
    } catch { return failure('access_changed'); }
    if (!unchanged(workspace, true)) return failure('editor_path_invalid');
    if (!unchanged(editor, false)) return failure('editor_unavailable');
    return requestWindow(spawn, editor.file, workspace.file, env);
  };
}

module.exports = { createLocalCursorEditor };
