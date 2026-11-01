'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const {spawnSync} = require('node:child_process');
const root = path.resolve(__dirname, '../../..');
const contractPath = path.resolve(__dirname, '../scripts/build-plan.cjs');

test('target plan routes each installer and portable format to isolated matching Go resources', () => {
  assert.ok(fs.existsSync(contractPath), 'target-aware packaging contract must exist');
  const {createPlan} = require(contractPath);
  for (const [platform, arch, format, goos, goarch, targets] of [
    ['win32', 'arm64', 'all', 'windows', 'arm64', ['nsis', 'portable']],
    ['darwin', 'x64', 'all', 'darwin', 'amd64', ['dmg', 'zip']],
    ['linux', 'arm64', 'all', 'linux', 'arm64', ['AppImage', 'deb', 'tar.gz']],
    ['linux', 'x64', 'dir', 'linux', 'amd64', ['dir']],
  ]) {
    const plan = createPlan(['--platform', platform, '--arch', arch, '--format', format], {root, hostPlatform: 'win32', hostArch: 'x64'});
    assert.equal(plan.goos, goos);
    assert.equal(plan.goarch, goarch);
    assert.deepEqual(plan.targets, targets);
    assert.equal(plan.output, path.join(root, 'dist/desktop', `${platform}-${arch}`));
    assert.equal(plan.resources.control, path.join(root, '.cache/desktop', `${platform}-${arch}`, 'control', platform === 'win32' ? 'suma-control.exe' : 'suma-control'));
    assert.equal(plan.build_allowed, platform === 'win32' || format === 'dir');
  }
});

test('plan validates unsupported, duplicate and ambiguous options before any build', () => {
  assert.ok(fs.existsSync(contractPath), 'target-aware packaging contract must exist');
  const {createPlan} = require(contractPath);
  for (const args of [['--platform','windows'], ['--arch','ia32'], ['--format','zip'], ['--publish'], ['--arch','x64','--arch','arm64'], ['--web-dir','relative'], ['--control-manifest',path.join(root,'receipt.json')]]) {
    assert.throws(() => createPlan(args, {root}), error => Boolean(error.code));
  }
});

test('CLI plan for a blocked cross-OS installer is read-only and returns the reason', () => {
  const scratch = path.join(root, '.cache/e2e/desktop-108/plan-does-not-create');
  const result = spawnSync(process.execPath, [path.resolve(__dirname, '../scripts/package.cjs'), '--platform', process.platform === 'linux' ? 'darwin' : 'linux', '--arch', 'arm64', '--plan', '--web-dir', scratch], {encoding:'utf8'});
  assert.equal(result.status, 0, result.stderr);
  const plan = JSON.parse(result.stdout);
  assert.equal(plan.build_allowed, false);
  assert.ok(plan.reasons.includes('native_host_required'));
  assert.equal(fs.existsSync(scratch), false);
});

