'use strict';
const fs = require('node:fs');
const path = require('node:path');
const {failure,within} = require('./build-plan.cjs');
const {fileHash,inventory,assertPlainPath,validateTarget} = require('./provenance.cjs');

function inspectorSourceInventory(plan) {
  const files = [];
  for (const name of ['packages/source-inspector/src','packages/source-inspector/scripts']) {
    const source = inventory(path.join(plan.root,name),{language:plan.language});
    files.push(...source.files.map(file=>({...file,path:`${name}/${file.path}`})));
  }
  for (const name of ['pnpm-lock.yaml','packages/source-inspector/package.json','packages/contracts/src/source-inspector-runtime.mjs']) {
    const filename = path.join(plan.root,name);assertPlainPath(filename,plan.language);
    files.push({path:name,size_bytes:fs.statSync(filename).size,sha256:fileHash(filename)});
  }
  files.sort((a,b)=>a.path.localeCompare(b.path,'en'));
  return require('node:crypto').createHash('sha256').update(JSON.stringify(files)).digest('hex');
}
function nodeLicense(plan,node) {
  // Only fixed distribution-relative locations are discovered. No search of
  // user documents, network download or mutation of the host environment.
  const directory = path.dirname(node);
  const candidates = plan.node_license_source ? [plan.node_license_source] : [
    path.join(directory,'LICENSE'),path.join(directory,'LICENSE.txt'),
    path.join(directory,'../LICENSE'),path.join(directory,'../share/doc/node/LICENSE'),
    path.join(directory,'../share/doc/nodejs/LICENSE'),
  ];
  for (const candidate of candidates) {
    if (!fs.existsSync(candidate)) continue;
    const resolved = fs.realpathSync(candidate);assertPlainPath(resolved,plan.language);
    const info = fs.statSync(resolved);
    if (info.isFile() && info.size > 0 && info.size <= 8<<20) return resolved;
  }
  throw failure('source_inspector_node','Node license',plan.language);
}
function validateSourceInspector(plan,directory,manifest) {
  if (!manifest || manifest.schema_version !== 1 || manifest.kind !== 'suma-source-inspector' || manifest.product_version !== plan.version || manifest.target !== plan.target || !/^[a-f0-9]{64}$/.test(manifest.source_sha256)) throw failure('source_inspector_bundle','manifest',plan.language);
  const expectedNode = plan.platform === 'win32' ? 'node.exe' : 'node';
  if (!manifest.node || !manifest.entry || manifest.node.file !== expectedNode || manifest.entry.file !== 'source-inspector.bundle.mjs' || !Array.isArray(manifest.notices) || manifest.notices.length !== 2 || manifest.notices[0]?.file !== 'NODE_LICENSE.txt' || manifest.notices[1]?.file !== 'THIRD_PARTY_NOTICES.txt') throw failure('source_inspector_bundle','files',plan.language);
  for (const file of [manifest.node,manifest.entry,...manifest.notices]) {
    const filename = path.join(directory,file.file);assertPlainPath(filename,plan.language);
    if (!fs.statSync(filename).isFile() || file.sha256 !== fileHash(filename)) throw failure('source_inspector_bundle',file.file,plan.language);
  }
  validateTarget(plan,path.join(directory,expectedNode));
  return manifest;
}
async function prepareSourceInspector(plan,env,checkedRun) {
  if (!plan.node_source) throw failure('source_inspector_node',plan.target,plan.language);
  const node = fs.realpathSync(plan.node_source);assertPlainPath(node,plan.language);
  if (node === plan.stage || within(plan.stage,node)) throw failure('unsafe_path',node,plan.language);
  validateTarget(plan,node);
  const license = nodeLicense(plan,node);
  if (license === plan.stage || within(plan.stage,license)) throw failure('unsafe_path',license,plan.language);
  const nodeSHA = fileHash(node), licenseSHA = fileHash(license);
  const native = plan.platform === plan.host.platform && plan.arch === plan.host.arch;
  let version = null;
  if (native) {
    const probe = await checkedRun(node,['--version'],{cwd:plan.root,env,stdio:'pipe'});
    version = probe.stdout?.trim();
    if (typeof version !== 'string' || !/^v[0-9]+\.[0-9]+\.[0-9]+$/.test(version) || Number(version.slice(1).split('.')[0]) < 22) throw failure('source_inspector_node','Node 22+',plan.language);
  }
  const sourceSHA = inspectorSourceInventory(plan);
  const root = path.join(plan.root,'packages/source-inspector');
  // One normal preparation builds the fixed bundle once. No install, script
  // from inspected manifests, project path or caller URL is executed here.
  await checkedRun(process.execPath,[path.join(root,'scripts/build.mjs')],{cwd:root,env,stdio:'inherit'});
  const directory = plan.resources.source_inspector;fs.mkdirSync(directory,{recursive:true});
  const nodeName = plan.platform === 'win32' ? 'node.exe' : 'node';
  for (const [source,name] of [[node,nodeName],[license,'NODE_LICENSE.txt'],[path.join(root,'dist/source-inspector.bundle.mjs'),'source-inspector.bundle.mjs'],[path.join(root,'dist/THIRD_PARTY_NOTICES.txt'),'THIRD_PARTY_NOTICES.txt']]) {
    assertPlainPath(source,plan.language);if (!fs.statSync(source).isFile() || fs.statSync(source).size === 0) throw failure('source_inspector_bundle',name,plan.language);fs.copyFileSync(source,path.join(directory,name));
  }
  if (plan.platform !== 'win32') fs.chmodSync(path.join(directory,nodeName),0o755);
  if (fileHash(path.join(directory,nodeName)) !== nodeSHA || fileHash(path.join(directory,'NODE_LICENSE.txt')) !== licenseSHA) throw failure('source_changed','Node distribution',plan.language);
  if (sourceSHA !== inspectorSourceInventory(plan)) throw failure('source_changed','source inspector',plan.language);
  const file = name=>({file:name,sha256:fileHash(path.join(directory,name))});
  const manifest = {schema_version:1,kind:'suma-source-inspector',product_version:plan.version,target:plan.target,source_sha256:sourceSHA,node:{...file(nodeName),version,version_verification:native ? 'native_version_and_format' : 'target_format_only'},entry:file('source-inspector.bundle.mjs'),notices:[file('NODE_LICENSE.txt'),file('THIRD_PARTY_NOTICES.txt')]};
  validateSourceInspector(plan,directory,manifest);
  fs.writeFileSync(path.join(directory,'manifest.json'),JSON.stringify(manifest,null,2)+'\n');
  return manifest;
}
module.exports = {prepareSourceInspector,validateSourceInspector,inspectorSourceInventory};
