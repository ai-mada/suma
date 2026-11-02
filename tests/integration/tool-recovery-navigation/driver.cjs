'use strict';
// Production Workspace, original HTTP readers and recovery journal; all public
// responses below are synthetic. This is not a machine scan or a repair proof.
const fs = require('node:fs');
const path = require('node:path');
const http = require('node:http');
const net = require('node:net');
const crypto = require('node:crypto');
const assert = require('node:assert/strict');
const {closeOwnedNative} = require('../support/close-native.cjs');

const root = path.resolve(__dirname, '../../..');
const base = path.join(root, '.cache/tool-recovery-navigation-217');
const out = path.join(base, 'web/out');
const directory = path.join(base, 'visual', new Date().toISOString().replace(/[^0-9TZ]/g, ''));
const hash = value => crypto.createHash('sha256').update(value).digest('hex');
const project = 'fixture-project-217', worker = 'ordinary-node-a', decoy = 'ordinary-node-b';
const shared = 'pool-shared-node-217', profileID = 'codex-on-node-a', sharedProfileID = 'codex-on-shared';
const session = {
  user: {id: 'fixture-owner-217', name: 'Fixture owner', email: 'owned@example.test', role: 'owner'},
  organization: {id: 'fixture-organization-217', name: 'Fixture organization'}, projects: [],
};
const definition = {
  command_id: 'node-a-codex', adapter: 'codex', protocol: 'codex-jsonl-v1',
  profile_sha256: 'a'.repeat(64), executable_sha256: 'b'.repeat(64),
  tool_version: '1.0.0', read_only: false,
};
const tool = (id, name, workerID) => ({
  id, project_id: project, worker_id: workerID, name, version: 1, state: 'enabled',
  profile: definition, enabled_by: session.user.id,
  created_at: '2026-11-03T00:00:00Z', updated_at: '2026-11-03T00:00:00Z',
  available: false, reason: 'inventory_stale',
});
const profiles = [tool(profileID, 'Codex · node A', worker), tool(sharedProfileID, 'Codex · shared Pool', shared)];
const unknownOnly = process.argv.includes('--unknown-only');
const englishOnly = process.argv.includes('--en-only') || unknownOnly;
const adapters = [{
  id: 'codex-jsonl', product: 'Codex', entrypoint_kind: 'cli', direction: 'suma_to_tool',
  implementation: 'implemented', setup_kind: 'runner_profile', adapter: 'codex',
  protocol: 'codex-jsonl-v1', capabilities: [], source_refs: [], limitations: [],
}];
const result = {
  stage: 'T-TOOL-RECOVERY-NAVIGATION-217',
  scope: 'Synthetic public HTTP + production Workspace; no actual scan, repair, Worker, vendor or model',
  browser_path: 'Installed isolated Electron/Playwright harness; active application browser not exercised',
  groups_requested: unknownOnly ? 'Unknown-operation departure only; three passed groups retained separately'
    : englishOnly ? 'English affected groups only; original Chinese passes retained separately' : 'All four representative groups',
  passed: false, groups: {}, requests: [], unexpected: [], images: [], page_errors: [], console_errors: [], console_warnings: [],
};
let inventoryAt, lastDiscoveryRead = 0;
fs.mkdirSync(directory, {recursive: true});

function currentProfile(profile) {
  if (profile.worker_id !== worker || !inventoryAt || Date.now() >= inventoryAt + 60000) return profile;
  return {...profile, available: true, reason: '', last_seen_at: new Date(inventoryAt).toISOString(),
    observation: {profile: definition, available: true, reason: ''}};
}

function diagnostics(profile, at) {
  return {
    tool: profile, checked_at: at, expires_at: new Date(Date.parse(at) + 60000).toISOString(),
    checks: ['authorization', 'worker', 'inventory', 'profile'].map(code => ({
      code, status: code === 'inventory' && !profile.available ? 'blocked' : 'pass',
      reason: code === 'inventory' && !profile.available ? 'inventory_stale' : '',
      next_action: code === 'inventory' && !profile.available ? 'open_worker' : 'none',
    })),
    next_action: 'open_worker',
    maintenance_resolution: {profile_id: profile.id, profile_version: profile.version,
      worker_id: profile.worker_id, action: 'open_worker', reason: 'maintenance_unknown'},
  };
}