function fixture(t) {
  const directory = fs.mkdtempSync(path.join(root,'.cache/e2e/desktop-108/fixture-'));
  t.after(()=>{
    assert.ok(path.relative(path.join(root,'.cache/e2e/desktop-108'),directory).startsWith('fixture-'));
    fs.rmSync(directory,{recursive:true,force:true});
  });
  for (const name of ['internal/buildinfo','cmd/suma-control','apps/desktop/src','input-web']) fs.mkdirSync(path.join(directory,name),{recursive:true});
  fs.writeFileSync(path.join(directory,'package.json'),'{"version":"1.0.0"}');
  fs.writeFileSync(path.join(directory,'apps/desktop/package.json'),'{"version":"1.0.0"}');
  fs.writeFileSync(path.join(directory,'internal/buildinfo/version.go'),'package buildinfo\nconst Version = "1.0.0"\n');
  fs.writeFileSync(path.join(directory,'cmd/suma-control/main.go'),'package main\nfunc main() {}\n');
  fs.writeFileSync(path.join(directory,'go.mod'),'module suma\n');
  fs.writeFileSync(path.join(directory,'go.sum'),'');
  fs.writeFileSync(path.join(directory,'input-web/index.html'),'<!doctype html><title>Suma</title>');
  return directory;
}
function binaryFixture(filename, platform = 'win32', arch = 'x64') {
  const data = Buffer.alloc(160);
  if (platform === 'win32') {
    data.write('MZ');data.writeUInt32LE(64,60);data.write('PE\0\0',64);data.writeUInt16LE(arch === 'x64' ? 0x8664 : 0xaa64,68);data.writeUInt16LE(0x20b,88);
  } else if (platform === 'linux') {
    data.set([0x7f,0x45,0x4c,0x46,2,1]);data.writeUInt16LE(arch === 'x64' ? 62 : 183,18);
  } else {
    data.writeUInt32LE(0xfeedfacf,0);data.writeUInt32LE(arch === 'x64' ? 0x01000007 : 0x0100000c,4);
  }
  fs.writeFileSync(filename,data);
}
// Controlled packaging protocol inputs, not a real Node distribution or parser
// execution. The injected command callback below is the sole build boundary.
function sourceInspectorFixture(directory) {
  for (const name of ['packages/source-inspector/src','packages/source-inspector/scripts','packages/contracts/src','input-node']) fs.mkdirSync(path.join(directory,name),{recursive:true});
  fs.writeFileSync(path.join(directory,'packages/source-inspector/src/cli.mjs'),'// Controlled parser source fixture\n');
  fs.writeFileSync(path.join(directory,'packages/source-inspector/scripts/build.mjs'),'// Controlled build boundary fixture\n');
  fs.writeFileSync(path.join(directory,'packages/source-inspector/package.json'),'{"name":"@suma/source-inspector","version":"1.0.0"}');
  fs.writeFileSync(path.join(directory,'packages/contracts/src/source-inspector-runtime.mjs'),'export const SCHEMA_VERSION=1;\n');
  fs.writeFileSync(path.join(directory,'pnpm-lock.yaml'),'lockfileVersion: 9.0\n');
  const node = path.join(directory,'input-node/node.exe');binaryFixture(node);
  const license = path.join(directory,'input-node/LICENSE');fs.writeFileSync(license,'Node distribution license protocol fixture, not a distributable license.\n');
  const calls = {builds:0,versions:0};
  return {args:['--node-binary',node,'--node-license',license],calls,run(command,argv){
    if (argv.length===2&&argv[0]==='--language=en'&&argv[1]==='--version') return {status:0,stdout:'suma-control 1.0.0\n'};
    if (command===node&&argv.length===1&&argv[0]==='--version'){calls.versions++;return {status:0,stdout:'v24.0.0\n'};}
    assert.equal(command,process.execPath);assert.deepEqual(argv,[path.join(directory,'packages/source-inspector/scripts/build.mjs')]);
    calls.builds++;const dist=path.join(directory,'packages/source-inspector/dist');fs.mkdirSync(dist,{recursive:true});
    fs.writeFileSync(path.join(dist,'source-inspector.bundle.mjs'),'// Controlled bundled parser bytes\n');
    fs.writeFileSync(path.join(dist,'THIRD_PARTY_NOTICES.txt'),'Controlled dependency license fixture\n');return {status:0};
  }};
}
function inspectorResourcesFixture(plan,directory) {
  const {fileHash} = require('../scripts/provenance.cjs');fs.mkdirSync(directory,{recursive:true});binaryFixture(path.join(directory,'node.exe'));
  for (const name of ['source-inspector.bundle.mjs','NODE_LICENSE.txt','THIRD_PARTY_NOTICES.txt']) fs.writeFileSync(path.join(directory,name),'Controlled packaged resource fixture: '+name);
  const file=name=>({file:name,sha256:fileHash(path.join(directory,name))});
  return {schema_version:1,kind:'suma-source-inspector',product_version:plan.version,target:plan.target,source_sha256:'a'.repeat(64),node:{...file('node.exe'),version:'v24.0.0',version_verification:'native_version_and_format'},entry:file('source-inspector.bundle.mjs'),notices:[file('NODE_LICENSE.txt'),file('THIRD_PARTY_NOTICES.txt')]};
}
fs.mkdirSync(path.join(root,'.cache/e2e/desktop-108'),{recursive:true});

