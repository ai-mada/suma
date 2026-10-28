'use strict';
const fs = require('node:fs');
const path = require('node:path');
const {createHash} = require('node:crypto');
const {failure, within} = require('./build-plan.cjs');
const sha256 = bytes => createHash('sha256').update(bytes).digest('hex');
const fileHash = filename => sha256(fs.readFileSync(filename));
const publicPath = filename => filename.split(path.sep).join('/');

function assertPlainPath(filename, language, allowMissing = false) {
  let current = path.resolve(filename);
  for (;;) {
    try { if (fs.lstatSync(current).isSymbolicLink()) throw failure('unsafe_path',filename,language); }
    catch (error) { if (!allowMissing || error.code !== 'ENOENT') throw error; }
    const parent = path.dirname(current);
    if (parent === current) return;
    current = parent;
  }
}
function inventory(directory, options = {}) {
  const files = [];
  assertPlainPath(directory,options.language);
  function visit(current) {
    for (const entry of fs.readdirSync(current,{withFileTypes:true})) {
      const filename = path.join(current,entry.name);
      if (entry.isSymbolicLink()) {
        if (!options.allowLinks) throw failure('unsafe_path',filename,options.language);
        const target = fs.readlinkSync(filename);
        const resolved = path.resolve(path.dirname(filename),target);
        if (path.isAbsolute(target) || !within(directory,resolved) || !within(directory,fs.realpathSync(filename))) throw failure('unsafe_path',filename,options.language);
        files.push({path:publicPath(path.relative(directory,filename)),type:'symlink',target:publicPath(target),sha256:sha256(target)});
      } else if (entry.isDirectory()) visit(filename);
      else if (entry.isFile()) {
        const stat = fs.statSync(filename);
        if (!options.filter || options.filter(filename)) files.push({path:publicPath(path.relative(directory,filename)),size_bytes:stat.size,sha256:fileHash(filename)});
      } else throw failure('unsafe_path',filename,options.language);
    }
  }
  visit(directory);
  files.sort((a,b)=>a.path < b.path ? -1 : a.path > b.path ? 1 : 0);
  return {sha256:sha256(JSON.stringify(files)),files};
}
function sourceInventory(root) {
  const files = [];
  for (const name of ['go.mod','go.sum']) {
    const filename = path.join(root,name);
    assertPlainPath(filename);
    files.push({path:name,size_bytes:fs.statSync(filename).size,sha256:fileHash(filename)});
  }
  for (const name of ['internal','cmd/suma-control']) {
    const directory = path.join(root,name);
    const entries = inventory(directory,{filter:filename=>filename.endsWith('.go') && !filename.endsWith('_test.go')});
    for (const entry of entries.files) {
      if (/^\s*\/\/go:embed\s/m.test(fs.readFileSync(path.join(directory,entry.path),'utf8'))) throw failure('embed_inventory',entry.path);
      files.push({...entry,path:`${name}/${entry.path}`});
    }
  }
  files.sort((a,b)=>a.path < b.path ? -1 : a.path > b.path ? 1 : 0);
  return {algorithm:'sha256(JSON.stringify(sorted [{path,size_bytes,sha256}])); UTF-8; repository-relative slash paths',sha256:sha256(JSON.stringify(files)),files};
}
function inspectBinary(filename, language) {
  assertPlainPath(filename,language);
  if (!fs.statSync(filename).isFile()) throw failure('binary_target',filename,language);
  const data = fs.readFileSync(filename);
  try {
    if (data[0] === 0x4d && data[1] === 0x5a) {
      const offset = data.readUInt32LE(0x3c);
      if (data.toString('binary',offset,offset+4) !== 'PE\0\0') throw Error();
      const machine = data.readUInt16LE(offset+4);
      const arch = machine === 0x8664 ? 'x64' : machine === 0xaa64 ? 'arm64' : null;
      if (!arch || data.readUInt16LE(offset+24) !== 0x20b) throw Error();
      return {platform:'win32',arch,format:'PE32+'};
    }
    if (data.subarray(0,4).equals(Buffer.from([0x7f,0x45,0x4c,0x46]))) {
      if (data[4] !== 2 || data[5] !== 1) throw Error();
      const machine = data.readUInt16LE(18);
      const arch = machine === 62 ? 'x64' : machine === 183 ? 'arm64' : null;
      if (!arch) throw Error();
      return {platform:'linux',arch,format:'ELF64'};
    }
    if (data.readUInt32LE(0) === 0xfeedfacf) {
      const machine = data.readUInt32LE(4);
      const arch = machine === 0x01000007 ? 'x64' : machine === 0x0100000c ? 'arm64' : null;
      if (!arch) throw Error();
      return {platform:'darwin',arch,format:'Mach-O64'};
    }
  } catch { /* A truncated or unsupported header is rejected below. */ }
  throw failure('binary_target',filename,language);
}
function validateTarget(plan, filename) {
  const info = inspectBinary(filename,plan.language);
  if (info.platform !== plan.platform || info.arch !== plan.arch) throw failure('binary_target',filename,plan.language);
  return info;
}
function createControlManifest(plan, filename, source = sourceInventory(plan.root), method = 'compiled_current_source') {
  const info = validateTarget(plan,filename);
  const versionFile = fs.readFileSync(path.join(plan.root,'internal/buildinfo/version.go'),'utf8');
  if (!versionFile.includes(`const Version = "${plan.version}"`)) throw failure('binary_version','product source version drift',plan.language);
  if (!['compiled_current_source','native_version_and_source_build_receipt'].includes(method)) throw failure('binary_manifest','verification method',plan.language);
  return {schema_version:1,kind:'suma-desktop-control',product_version:plan.version,target:plan.target,...info,binary_sha256:fileHash(filename),source_sha256:source.sha256,source_algorithm:source.algorithm,version_verification:method};
}
function validateControlManifest(plan, filename, manifest, source = sourceInventory(plan.root)) {
  validateTarget(plan,filename);
  if (!manifest || manifest.schema_version !== 1 || manifest.kind !== 'suma-desktop-control' || manifest.product_version !== plan.version || manifest.target !== plan.target || manifest.platform !== plan.platform || manifest.arch !== plan.arch || manifest.binary_sha256 !== fileHash(filename) || manifest.source_sha256 !== source.sha256 || !['compiled_current_source','native_version_and_source_build_receipt'].includes(manifest.version_verification)) throw failure('binary_manifest',filename,plan.language);
  return manifest;
}
module.exports = {sha256,fileHash,inventory,sourceInventory,inspectBinary,validateTarget,assertPlainPath,createControlManifest,validateControlManifest};