function closedPort(port) {
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
      const route = url.pathname.replace(/^\/api\/v1/, '');
      const at = new Date(Math.max(Date.now(), inventoryAt || 0)).toISOString();
      const recorded = {method: request.method, path: url.pathname, query: url.search};
      result.requests.push(recorded);
      const send = (body, status = 200) => {
        response.writeHead(status, {'Content-Type': 'application/json', 'Cache-Control': 'no-store'});
        response.end(JSON.stringify(body));
      };
      if (request.method !== 'GET') {
        if (request.method === 'POST' && route === '/tool-profiles/matches') {
          let body = '';
          request.on('data', chunk => { body += chunk; });
          request.on('end', () => {
            recorded.body = JSON.parse(body);
            send({project_id: project, worker_id: recorded.body.worker_id, profile: recorded.body.profile, items: [], total: 0});
          });
          return;
        }
        result.unexpected.push(request.method + ' ' + route);
        send({error: {code: 'fixture_write_refused', message: 'This synthetic interface fixture refuses business mutations'}}, 405);
        return;
      }
      if (url.pathname === '/__fixture/view') return send({project_id: project, worker_id: worker, initial_profile: profileID, session});
      if (url.pathname === '/__fixture/closed') {
        response.writeHead(200, {'Content-Type': 'text/html', 'Cache-Control': 'no-store'});
        response.end('<!doctype html><title>Fixture closed</title><p>Explicitly left the synthetic product view.</p>');
        return;
      }
      if (url.pathname === '/favicon.ico') { response.writeHead(204); response.end(); return; }
      if (route === '/tool-profiles') return send({items: profiles.map(currentProfile), total: profiles.length});
      if (route === '/release-workers') return send({items: [worker, decoy].map((id, index) => ({
        id: 'owned-identity-' + index, worker_id: id, status: 'active', version: 1, profiles: [], deployment: false,
      }))});
      if (route === '/project-execution/runners') return send({as_of: at, items: [{
        id: 'shared-node-217', worker_id: shared, name: 'Shared Pool node', pool_id: 'shared-pool-217',
        status: 'active', max_slots: 2, version: 1,
      }]});
      if (route === '/adapter-catalog') return send({project_id: project, as_of: at, items: adapters});
      if (route === '/tool-discovery') {
        const selectedWorker = url.searchParams.get('worker_id');
        lastDiscoveryRead = Date.parse(at);
        if (selectedWorker === worker && inventoryAt) return send({
          project_id: project, worker_id: worker, state: 'fresh', as_of: at,
          observed_at: new Date(inventoryAt).toISOString(), expires_at: new Date(inventoryAt + 60000).toISOString(),
          items: [{profile: definition, available: true, reason: ''}],
        });
        return send({project_id: project, worker_id: selectedWorker, state: 'unreported', as_of: at, items: []});
      }
      const profile = profiles.find(value => route === '/tool-profiles/' + value.id || route === '/tool-profiles/' + value.id + '/diagnostics');
      if (profile) return send(route.endsWith('/diagnostics') ? diagnostics(currentProfile(profile), at) : currentProfile(profile));
      if (/^\/tool-profiles\/[^/]+\/(replacements|inspections)$/.test(route)) return send({
        project_id: project, profile_id: route.split('/')[2], direction: url.searchParams.get('direction'), items: [],
      });
      if (url.pathname.startsWith('/api/')) {
        result.unexpected.push(request.method + ' ' + route);
        return send({error: {code: 'fixture_missing_read', message: 'No synthetic data for this route'}}, 404);
      }
      const file = {'/': 'index.html', '/fixture.js': 'fixture.js', '/fixture.css': 'fixture.css'}[url.pathname];
      if (!file) { response.writeHead(404); response.end(); return; }
      response.writeHead(200, {'Content-Type': file.endsWith('.js') ? 'text/javascript' : file.endsWith('.css') ? 'text/css' : 'text/html'});
      response.end(fs.readFileSync(path.join(out, file)));
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
      SUMA_FIXTURE_ORIGIN: origin, SUMA_FIXTURE_DIRECTORY: directory, SUMA_FIXTURE_PARTITION: 'suma-fixture-tool-recovery217'});
    fs.mkdirSync(env.TEMP, {recursive: true});
    app = await _electron.launch({executablePath: path.join(root, 'apps/desktop/node_modules/electron/dist/electron.exe'),
      args: [path.join(__dirname, '../support/native.cjs')], cwd: root, env, timeout: 15000});
    result.native_pid = app.process().pid;
    nativeExit = new Promise(resolve => app.process().once('exit', (code, signal) => resolve({code, signal})));
    const page = await app.firstWindow();
    page.setDefaultTimeout(12000);
    let explicitDeparture = false;
    const dialogActions = [];
    result.observed_beforeunload_events = 0;
    page.on('pageerror', error => result.page_errors.push(error.message));
    page.on('dialog', dialog => {
      if (explicitDeparture && dialog.type() === 'beforeunload'
        && new URL(page.url()).origin === origin && result.observed_beforeunload_events === 0) {
        // This Electron host has no browser-native dialog to accept. Its
        // one explicit main-process Leave decision is made below instead.
        result.observed_beforeunload_events++;
      } else {
        result.page_errors.push('Unexpected ' + dialog.type());
        dialogActions.push(dialog.dismiss().catch(error => result.page_errors.push('Dialog dismissal failed: ' + String(error))));
      }
    });
    page.on('console', message => {
      if (message.type() === 'error') result.console_errors.push(message.text());
      if (message.type() === 'warning') result.console_warnings.push(message.text());
    });
    async function visit(locale, width, theme, suffix = '') {
      await page.setViewportSize({width, height: width === 390 ? 844 : 900});
      await page.goto(origin + '/?locale=' + locale + '&theme=' + theme + suffix);
      await page.locator('[data-tool-recovery-fixture]').waitFor();
      assert.equal(await page.title(), 'suma · tool recovery navigation fixture');
      assert.ok(await page.locator('.tool-workspace').isVisible());
      assert.equal(await page.locator('[data-tool-workspace-configurations]').count(), 1);
      assert.equal(await page.locator('vite-error-overlay').count(), 0);
    }
    async function group(name, run) {
      result.groups[name] = {passed: false};
      try { await run(result.groups[name]); result.groups[name].passed = true; }
      catch (error) {
        result.groups[name].error = String(error.stack || error);
        await page.screenshot({path: path.join(directory, name + '-failure.png')}).catch(() => {});
        throw error;
      } finally { save(); }
    }
    async function image(name) {
      await page.screenshot({path: path.join(directory, name), fullPage: false});
      result.images.push({path: name, sha256: hash(fs.readFileSync(path.join(directory, name)))});
    }
    async function layout(element) {
      const value = await element.evaluate(node => ({width: innerWidth, scroll_width: document.documentElement.scrollWidth,
        controls: [...node.querySelectorAll('button,summary,input:not([type=checkbox]),select')]
          .filter(control => control.getClientRects().length).map(control => ({
            label: control.getAttribute('aria-label') || control.textContent?.trim().slice(0, 70) || control.tagName,
            height: control.getBoundingClientRect().height,
          })),
      }));
      assert.ok(value.scroll_width <= value.width, 'Page must not overflow horizontally');
      assert.ok(value.controls.every(control => control.height >= 43.5), JSON.stringify(value.controls.filter(control => control.height < 43.5)));
      return value;
    }
    if (!englishOnly) {
    await group('cn_exact_worker_to_original_guide', async facts => {
      await visit('zh-CN', 1366, 'light');
      await page.locator('[data-tool-doctor-state="ready"]').waitFor();
      await page.locator('[data-tool-doctor-maintenance-action="open_worker"]').click();
      const recovery = page.locator('[data-tool-worker-recovery="' + worker + '"]');
      await recovery.waitFor();
      assert.equal(await page.locator('[data-tool-worker-recovery="' + decoy + '"]').count(), 0);
      const prepare = page.locator('[data-tool-worker-prepare="' + worker + '"]');
      await prepare.click();
      const guide = page.locator('[data-remote-runner-setup]');
      await guide.waitFor();
      assert.equal(await page.locator('[name=remote-runner-worker]').inputValue(), worker);
      assert.equal(await page.locator('.tool-cli-setup-select select').inputValue(), 'codex');
      await page.waitForFunction(() => document.activeElement?.matches('[data-remote-runner-setup]>summary'));
      await page.waitForTimeout(220);
      assert.ok(await page.evaluate(() => document.activeElement?.matches('[data-remote-runner-setup]>summary')));
      facts.layout = await layout(guide);
      facts.worker_id = worker;
      facts.focus = await page.evaluate(() => document.activeElement?.outerHTML.slice(0, 300));
      await guide.evaluate(node => node.scrollIntoView({block: 'start'}));
      await image('cn-exact-worker-guide.png');
    });
    await group('cn_synthetic_new_inventory_to_exact_quick', async facts => {
      await page.locator('[data-tool-report-check]').click();
      await page.locator('[data-tool-report-status="unreported"]').waitFor();
      await page.locator('[data-tool-report-wait]').click();
      await page.locator('[data-tool-report-status="waiting"]').waitFor();
      inventoryAt = Math.max(Date.now(), lastDiscoveryRead + 1);
      await page.locator('[data-tool-report-status="new"]').waitFor();
      await page.locator('[data-tool-report-selection="' + definition.command_id + '"]').focus();
      await page.keyboard.press('Enter');
      const dialog = page.locator('[role=dialog]');
      await dialog.waitFor();
      assert.equal(await dialog.locator('[data-quick-tool]').inputValue(), 'codex-jsonl');
      assert.equal(await dialog.locator('[data-quick-worker]').inputValue(), worker);
      // This is the original profile and the same exact configuration. The
      // production Quick view must retain that source rather than offer a
      // duplicate registration or discard its explicit name.
      await dialog.locator('[data-quick-replacement-unchanged]').waitFor();
      await dialog.locator('[data-existing-tool-open="' + profileID + '"]').waitFor();
      assert.equal(await dialog.locator('[data-quick-register]').count(), 0);
      facts.worker_id = worker; facts.command_id = definition.command_id;
      facts.original_profile_retained = profileID;
      await page.keyboard.press('Escape');
      await page.waitForFunction(() => !document.querySelector('[role=dialog]'));
    });
    }
    if (!unknownOnly) {
    await group('en_shared_boundary_and_ordinary_actions', async facts => {
      await visit('en-US', 390, 'dark', '&selected=' + sharedProfileID);
      await page.locator('[data-tool-doctor-state="ready"]').waitFor();
      await page.locator('[data-tool-doctor-maintenance-action="open_worker"]').click();
      await page.waitForFunction(id => document.activeElement?.getAttribute('data-tool-shared-worker') === id, shared);
      assert.equal(await page.locator('[data-tool-worker-prepare="' + shared + '"]').count(), 0);
      facts.shared_pool_does_not_offer_project_identity = true;
      await visit('en-US', 390, 'dark');
      await page.locator('[data-tool-doctor-state="ready"]').waitFor();
      const actions = page.locator('[data-tool-profile-actions]');
      await actions.waitFor();
      facts.actionbar_layout = await layout(actions);
      await page.locator('[data-tool-doctor-maintenance-action="open_worker"]').click();
      const prepare = page.locator('[data-tool-worker-prepare="' + worker + '"]');
      await prepare.waitFor();
      assert.ok((await prepare.innerText()).length > 0);
      assert.ok((await prepare.boundingBox()).height >= 43.5);
      facts.prepare_label = await prepare.innerText();
      facts.layout = await layout(page.locator('[data-tool-worker-recovery="' + worker + '"]'));
      await actions.evaluate(node => node.scrollIntoView({block: 'start'}));
      await image('en-ordinary-actions.png');
    });
    }
    await group('en_original_unknown_operation_remains_first', async facts => {
      await visit('en-US', 390, 'dark', '&pending=1');
      const notice = page.locator('.tool-recovery').first();
      await notice.waitFor();
      const key = 'suma-tool-recovery-v1:' + session.organization.id + ':' + session.user.id + ':' + project + ':profiles';
      const before = await page.evaluate(key => sessionStorage.getItem(key), key);
      assert.ok(before);
      const original = JSON.parse(before);
      assert.ok((await notice.innerText()).includes(original.idempotency_key));
      assert.equal(await page.locator('[data-tools-add]').isDisabled(), true);
      const preparations = page.locator('[data-tool-worker-prepare]');
      for (let index = 0; index < await preparations.count(); index++) {
        assert.equal(await preparations.nth(index).isDisabled(), true);
      }
      assert.equal(await notice.evaluate(node => !!node.closest('details')), false);
      assert.equal(await page.evaluate(key => sessionStorage.getItem(key), key), before);
      facts.original_journal_sha256 = hash(before);
      facts.original_key = original.idempotency_key;
      // The owned synthetic Electron host makes one explicit Leave choice,
      // using its official will-prevent-unload event. This does not disable a
      // product guard, alter production Desktop or confirm any business work.
      explicitDeparture = true;
      try {
        result.departure_host_choice = await app.evaluate(async ({BrowserWindow}, input) => {
          const windows = BrowserWindow.getAllWindows();
          if (windows.length !== 1) throw Error('Expected the single owned fixture window');
          const contents = windows[0].webContents;
          const destination = new URL(input.destination);
          if (new URL(contents.getURL()).origin !== input.origin || destination.origin !== input.origin
            || destination.pathname !== '/__fixture/closed' || destination.search || destination.hash) {
            throw Error('Explicit fixture departure target changed');
          }
          let budget = 1, decisions = 0;
          const leave = event => {
            if (budget !== 1 || contents.isDestroyed() || new URL(contents.getURL()).origin !== input.origin) return;
            budget = 0;
            decisions++;
            event.preventDefault();
          };
          contents.on('will-prevent-unload', leave);
          try {
            await contents.loadURL(input.destination);
            return {source_origin: input.origin, destination: input.destination, explicit_leave_decisions: decisions};
          } finally { contents.removeListener('will-prevent-unload', leave); }
        }, {origin, destination: origin + '/__fixture/closed'});
      }
      finally { explicitDeparture = false; }
      await Promise.all(dialogActions);
      assert.equal(page.url(), origin + '/__fixture/closed');
      assert.equal(await page.evaluate(key => sessionStorage.getItem(key), key), before);
      facts.journal_retained_after_explicit_departure = true;
    });
    const matches = result.requests.filter(request => request.method === 'POST' && request.path === '/api/v1/tool-profiles/matches');
    if (!englishOnly) assert.ok(matches.length >= 1);
    assert.ok(matches.every(request => request.body.project_id === project && request.body.worker_id === worker
      && request.body.profile.command_id === definition.command_id && request.body.profile.profile_sha256 === definition.profile_sha256));
    result.read_only_match_posts = matches.length;
    result.business_writes = result.requests.filter(request => request.method !== 'GET' && !matches.includes(request)).length;
    assert.equal(result.business_writes, 0);
    assert.ok(result.requests.filter(request => request.path === '/api/v1/tool-discovery').every(request => new URLSearchParams(request.query).get('worker_id') === worker));
    assert.deepEqual(result.unexpected, []);
    assert.deepEqual(result.page_errors, []);
    assert.deepEqual(result.console_errors, []);
    result.checks_passed = true;
  } finally {
    if (app) Object.assign(result, await closeOwnedNative(app, nativeExit));
    if (server) {
      await new Promise(resolve => server.close(resolve));
      result.port_closed = await closedPort(result.port);
    }
    save();
  }
  assert.equal(result.forced, false);
  assert.deepEqual(result.native_exit, {code: 0, signal: null});
  assert.equal(result.port_closed, true);
  result.passed = true;
  save();
  console.log(JSON.stringify({directory: path.relative(root, directory), passed: result.passed,
    groups: Object.keys(result.groups), images: result.images, read_only_match_posts: result.read_only_match_posts,
    business_writes: result.business_writes, native_exit: result.native_exit, port_closed: result.port_closed}));
}
main().catch(error => { console.error(error.stack || String(error)); process.exitCode = 1; });