test('prebuilt control rejects a wrong CPU and rejects a binary changed after its source attestation', t => {
  const directory = fixture(t);
  const {createPlan} = require(contractPath);
  const {sourceInventory,createControlManifest,validateControlManifest} = require('../scripts/provenance.cjs');
  const plan = createPlan(['--platform','win32','--arch','x64'],{root:directory,hostPlatform:'win32'});
  const binary = path.join(directory,'control.exe');
  binaryFixture(binary,'win32','arm64');
  assert.throws(()=>createControlManifest(plan,binary),{code:'binary_target'});
  binaryFixture(binary);
  const source = sourceInventory(directory);
  const manifest = createControlManifest(plan,binary,source);
  assert.equal(validateControlManifest(plan,binary,manifest,source).product_version,'1.0.0');
  fs.appendFileSync(binary,'tampered');
  assert.throws(()=>validateControlManifest(plan,binary,manifest,source),{code:'binary_manifest'});
});

test('prebuilt control attestation becomes stale when current Go source changes', t => {
  const directory = fixture(t);
  const {createPlan} = require(contractPath);
  const {createControlManifest,validateControlManifest} = require('../scripts/provenance.cjs');
  const plan = createPlan(['--platform','win32','--arch','x64'],{root:directory,hostPlatform:'win32'});
  const binary = path.join(directory,'control.exe');binaryFixture(binary);
  const manifest = createControlManifest(plan,binary);
  fs.appendFileSync(path.join(directory,'cmd/suma-control/main.go'),'// changed\n');
  assert.throws(()=>validateControlManifest(plan,binary,manifest),{code:'binary_manifest'});
});

test('preparation reuses validated control and copies web without writing shared resources or mutating parent environment', async t => {
  const directory = fixture(t);
  const {createPlan,childEnvironment} = require(contractPath);
  const {createControlManifest} = require('../scripts/provenance.cjs');
  const {prepare} = require('../scripts/prepare.cjs');
  assert.equal(typeof prepare,'function','preparation must be callable without running a build at require time');
  const binary = path.join(directory,'control.exe');binaryFixture(binary);
  const inspector = sourceInspectorFixture(directory);
  const args = ['--platform','win32','--arch','x64','--web-dir',path.join(directory,'input-web'),'--control-binary',binary,...inspector.args];
  const plan = createPlan(args,{root:directory,hostPlatform:'win32',hostArch:'x64'});
  fs.writeFileSync(`${binary}.manifest.json`,JSON.stringify(createControlManifest(plan,binary)));
  const parent = {TEMP:'original',GOOS:'original',CSC_LINK:'secret-not-for-unsigned-build'};
  const env = childEnvironment(plan,parent);
  assert.equal(parent.TEMP,'original');assert.equal(parent.GOOS,'original');assert.equal(parent.CSC_LINK,'secret-not-for-unsigned-build');
  assert.equal(env.GOOS,'windows');assert.equal(env.GOARCH,'amd64');assert.equal(env.CGO_ENABLED,'0');assert.equal(env.CSC_LINK,undefined);
  const receipt = await prepare(plan,{run:inspector.run});
  assert.equal(receipt.control.binary_sha256,createControlManifest(plan,binary).binary_sha256);
  assert.equal(fs.readFileSync(plan.resources.web + '/index.html','utf8'),'<!doctype html><title>Suma</title>');
  assert.equal(fs.existsSync(path.join(directory,'.cache/desktop/control')),false);
  assert.equal(fs.existsSync(path.join(directory,'apps/web/dist')),false);
  assert.equal(inspector.calls.builds,1);assert.equal(inspector.calls.versions,1);
  assert.equal(receipt.source_inspector.node.version,'v24.0.0');
  assert.equal(require('../scripts/source-inspector.cjs').validateSourceInspector(plan,plan.resources.source_inspector,receipt.source_inspector),receipt.source_inspector);
});

