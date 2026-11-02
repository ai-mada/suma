'use strict';
// Production two-field preparation -> exact copied shell -> original CLI.
// Only the fixed Codex passive metadata probe may run; no login or model turn.
const fs = require('node:fs'), path = require('node:path'), http = require('node:http');
const cp = require('node:child_process'), net = require('node:net'), crypto = require('node:crypto'), assert = require('node:assert/strict');
const {closeOwnedNative} = require('../support/close-native.cjs');
const root = path.resolve(__dirname, '../../..'), base = path.join(root, '.cache/local-tool-preparation-220/actual'), out = path.join(base, 'out');
const directory = path.join(base, 'visual', new Date().toISOString().replace(/[^0-9TZ]/g, ''));
const hash = bytes => crypto.createHash('sha256').update(bytes).digest('hex');
const project = 'd'.repeat(32), tenant = 'a'.repeat(32);
const session = {user: {id: 'fixture-owner-220', name: 'Fixture owner', email: 'owned@example.test', role: 'owner'}, organization: {id: tenant, name: 'Fixture organization'}, projects: []};
const result = {stage: 'T-LOCAL-TOOL-PREPARATION-220', scope: 'Production Guide/card/Remote draft adoption and original compiled CLI; synthetic public view, no platform backend or registration', browser_path: 'Installed isolated Electron/Playwright harness; active application browser not exercised', groups: {}, requests: [], unexpected: [], cli: [], images: [], page_errors: [], console_errors: [], console_warnings: [], passed: false};
fs.mkdirSync(directory, {recursive: true});
const save = () => fs.writeFileSync(path.join(directory, 'result.json'), JSON.stringify(result, null, 2) + '\n');
const newDirectory = name => {const value = path.join(directory, name); fs.mkdirSync(value, {recursive: true}); return value;};
/** A fresh process environment excludes API keys, login paths and the user's
 * normal Windows/XDG homes. Nothing changes machine or user environment. */
function isolatedEnvironment(name, searchPath) {
  const home = newDirectory(name), env = {};
  for (const key of ['SystemRoot', 'WINDIR', 'ComSpec', 'PATHEXT']) if (process.env[key]) env[key] = process.env[key];
  Object.assign(env, {PATH: searchPath, HOME: home, USERPROFILE: home,
    APPDATA: newDirectory(name + '/appdata'), LOCALAPPDATA: newDirectory(name + '/localappdata'),
    XDG_CONFIG_HOME: newDirectory(name + '/xdg-config'), XDG_DATA_HOME: newDirectory(name + '/xdg-data'), XDG_STATE_HOME: newDirectory(name + '/xdg-state'),
    TEMP: newDirectory(name + '/tmp'), TMP: newDirectory(name + '/tmp')});
  return env;
}
function closedPort(port) {return new Promise(resolve => {const socket = net.connect({host: '127.0.0.1', port}); socket.once('connect', () => {socket.destroy(); resolve(false);}); socket.once('error', error => resolve(error.code === 'ECONNREFUSED')); socket.setTimeout(1000, () => {socket.destroy(); resolve(false);});});}

