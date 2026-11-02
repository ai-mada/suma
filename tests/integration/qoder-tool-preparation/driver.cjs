'use strict';
// Static Qoder configuration only. The owned Runner executable is an explicitly
// non-Qoder PE fixture: its bytes are pinned, never launched with --acp.
const fs = require('node:fs'), path = require('node:path'), http = require('node:http');
const cp = require('node:child_process'), net = require('node:net'), crypto = require('node:crypto'), assert = require('node:assert/strict');
const {closeOwnedNative} = require('../support/close-native.cjs');
const root = path.resolve(__dirname, '../../..'), base = path.join(root, '.cache/qoder-tool-preparation-221/actual'), out = path.join(base, 'out');
const directory = path.join(base, 'visual', new Date().toISOString().replace(/[^0-9TZ]/g, ''));
const hash = bytes => crypto.createHash('sha256').update(bytes).digest('hex');
const project = 'd'.repeat(32), tenant = 'a'.repeat(32);
const englishOnly = process.argv.includes('--en-only');
const session = {user: {id: 'fixture-owner-221', name: 'Fixture owner', email: 'owned@example.test', role: 'owner'}, organization: {id: tenant, name: 'Fixture organization'}, projects: []};
const result = {stage: 'T-QODER-TOOL-PREPARATION-221', scope: 'Production Guide/Qoder form/Remote adoption and original static CLI writer using owned non-Qoder PE bytes; no vendor execution', browser_path: 'Installed isolated Electron/Playwright harness; active application browser not exercised', groups: {}, requests: [], unexpected: [], cli: [], images: [], page_errors: [], console_errors: [], console_warnings: [], passed: false};
fs.mkdirSync(directory, {recursive: true});
const save = () => fs.writeFileSync(path.join(directory, 'result.json'), JSON.stringify(result, null, 2) + '\n');
const newDirectory = name => {const value = path.join(directory, name); fs.mkdirSync(value, {recursive: true}); return value;};
function environment(name, searchPath) {
  const home = newDirectory(name), env = {};
  for (const key of ['SystemRoot', 'WINDIR', 'ComSpec', 'PATHEXT']) if (process.env[key]) env[key] = process.env[key];
  Object.assign(env, {PATH: searchPath, HOME: home, USERPROFILE: home, APPDATA: newDirectory(name + '/appdata'), LOCALAPPDATA: newDirectory(name + '/localappdata'), XDG_CONFIG_HOME: newDirectory(name + '/xdg-config'), XDG_DATA_HOME: newDirectory(name + '/xdg-data'), XDG_STATE_HOME: newDirectory(name + '/xdg-state'), TEMP: newDirectory(name + '/tmp'), TMP: newDirectory(name + '/tmp')});
  return env;
}
function closedPort(port) {return new Promise(resolve => {const socket = net.connect({host: '127.0.0.1', port}); socket.once('connect', () => {socket.destroy(); resolve(false);}); socket.once('error', error => resolve(error.code === 'ECONNREFUSED')); socket.setTimeout(1000, () => {socket.destroy(); resolve(false);});});}