test('web input rejects accidental credentials and databases before staged resources are copied', async t => {
  const directory = fixture(t);
  const {createPlan} = require(contractPath);
  const {prepare} = require('../scripts/prepare.cjs');
  const {createControlManifest} = require('../scripts/provenance.cjs');
  const binary = path.join(directory,'control.exe');binaryFixture(binary);
  const plan = createPlan(['--platform','win32','--arch','x64','--web-dir',path.join(directory,'input-web'),'--control-binary',binary],{root:directory,hostPlatform:'win32'});
  fs.writeFileSync(`${binary}.manifest.json`,JSON.stringify(createControlManifest(plan,binary)));
  for (const name of ['.env.production','suma.db-wal','desktop-workspace.key','certificate.p12']) {
    const file = path.join(directory,'input-web',name);fs.writeFileSync(file,'must not bundle');
    await assert.rejects(()=>prepare(plan,{run:()=>{throw Error('private web input must be rejected before any executable runs');}}),{code:'private_web_data'});
    assert.equal(fs.existsSync(plan.resources.web),false);
    fs.unlinkSync(file);
  }
});

test('final delivery hashes only selected current SDK artifacts and never ships SDK configuration', t => {
  const directory = fixture(t);
  const {createPlan} = require(contractPath);
  const {finalize} = require('../scripts/package.cjs');
  const {fileHash} = require('../scripts/provenance.cjs');
  const plan = createPlan(['--platform','win32','--arch','x64'],{root:directory,hostPlatform:'win32'});
  const raw = path.join(plan.stage,'sdk-fixture');fs.mkdirSync(raw,{recursive:true});
  const filename = 'Suma-1.0.0-win32-x64-installer.exe';
  fs.writeFileSync(path.join(raw,filename),'actual bytes fixture');
  fs.writeFileSync(path.join(raw,'builder-effective-config.yaml'),'private absolute config path');
  const manifest = finalize(plan,{control:{binary_sha256:'fixture'},web:{sha256:'web-fixture',files:[]}},raw,{artifacts:[path.join(raw,filename)],directories:[]});
  assert.deepEqual(fs.readdirSync(plan.output).sort(),[filename,'SHA256SUMS','manifest.json'].sort());
  assert.equal(manifest.files[0].sha256,fileHash(path.join(plan.output,filename)));
  assert.equal(manifest.verification.gui,false);assert.equal(manifest.signing.status,'unsigned');
  assert.equal(JSON.stringify(manifest).includes(directory),false);
  assert.ok(fs.readFileSync(path.join(plan.output,'SHA256SUMS'),'utf8').includes(`${fileHash(path.join(plan.output,'manifest.json'))}  manifest.json`));
  assert.throws(()=>finalize(plan,{control:{},web:{files:[]}},raw,{artifacts:[path.join(raw,filename)]}),{code:'output_nonempty'});
});

test('directory delivery hashes nested resources and rejects escaping SDK artifact paths', t => {
  const directory = fixture(t);
  const {createPlan} = require(contractPath);
  const {finalize} = require('../scripts/package.cjs');
  const plan = createPlan(['--platform','linux','--arch','x64','--format','dir'],{root:directory,hostPlatform:'linux'});
  const raw = path.join(plan.stage,'sdk-fixture');const unpacked = path.join(raw,'linux-unpacked');
  fs.mkdirSync(path.join(unpacked,'resources/web'),{recursive:true});
  fs.writeFileSync(path.join(unpacked,'resources/web/index.html'),'actual nested resource');
  const prepared = {control:{},web:{sha256:'fixture',files:[]}};
  assert.throws(()=>finalize(plan,prepared,raw,{artifacts:[path.join(directory,'package.json')]}),{code:'unsafe_path'});
  const manifest = finalize(plan,prepared,raw,{artifacts:[],directories:[unpacked]});
  assert.deepEqual(manifest.artifacts,['Suma-1.0.0-linux-x64-dir']);
  assert.equal(manifest.files[0].path,'Suma-1.0.0-linux-x64-dir/resources/web/index.html');
});

