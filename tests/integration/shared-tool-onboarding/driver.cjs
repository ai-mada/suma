'use strict';
// Synthetic public metadata, no registered ToolProfile and no live inventory.
// Copied commands are retained in an owned clipboard and never executed.
const fs = require('node:fs'), path = require('node:path'), http = require('node:http');
const net = require('node:net'), crypto = require('node:crypto'), assert = require('node:assert/strict');
const {closeOwnedNative} = require('../support/close-native.cjs');
const root = path.resolve(__dirname, '../../..');
const base = path.join(root, '.cache/shared-tool-onboarding-219/actual');
const out = path.join(base, 'out');
const directory = path.join(base, 'visual', new Date().toISOString().replace(/[^0-9TZ]/g, ''));
const hash = value => crypto.createHash('sha256').update(value).digest('hex');
const tenant = 'a'.repeat(32), poolID = 'b'.repeat(32), runnerID = 'c'.repeat(32);
const project = 'd'.repeat(32), grantID = 'e'.repeat(32), worker = 'pool-' + runnerID;
const adapter = {id: 'codebuddy-cli', product: 'CodeBuddy', entrypoint_kind: 'cli', direction: 'suma_to_tool', implementation: 'implemented', setup_kind: 'runner_profile', adapter: 'codebuddy', protocol: 'codebuddy-jsonl-v1', capabilities: [], source_refs: [], limitations: []};
const session = {user: {id: 'fixture-owner-219', name: 'Fixture owner', email: 'owned@example.test', role: 'owner'}, organization: {id: tenant, name: 'Fixture organization'}, projects: []};
const at = () => new Date().toISOString();
let credentialExpiry, ownerDenied = false, sharedSource = true;
const runner = () => ({id: runnerID, pool_id: poolID, worker_id: worker, name: 'Exact shared machine', version: 2, status: 'active', max_slots: 2, credential_version: 1, credential_expires_at: credentialExpiry, allowed_actions: ['rotate_credential', 'revoke'], created_at: '2026-11-03T00:00:00Z'});
const pool = () => ({id: poolID, owner_tenant_id: tenant, name: 'Team execution Pool', description: 'Current owner administration', version: 1, status: 'active', allowed_actions: ['pause', 'register_runner', 'preview_policy'], created_at: '2026-11-03T00:00:00Z', updated_at: '2026-11-03T00:00:00Z', policy: {concurrency_limit: 2, resource_limits: {}, budget_limit: null, tenant_weights: {}, version: 1, budget_spent: 0, resources: [], fairness: {mode: 'tenant_project_weighted_round_robin', scope: 'pool'}, as_of: at()}});
const setupPanel = () => 'pool-tool-setup:' + [poolID, runnerID, worker, tenant, project, adapter.id, adapter.adapter, adapter.protocol].join(':');
const focusOnly = process.argv.includes('--focus-only');
const continuation = process.argv.includes('--continue-from-owner');
const result = {stage: 'T-SHARED-TOOL-ONBOARDING-219', scope: 'Production ToolWorkspace/Guide/Pool preparation/connection/Quick with synthetic public HTTP and a small fixture router; not full App or a real platform backend', browser_path: 'Installed isolated Electron/Playwright harness; active application browser not exercised', passed: false, groups: {}, requests: [], unexpected: [], images: [], page_errors: [], console_errors: [], console_warnings: [], expected_forbidden_console: []};
if (continuation) {
  const prior = '.cache/shared-tool-onboarding-219/actual/visual/20261102T203036401Z/result.json';
  result.reused_partial = {path: prior, sha256: hash(fs.readFileSync(path.join(root, prior))), scope: 'Only prior successful exact Guide focus/44px and setup-owner entry/44px, two retained images; original overall group failed on missing fixture Shell and is not called passed.'};
}
fs.mkdirSync(directory, {recursive: true});
const save = () => fs.writeFileSync(path.join(directory, 'result.json'), JSON.stringify(result, null, 2) + '\n');
function closedPort(port) {
  return new Promise(resolve => {const socket = net.connect({host: '127.0.0.1', port}); socket.once('connect', () => {socket.destroy(); resolve(false);}); socket.once('error', error => resolve(error.code === 'ECONNREFUSED')); socket.setTimeout(1000, () => {socket.destroy(); resolve(false);});});
}