async function main() {
  let server, app, exit, page;
  try {
    const powershell = process.env.SUMA_PWSH_PATH, binary = path.join(base, 'bin/suma-runner.exe');
    assert.ok(powershell && path.isAbsolute(powershell) && fs.statSync(powershell).isFile()); assert.ok(fs.statSync(binary).isFile());
    result.binary = {path: path.relative(root, binary), sha256: hash(fs.readFileSync(binary))};
    result.native_file_fixture = {...result.binary, label: 'Owned Runner PE file, not Qoder; static read/pin target only', vendor_identity_claim: false, executed_with_acp: false};
    const destination = path.join(newDirectory('machine files'), "Qoder 新 code's $config.json");
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
    const {_electron} = require(process.env.SUMA_PLAYWRIGHT_MODULE), system = process.env.SystemRoot || 'C:/Windows';
    const trustedPath = [path.join(system, 'System32'), system, path.join(system, 'System32/Wbem')].join(';');
    const env = environment('browser-home', trustedPath); Object.assign(env, {SUMA_FIXTURE_ORIGIN: origin, SUMA_FIXTURE_DIRECTORY: directory, SUMA_FIXTURE_PARTITION: 'suma-fixture-qoder-preparation221'});
    app = await _electron.launch({executablePath: path.join(root, 'apps/desktop/node_modules/electron/dist/electron.exe'), args: [path.join(__dirname, '../support/native.cjs')], cwd: root, env, timeout: 15000});
    result.native_pid = app.process().pid; exit = new Promise(resolve => app.process().once('exit', (code, signal) => resolve({code, signal}))); save();
    page = await app.firstWindow(); page.setDefaultTimeout(12000);
    page.on('pageerror', error => result.page_errors.push(error.message)); page.on('console', message => {if (message.type() === 'error') result.console_errors.push(message.text()); else if (message.type() === 'warning') result.console_warnings.push(message.text());});
    await page.addInitScript(() => {
      globalThis.__ownedClipboard = {calls: [], hold: false};
      Object.defineProperty(navigator, 'clipboard', {configurable: true, value: {writeText: value => {globalThis.__ownedClipboard.calls.push(value); return globalThis.__ownedClipboard.hold ? new Promise(resolve => {globalThis.__ownedClipboard.release = resolve;}) : Promise.resolve();}}});
    });
    const card = () => page.locator('[data-qoder-setup]');
    async function visit(locale, width, theme, mode = 'ordinary') {await page.setViewportSize({width, height: width === 390 ? 844 : 960}); await page.goto(origin + '/?' + new URLSearchParams({locale, theme, mode})); await card().waitFor();}
    async function field(suffix) {
      const input = page.locator('[data-qoder-setup-' + suffix + ']');
      // Open native disclosures through their actual summary, never force DOM
      // visibility or assume a remote machine's platform from the browser.
      const disclosures = input.locator('xpath=ancestor::details');
      for (let i = 0; i < await disclosures.count(); i++) {const disclosure = disclosures.nth(i); if (await disclosure.getAttribute('open') === null) await disclosure.locator(':scope > summary').click();}
      return input;
    }
    async function fill(shell, output) {
      await (await field('shell')).selectOption(shell); await (await field('executable')).fill(binary); await (await field('version')).fill('fixture-reviewed-221'); await (await field('model')).fill('fixture-model-221'); await (await field('credential')).fill(''); await (await field('output')).fill(output);
    }
    async function geometry() {const value = await card().evaluate(node => ({width: innerWidth, scroll: document.documentElement.scrollWidth, controls: [...node.querySelectorAll('input,select,button,summary')].filter(control => control.getClientRects().length).map(control => control.getBoundingClientRect().height)})); assert.ok(value.scroll <= value.width); assert.ok(value.controls.every(height => height >= 43.5)); return value;}
    async function capture(name) {await card().scrollIntoViewIfNeeded(); await page.screenshot({path: path.join(directory, name), fullPage: false}); result.images.push({path: name, sha256: hash(fs.readFileSync(path.join(directory, name)))});}
    async function group(name, run) {result.groups[name] = {passed: false}; try {await run(result.groups[name]); result.groups[name].passed = true;} catch (error) {result.groups[name].error = String(error.stack || error); if (page) {await page.screenshot({path: path.join(directory, name + '-failure.png')}).catch(() => {}); result.groups[name].failure_fields = await page.evaluate(() => ({shell: document.querySelector('[name=remote-runner-shell]')?.value, commands: document.querySelector('[name=remote-runner-commands]')?.value, localTool: document.querySelector('[name=remote-runner-localTool]')?.value, use_disabled: document.querySelector('[data-qoder-setup-use]')?.disabled})).catch(() => null);} throw error;} finally {save();}}
    if (!englishOnly) {
    await group('cn_private_five_fields_static_native_file_new_write_refuse_overwrite_and_ordinary_adoption', async facts => {
      await visit('zh-CN', 1366, 'light'); await fill('powershell', destination);
      const request = JSON.parse(await page.locator('[data-qoder-setup-request]').textContent()); assert.deepEqual(Object.keys(request).sort(), ['adapter_id', 'credential_env_ref', 'executable', 'model', 'reviewed_version'].sort()); assert.equal(request.adapter_id, 'qoder-acp'); assert.equal(request.executable, binary); assert.equal(request.credential_env_ref, ''); facts.private_request_closed_five = true;
      await page.locator('[data-qoder-setup-copy]').click(); const command = await page.locator('[data-qoder-setup-preview]').textContent(); await page.waitForFunction(value => globalThis.__ownedClipboard.calls.at(-1) === value, command); facts.geometry = await geometry(); await capture('cn-qoder-static-preparation.png');
      const cliEnv = environment('cli-home', [path.dirname(binary), trustedPath].join(';'));
      const run = () => cp.spawnSync(powershell, ['-NoLogo', '-NoProfile', '-NonInteractive', '-Command', command + '\nexit $LASTEXITCODE'], {cwd: directory, env: cliEnv, encoding: 'utf8', windowsHide: true, timeout: 15000, maxBuffer: 1 << 20});
      const first = run(); result.cli.push({purpose: 'exact UI script static new-file preparation', exit_code: first.status, signal: first.signal, stdout: first.stdout, stderr: first.stderr}); assert.equal(first.error, undefined); assert.equal(first.status, 0, first.stderr);
      const receipt = JSON.parse(first.stdout); assert.equal(receipt.protocol, 'suma-runner-tools-preparation-v1'); assert.equal(receipt.prepared, true); assert.equal(receipt.command_count, 1);
      const bytes = fs.readFileSync(destination), commands = JSON.parse(bytes); assert.deepEqual(Object.keys(commands), ['local-qoder-acp']); const spec = commands['local-qoder-acp']; assert.equal(spec.adapter, 'qoder'); assert.equal(spec.protocol, 'acp-stdio-v1'); assert.equal(path.resolve(spec.executable), path.resolve(binary)); assert.deepEqual(spec.args, ['--acp']); assert.equal(spec.acp.runtime, 'qoder'); assert.equal(spec.acp.reviewed_version, request.reviewed_version); assert.equal(spec.acp.model, request.model);
      result.configuration = {path: path.relative(root, destination), bytes: bytes.length, sha256: hash(bytes), command_keys: Object.keys(commands)}; facts.target_file_static_only_not_qoder_runtime = true;
      const second = run(); result.cli.push({purpose: 'same UI script refuses existing file', exit_code: second.status, signal: second.signal, stdout: second.stdout, stderr: second.stderr}); assert.equal(second.error, undefined); assert.equal(second.status, 1); assert.equal(hash(fs.readFileSync(destination)), hash(bytes));
      await page.locator('[data-qoder-setup-use]').click(); assert.equal(await page.locator('[name=remote-runner-shell]').inputValue(), 'powershell'); assert.equal(await page.locator('[name=remote-runner-commands]').inputValue(), destination); assert.equal(await page.locator('[name=remote-runner-localTool]').inputValue(), 'qoder-acp'); facts.ordinary_original_fields_adopted = true;
    });
    await group('shared_qoder_adoption_keeps_only_local_output_reference', async facts => {
      await visit('zh-CN', 1366, 'light', 'shared'); await fill('bash', '/srv/owned/qoder-shared-new.json'); await page.locator('[data-qoder-setup-use]').click(); await page.locator('[data-shared-pool-output]').waitFor(); assert.equal(await page.locator('[data-shared-pool-output] code').textContent(), '/srv/owned/qoder-shared-new.json'); assert.equal(await page.locator('[data-remote-runner-setup],[name=remote-runner-identityPath]').count(), 0); facts.no_ordinary_identity_or_pool_write = true;
    });
    }
    await group('en_390_dark_qoder_aba_ignores_old_clipboard_ack', async facts => {
      await visit('en-US', 390, 'dark'); await fill('bash', '/srv/owned/qoder-old.json'); await page.evaluate(() => {globalThis.__ownedClipboard.hold = true;}); await page.locator('[data-qoder-setup-copy]').click(); await page.waitForFunction(() => typeof globalThis.__ownedClipboard.release === 'function');
      const tool = page.locator('.tool-cli-setup-select select').first(); await tool.selectOption('codex'); await page.locator('[data-local-tool-preparation]').waitFor(); await tool.selectOption('qoder'); await card().waitFor(); await page.evaluate(() => {globalThis.__ownedClipboard.hold = false; globalThis.__ownedClipboard.release();});
      assert.equal(await page.locator('[data-qoder-setup-output]').inputValue(), ''); assert.equal(await page.locator('[data-qoder-setup-use]').isDisabled(), true); assert.equal(await page.locator('[data-qoder-setup-preview]').count(), 0); facts.old_ack_cannot_restore_prepared_draft = true;
      await fill('bash', '/srv/owned/qoder-current-new.json'); facts.geometry = await geometry(); await capture('en-mobile-qoder-static-preparation.png'); await page.locator('[data-qoder-setup-use]').click();
      // Configuration adoption is committed by the original React effect.
      // Observe its actual state, rather than reading before that effect runs.
      await page.waitForFunction(() => document.querySelector('[name=remote-runner-shell]')?.value === 'bash' && document.querySelector('[name=remote-runner-commands]')?.value === '/srv/owned/qoder-current-new.json' && document.querySelector('[name=remote-runner-localTool]')?.value === 'qoder-acp');
      assert.equal(await page.locator('[name=remote-runner-shell]').inputValue(), 'bash'); assert.equal(await page.locator('[name=remote-runner-commands]').inputValue(), '/srv/owned/qoder-current-new.json'); assert.equal(await page.locator('[name=remote-runner-localTool]').inputValue(), 'qoder-acp'); facts.new_current_draft_adopted = true;
    });
    result.business_writes = result.requests.filter(request => request.method !== 'GET').length; assert.equal(result.business_writes, 0); assert.deepEqual(result.unexpected, []); assert.deepEqual(result.page_errors, []); assert.deepEqual(result.console_errors, []); result.vendor_processes = 0; result.model_calls = 0; result.checks_passed = true;
  } finally {if (app) Object.assign(result, await closeOwnedNative(app, exit)); if (server) {await new Promise(resolve => server.close(resolve)); result.port_closed = await closedPort(result.port);} save();}
  assert.equal(result.forced, false); assert.deepEqual(result.native_exit, {code: 0, signal: null}); assert.equal(result.port_closed, true); result.passed = true; save(); console.log(JSON.stringify({directory: path.relative(root, directory), passed: true, groups: Object.keys(result.groups), native_exit: result.native_exit, port_closed: result.port_closed, images: result.images}));
}
main().catch(error => {console.error(error.stack || String(error)); process.exitCode = 1;});
