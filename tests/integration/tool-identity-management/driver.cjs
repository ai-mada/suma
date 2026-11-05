'use strict';

const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const http = require('node:http');
const net = require('node:net');
const path = require('node:path');

const root = path.resolve(__dirname, '../../..');
const base = path.join(root, '.cache/tool-identity-management');
const output = path.join(base, 'web/out');
const visual = path.join(base, 'visual');
const project = '1'.repeat(32);
const session = {
  user: {id: '2'.repeat(32), name: 'Fixture owner', email: 'owner@example.test', role: 'owner'},
  organization: {id: '3'.repeat(32), name: 'Fixture organization'},
  projects: [],
};
const api = '/api/v1';
const at = Date.now();
const date = delta => new Date(at + delta).toISOString();
const worker = (id, worker_id, status, expires_at, version) => ({
  id: id.repeat(32), project_id: project, worker_id, status, version,
  profiles: [], deployment: false, created_at: date(-86400000), expires_at,
});
const nodeToRevoke = worker('4', 'review-current', 'active', date(3600000), 3);
const nodeToConfigure = worker('5', 'review-connect', 'active', date(7200000), 1);
const expired = worker('6', 'review-expired', 'active', date(-3600000), 1);
const revoked = worker('7', 'review-revoked', 'revoked', date(3600000), 2);
const readScope = 'user.task.readable';
const createScope = 'user.task.invokable';
const rawReason = 'synthetic vendor detail C:/private/fixture-account';
const connection = {
  id: '8'.repeat(32), project_id: project, owner_id: session.user.id,
  client_id: 'fixture-workbuddy-app', open_id: 'review-account',
  version: 3, scopes: [readScope], state: 'connected',
  grant_generation: 1, token_generation: 2, expires_at: date(3600000),
  created_at: date(-86400000), updated_at: date(-60000),
  last_error: rawReason, allowed_actions: ['authorize', 'refresh', 'disconnect'],
};
const oldConnection = {
  ...connection, id: '9'.repeat(32), open_id: 'previous-account', version: 4,
  state: 'disconnected', grant_generation: 2, expires_at: '', last_error: '',
  allowed_actions: ['authorize'],
};
const revokePath = '/release-workers/' + nodeToRevoke.id;
const disconnectPath = '/workbuddy-connections/' + connection.id + '/disconnect';
const adapter = {
  id: 'codex-jsonl', product: 'Codex', entrypoint_kind: 'cli', direction: 'suma_to_tool',
  implementation: 'implemented', setup_kind: 'runner_profile', adapter: 'codex',
  protocol: 'codex-jsonl-v1', capabilities: [],
  source_refs: ['internal/runner/tool_stream.go'], limitations: ['foreground_only_no_resume'],
};
const hash = bytes => crypto.createHash('sha256').update(bytes).digest('hex');

function json(response, value, status = 200) {
  response.writeHead(status, {'Content-Type': 'application/json', 'Cache-Control': 'no-store'});
  response.end(JSON.stringify(value));
}

async function mutationBody(request, expected) {
  assert.match(request.headers['content-type'] || '', /^application\/json\b/i);
  let size = 0;
  const chunks = [];
  for await (const chunk of request) {
    size += chunk.length;
    assert.ok(size <= 8192, 'Fixture write body exceeds the limit');
    chunks.push(chunk);
  }
  const value = JSON.parse(Buffer.concat(chunks).toString('utf8'));
  const {idempotency_key, ...fields} = value;
  assert.deepEqual(fields, expected);
  assert.match(idempotency_key, /^[A-Za-z0-9][A-Za-z0-9_.:-]{7,127}$/);
  return value;
}

function checkPortClosed(port) {
  return new Promise(resolve => {
    const socket = net.connect({host: '127.0.0.1', port});
    socket.setTimeout(1000);
    socket.once('connect', () => { socket.destroy(); resolve(false); });
    socket.once('timeout', () => { socket.destroy(); resolve(false); });
    socket.once('error', error => resolve(error.code === 'ECONNREFUSED'));
  });
}

