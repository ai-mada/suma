'use strict';

const { EventEmitter } = require('node:events');
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { createLocalCursorEditor } = require('../src/local-cursor-editor.cjs');

function setup(t) {
  const temporary = fs.realpathSync(os.tmpdir());
  const root = fs.mkdtempSync(path.join(temporary, 'suma-cursor-editor-'));
  t.after(() => {
    const resolved = fs.realpathSync(root);
    assert.equal(path.dirname(resolved), temporary);
    assert.match(path.basename(resolved), /^suma-cursor-editor-/);
    fs.rmSync(resolved, { recursive: true, force: true });
  });
  const repository = path.join(root, "work O'Neil");
  fs.mkdirSync(repository);
  const environment = {
    LOCALAPPDATA: path.join(root, 'LocalAppData'),
    ProgramFiles: path.join(root, 'Program Files'),
    SYSTEMROOT: path.join(root, 'Windows'),
    APPDATA: path.join(root, 'Roaming'),
    SUMA_DESKTOP_WORKSPACE_KEY: 'suma-secret',
    NODE_OPTIONS: '--require injected',
    ELECTRON_RUN_AS_NODE: '1',
    OPENAI_API_KEY: 'provider-secret',
  };
  const userBinary = path.join(environment.LOCALAPPDATA, 'Programs', 'cursor', 'Cursor.exe');
  const registeredBinary = path.join(root, 'Alternate Programs', 'cursor', 'Cursor.exe');
  const registryBinary = path.join(environment.SYSTEMROOT, 'System32', 'reg.exe');
  for (const binary of [userBinary, registeredBinary, registryBinary]) {
    fs.mkdirSync(path.dirname(binary), { recursive: true });
    fs.writeFileSync(binary, 'fixture; spawn is intercepted');
  }
  return { root, repository, environment, userBinary, registeredBinary, registryBinary };
}

function launchRecorder(event = 'spawn') {
  const calls = [];
  let unrefs = 0;
  const spawn = (...args) => {
    calls.push(args);
    const child = new EventEmitter();
    child.unref = () => { unrefs++; };
    queueMicrotask(() => child.emit(event, event === 'error' ? Error('private diagnostic') : undefined));
    return child;
  };
  return { calls, spawn, unrefs: () => unrefs };
}

test('selects the registered native executable and starts only the authorized folder', { skip: process.platform !== 'win32' }, async t => {
  const fixture = setup(t);
  const recorder = launchRecorder();
  let registryCalls = 0;
  let authorizationCalls = 0;
  const execFile = (executable, args, options, callback) => {
    registryCalls++;
    assert.equal(executable, fixture.registryBinary);
    assert.deepEqual(args.slice(0, 1), ['query']);
    assert.match(args[1], /^HKLM\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\/);
    assert.equal(args.length, 2);
    assert.equal(options.shell, false);
    assert.ok(options.timeout <= 2000 && options.maxBuffer <= 32768);
    callback(null, '    DisplayName    REG_SZ    Cursor\r\n    Publisher    REG_SZ    Anysphere\r\n    InstallLocation    REG_SZ    ' + path.dirname(fixture.registeredBinary) + '\\\r\n');
  };
  const open = createLocalCursorEditor({ environment: fixture.environment, execFile, spawn: recorder.spawn });
  const result = await open(fixture.repository, async () => {
    authorizationCalls++;
    assert.equal(registryCalls, 1);
    assert.equal(recorder.calls.length, 0);
    return true;
  });
  assert.deepEqual(result, { status: 'requested' });
  assert.equal(authorizationCalls, 1);
  assert.equal(recorder.calls.length, 1);
  const [binary, args, options] = recorder.calls[0];
  assert.equal(binary, fixture.registeredBinary);
  assert.deepEqual(args, ['--classic', '--new-window', fixture.repository]);
  assert.equal(options.cwd, path.dirname(binary));
  assert.equal(options.shell, false);
  assert.equal(options.detached, true);
  assert.equal(options.stdio, 'ignore');
  assert.equal(options.env.APPDATA, fixture.environment.APPDATA);
  for (const name of ['SUMA_DESKTOP_WORKSPACE_KEY', 'NODE_OPTIONS', 'ELECTRON_RUN_AS_NODE', 'OPENAI_API_KEY']) {
    assert.equal(Object.hasOwn(options.env, name), false);
  }
  assert.equal(recorder.unrefs(), 1);
});