async function main() {
  let server, app, exit, page;
  try {
    const native = process.env.SUMA_NATIVE_CODEX, powershell = process.env.SUMA_PWSH_PATH;
    assert.ok(native && path.isAbsolute(native) && fs.statSync(native).isFile(), 'Explicit installed Codex native required; do not fake vendor coverage');
    assert.ok(powershell && path.isAbsolute(powershell) && fs.statSync(powershell).isFile());
    const binary = path.join(base, 'bin/suma-runner.exe'); assert.ok(fs.statSync(binary).isFile());
    result.binary = {path: path.relative(root, binary), sha256: hash(fs.readFileSync(binary))};
    result.native = {path: native, sha256: hash(fs.readFileSync(native)), purpose: 'Original fixed passive version/help proof only; not model execution'};
    const destination = path.join(newDirectory('machine files'), "新 code's $native.json");
    server = http.createServer((request, response) => {
      const url = new URL(request.url, 'http://127.0.0.1'), route = url.pathname.replace(/^\/api\/v1/, ''); result.requests.push({method: request.method, path: url.pathname, query: url.search});
      const send = body => {response.writeHead(200, {'Content-Type': 'application/json', 'Cache-Control': 'no-store'}); response.end(JSON.stringify(body));};
      if (request.method === 'GET' && url.pathname === '/__fixture/view') return send({session, project_id: project});
      if (request.method === 'GET' && route === '/adapter-catalog') return send({project_id: project, as_of: new Date().toISOString(), items: []});
      if (request.method === 'GET' && route === '/tool-profiles') return send({items: [], total: 0});
      if (url.pathname === '/favicon.ico') {response.writeHead(204); response.end(); return;}
      const file = {'/': 'index.html', '/fixture.js': 'fixture.js', '/fixture.css': 'fixture.css'}[url.pathname];
      if (request.method !== 'GET' || !file) {result.unexpected.push(request.method + ' ' + route); response.writeHead(404); response.end(); return;}
      response.writeHead(200, {'Content-Type': file.endsWith('.js') ? 'text/javascript' : file.endsWith('.css') ? 'text/css' : 'text/html', 'Cache-Control': 'no-store'}); response.end(fs.readFileSync(path.join(out, file)));
    });
    await new Promise(resolve => server.listen(0, '127.0.0.1', resolve)); result.port = server.address().port; const origin = 'http://127.0.0.1:' + result.port; save();
    const {_electron} = require(process.env.SUMA_PLAYWRIGHT_MODULE);
    const system = process.env.SystemRoot || 'C:/Windows', trustedPath = [path.join(system, 'System32'), system, path.join(system, 'System32/Wbem')].join(';');
    const env = isolatedEnvironment('browser-home', trustedPath);
    Object.assign(env, {SUMA_FIXTURE_ORIGIN: origin, SUMA_FIXTURE_DIRECTORY: directory, SUMA_FIXTURE_PARTITION: 'suma-fixture-local-preparation220'});
    app = await _electron.launch({executablePath: path.join(root, 'apps/desktop/node_modules/electron/dist/electron.exe'), args: [path.join(__dirname, '../support/native.cjs')], cwd: root, env, timeout: 15000});
    result.native_pid = app.process().pid; exit = new Promise(resolve => app.process().once('exit', (code, signal) => resolve({code, signal}))); save();
    page = await app.firstWindow(); page.setDefaultTimeout(12000);
    page.on('pageerror', error => result.page_errors.push(error.message));
    page.on('console', message => {if (message.type() === 'error') result.console_errors.push(message.text()); else if (message.type() === 'warning') result.console_warnings.push(message.text());});
    await page.addInitScript(() => {
      globalThis.__ownedClipboard = {calls: [], hold: false};
      Object.defineProperty(navigator, 'clipboard', {configurable: true, value: {writeText: value => {
        globalThis.__ownedClipboard.calls.push(value);
        return globalThis.__ownedClipboard.hold ? new Promise(resolve => {globalThis.__ownedClipboard.release = resolve;}) : Promise.resolve();
      }}});
    });
    const card = () => page.locator('[data-local-tool-preparation]');
    async function visit(locale, width, theme, mode = 'ordinary') {
      await page.setViewportSize({width, height: width === 390 ? 844 : 960}); await page.goto(origin + '/?' + new URLSearchParams({locale, theme, mode}));
      await page.locator('[data-local-tool-preparation][data-local-tool=codex-jsonl]').waitFor();
    }
    async function fill(shell, output) {await page.locator('[data-local-tool-shell]').selectOption(shell); await page.locator('[data-local-tool-output]').fill(output);}
    async function capture(name) {await card().scrollIntoViewIfNeeded(); await page.screenshot({path: path.join(directory, name), fullPage: false}); result.images.push({path: name, sha256: hash(fs.readFileSync(path.join(directory, name)))});}
    async function geometry() {const value = await card().evaluate(node => ({width: innerWidth, scroll: document.documentElement.scrollWidth, controls: [...node.querySelectorAll('input,select,button')].map(control => control.getBoundingClientRect().height)})); assert.ok(value.scroll <= value.width); assert.ok(value.controls.every(height => height >= 43.5)); return value;}
    async function group(name, run) {
      result.groups[name] = {passed: false};
      try {await run(result.groups[name]); result.groups[name].passed = true;}
      catch (error) {result.groups[name].error = String(error.stack || error); if (page) {await page.screenshot({path: path.join(directory, name + '-failure.png')}).catch(() => {}); result.failure_text = await page.locator('body').innerText().catch(() => '');} throw error;}
      finally {save();}
    }
    await group('cn_two_fields_original_cli_new_file_refuse_overwrite_and_ordinary_adoption', async facts => {
      await visit('zh-CN', 1366, 'light'); await fill('powershell', destination);
      await page.locator('[data-local-tool-copy]').click(); await page.locator('[data-local-tool-status=copied]').waitFor();
      const command = await page.locator('[data-local-tool-preview]').textContent(); assert.equal(await page.evaluate(() => globalThis.__ownedClipboard.calls.at(-1)), command); facts.geometry = await geometry(); await capture('cn-local-tool-preparation.png');
      const cliEnv = isolatedEnvironment('cli-home', [path.dirname(binary), path.dirname(native), trustedPath].join(';'));
      const run = () => cp.spawnSync(powershell, ['-NoLogo', '-NoProfile', '-NonInteractive', '-Command', command + '\nexit $LASTEXITCODE'], {cwd: directory, env: cliEnv, encoding: 'utf8', windowsHide: true, timeout: 20000, maxBuffer: 1 << 20});
      const first = run(); result.cli.push({purpose: 'exact production command new file', exit_code: first.status, signal: first.signal, stdout: first.stdout, stderr: first.stderr}); assert.equal(first.error, undefined); assert.equal(first.status, 0, first.stderr);
      const receipt = JSON.parse(first.stdout); assert.equal(receipt.protocol, 'suma-runner-tools-preparation-v1'); assert.equal(receipt.prepared, true); assert.equal(receipt.command_count, 1);
      const bytes = fs.readFileSync(destination), config = JSON.parse(bytes), keys = Object.keys(config); assert.deepEqual(keys, ['local-codex-jsonl']); assert.equal(config[keys[0]].adapter, 'codex'); assert.equal(config[keys[0]].protocol, 'codex-jsonl-v1'); assert.equal(path.resolve(config[keys[0]].executable), path.resolve(native));
      result.configuration = {path: path.relative(root, destination), sha256: hash(bytes), command_keys: keys}; facts.real_installed_codex_passive_probe_succeeded = true; facts.model_login_install_or_business_execution = false;
      const second = run(); result.cli.push({purpose: 'same exact command refuses overwrite before another probe', exit_code: second.status, signal: second.signal, stdout: second.stdout, stderr: second.stderr}); assert.equal(second.error, undefined); assert.equal(second.status, 1); assert.equal(hash(fs.readFileSync(destination)), hash(bytes));
      await page.locator('[data-local-tool-use]').click(); assert.equal(await page.locator('[name=remote-runner-shell]').inputValue(), 'powershell'); assert.equal(await page.locator('[name=remote-runner-commands]').inputValue(), destination); assert.equal(await page.locator('[name=remote-runner-worker]').inputValue(), 'ordinary-worker-220'); facts.original_ordinary_fields_adopted = true;
    });
    await group('shared_adoption_is_output_reference_only_without_ordinary_identity', async facts => {
      await visit('zh-CN', 1366, 'light', 'shared'); await fill('bash', '/srv/owned/shared-new.json'); await page.locator('[data-local-tool-use]').click();
      await page.locator('[data-shared-pool-output]').waitFor(); assert.equal(await page.locator('[data-shared-pool-output] code').textContent(), '/srv/owned/shared-new.json'); assert.equal(await page.locator('[data-remote-runner-setup],[name=remote-runner-identityPath]').count(), 0); facts.output_reference_only = true;
    });
    await group('en_390_dark_tool_aba_rejects_old_clipboard_ack_then_adopts_current_draft', async facts => {
      await visit('en-US', 390, 'dark'); await fill('bash', '/srv/owned/old-draft.json');
      await page.evaluate(() => {globalThis.__ownedClipboard.hold = true;}); await page.locator('[data-local-tool-copy]').click();
      await page.waitForFunction(() => typeof globalThis.__ownedClipboard.release === 'function');
      const tools = page.locator('.tool-cli-setup-select select').first(); await tools.selectOption('codebuddy'); await page.locator('[data-local-tool=codebuddy-cli]').waitFor(); await tools.selectOption('codex'); await page.locator('[data-local-tool=codex-jsonl]').waitFor();
      await page.evaluate(() => {globalThis.__ownedClipboard.hold = false; globalThis.__ownedClipboard.release();});
      assert.equal(await page.locator('[data-local-tool-output]').inputValue(), ''); assert.equal(await page.locator('[data-local-tool-shell]').inputValue(), ''); assert.equal(await page.locator('[data-local-tool-status=copied],[data-local-tool-preview]').count(), 0); assert.equal(await page.locator('[data-local-tool-use]').isDisabled(), true); facts.old_ack_not_adopted_after_aba = true;
      await fill('bash', '/srv/owned/current-new.json'); await page.locator('[data-local-tool-copy]').click(); await page.locator('[data-local-tool-status=copied]').waitFor(); facts.geometry = await geometry(); await capture('en-mobile-local-tool-preparation.png');
      await page.locator('[data-local-tool-use]').click(); assert.equal(await page.locator('[name=remote-runner-shell]').inputValue(), 'bash'); assert.equal(await page.locator('[name=remote-runner-commands]').inputValue(), '/srv/owned/current-new.json'); facts.new_current_draft_adopted = true;
    });
    result.business_writes = result.requests.filter(request => request.method !== 'GET').length; assert.equal(result.business_writes, 0); assert.deepEqual(result.unexpected, []); assert.deepEqual(result.page_errors, []); assert.deepEqual(result.console_errors, []); result.checks_passed = true;
  } finally {
    if (app) Object.assign(result, await closeOwnedNative(app, exit)); if (server) {await new Promise(resolve => server.close(resolve)); result.port_closed = await closedPort(result.port);} save();
  }
  assert.equal(result.forced, false); assert.deepEqual(result.native_exit, {code: 0, signal: null}); assert.equal(result.port_closed, true); result.passed = true; save();
  console.log(JSON.stringify({directory: path.relative(root, directory), passed: true, groups: Object.keys(result.groups), native_exit: result.native_exit, port_closed: result.port_closed, images: result.images}));
}
main().catch(error => {console.error(error.stack || String(error)); process.exitCode = 1;});