async function main() {
  assert.ok(process.env.SUMA_PLAYWRIGHT_MODULE, 'Set SUMA_PLAYWRIGHT_MODULE to the existing Playwright runtime');
  const {chromium} = require(process.env.SUMA_PLAYWRIGHT_MODULE);
  fs.mkdirSync(visual, {recursive: true});
  const directory = fs.mkdtempSync(path.join(visual, 'run-'));
  const report = {
    scope: 'Production ToolWorkspace + synthetic loopback HTTP; two isolated interface journeys',
    boundaries: {
      full_app: false, production_ipc: false, real_database: false,
      provider_requests: 0, model_requests: 0, runner_or_command_execution: 0,
    },
    groups: {}, requests: [], writes: [], unexpected: [], images: [],
    page_errors: [], console_errors: [], passed: false,
  };
  let group = 'nodes';
  let allowedWrite = '';
  let node = nodeToRevoke;
  let account = connection;
  let releaseDisconnect;
  const disconnectAcknowledgement = new Promise(resolve => { releaseDisconnect = resolve; });
  let browser, port;

  // Every mutation is confined to this in-memory fixture. The driver grants a
  // one-use exact method/path only at the corresponding confirmation click.
  const server = http.createServer((request, response) => {
    async function serve() {
      const url = new URL(request.url, 'http://127.0.0.1');
      const method = request.method;
      const route = url.pathname;
      report.requests.push({group, method, path: route, query: url.search});
      if (route.startsWith(api + '/')) {
        const endpoint = route.slice(api.length);
        if (method !== 'GET') {
          assert.equal(request.headers['x-suma-expected-user'], session.user.id);
          assert.equal(request.headers['x-suma-expected-organization'], session.organization.id);
          const operation = method + ' ' + endpoint;
          assert.equal(operation, allowedWrite);
          allowedWrite = '';
          if (group === 'nodes' && operation === 'PATCH ' + revokePath) {
            const body = await mutationBody(request, {project_id: project, expected_version: node.version, status: 'revoked'});
            node = {...node, status: 'revoked', version: node.version + 1};
            report.writes.push({group, method, path: endpoint, body, response_version: node.version});
            return json(response, node);
          }
          if (group === 'workbuddy' && operation === 'POST ' + disconnectPath) {
            const body = await mutationBody(request, {
              project_id: project, expected_version: account.version, reason: 'user_requested_local_disconnect',
            });
            account = {
              ...account, state: 'disconnected', version: account.version + 1,
              grant_generation: account.grant_generation + 1, expires_at: '',
              last_error: '', allowed_actions: ['authorize'], updated_at: new Date().toISOString(),
            };
            report.writes.push({group, method, path: endpoint, body, response_version: account.version});
            await disconnectAcknowledgement;
            return json(response, account);
          }
          throw new Error('Unexpected fixture write');
        }
        assert.equal(url.searchParams.get('project_id'), project);
        const as_of = new Date().toISOString();
        if (endpoint === '/tool-profiles') return json(response, {items: [], total: 0});
        if (endpoint === '/release-workers') return json(response, {items: [node, nodeToConfigure, expired, revoked]});
        if (endpoint === '/project-execution/runners') return json(response, {items: [], as_of});
        if (endpoint === '/adapter-catalog') return json(response, {project_id: project, as_of, items: [adapter]});
        if (endpoint === '/tool-discovery') {
          assert.equal(url.searchParams.get('worker_id'), nodeToConfigure.worker_id);
          return json(response, {
            project_id: project, worker_id: nodeToConfigure.worker_id, state: 'unreported',
            as_of, observed_at: '', expires_at: '', items: [],
          });
        }
        if (endpoint === '/workbuddy-connections' && group === 'workbuddy') {
          assert.equal(url.searchParams.get('limit'), '20');
          return json(response, {
            project_id: project, configured: true, can_authorize: true,
            available_scopes: [readScope, createScope], limit: 20, next_cursor: '',
            items: [account, oldConnection],
          });
        }
        throw new Error('Unexpected fixture read');
      }
      assert.equal(method, 'GET');
      if (route === '/__fixture/view') return json(response, {project_id: project, session});
      if (route === '/favicon.ico') { response.writeHead(204); response.end(); return; }
      const assets = {
        '/': ['index.html', 'text/html'],
        '/fixture.js': ['fixture.js', 'text/javascript'],
        '/fixture.css': ['fixture.css', 'text/css'],
      };
      assert.ok(Object.hasOwn(assets, route), 'Unexpected fixture asset');
      const [name, contentType] = assets[route];
      response.writeHead(200, {'Content-Type': contentType, 'Cache-Control': 'no-store'});
      response.end(fs.readFileSync(path.join(output, name)));
    }
    void serve().catch(error => {
      report.unexpected.push({group, method: request.method, path: request.url, reason: error.message});
      if (response.headersSent) response.end();
      else json(response, {error: {code: 'fixture_refused', message: 'Fixture request refused'}}, 400);
    });
  });

  try {
    await new Promise((resolve, reject) => {
      server.once('error', reject);
      server.listen(0, '127.0.0.1', resolve);
    });
    port = server.address().port;
    const origin = 'http://127.0.0.1:' + port;
    report.origin_port = port;
    const executablePath = process.env.SUMA_BROWSER_EXECUTABLE;
    assert.ok(executablePath && path.isAbsolute(executablePath), 'Set SUMA_BROWSER_EXECUTABLE to an installed Chromium browser');
    browser = await chromium.launch({executablePath, headless: true, chromiumSandbox: true, timeout: 15000});
    const context = await browser.newContext();
    await context.route('**/*', route => {
      if (new URL(route.request().url()).origin === origin) return route.continue();
      return route.abort();
    });
    const page = await context.newPage();
    page.setDefaultTimeout(10000);
    page.on('pageerror', error => report.page_errors.push(error.message));
    page.on('console', message => {
      if (message.type() === 'error') report.console_errors.push(message.text());
    });

    const locateWorker = value => page.locator('[data-tool-worker="' + value.worker_id + '"]');
    const filter = value => page.locator('[data-tool-worker-filter="' + value + '"]');
    const writeRequests = () => report.requests.filter(request => request.method !== 'GET');
    async function verifyPage(locale, theme, title) {
      await page.locator('[data-tool-identity-fixture="' + project + '"] .tool-workspace').waitFor();
      assert.equal(await page.title(), title + ' · suma');
      assert.equal(await page.getByRole('heading', {name: title, level: 1, exact: true}).count(), 1);
      assert.equal(await page.locator('html').getAttribute('lang'), locale);
      assert.equal(await page.locator('html').getAttribute('data-theme'), theme);
      assert.equal(await page.locator('vite-error-overlay, [role="dialog"]:visible').count(), 0);
      assert.ok((await page.locator('.tool-workspace').innerText()).trim().length > 80);
    }
    async function metrics(panel) {
      const value = await panel.evaluate(element => ({
        width: innerWidth,
        scrollWidth: document.documentElement.scrollWidth,
        controls: [...element.querySelectorAll('button,summary,input:not([type=checkbox]),select')]
          .filter(control => control.getClientRects().length)
          .map(control => ({
            label: control.getAttribute('aria-label') || control.textContent?.trim().slice(0, 80) || control.tagName,
            height: control.getBoundingClientRect().height,
          })),
      }));
      assert.ok(value.controls.length > 0);
      assert.ok(value.scrollWidth <= value.width, JSON.stringify(value));
      assert.ok(value.controls.every(control => control.height >= 43.5), JSON.stringify(value.controls));
      return value;
    }
    async function screenshot(name, focus) {
      await focus.evaluate(element => element.scrollIntoView({block: 'start'}));
      const file = path.join(directory, name);
      await page.screenshot({path: file, fullPage: false, animations: 'disabled'});
      report.images.push({path: name, sha256: hash(fs.readFileSync(file))});
    }

    await page.setViewportSize({width: 1366, height: 900});
    await page.goto(origin + '/?locale=zh-CN&theme=light');
    await verifyPage('zh-CN', 'light', '工具管理');
    await locateWorker(nodeToRevoke).waitFor();
    assert.equal(await filter('current').getAttribute('aria-pressed'), 'true');
    assert.equal(await locateWorker(expired).isVisible(), false);
    assert.equal(await locateWorker(revoked).isVisible(), false);
    const nodePanel = page.locator('.tool-worker-identities');
    const nodeLayout = await metrics(nodePanel);
    await screenshot('cn-current-identities.png', nodePanel);
    await filter('attention').click();
    await locateWorker(expired).waitFor();
    assert.equal(await locateWorker(expired).getAttribute('data-tool-worker-credential-state'), 'expired');
    assert.equal(await locateWorker(expired).locator('[data-tool-worker-scan], [data-tool-worker-connect]').count(), 0);
    await filter('revoked').click();
    await locateWorker(revoked).waitFor();
    assert.equal(await locateWorker(revoked).locator('[data-tool-worker-revoke], [data-tool-worker-scan]').count(), 0);
    await filter('current').click();
    await locateWorker(nodeToRevoke).locator('[data-tool-worker-revoke]').click();
    const dialog = page.getByRole('dialog');
    await dialog.waitFor();
    assert.ok((await dialog.innerText()).includes(nodeToRevoke.worker_id));
    assert.equal(writeRequests().length, 0);
    allowedWrite = 'PATCH ' + revokePath;
    const [revokeResponse] = await Promise.all([
      page.waitForResponse(response => new URL(response.url()).pathname === api + revokePath && response.request().method() === 'PATCH'),
      dialog.locator('form .dialog-actions button.primary').click(),
    ]);
    assert.equal(revokeResponse.status(), 200);
    await dialog.waitFor({state: 'hidden'});
    await locateWorker(nodeToRevoke).waitFor({state: 'hidden'});
    await filter('revoked').click();
    await locateWorker(nodeToRevoke).waitFor();
    assert.equal(await locateWorker(nodeToRevoke).getAttribute('data-tool-worker-credential-state'), 'revoked');
    assert.equal(writeRequests().length, 1);
    await screenshot('cn-revoked-identities.png', nodePanel);

    await filter('current').click();
    await locateWorker(nodeToConfigure).locator('[data-tool-worker-connect]').click();
    const toolChoice = dialog.locator('[data-quick-tool]');
    await toolChoice.locator('option[value="codex-jsonl"]').waitFor({state: 'attached'});
    const [inventoryRequest] = await Promise.all([
      page.waitForRequest(request => new URL(request.url()).pathname === api + '/tool-discovery'),
      toolChoice.selectOption('codex-jsonl'),
    ]);
    assert.equal(new URL(inventoryRequest.url()).searchParams.get('worker_id'), nodeToConfigure.worker_id);
    await page.keyboard.press('Escape');
    await dialog.waitFor({state: 'hidden'});
    assert.equal(writeRequests().length, 1);
    await verifyPage('zh-CN', 'light', '工具管理');
    report.groups.cn_nodes = {
      passed: true, layout: nodeLayout, expired_scan_available: false,
      configure_worker: nodeToConfigure.worker_id, configure_writes: 0, confirmed_patches: 1,
    };

    group = 'workbuddy';
    await page.setViewportSize({width: 390, height: 844});
    await page.goto(origin + '/?locale=en-US&theme=dark');
    await verifyPage('en-US', 'dark', 'Tool management');
    const workbuddy = page.locator('.workbuddy-connections');
    await workbuddy.locator('.workbuddy-disclosure').click();
    const accountRow = workbuddy.locator('[data-workbuddy-connection-id="' + connection.id + '"]');
    const previousRow = workbuddy.locator('[data-workbuddy-connection-id="' + oldConnection.id + '"]');
    await accountRow.waitFor();
    const history = workbuddy.locator('.workbuddy-history-toggle');
    assert.equal(await history.getAttribute('aria-expanded'), 'false');
    assert.equal(await previousRow.isVisible(), false);
    await history.click();
    await previousRow.waitFor();
    await history.click();
    await previousRow.waitFor({state: 'hidden'});
    assert.match(await accountRow.locator('.workbuddy-connection-reason').innerText(), /The connection needs attention/);
    assert.ok(!(await workbuddy.innerText()).includes(rawReason));

    await accountRow.getByRole('button', {name: 'Authorize again', exact: true}).click();
    const form = workbuddy.locator('.workbuddy-form');
    const checks = form.locator('input[type="checkbox"]');
    assert.equal(await checks.count(), 2);
    assert.equal(await form.locator('input:checked').count(), 0);
    assert.equal(await form.locator('button[type="submit"]').isDisabled(), true);
    await form.getByRole('button', {name: 'Reuse current requestable permissions', exact: true}).click();
    assert.equal(await form.locator('input[value="' + readScope + '"]').isChecked(), true);
    assert.equal(await form.locator('input[value="' + createScope + '"]').isChecked(), false);
    assert.match(await form.locator('section[aria-label="Requested again"]').innerText(), /Read WorkBuddy tasks/);
    assert.match(await form.locator('section[aria-label="New requests"]').innerText(), /None/);
    assert.equal(writeRequests().filter(request => request.group === 'workbuddy').length, 0);
    const scopeLayout = await metrics(workbuddy);
    await screenshot('en-permission-review.png', form);
    await form.getByRole('button', {name: 'Cancel', exact: true}).click();

    await accountRow.getByRole('button', {name: 'Disconnect locally', exact: true}).click();
    const confirm = form.getByRole('button', {name: 'Confirm local disconnection', exact: true});
    assert.equal(await confirm.isDisabled(), true);
    assert.equal(writeRequests().filter(request => request.group === 'workbuddy').length, 0);
    await form.locator('input[name="confirm-disconnect"]').check();
    assert.equal(writeRequests().filter(request => request.group === 'workbuddy').length, 0);
    allowedWrite = 'POST ' + disconnectPath;
    const [disconnectRequest] = await Promise.all([
      page.waitForRequest(request => new URL(request.url()).pathname === api + disconnectPath && request.method() === 'POST'),
      confirm.click(),
    ]);
    const recoveryKey = 'suma-write-recovery-v1:workbuddy-connections:' + JSON.stringify([session.organization.id, session.user.id, project]);
    const pending = await page.evaluate(key => JSON.parse(sessionStorage.getItem(key) || 'null'), recoveryKey);
    const submitted = disconnectRequest.postDataJSON();
    assert.deepEqual(pending, {
      path: disconnectPath, method: 'POST', body: submitted, idempotency_key: submitted.idempotency_key,
    });
    const responsePromise = page.waitForResponse(response => new URL(response.url()).pathname === api + disconnectPath && response.request().method() === 'POST');
    releaseDisconnect();
    assert.equal((await responsePromise).status(), 200);
    await history.filter({hasText: 'Disconnected records in this read · 2'}).waitFor();
    assert.equal(await history.getAttribute('aria-expanded'), 'false');
    assert.equal(await accountRow.isVisible(), false);
    assert.equal(await page.evaluate(key => sessionStorage.getItem(key), recoveryKey), null);
    await history.click();
    await accountRow.waitFor();
    assert.equal(await accountRow.locator('.workbuddy-state').innerText(), 'Disconnected locally');
    assert.equal(await accountRow.locator('button').filter({hasText: 'Disconnect locally'}).count(), 0);
    assert.ok(!(await workbuddy.innerText()).includes(rawReason));
    const historyLayout = await metrics(workbuddy);
    await screenshot('en-disconnected-history.png', workbuddy.locator('.workbuddy-history'));
    await verifyPage('en-US', 'dark', 'Tool management');
    const wbWrites = writeRequests().filter(request => request.group === 'workbuddy');
    assert.equal(wbWrites.length, 1);
    assert.equal(report.writes.length, 2);
    assert.deepEqual(report.writes[1].body, submitted);
    report.groups.en_workbuddy = {
      passed: true, layout: {scope: scopeLayout, history: historyLayout},
      reused_scopes: [readScope], authorized_scope_expansions: 0,
      before_confirmation_posts: 0, confirmed_posts: 1, pending_record_matches_request: true,
    };
    assert.deepEqual(report.page_errors, []);
    assert.deepEqual(report.console_errors, []);
    assert.deepEqual(report.unexpected, []);
    report.passed = true;
  } catch (error) {
    report.failure = error.message;
    throw error;
  } finally {
    releaseDisconnect();
    try {
      if (browser) {
        await browser.close();
        report.browser_closed = !browser.isConnected();
        assert.equal(report.browser_closed, true);
      }
    } catch (error) {
      report.passed = false;
      report.cleanup_failure = error.message;
      throw error;
    } finally {
      if (server.listening) await new Promise(resolve => server.close(resolve));
      if (port) {
        report.port_closed = await checkPortClosed(port);
        if (!report.port_closed) report.passed = false;
      }
      fs.writeFileSync(path.join(directory, 'result.json'), JSON.stringify(report, null, 2) + '\n', {flag: 'wx'});
    }
  }
  assert.equal(report.port_closed, true);
  console.log(JSON.stringify({
    directory: path.relative(root, directory), passed: report.passed,
    groups: Object.keys(report.groups), images: report.images, browser_closed: report.browser_closed,
  }));
}

main().catch(error => {
  console.error(error.stack || String(error));
  process.exitCode = 1;
});