test('falls back to the standard installation and rejects stale authorization', { skip: process.platform !== 'win32' }, async t => {
  const fixture = setup(t);
  const recorder = launchRecorder();
  const execFile = (_binary, _args, _options, callback) => callback(null,
    '    DisplayName    REG_SZ    Cursor\r\n    Publisher    REG_SZ    Other\r\n    InstallLocation    REG_SZ    ' + path.dirname(fixture.registeredBinary));
  const open = createLocalCursorEditor({ environment: fixture.environment, execFile, spawn: recorder.spawn });
  assert.deepEqual(await open(fixture.repository, async () => false), { status: 'failed', code: 'access_changed' });
  assert.deepEqual(await open(fixture.repository, async () => { throw Error('private diagnostic'); }), { status: 'failed', code: 'access_changed' });
  assert.deepEqual(await open(fixture.repository), { status: 'failed', code: 'access_changed' });
  assert.equal(recorder.calls.length, 0);
  assert.deepEqual(await open(fixture.repository, async () => true), { status: 'requested' });
  assert.equal(recorder.calls[0][0], fixture.userBinary);
});

test('rejects unsafe workspaces and a candidate changed during authorization', { skip: process.platform !== 'win32' }, async t => {
  const fixture = setup(t);
  const recorder = launchRecorder();
  const open = createLocalCursorEditor({ environment: fixture.environment, execFile: (_bin, _args, _options, callback) => callback(Error()), spawn: recorder.spawn });
  for (const workspace of ['\\\\server\\share', '\\\\?\\C:\\work', fixture.repository + ':stream', fixture.repository + '.']) {
    assert.deepEqual(await open(workspace, async () => assert.fail('unsafe workspace reached authorization')), { status: 'failed', code: 'editor_path_invalid' });
  }
  assert.deepEqual(await open(fixture.repository, async () => {
    fs.appendFileSync(fixture.userBinary, 'changed');
    return true;
  }), { status: 'failed', code: 'editor_unavailable' });
  assert.equal(recorder.calls.length, 0);
});

test('refuses a workspace executable and an installation directory junction', { skip: process.platform !== 'win32' }, async t => {
  const fixture = setup(t);
  const recorder = launchRecorder();
  const programFiles = path.join(fixture.repository, 'Program Files');
  const workspaceEditor = path.join(programFiles, 'cursor', 'Cursor.exe');
  fs.mkdirSync(path.dirname(workspaceEditor), { recursive: true });
  fs.writeFileSync(workspaceEditor, 'workspace executable must not be invoked');
  const junction = path.join(fixture.root, 'linked');
  fs.symlinkSync(fixture.environment.LOCALAPPDATA, junction, 'junction');
  try {
    const open = createLocalCursorEditor({
      environment: { ProgramFiles: programFiles, LOCALAPPDATA: junction },
      spawn: recorder.spawn,
    });
    assert.deepEqual(await open(fixture.repository, async () => assert.fail('unsafe install reached authorization')), { status: 'failed', code: 'editor_unavailable' });
  } finally {
    fs.unlinkSync(junction);
  }
  assert.equal(recorder.calls.length, 0);
});

test('keeps launch errors closed and refuses platforms outside the Windows contract', async t => {
  const unsupported = createLocalCursorEditor({ platform: 'linux', spawn: () => assert.fail('unsupported platform launched') });
  assert.deepEqual(await unsupported('/tmp/repo', async () => true), { status: 'failed', code: 'editor_unavailable' });
  if (process.platform !== 'win32') return;
  const fixture = setup(t);
  const recorder = launchRecorder('error');
  const open = createLocalCursorEditor({ environment: { LOCALAPPDATA: fixture.environment.LOCALAPPDATA }, spawn: recorder.spawn });
  assert.deepEqual(await open(fixture.repository, async () => true), { status: 'failed', code: 'editor_launch_failed' });
  assert.equal(recorder.calls.length, 1);
  assert.equal(recorder.unrefs(), 0);
});
