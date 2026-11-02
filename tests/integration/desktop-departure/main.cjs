'use strict';
// Real Electron lifecycle and production departure controller; the dialog
// facade below represents explicit test choices, not native OS button clicks.
const {app, BrowserWindow, session} = require('electron');
const fs = require('node:fs');
const path = require('node:path');
const {createWindowDepartureController} = require('../../../apps/desktop/src/window-departure.cjs');

const origin = process.env.SUMA_FIXTURE_ORIGIN;
const directory = process.env.SUMA_FIXTURE_DIRECTORY;
if (!/^http:\/\/127\.0\.0\.1:\d+$/.test(origin || '') || !path.isAbsolute(directory || '')) {
  throw Error('Expected an owned local origin and isolated fixture directory');
}
app.commandLine.appendSwitch('force-device-scale-factor', '1');
app.disableHardwareAcceleration();
for (const [key, name] of Object.entries({userData: 'profile', sessionData: 'session', temp: 'tmp', crashDumps: 'crash'})) {
  const destination = path.join(directory, name);
  fs.mkdirSync(destination, {recursive: true});
  app.setPath(key, destination);
}

let window, controller, locale = 'zh-CN', synchronousChoice = 0;
const decisions = [], events = [];
const recordEvent = (kind, url) => {
  events.push({kind, url});
  if (events.length > 128) events.shift();
};
function recordDialog(owner, options, kind) {
  const record = {
    id: decisions.length + 1, kind, locale, source: owner.webContents.getURL(),
    same_owner: owner === window, settled: kind === 'sync',
    options: {type: options.type, title: options.title, message: options.message,
      detail: options.detail, buttons: options.buttons, defaultId: options.defaultId,
      cancelId: options.cancelId, noLink: options.noLink},
    signal: options.signal,
  };
  decisions.push(record);
  return record;
}
const dialog = {
  showMessageBox(owner, options) {
    const record = recordDialog(owner, options, 'async');
    return new Promise(resolve => { record.resolve = resolve; });
  },
  showMessageBoxSync(owner, options) {
    const record = recordDialog(owner, options, 'sync');
    record.response = synchronousChoice;
    return synchronousChoice;
  },
};
function installController(nextLocale) {
  if (!['zh-CN', 'en-US'].includes(nextLocale) || !window || window.isDestroyed()) throw Error('Invalid fixture locale/window');
  controller?.dispose();
  locale = nextLocale;
  controller = createWindowDepartureController({parent: window, dialog, origin, locale, getWindow: () => window});
}

globalThis.__departure218 = {
  snapshot() {
    return {alive: !!window && !window.isDestroyed(), locale, versions: process.versions,
      events: events.slice(), decisions: decisions.map(({resolve, signal, ...record}) => ({...record, aborted: signal?.aborted ?? false}))};
  },
  close() { window.close(); },
  quit() { return controller.requestQuit(); },
  reload() { window.webContents.reload(); },
  locale(value) { installController(value); },
  sync(value) { if (![0, 1].includes(value)) throw Error('Expected Stay or Leave'); synchronousChoice = value; },
  async answer(id, response) {
    const record = decisions.find(item => item.id === id);
    if (!record || record.kind !== 'async' || record.settled || ![0, 1].includes(response)) throw Error('No exact pending facade choice');
    record.settled = true;
    record.response = response;
    const resolve = record.resolve;
    delete record.resolve;
    resolve({response});
    // Let the production decision/finally microtasks settle before the next
    // explicit fixture action. Final Leave may instead close this RPC owner.
    await new Promise(done => setImmediate(done));
  },
};

app.whenReady().then(async () => {
  const isolated = session.fromPartition('suma-fixture-desktop-departure218');
  isolated.setPermissionRequestHandler((_window, _permission, done) => done(false));
  isolated.webRequest.onBeforeRequest((request, done) => {
    const url = new URL(request.url);
    done({cancel: !['about:', 'data:', 'blob:'].includes(url.protocol) && url.origin !== origin});
  });
  window = new BrowserWindow({show: false, width: 1100, height: 760,
    webPreferences: {offscreen: true, sandbox: true, contextIsolation: true, nodeIntegration: false,
      backgroundThrottling: false, session: isolated}});
  window.webContents.setWindowOpenHandler(() => ({action: 'deny'}));
  installController('zh-CN');
  window.on('close', () => recordEvent('close', window?.webContents.getURL()));
  window.webContents.on('will-prevent-unload', () => recordEvent('will-prevent-unload', window?.webContents.getURL()));
  window.webContents.on('will-navigate', (_event, url) => recordEvent('will-navigate', url));
  window.webContents.on('did-start-navigation', (_event, url, same, main) => { if (main) recordEvent(same ? 'same-document-start' : 'navigation-start', url); });
  window.webContents.on('did-navigate-in-page', (_event, url) => recordEvent('same-document', url));
  window.webContents.on('did-navigate', (_event, url) => recordEvent('did-navigate', url));
  window.on('closed', () => { controller?.dispose(); window = undefined; });
  await window.loadURL(origin + '/work');
});
app.on('window-all-closed', () => app.quit());