test('an SDK failure leaves no completed public delivery', async t => {
  const directory = fixture(t);
  const {packageDesktop} = require('../scripts/package.cjs');
  const {createPlan} = require(contractPath);
  const {createControlManifest} = require('../scripts/provenance.cjs');
  const binary = path.join(directory,'control.exe');binaryFixture(binary);
  const inspector = sourceInspectorFixture(directory);
  const args = ['--platform','win32','--arch','x64','--web-dir',path.join(directory,'input-web'),'--control-binary',binary,...inspector.args];
  const plan = createPlan(args,{root:directory,hostPlatform:'win32'});
  fs.writeFileSync(`${binary}.manifest.json`,JSON.stringify(createControlManifest(plan,binary)));
  await assert.rejects(()=>packageDesktop(args,{root:directory,hostPlatform:'win32',run:inspector.run,sdkBuild:async()=>{throw Error('controlled SDK failure');}}),/controlled SDK failure/);
  assert.equal(fs.existsSync(plan.output),false);
  assert.equal(fs.existsSync(path.join(plan.stage,'build.lock')),false);
});

test('one missing requested format prevents an all-format delivery receipt', t => {
  const directory = fixture(t);
  const {createPlan} = require(contractPath);
  const {finalize} = require('../scripts/package.cjs');
  const plan = createPlan(['--platform','win32','--arch','x64','--format','all'],{root:directory,hostPlatform:'win32'});
  const raw = path.join(plan.stage,'sdk-fixture');fs.mkdirSync(raw,{recursive:true});
  const installer = path.join(raw,'Suma-1.0.0-win32-x64-installer.exe');fs.writeFileSync(installer,'only one requested format');
  assert.throws(()=>finalize(plan,{control:{},web:{files:[]}},raw,{artifacts:[installer],directories:[]}),{code:'build_empty'});
  assert.equal(fs.existsSync(plan.output),false);
});

test('packed app resource validation rejects wrong runtime CPU or stale embedded web bytes', t => {
  const directory = fixture(t);
  const {createPlan} = require(contractPath);
  const {validatePackagedApplication} = require('../scripts/package.cjs');
  assert.equal(typeof validatePackagedApplication,'function');
  const {createControlManifest,inventory} = require('../scripts/provenance.cjs');
  const plan = createPlan(['--platform','win32','--arch','x64'],{root:directory,hostPlatform:'win32'});
  const unpacked = path.join(plan.stage,'sdk-fixture/win-unpacked');
  fs.mkdirSync(path.join(unpacked,'resources/control'),{recursive:true});
  fs.mkdirSync(path.join(unpacked,'resources/web'),{recursive:true});
  const runtime = path.join(unpacked,'Suma.exe');binaryFixture(runtime,'win32','arm64');
  const control = path.join(unpacked,'resources/control/suma-control.exe');binaryFixture(control);
  fs.writeFileSync(path.join(unpacked,'resources/web/index.html'),'current');
  const prepared = {control:createControlManifest(plan,control),web:inventory(path.join(unpacked,'resources/web')),source_inspector:inspectorResourcesFixture(plan,path.join(unpacked,'resources/source-inspector'))};
  assert.throws(()=>validatePackagedApplication(plan,prepared,unpacked),{code:'binary_target'});
  binaryFixture(runtime);
  validatePackagedApplication(plan,prepared,unpacked);
  fs.writeFileSync(path.join(unpacked,'resources/web/index.html'),'stale-or-changed');
  assert.throws(()=>validatePackagedApplication(plan,prepared,unpacked),{code:'source_changed'});
});

test('SDK policy preserves hardened runtime, isolated resources, bilingual per-user installer and signer requirements', () => {
  const {createPlan} = require(contractPath);
  const {builderConfig} = require('../scripts/builder-sdk.cjs');
  const plan = createPlan(['--platform','win32','--arch','arm64','--signing','required','--format','all'],{root,hostPlatform:'win32'});
  const config = builderConfig(plan,path.join(plan.stage,'sdk-fixture'));
  assert.equal(config.forceCodeSigning,true);assert.equal(config.win.signExecutable,true);
  assert.equal(config.win.signAndEditExecutable,undefined);
  assert.equal(config.mac.hardenedRuntime,true);assert.equal(config.publish,null);
  assert.equal(config.extraResources[0].from,path.join(root,'.cache/desktop/win32-arm64/control/suma-control.exe'));
  assert.equal(config.nsis.perMachine,false);assert.equal(config.nsis.allowElevation,false);assert.equal(config.nsis.deleteAppDataOnUninstall,false);
  assert.deepEqual(config.nsis.installerLanguages,['zh_CN','en_US']);assert.equal(config.nsis.displayLanguageSelector,true);
  assert.notEqual(config.nsis.artifactName,config.portable.artifactName);
});

