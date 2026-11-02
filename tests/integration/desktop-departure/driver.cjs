'use strict';
const fs = require('node:fs');
const path = require('node:path');
const http = require('node:http');
const net = require('node:net');
const crypto = require('node:crypto');
const assert = require('node:assert/strict');
const {closeOwnedNative} = require('../support/close-native.cjs');

const root = path.resolve(__dirname, '../../..');
const directory = path.join(root, '.cache/team-workstation-recovery-218/desktop-actual', new Date().toISOString().replace(/[^0-9TZ]/g, ''));
const hash = value => crypto.createHash('sha256').update(value).digest('hex');
const markerKey = 'suma-departure-fixture-original';
const result = {stage: 'T-TEAM-WORKSTATION-RECOVERY-218', passed: false,
  scope: 'Real BrowserWindow/events and production departure controller only; injected dialog facade simulates choices, not OS button clicks. No production main drain, Worker, account, model or business execution.',
  groups: {}, requests: [], images: [], page_errors: [], console_errors: [], console_warnings: [], observed_protocol_dialogs: []};
fs.mkdirSync(directory, {recursive: true});

async function bounded(promise, milliseconds, label) {
  let timer;
  try { return await Promise.race([promise, new Promise((_, reject) => { timer = setTimeout(() => reject(Error(label)), milliseconds); })]); }
  finally { clearTimeout(timer); }
}
async function portClosed(port) {
  return new Promise(resolve => {
    const socket = net.connect({host: '127.0.0.1', port});
    socket.once('connect', () => { socket.destroy(); resolve(false); });
    socket.once('error', error => resolve(error.code === 'ECONNREFUSED'));
    socket.setTimeout(1000, () => { socket.destroy(); resolve(false); });
  });
}
async function main() {
  let server, app, nativeExit;
  const save = () => fs.writeFileSync(path.join(directory, 'result.json'), JSON.stringify(result, null, 2) + '\n');
  try {
    server = http.createServer((request, response) => {
      const url = new URL(request.url, 'http://127.0.0.1');
      result.requests.push({method: request.method, path: url.pathname});
      if (request.method !== 'GET') { response.writeHead(405); response.end(); return; }
      response.setHeader('Cache-Control', 'no-store');
      response.setHeader('Content-Security-Policy', "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'none'; img-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'");
      if (url.pathname === '/favicon.ico') { response.writeHead(204); response.end(); return; }
      if (url.pathname === '/fixture.css') {
        response.writeHead(200, {'Content-Type': 'text/css'});
        response.end('body{font:16px system-ui,sans-serif;background:#f5f6fa;color:#172033;margin:0;padding:40px}main{max-width:900px;margin:auto;background:white;border:1px solid #d9deea;border-radius:12px;padding:28px}.eyebrow{color:#6555be}h1{font-size:26px}dt{font-weight:650;margin-top:20px}dd{margin:8px 0;overflow-wrap:anywhere}pre{white-space:pre-wrap;padding:16px;background:#f5f6fa}button,a{display:inline-flex;align-items:center;min-height:44px;padding:0 16px;margin:12px 12px 0 0;border:1px solid #c8c1e8;border-radius:8px;color:#51419b;background:white;font:inherit}button{cursor:pointer}');
        return;
      }
      const name = ['/work', '/destination'].includes(url.pathname) ? 'index.html' : url.pathname === '/script.js' ? 'script.js' : undefined;
      if (!name) { response.writeHead(404); response.end(); return; }
      response.writeHead(200, {'Content-Type': name.endsWith('.js') ? 'text/javascript' : 'text/html'});
      response.end(fs.readFileSync(path.join(__dirname, name)));
    });
    await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
    result.port = server.address().port;
    const origin = 'http://127.0.0.1:' + result.port;
    const {_electron} = require(process.env.SUMA_PLAYWRIGHT_MODULE);
    const env = {};
    for (const [key, value] of Object.entries(process.env)) {
      if (['path','systemroot','windir','comspec','pathext','home','userprofile','homedrive','homepath','appdata','localappdata','programdata','programfiles','programfiles(x86)','programw6432'].includes(key.toLowerCase())) env[key] = value;
    }
    Object.assign(env, {TEMP: path.join(directory, 'tmp'), TMP: path.join(directory, 'tmp'),
      SUMA_FIXTURE_ORIGIN: origin, SUMA_FIXTURE_DIRECTORY: directory});
    fs.mkdirSync(env.TEMP, {recursive: true});
    app = await _electron.launch({executablePath: path.join(root, 'apps/desktop/node_modules/electron/dist/electron.exe'),
      args: [path.join(__dirname, 'main.cjs')], cwd: root, env, timeout: 15000});
    result.native_pid = app.process().pid;
    nativeExit = new Promise(resolve => app.process().once('exit', (code, signal) => resolve({code, signal})));
    save();
    const page = await app.firstWindow();
    page.setDefaultTimeout(10000);
    page.on('pageerror', error => result.page_errors.push(error.message));
    page.on('console', message => {
      if (message.type() === 'error') result.console_errors.push(message.text());
      if (message.type() === 'warning') result.console_warnings.push(message.text());
    });
    // Observe only. With no listener Playwright otherwise tries to handle the
    // transient CDP dialog at the wrong layer. Production main remains the
    // sole Electron will-prevent-unload decision maker; no accept or dismiss.
    page.on('dialog', dialog => { result.observed_protocol_dialogs.push({type: dialog.type()}); save(); });
    const action = (name, ...args) => app.evaluate((_, input) => globalThis.__departure218[input.name](...input.args), {name, args});
    const snapshot = () => action('snapshot');
    async function poll(read, predicate, label) {
      const deadline = Date.now() + 5000;
      do {
        const value = await read();
        if (predicate(value)) return value;
        await new Promise(resolve => setTimeout(resolve, 20));
      } while (Date.now() < deadline);
      throw Error(label);
    }
    const renderer = () => page.evaluate(key => ({marker: sessionStorage.getItem(key), loads: Number(sessionStorage.getItem('suma-departure-fixture-loads')), url: location.href}), markerKey);
    await page.locator('#activate').waitFor();
    assert.equal((await snapshot()).versions.electron, '35.7.5');
    const original = await renderer();
    result.original_marker_sha256 = hash(original.marker);
    async function unchanged() {
      const current = await renderer();
      assert.equal(current.marker, original.marker);
      assert.equal(current.loads, original.loads);
      assert.equal(current.url, original.url);
      assert.equal((await snapshot()).alive, true);
    }
    function validOptions(record, locale) {
      assert.equal(record.same_owner, true);
      assert.equal(record.options.defaultId, 0);
      assert.equal(record.options.cancelId, 0);
      assert.deepEqual(record.options.buttons, locale === 'zh-CN' ? ['留下', '离开'] : ['Stay', 'Leave']);
      assert.ok(record.options.title && record.options.message && record.options.detail);
    }
    async function pendingAfter(count) {
      const state = await poll(snapshot, value => value.decisions.length > count, 'Native facade did not receive a departure decision');
      const last = state.decisions.at(-1);
      assert.equal(last.kind, 'async');
      return last;
    }
    async function group(name, run) {
      result.groups[name] = {passed: false};
      save();
      try { await run(result.groups[name]); result.groups[name].passed = true; }
      catch (error) { result.groups[name].error = String(error.stack || error); await page.screenshot({path: path.join(directory, name + '-failure.png')}).catch(() => {}); throw error; }
      finally { save(); }
    }
    async function image(name) {
      await page.screenshot({path: path.join(directory, name), fullPage: false});
      result.images.push({path: name, sha256: hash(fs.readFileSync(path.join(directory, name)))});
    }
    await group('known_close_and_quit_stay_preserve_window', async facts => {
      await page.locator('#activate').click();
      await action('close');
      let decision = await pendingAfter(0);
      validOptions(decision, 'zh-CN');
      await action('answer', decision.id, 0);
      await unchanged();
      await action('quit');
      decision = await pendingAfter(1);
      validOptions(decision, 'zh-CN');
      await action('answer', decision.id, 0);
      await unchanged();
      facts.close_and_controller_quit_choices = [0, 0];
      await image('cn-stay-original-marker.png');
    });
    await group('source_aba_rejects_late_leave', async facts => {
      const before = await snapshot();
      await action('close');
      const decision = await pendingAfter(before.decisions.length);
      await page.evaluate(() => { location.hash = 'new-source'; });
      await page.waitForURL(original.url + '#new-source');
      await page.evaluate(url => history.replaceState(null, '', url), original.url);
      await page.waitForURL(original.url);
      const changed = await poll(snapshot, value => value.decisions.at(-1).aborted, 'Actual source change did not abort the old decision');
      const closeCount = changed.events.filter(event => event.kind === 'close').length;
      await action('answer', decision.id, 1);
      await unchanged();
      const after = await snapshot();
      assert.equal(after.events.filter(event => event.kind === 'close').length, closeCount);
      facts.aborted = true; facts.same_url_after_aba = original.url; facts.old_leave_replayed = false;
    });
    await group('exact_same_origin_navigation_leave', async facts => {
      const before = await snapshot();
      await action('sync', 1);
      await page.locator('#navigate').click({noWaitAfter: true});
      const state = await poll(snapshot, value => value.decisions.length > before.decisions.length, 'Navigation did not reach the production controller');
      const decision = state.decisions.at(-1);
      validOptions(decision, 'zh-CN');
      if (decision.kind === 'async') await action('answer', decision.id, 1);
      await page.waitForURL(origin + '/destination');
      await page.locator('#activate').waitFor();
      assert.equal((await renderer()).marker, original.marker);
      facts.branch = decision.kind === 'sync' ? 'official synchronous fallback; destination not exposed before unload' : 'captured exact asynchronous intent';
      facts.destination = page.url();
    });
    await group('english_unknown_reload_sync_stay_and_leave', async facts => {
      await action('locale', 'en-US');
      await page.locator('#activate').click();
      const before = await renderer(), state = await snapshot();
      await action('sync', 0);
      await action('reload');
      let current = await poll(snapshot, value => value.decisions.length > state.decisions.length, 'Reload Stay did not reach the native controller');
      let decision = current.decisions.at(-1);
      validOptions(decision, 'en-US');
      assert.equal(decision.kind, 'sync');
      assert.deepEqual(await renderer(), before);
      await action('sync', 1);
      const count = current.decisions.length;
      await action('reload');
      current = await poll(snapshot, value => value.decisions.length > count, 'Reload Leave did not reach the native controller');
      decision = current.decisions.at(-1);
      validOptions(decision, 'en-US');
      assert.equal(decision.kind, 'sync');
      await page.waitForFunction(loads => Number(sessionStorage.getItem('suma-departure-fixture-loads')) > loads, before.loads);
      assert.equal((await renderer()).marker, original.marker);
      facts.mode = 'official synchronous fallback for unknown reload';
      facts.marker_preserved = true;
      await image('en-reload-original-marker.png');
    });
    await group('final_explicit_leave_closes_normally', async facts => {
      await page.locator('#activate').click();
      assert.equal((await renderer()).marker, original.marker);
      const state = await snapshot();
      await action('close');
      const decision = await pendingAfter(state.decisions.length);
      validOptions(decision, 'en-US');
      result.decisions_before_final_leave = (await snapshot()).decisions;
      try { await action('answer', decision.id, 1); }
      catch (error) { result.final_answer_transport = String(error); }
      const exit = await bounded(nativeExit, 10000, 'Explicit current Leave did not close the owned native host');
      assert.deepEqual(exit, {code: 0, signal: null});
      facts.native_exit = exit;
    });
    assert.ok(result.requests.every(request => request.method === 'GET'));
    assert.deepEqual(result.page_errors, []);
    assert.deepEqual(result.console_errors, []);
    result.checks_passed = true;
  } finally {
    if (app) {
      if (app.process().exitCode !== null) { result.forced = false; result.native_exit = await nativeExit; }
      else Object.assign(result, await closeOwnedNative(app, nativeExit));
    }
    if (server) { await new Promise(resolve => server.close(resolve)); result.port_closed = await portClosed(result.port); }
    save();
  }
  assert.equal(result.forced, false);
  assert.deepEqual(result.native_exit, {code: 0, signal: null});
  assert.equal(result.port_closed, true);
  result.passed = true;
  save();
  console.log(JSON.stringify({directory: path.relative(root, directory), passed: true, groups: Object.keys(result.groups), images: result.images,
    native_exit: result.native_exit, forced: result.forced, port_closed: result.port_closed}));
}
main().catch(error => { console.error(error.stack || String(error)); process.exitCode = 1; });
