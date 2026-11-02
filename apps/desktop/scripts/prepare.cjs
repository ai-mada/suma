'use strict';
const fs = require('node:fs');
const path = require('node:path');
const {randomUUID} = require('node:crypto');
const {spawnSync} = require('node:child_process');
const {createPlan,childEnvironment,failure,within} = require('./build-plan.cjs');
const {inventory,sourceInventory,assertPlainPath,createControlManifest,validateControlManifest,validateTarget} = require('./provenance.cjs');
const {prepareSourceInspector} = require('./source-inspector.cjs');
const {prepareOpenCodeRuntime} = require('./opencode-runtime.cjs');
const {prepareClaudeSDKRuntime} = require('./claude-sdk-runtime.cjs');

function assertScratch(plan) {
  const directories = [plan.stage,plan.tmp,plan.cache,path.join(plan.cache,'go-build'),path.join(plan.cache,'go-mod'),path.join(plan.cache,'electron'),path.join(plan.cache,'electron-builder'),path.join(plan.stage,'previous'),path.dirname(plan.resources.control),plan.resources.web,plan.resources.source_inspector,plan.resources.opencode_runtime,plan.resources.claude_sdk_runtime];
  for (const directory of directories) {
    if (directory !== plan.stage && !within(plan.stage,directory)) throw failure('unsafe_path',directory,plan.language);
    assertPlainPath(directory,plan.language,true);
  }
  function inspect(directory) {
    if (!fs.existsSync(directory)) return;
    if (!fs.statSync(directory).isDirectory()) throw failure('unsafe_path',directory,plan.language);
    for (const entry of fs.readdirSync(directory,{withFileTypes:true})) {
      const filename = path.join(directory,entry.name);
      if (entry.isSymbolicLink() || (!entry.isDirectory() && !entry.isFile())) throw failure('unsafe_path',filename,plan.language);
      if (entry.isDirectory()) inspect(filename);
    }
  }
  // Existing cache seeds must not route a later Go/SDK write through a nested link.
  inspect(plan.cache);inspect(plan.tmp);
}
async function withTargetLock(plan, action) {
  assertScratch(plan);
  fs.mkdirSync(plan.stage,{recursive:true});
  const lock = path.join(plan.stage,'build.lock');
  let handle;
  try { handle = fs.openSync(lock,'wx'); }
  catch (error) { if (error.code === 'EEXIST') throw failure('build_busy',plan.target,plan.language); throw error; }
  fs.writeFileSync(handle,JSON.stringify({pid:process.pid,started_at:new Date().toISOString()}));
  try { return await action(); }
  finally { fs.closeSync(handle);fs.unlinkSync(lock); }
}
function run(command,args,options) { return spawnSync(command,args,{...options,windowsHide:true,encoding:'utf8'}); }
async function checkedRun(command,args,options,plan,execute=run) {
  const result = await execute(command,args,options);
  if (result.error || result.status !== 0) throw failure('command_failed',result.error?.message || `${path.basename(command)} (${result.status ?? 'signal'})`,plan.language);
  return result;
}
function archivePrevious(plan) {
  // Preserve prior generated resources, including unexpected unique files. No recursive deletion.
  const previous = path.join(plan.stage,'previous',randomUUID());
  assertPlainPath(previous,plan.language,true);
  for (const name of ['control','web','source-inspector','prepared.json']) {
    const filename = path.join(plan.stage,name);
    if (!fs.existsSync(filename)) continue;
    assertPlainPath(filename,plan.language);
    fs.mkdirSync(previous,{recursive:true});
    fs.renameSync(filename,path.join(previous,name));
  }
}
function validateWeb(web, plan) {
  if (!web.files.some(file=>file.path === 'index.html' && file.size_bytes > 0)) throw failure('missing_web','index.html',plan.language);
  for (const file of web.files) {
    const segments = file.path.split('/');
    if (segments.some(name=>/^\.env(?:\.|$)/i.test(name) || /^desktop-workspace\.key$/i.test(name) || /\.(?:db|sqlite|sqlite3)(?:-(?:wal|shm))?$/i.test(name) || /\.(?:key|pem|pfx|p12)$/i.test(name))) throw failure('private_web_data',file.path,plan.language);
  }
}
async function prepare(plan, context = {}) {
  if (plan.plan_only) return plan;
  if (plan.signing !== 'off') throw failure(plan.platform === 'linux' ? 'signing_unsupported' : 'signing_pipeline_unverified','',plan.language);
  if (!context.locked) return withTargetLock(plan,()=>prepare(plan,{...context,locked:true}));
  assertScratch(plan);
  const execute = context.run || run;
  const source = sourceInventory(plan.root);
  let prebuilt;
  for (const input of [plan.web_source,plan.control_source]) if (input && (input === plan.stage || within(plan.stage,input))) throw failure('unsafe_path',input,plan.language);
  if (plan.control_source) {
    assertPlainPath(plan.control_manifest,plan.language);
    try { prebuilt = JSON.parse(fs.readFileSync(plan.control_manifest,'utf8')); }
    catch { throw failure('binary_manifest',plan.control_manifest,plan.language); }
    validateControlManifest(plan,plan.control_source,prebuilt,source);
  }
  let web;
  if (plan.web_source) {
    web = inventory(plan.web_source,{language:plan.language});
    validateWeb(web,plan);
  }
  archivePrevious(plan);
  for (const directory of [path.dirname(plan.resources.control),plan.resources.web,plan.tmp,plan.cache,path.join(plan.cache,'go-build'),path.join(plan.cache,'go-mod')]) fs.mkdirSync(directory,{recursive:true});
  const env = childEnvironment(plan,context.env || process.env);
  if (prebuilt) fs.copyFileSync(plan.control_source,plan.resources.control);
  else await checkedRun(env.SUMA_GO_EXE || 'go',['build','-trimpath','-buildvcs=false','-mod=readonly','-o',plan.resources.control,'./cmd/suma-control'],{cwd:plan.root,env,stdio:'inherit'},plan,execute);
  validateTarget(plan,plan.resources.control);
  if (plan.platform !== 'win32') fs.chmodSync(plan.resources.control,0o755);
  if (plan.platform === plan.host.platform && plan.arch === plan.host.arch) {
    const version = await checkedRun(plan.resources.control,['--language=en','--version'],{cwd:plan.stage,env,stdio:'pipe'},plan,execute);
    if (version.stdout?.trim() !== `suma-control ${plan.version}`) throw failure('binary_version','suma-control',plan.language);
  }
  const control = prebuilt || createControlManifest(plan,plan.resources.control,source);
  const sourceInspector = await prepareSourceInspector(plan,env,(command,args,options)=>checkedRun(command,args,options,plan,execute));
  const openCodeRuntime = await prepareOpenCodeRuntime(plan,env,(command,args,options)=>checkedRun(command,args,options,plan,execute));
  const claudeSDKRuntime = await prepareClaudeSDKRuntime(plan,env,(command,args,options)=>checkedRun(command,args,options,plan,execute),openCodeRuntime);
  if (web) {
    for (const file of web.files) {
      const destination = path.join(plan.resources.web,file.path);
      fs.mkdirSync(path.dirname(destination),{recursive:true});
      fs.copyFileSync(path.join(plan.web_source,file.path),destination);
    }
  } else {
    const webRoot = path.join(plan.root,'apps/web');
    const tsc = require.resolve('typescript/bin/tsc',{paths:[webRoot]});
    const vite = path.join(path.dirname(require.resolve('vite/package.json',{paths:[webRoot]})),'bin/vite.js');
    await checkedRun(process.execPath,[tsc,'--project',path.join(webRoot,'tsconfig.json'),'--noEmit'],{cwd:webRoot,env,stdio:'inherit'},plan,execute);
    await checkedRun(process.execPath,[vite,'build','--outDir',plan.resources.web,'--emptyOutDir','--configLoader','runner'],{cwd:webRoot,env,stdio:'inherit'},plan,execute);
    web = inventory(plan.resources.web,{language:plan.language});
    validateWeb(web,plan);
  }
  const stagedWeb = inventory(plan.resources.web,{language:plan.language});
  if (web.sha256 !== stagedWeb.sha256 || source.sha256 !== sourceInventory(plan.root).sha256) throw failure('source_changed','',plan.language);
  fs.writeFileSync(`${plan.resources.control}.manifest.json`,`${JSON.stringify(control,null,2)}\n`);
  const receipt = {schema_version:1,product_version:plan.version,target:plan.target,control,web:stagedWeb,source_inspector:sourceInspector,opencode_runtime:openCodeRuntime,claude_sdk_runtime:claudeSDKRuntime};
  fs.writeFileSync(path.join(plan.stage,'prepared.json'),`${JSON.stringify(receipt,null,2)}\n`);
  return receipt;
}
async function prepareDesktop(argv = process.argv.slice(2), context = {}) {
  const plan = createPlan(argv,context);
  if (plan.plan_only) return plan;
  if (plan.help) return {usage:'node apps/desktop/scripts/prepare.cjs --platform win32|darwin|linux --arch x64|arm64 [--plan] [--web-dir ABS] [--control-binary ABS] [--control-manifest ABS] [--node-binary ABS] [--node-license ABS] [--language zh-CN|en-US]'};
  return prepare(plan,context);
}
if (require.main === module) prepareDesktop().then(value=>process.stdout.write(`${JSON.stringify(value,null,2)}\n`)).catch(error=>{process.stderr.write(`[${error.code || 'prepare_failed'}] ${error.message}\n`);process.exitCode=1;});
module.exports = {prepare,prepareDesktop,withTargetLock,checkedRun,assertScratch};