test('local Electron distributions require matching actual CPU and the installed runtime version', t => {
  const directory = fixture(t);
  const {createPlan} = require(contractPath);
  const {validateElectron} = require('../scripts/builder-sdk.cjs');
  const distribution = path.join(directory,'electron-distribution');fs.mkdirSync(distribution);
  binaryFixture(path.join(distribution,'electron.exe'),'win32','arm64');
  const version = JSON.parse(fs.readFileSync(require.resolve('electron/package.json',{paths:[path.join(root,'apps/desktop')]}),'utf8')).version;
  fs.mkdirSync(path.join(directory,'apps/desktop/node_modules/electron'),{recursive:true});
  fs.writeFileSync(path.join(directory,'apps/desktop/node_modules/electron/package.json'),JSON.stringify({version}));
  fs.writeFileSync(path.join(distribution,'version'),version);
  const plan = createPlan(['--platform','win32','--arch','x64','--electron-dist',distribution],{root:directory,hostPlatform:'win32'});
  assert.throws(()=>validateElectron(plan),{code:'electron_target'});
  binaryFixture(path.join(distribution,'electron.exe'),'win32','x64');
  fs.writeFileSync(path.join(distribution,'version'),'0.0.0');
  assert.throws(()=>validateElectron(plan),{code:'electron_target'});
  fs.writeFileSync(path.join(distribution,'version'),version);
  validateElectron(plan);
});

test('binary inspection identifies all six supported executable targets without executing them', t => {
  const directory = fixture(t);
  const {inspectBinary} = require('../scripts/provenance.cjs');
  for (const platform of ['win32','darwin','linux']) for (const arch of ['x64','arm64']) {
    const binary = path.join(directory,`${platform}-${arch}`);binaryFixture(binary,platform,arch);
    const info = inspectBinary(binary);
    assert.equal(info.platform,platform);assert.equal(info.arch,arch);
  }
  const truncated = path.join(directory,'truncated');fs.writeFileSync(truncated,Buffer.from('MZ'));
  assert.throws(()=>inspectBinary(truncated),{code:'binary_target'});
});

test('prepare help is read-only and cannot start the Go or web toolchain', async t => {
  const directory = fixture(t);
  const {prepareDesktop} = require('../scripts/prepare.cjs');
  assert.equal(typeof prepareDesktop,'function');
  const result = await prepareDesktop(['--help'],{root:directory,run:()=>{throw Error('help must not run any subprocess');}});
  assert.match(result.usage,/prepare\.cjs/);
  assert.equal(fs.existsSync(path.join(directory,'.cache')),false);
});