async function main() {
  let server, app, exit;
  try {
    credentialExpiry = new Date(Date.now() + 86400000).toISOString();
    server = http.createServer((request, response) => {
      const url = new URL(request.url, 'http://127.0.0.1'), route = url.pathname.replace(/^\/api\/v1/, ''), time = at();
      const recorded = {method: request.method, path: url.pathname, query: url.search}; result.requests.push(recorded);
      const send = (body, status = 200) => {recorded.status = status; response.writeHead(status, {'Content-Type': 'application/json', 'Cache-Control': 'no-store'}); response.end(JSON.stringify(body));};
      if (request.method !== 'GET') {
        if (request.method === 'POST' && route === '/tool-profiles/matches') {
          let body = ''; request.on('data', chunk => body += chunk); request.on('end', () => {recorded.body = JSON.parse(body); send({...recorded.body, items: [], total: 0});}); return;
        }
        result.unexpected.push(request.method + ' ' + route); send({error: {code: 'fixture_write_refused', message: 'No business mutation authorized'}}, 405); return;
      }
      if (url.pathname === '/__fixture/view') return send({session, project_id: project});
      if (url.pathname === '/favicon.ico') {response.writeHead(204); response.end(); return;}
      if (route === '/tool-profiles') return send({items: [], total: 0});
      if (route === '/release-workers') return send({items: []});
      if (route === '/project-execution/runners') return send({as_of: time, items: sharedSource ? [{id: runnerID, pool_id: poolID, worker_id: worker, name: 'Shared machine', status: 'active', credential_expires_at: credentialExpiry}] : []});
      if (route === '/adapter-catalog') return send({project_id: project, as_of: time, items: [adapter]});
      if (route === '/tool-discovery') return send({project_id: project, worker_id: worker, state: 'unreported', as_of: time, items: []});
      if (route === '/execution-pools') return send({items: [pool()], total: 1, allowed_actions: ['create']});
      if (route === '/execution-pools/' + poolID) return ownerDenied ? send({error: {code: 'forbidden', message: 'Current Pool owner authority is required'}}, 403) : send(pool());
      if (route === '/execution-pools/' + poolID + '/runners') return send({items: [], total: 1, next_cursor: 'fixture-not-target-page', allowed_actions: ['register']});
      if (route === '/execution-pools/' + poolID + '/grants') return send({items: [], total: 1, next_cursor: 'fixture-not-grant-page', allowed_actions: []});
      if (route === '/execution-pools/' + poolID + '/runners/' + runnerID) return ownerDenied ? send({error: {code: 'forbidden', message: 'Current Pool owner authority is required'}}, 403) : send(runner());
      if (route === '/execution-pools/' + poolID + '/runners/' + runnerID + '/readiness') return ownerDenied ? send({error: {code: 'forbidden', message: 'Current Pool owner authority is required'}}, 403) : send({pool_id: poolID, runner_id: runnerID, worker_id: worker, pool_status: 'active', checked_at: time, runner: {status: 'active', version: 2, credential_version: 1, credential_state: 'valid', credential_expires_at: credentialExpiry, historical_last_seen_at: ''}, scope: {tenant_id: tenant, project_id: project, grant_id: grantID, grant_version: 1, grant_status: 'approved', shared_project_name: 'Original project', can_open_project: true, route_state: 'current', source_current: false, source_reason: 'inventory_unreported', inventory_observed_at: '', inventory_expires_at: ''}});
      if (url.pathname.startsWith('/api/')) {result.unexpected.push(request.method + ' ' + route); return send({error: {code: 'fixture_missing_read', message: 'No synthetic response'}}, 404);}
      const file = {'/': 'index.html', '/fixture.js': 'fixture.js', '/fixture.css': 'fixture.css'}[url.pathname];
      if (!file) {response.writeHead(404); response.end(); return;}
      response.writeHead(200, {'Content-Type': file.endsWith('.js') ? 'text/javascript' : file.endsWith('.css') ? 'text/css' : 'text/html'}); response.end(fs.readFileSync(path.join(out, file)));
    });
    await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
    result.port = server.address().port; const origin = 'http://127.0.0.1:' + result.port; save();
    const {_electron} = require(process.env.SUMA_PLAYWRIGHT_MODULE), env = {};
    for (const [key, value] of Object.entries(process.env)) if (['path', 'systemroot', 'windir', 'comspec', 'pathext', 'home', 'userprofile', 'homedrive', 'homepath', 'appdata', 'localappdata', 'programdata', 'programfiles', 'programfiles(x86)', 'programw6432'].includes(key.toLowerCase())) env[key] = value;
    Object.assign(env, {TEMP: path.join(directory, 'tmp'), TMP: path.join(directory, 'tmp'), SUMA_FIXTURE_ORIGIN: origin, SUMA_FIXTURE_DIRECTORY: directory, SUMA_FIXTURE_PARTITION: 'suma-fixture-shared-setup219'}); fs.mkdirSync(env.TEMP, {recursive: true});
    app = await _electron.launch({executablePath: path.join(root, 'apps/desktop/node_modules/electron/dist/electron.exe'), args: [path.join(__dirname, '../support/native.cjs')], cwd: root, env, timeout: 15000});
    result.native_pid = app.process().pid; exit = new Promise(resolve => app.process().once('exit', (code, signal) => resolve({code, signal}))); save();
    const page = await app.firstWindow(); page.setDefaultTimeout(12000);
    page.on('pageerror', error => result.page_errors.push(error.message));
    page.on('console', message => {
      const detail = {text: message.text(), url: message.location().url};
      if (message.type() === 'error') {
        const expected = message.text().includes('403') && result.requests.some(request => request.status === 403 && origin + request.path + request.query === detail.url);
        (expected ? result.expected_forbidden_console : result.console_errors).push(detail);
      } else if (message.type() === 'warning') result.console_warnings.push(detail);
    });
    await page.addInitScript(() => {
      Object.defineProperty(navigator, 'clipboard', {configurable: true, value: {writeText: async value => {globalThis.__ownedClipboard = value;}}});
      // Observation only: never move focus or affect a production event.
      globalThis.__ownedFocusHistory = [];
      document.addEventListener('focusin', event => {
        if (globalThis.__ownedFocusHistory.length < 80) globalThis.__ownedFocusHistory.push({at: performance.now(), tag: event.target.tagName, id: event.target.id, hook: [...event.target.attributes].filter(attr => attr.name.startsWith('data-')).map(attr => attr.name).join(','), text: event.target.textContent?.trim().slice(0, 70)});
      });
    });
    async function visit(locale, width, theme) {
      await page.setViewportSize({width, height: width === 390 ? 844 : 960}); await page.goto(origin + '/?' + new URLSearchParams({locale, theme}));
      await page.locator('[data-shared-setup-view=tools] [data-tools-add]').waitFor();
    }
    async function openSharedGuide() {
      await page.locator('[data-tools-add]').click();
      await page.locator('[data-quick-tool]').selectOption(adapter.id);
      await page.locator('.tool-quick-node').waitFor();
      assert.match(await page.locator('.tool-quick-node').textContent(), new RegExp(worker));
      await page.evaluate(() => {
        globalThis.__ownedTransition = [];
        let previous = '';
        const sample = reason => {
          const node = document.querySelector('[data-shared-pool-prepare]'), active = document.activeElement;
          const value = {focus_tag: active?.tagName, focus_id: active?.id, focus_hook: active ? [...active.attributes].filter(attr => attr.name.startsWith('data-')).map(attr => attr.name).join(',') : '', focus_text: active?.textContent?.trim().slice(0, 65), main_present: !!document.querySelector('#main'), modal_count: document.querySelectorAll('[role=dialog]').length, button: node ? {disabled: node.disabled, visible: node.getClientRects().length > 0, text: node.textContent} : null};
          const signature = JSON.stringify(value);
          if (signature !== previous && globalThis.__ownedTransition.length < 120) {previous = signature; globalThis.__ownedTransition.push({at: performance.now(), reason, ...value});}
        };
        sample('before-advanced');
        const focused = () => sample('focusin');
        const observer = new MutationObserver(() => sample('mutation'));
        observer.observe(document.body, {subtree: true, childList: true, attributes: true, attributeFilter: ['disabled', 'aria-hidden', 'class', 'open']});
        document.addEventListener('focusin', focused);
        globalThis.__ownedStopTransition = () => {sample('final'); observer.disconnect(); document.removeEventListener('focusin', focused);};
      });
      await page.locator('[data-quick-advanced]').click();
      await page.locator('[data-shared-pool-prepare]').waitFor();
      try {await page.waitForFunction(() => !document.querySelector('[role=dialog]') && document.activeElement?.hasAttribute('data-shared-pool-prepare'));}
      finally {await page.evaluate(() => globalThis.__ownedStopTransition?.());}
      assert.equal(await page.locator('[data-remote-runner-setup]').count(), 0);
      assert.equal(await page.locator('.tool-cli-setup-select select').first().inputValue(), 'codebuddy');
    }
    async function image(name) {await page.screenshot({path: path.join(directory, name), fullPage: false}); result.images.push({path: name, sha256: hash(fs.readFileSync(path.join(directory, name)))});}
    async function layout(locator) {
      const fact = await locator.evaluate(node => ({width: innerWidth, scroll_width: document.documentElement.scrollWidth, controls: [...node.querySelectorAll('button,input:not([type=checkbox]),select,summary')].filter(control => control.getClientRects().length).map(control => ({name: control.getAttribute('aria-label') || control.textContent?.trim().slice(0, 60) || control.tagName, height: control.getBoundingClientRect().height}))}));
      assert.ok(fact.scroll_width <= fact.width, 'No horizontal page overflow'); assert.ok(fact.controls.every(control => control.height >= 43.5), JSON.stringify(fact.controls.filter(control => control.height < 43.5))); return fact;
    }
    async function group(name, run) {
      result.groups[name] = {passed: false};
      try {await run(result.groups[name]); result.groups[name].passed = true;}
      catch (error) {
        result.groups[name].error = String(error.stack || error);
        result.groups[name].failure_dom = await page.evaluate(() => ({focus: {tag: document.activeElement?.tagName, id: document.activeElement?.id, text: document.activeElement?.textContent?.trim().slice(0, 90)}, focus_history: globalThis.__ownedFocusHistory, transition: globalThis.__ownedTransition, dialogs: document.querySelectorAll('[role=dialog]').length, shared_button: (() => {const node = document.querySelector('[data-shared-pool-prepare]'); return node ? {disabled: node.disabled, connected: node.isConnected, visible: node.getClientRects().length > 0, text: node.textContent} : null;})()})).catch(() => null);
        await page.screenshot({path: path.join(directory, name + '-failure.png')}).catch(() => {}); throw error;
      }
      finally {save();}
    }
    if (focusOnly) {
      await group('focused_advanced_to_guide_passive_diagnostic', async facts => {
        await visit('zh-CN', 1366, 'light'); await openSharedGuide();
        facts.transition = await page.evaluate(() => globalThis.__ownedTransition);
      });
      result.diagnostic_only = true;
    } else {
    await group('cn_exact_shared_guide_owner_preparation_and_new_quick', async facts => {
      if (continuation) {
        await page.setViewportSize({width: 1366, height: 960});
        await page.goto(origin + '/?' + new URLSearchParams({locale: 'zh-CN', theme: 'light', view: 'settings', sub: 'execution-pools', record: poolID, panel: setupPanel()}));
        facts.prior_successful_guide_and_owner_entry_reused = true;
      } else {
        await visit('zh-CN', 1366, 'light'); await openSharedGuide();
        facts.guide_focus = await page.locator('[data-shared-pool-prepare]').evaluate(node => node === document.activeElement);
        facts.guide_layout = await layout(page.locator('[data-shared-pool-guide]')); await image('cn-shared-tool-guide.png');
        await page.locator('[data-shared-pool-prepare]').press('Enter');
      }
      await page.locator('[data-shared-setup-view=pool] [data-pool-recovery-state=read]').waitFor();
      assert.equal(await page.locator('[data-pool-tool-intent=setup]').getAttribute('data-target'), setupPanel());
      if (!continuation) {const card = page.locator('[data-pool-tool-recovery]'); facts.card_layout = await layout(card); await card.scrollIntoViewIfNeeded(); await image('cn-shared-tool-owner.png');}
      await page.locator('[data-pool-recovery-prepare]').click(); await page.locator('[data-pool-config-preparation]').waitFor();
      const row = page.locator('[data-pool-config-row=scope-1]');
      for (const [field, value] of Object.entries({tenant_id: tenant, project_id: project, workspace: '', commands_file: '', repository: '', base_commit: '', worktrees: '', model_deployments_file: '', external_config_file: ''})) assert.equal(await row.locator('[data-pool-config-scope-field=' + field + ']').inputValue(), value);
      await page.locator('[data-pool-config-field=shell]').selectOption('powershell');
      await page.locator('[data-pool-config-field=output]').fill('D:/owned/shared-new-config.json');
      await row.locator('[data-pool-config-scope-field=workspace]').fill('D:/owned/project');
      await page.locator('[data-pool-config-copy]').click();
      const script = await page.locator('[data-pool-config-prepare]').textContent(); assert.match(script, /--prepare-pool-config/); assert.equal(await page.evaluate(() => globalThis.__ownedClipboard), script);
      facts.command_copied_not_executed = true;
      await page.locator('[data-pool-config-connection]>summary').click(); await page.locator('[data-pool-connection-refresh]').click();
      const back = page.locator('[data-pool-connection-tools]'); await back.waitFor();
      const href = new URL(await back.getAttribute('href'), origin); assert.equal(href.searchParams.get('panel'), setupPanel()); assert.equal(href.searchParams.get('record'), ''); await back.click();
      await page.locator('[data-quick-tool]').waitFor(); assert.equal(await page.locator('[data-quick-tool]').inputValue(), adapter.id); assert.match(await page.locator('.tool-quick-node').textContent(), new RegExp(worker));
      assert.equal(await page.locator('[data-quick-replacement-unchanged],[data-existing-tool-open]').count(), 0);
      facts.new_setup_not_existing_profile = true; facts.exact_adapter = adapter.id; facts.exact_worker = worker;
    });
    await group('source_invalid_and_owner_forbidden_do_not_fall_back_to_ordinary_identity', async facts => {
      await visit('zh-CN', 1366, 'light'); await openSharedGuide();
      sharedSource = false; const before = result.requests.length; await page.locator('[data-shared-pool-prepare]').click();
      await page.waitForFunction(() => document.querySelector('[data-shared-setup-view=tools]') && [...document.querySelectorAll('[role=alert]')].some(node => node.textContent?.trim()));
      assert.ok(result.requests.slice(before).some(request => request.path === '/api/v1/project-execution/runners'));
      assert.equal(await page.locator('[data-shared-setup-view=pool],[data-remote-runner-setup]').count(), 0); facts.invalid_current_shared_source_rejected = true;
      sharedSource = true; await page.locator('[data-shared-pool-prepare]').click();
      await page.locator('[data-pool-recovery-state=read]').waitFor();
      await page.locator('[data-pool-recovery-prepare]').click();
      await page.locator('[data-pool-config-preparation]').waitFor();
      await page.locator('[data-pool-config-field=output]').fill('D:/owned/not-written-draft.json');
      // Use the actual monotonic expiry, not fixture wall-clock substitution.
      // A current-authority reread may then retire this exact retained form.
      await page.locator('[data-pool-recovery-form-refresh]').waitFor({timeout: 35000});
      assert.equal(await page.locator('[data-pool-config-field=output]').inputValue(), 'D:/owned/not-written-draft.json');
      facts.expiry_preserved_exact_draft = true;
      ownerDenied = true; await page.locator('[data-pool-recovery-form-refresh]').click();
      await page.locator('[data-pool-recovery-state=restricted]').waitFor();
      await page.locator('[data-pool-config-preparation]').waitFor({state: 'detached'});
      assert.equal(await page.locator('[data-remote-runner-setup]').count(), 0);
      assert.ok(result.requests.some(request => request.path === '/api/v1/execution-pools/' + poolID + '/runners/' + runnerID && request.status === 403));
      facts.owner_403_closed_exact_retained_form = true;
    });
    await group('en_390_dark_shared_target_progressive_preparation', async facts => {
      ownerDenied = false; sharedSource = true; await visit('en-US', 390, 'dark'); await openSharedGuide();
      facts.guide_layout = await layout(page.locator('[data-shared-pool-guide]')); await page.locator('[data-shared-pool-prepare]').press('Enter');
      await page.locator('[data-pool-recovery-state=read]').waitFor(); assert.equal(await page.locator('[data-pool-tool-intent=setup]').getAttribute('data-target'), setupPanel());
      facts.card_layout = await layout(page.locator('[data-pool-tool-recovery]')); await page.locator('[data-pool-recovery-prepare]').click();
      const panel = page.locator('[data-pool-config-preparation]'); await panel.waitFor(); facts.form_layout = await layout(panel);
      await page.locator('[data-pool-config-row=scope-1]').scrollIntoViewIfNeeded(); await image('en-mobile-shared-tool-preparation.png');
      assert.equal(await page.locator('[data-remote-runner-setup]').count(), 0); facts.no_ordinary_identity = true;
    });
    }
    const matches = result.requests.filter(request => request.method === 'POST' && request.path === '/api/v1/tool-profiles/matches');
    assert.ok(matches.every(request => request.body.project_id === project && request.body.worker_id === worker));
    result.read_only_matches_posts = matches.length;
    result.business_writes = result.requests.filter(request => request.method !== 'GET' && !matches.includes(request)).length;
    result.application_api_requests = result.requests.filter(request => request.path.startsWith('/api/')).length;
    result.invented_profile_reads = result.requests.filter(request => /^\/api\/v1\/tool-profiles\//.test(request.path)).length;
    assert.equal(result.business_writes, 0); assert.equal(result.invented_profile_reads, 0); assert.deepEqual(result.unexpected, []); assert.deepEqual(result.page_errors, []); assert.deepEqual(result.console_errors, []); result.checks_passed = true;
  } finally {
    if (app) Object.assign(result, await closeOwnedNative(app, exit));
    if (server) {await new Promise(resolve => server.close(resolve)); result.port_closed = await closedPort(result.port);} save();
  }
  assert.equal(result.forced, false); assert.deepEqual(result.native_exit, {code: 0, signal: null}); assert.equal(result.port_closed, true); result.passed = true; save();
  console.log(JSON.stringify({directory: path.relative(root, directory), passed: true, groups: Object.keys(result.groups), native_exit: result.native_exit, port_closed: result.port_closed, business_writes: result.business_writes, images: result.images}));
}
main().catch(error => {console.error(error.stack || String(error)); process.exitCode = 1;});