test('Linux deb requires a configured real maintainer while portable and directory packages remain available', t => {
  const directory = fixture(t);
  const {createPlan} = require(contractPath);
  for (const format of ['installer','all']) {
    const missing = createPlan(['--platform','linux','--arch','x64','--format',format],{root:directory,hostPlatform:'linux',env:{}});
    assert.equal(missing.build_allowed,false);
    assert.ok(missing.reasons.includes('maintainer_required'));
  }
  for (const format of ['portable','dir']) assert.equal(createPlan(['--platform','linux','--arch','x64','--format',format],{root:directory,hostPlatform:'linux',env:{}}).build_allowed,true);
  const configured = createPlan(['--platform','linux','--arch','x64','--maintainer','Release team <release@example.org>'],{root:directory,hostPlatform:'linux',env:{}});
  assert.equal(configured.build_allowed,true);assert.equal(configured.maintainer,'Release team <release@example.org>');
  const config = require('../scripts/builder-sdk.cjs').builderConfig({...configured,root},path.join(configured.stage,'sdk-fixture'));
  assert.equal(config.linux.maintainer,'Release team <release@example.org>');assert.equal(config.linux.executableName,'suma');assert.equal(config.linux.packageName,undefined);assert.equal(config.deb.packageName,'suma');
  const envConfigured = createPlan(['--platform','linux','--arch','arm64'],{root:directory,hostPlatform:'linux',env:{SUMA_DESKTOP_MAINTAINER:'Release team <release@example.org>'}});
  assert.equal(envConfigured.build_allowed,true);
  for (const value of ['Suma contributors','release@example.org','Release <bad@-domain.example>','Release <bad@domain..example>']) assert.throws(()=>createPlan(['--platform','linux','--arch','x64','--maintainer',value],{root:directory,hostPlatform:'linux',env:{}}),{code:'maintainer_invalid'});
});

test('canonical DEB maintainer is overridden by environment and then explicit CLI input', t => {
  const directory = fixture(t);
  fs.writeFileSync(path.join(directory,'package.json'),JSON.stringify({version:'1.0.0',config:{desktopMaintainer:'Default team <default@example.org>'}}));
  const {createPlan} = require(contractPath);
  const args = ['--platform','linux','--arch','x64'];
  const context = {root:directory,hostPlatform:'linux',env:{}};
  const plan = createPlan(args,context);
  assert.equal(plan.build_allowed,true);assert.equal(plan.maintainer,'Default team <default@example.org>');
  context.env.SUMA_DESKTOP_MAINTAINER='Environment team <environment@example.org>';
  assert.equal(createPlan(args,context).maintainer,'Environment team <environment@example.org>');
  assert.equal(createPlan([...args,'--maintainer','CLI team <cli@example.org>'],context).maintainer,'CLI team <cli@example.org>');
});

test('linked scratch descendants are rejected before cached resources or child processes are written', async t => {
  const directory = fixture(t);
  const {createPlan} = require(contractPath);
  const {prepare} = require('../scripts/prepare.cjs');
  const plan = createPlan(['--platform','win32','--arch','x64'],{root:directory,hostPlatform:'win32'});
  const outside = path.join(directory,'preserved-outside-scratch');fs.mkdirSync(outside);fs.writeFileSync(path.join(outside,'keep.txt'),'original');
  fs.mkdirSync(plan.stage,{recursive:true});
  for (const name of ['cache','tmp','previous']) {
    const link = path.join(plan.stage,name);
    fs.symlinkSync(outside,link,process.platform === 'win32' ? 'junction' : 'dir');
    await assert.rejects(()=>prepare(plan,{run:()=>{throw Error('linked scratch must fail before running a toolchain');}}),{code:'unsafe_path'});
    assert.deepEqual(fs.readdirSync(outside),['keep.txt']);
    assert.equal(fs.readFileSync(path.join(outside,'keep.txt'),'utf8'),'original');
    fs.unlinkSync(link);
  }
});

test('unverified signing modes fail preflight before preparation, SDK or certificate use', async t => {
  const directory = fixture(t);
  const {createPlan} = require(contractPath);
  const {packageDesktop} = require('../scripts/package.cjs');
  const {prepare} = require('../scripts/prepare.cjs');
  for (const signing of ['auto','required']) {
    const args = ['--platform','win32','--arch','x64','--signing',signing];
    const context = {root:directory,hostPlatform:'win32',run:()=>{throw Error('signing preflight must not run subprocesses');},sdkBuild:()=>{throw Error('signing preflight must not invoke SDK');}};
    const plan = createPlan(args,context);
    assert.equal(plan.build_allowed,false);assert.ok(plan.reasons.includes('signing_pipeline_unverified'));
    await assert.rejects(()=>packageDesktop(args,context),{code:'signing_pipeline_unverified'});
    await assert.rejects(()=>prepare(plan,context),{code:'signing_pipeline_unverified'});
    assert.equal(fs.existsSync(path.join(directory,'.cache')),false);
  }
});
