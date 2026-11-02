'use strict';
const fs = require('node:fs');
const path = require('node:path');
const {randomUUID} = require('node:crypto');
const {createPlan,assertBuildAllowed,childEnvironment,failure,within} = require('./build-plan.cjs');
const {prepare,withTargetLock,checkedRun} = require('./prepare.cjs');
const {inventory,fileHash,sourceInventory,validateControlManifest,validateTarget,assertPlainPath} = require('./provenance.cjs');
const {validateElectron} = require('./builder-sdk.cjs');
const {validateSourceInspector,inspectorSourceInventory} = require('./source-inspector.cjs');
const {validateOpenCodeRuntime,openCodeSourceInventory} = require('./opencode-runtime.cjs');
const {validateClaudeSDKRuntime,claudeSDKSourceInventory} = require('./claude-sdk-runtime.cjs');
const {smokeControl} = require('./startup-smoke.cjs');

function assertOutput(plan) {
  assertPlainPath(plan.output,plan.language,true);
  if (fs.existsSync(plan.output) && fs.readdirSync(plan.output).length) throw failure('output_nonempty',plan.output,plan.language);
}
async function runSDK(plan,rawOutput,execute) {
  const input = path.join(rawOutput,'sdk-input.json');
  const result = path.join(rawOutput,'sdk-result.json');
  fs.writeFileSync(input,JSON.stringify({plan,rawOutput}));
  await checkedRun(process.execPath,[path.join(__dirname,'builder-sdk.cjs'),input,result],{cwd:plan.root,env:childEnvironment(plan),stdio:'inherit'},plan,execute);
  return JSON.parse(fs.readFileSync(result,'utf8'));
}
function validatePackagedApplication(plan,prepared,directory) {
  const app = plan.platform === 'darwin' ? path.join(directory,'Suma.app/Contents') : directory;
  const runtime = path.join(app,plan.platform === 'win32' ? 'Suma.exe' : plan.platform === 'darwin' ? 'MacOS/Suma' : 'suma');
  validateTarget(plan,runtime);
  const resources = path.join(app,plan.platform === 'darwin' ? 'Resources' : 'resources');
  validateControlManifest(plan,path.join(resources,'control',path.basename(plan.resources.control)),prepared.control);
  if (inventory(path.join(resources,'web'),{language:plan.language}).sha256 !== prepared.web.sha256) throw failure('source_changed','packaged web',plan.language);
  validateSourceInspector(plan,path.join(resources,'source-inspector'),prepared.source_inspector);
  validateOpenCodeRuntime(plan,path.join(resources,'control','opencode-runtime'),prepared.opencode_runtime);
  validateClaudeSDKRuntime(plan,path.join(resources,'control','claude-runtime'),prepared.claude_sdk_runtime);
}
function moveReadyDirectory(source,destination) {
  for (let attempt = 0;;attempt++) {
    try { fs.renameSync(source,destination);return; }
    catch (error) {
      if (!['EPERM','EBUSY','EACCES'].includes(error.code) || attempt >= 5) throw error;
      // Windows file scanners can briefly retain newly closed packaged files.
      Atomics.wait(new Int32Array(new SharedArrayBuffer(4)),0,0,20 * 2 ** attempt);
    }
  }
}
function finalize(plan,prepared,rawOutput,result) {
  assertOutput(plan);
  const selected = [];
  for (const artifact of [...new Set(result.artifacts || [])]) {
    const filename = path.resolve(artifact);
    if (!within(rawOutput,filename)) throw failure('unsafe_path',filename,plan.language);
    assertPlainPath(filename,plan.language);
    if (!fs.statSync(filename).isFile() || !path.basename(filename).startsWith(`${plan.artifact_prefix}-`)) throw failure('unsafe_path',filename,plan.language);
    selected.push({from:filename,to:path.basename(filename)});
  }
  if (plan.format === 'dir') {
    if (result.directories?.length !== 1) throw failure('build_empty','application directory',plan.language);
    const directory = path.resolve(result.directories[0]);
    if (!within(rawOutput,directory)) throw failure('unsafe_path',directory,plan.language);
    inventory(directory,{language:plan.language,allowLinks:true});
    selected.push({from:directory,to:`${plan.artifact_prefix}-dir`});
  }
  else {
    const names = {nsis:'installer.exe',portable:'portable.exe',dmg:'installer.dmg',zip:'portable.zip',AppImage:'installer.AppImage',deb:'installer.deb','tar.gz':'portable.tar.gz'};
    for (const target of plan.targets) if (!selected.some(artifact=>artifact.to === `${plan.artifact_prefix}-${names[target]}`)) throw failure('build_empty',target,plan.language);
  }
  if (!selected.length) throw failure('build_empty','',plan.language);
  const delivery = path.join(rawOutput,'delivery');
  fs.mkdirSync(delivery);
  for (const artifact of selected) fs.cpSync(artifact.from,path.join(delivery,artifact.to),{recursive:true,dereference:false,verbatimSymlinks:true,preserveTimestamps:true});
  const contents = inventory(delivery,{language:plan.language,allowLinks:true});
  if (!contents.files.length) throw failure('build_empty','',plan.language);
  const manifest = {
    schema_version:1,product:'suma',product_version:plan.version,platform:plan.platform,arch:plan.arch,target:plan.target,format:plan.format,targets:plan.targets,
    artifacts:selected.map(artifact=>artifact.to),files:contents.files,
    control:prepared.control,web:{sha256:prepared.web.sha256,files:prepared.web.files.length},source_inspector:prepared.source_inspector,opencode_runtime:prepared.opencode_runtime,claude_sdk_runtime:prepared.claude_sdk_runtime,...(prepared.desktop_source_sha256 ? {desktop_source_sha256:prepared.desktop_source_sha256} : {}),
    signing:{policy:plan.signing,status:plan.signing === 'off' ? 'unsigned' : 'not_independently_verified'},
    verification:{packaging:'sdk_completed',control_startup:prepared.control_startup || 'not_run',installation:false,gui:false,signature:false,notarization:false},
  };
  fs.writeFileSync(path.join(delivery,'manifest.json'),`${JSON.stringify(manifest,null,2)}\n`);
  const checksumFiles = [...contents.files.filter(file=>file.type !== 'symlink'),{path:'manifest.json',sha256:fileHash(path.join(delivery,'manifest.json'))}];
  fs.writeFileSync(path.join(delivery,'SHA256SUMS'),checksumFiles.map(file=>`${file.sha256}  ${file.path}`).join('\n')+'\n');
  fs.mkdirSync(path.dirname(plan.output),{recursive:true});
  if (fs.existsSync(plan.output)) fs.rmdirSync(plan.output); // Only an already validated empty directory.
  moveReadyDirectory(delivery,plan.output);
  return manifest;
}
async function packageDesktop(argv = process.argv.slice(2), context = {}) {
  const plan = createPlan(argv, context);
  if (plan.plan_only) return plan;
  if (plan.help) return {usage:'node apps/desktop/scripts/package.cjs --platform win32|darwin|linux --arch x64|arm64 --format installer|portable|dir|all [--plan] [--web-dir ABS] [--control-binary ABS] [--control-manifest ABS] [--electron-dist ABS] [--node-binary ABS] [--node-license ABS] [--out-dir ABS] [--signing off] [--maintainer "Name <email>"] [--language zh-CN|en-US]',unavailable_signing:failure('signing_pipeline_unverified','',plan.language).message};
  assertBuildAllowed(plan);
  assertOutput(plan);
  validateElectron(plan);
  return withTargetLock(plan,async()=>{
    const desktopSource = inventory(path.join(plan.root,'apps/desktop/src'),{language:plan.language}).sha256;
    const prepared = await prepare(plan,{locked:true,run:context.run});
    prepared.desktop_source_sha256 = desktopSource;
    const rawOutput = path.join(plan.stage,`package-${randomUUID()}`);
    fs.mkdirSync(rawOutput);
    const result = await (context.sdkBuild || runSDK)(plan,rawOutput,context.run);
    validateControlManifest(plan,plan.resources.control,prepared.control,sourceInventory(plan.root));
    if (prepared.web.sha256 !== inventory(plan.resources.web,{language:plan.language}).sha256) throw failure('source_changed','web',plan.language);
    validateSourceInspector(plan,plan.resources.source_inspector,prepared.source_inspector);
    validateOpenCodeRuntime(plan,plan.resources.opencode_runtime,prepared.opencode_runtime);
    validateClaudeSDKRuntime(plan,plan.resources.claude_sdk_runtime,prepared.claude_sdk_runtime);
    if (prepared.claude_sdk_runtime.source_sha256 !== claudeSDKSourceInventory(plan)) throw failure('source_changed','Claude SDK',plan.language);
    if (prepared.opencode_runtime.source_sha256 !== openCodeSourceInventory(plan)) throw failure('source_changed','OpenCode SDK',plan.language);
    if (prepared.source_inspector.source_sha256 !== inspectorSourceInventory(plan)) throw failure('source_changed','source inspector',plan.language);
    if (desktopSource !== inventory(path.join(plan.root,'apps/desktop/src'),{language:plan.language}).sha256) throw failure('source_changed','desktop',plan.language);
    if (result.directories?.length !== 1) throw failure('build_empty','packed application',plan.language);
    if (!within(rawOutput,path.resolve(result.directories[0]))) throw failure('unsafe_path','packed application',plan.language);
    validatePackagedApplication(plan,prepared,result.directories[0]);
    const app = plan.platform === 'darwin' ? path.join(result.directories[0],'Suma.app/Contents/Resources') : path.join(result.directories[0],'resources');
    prepared.control_startup = await smokeControl(plan,rawOutput,app);
    return finalize(plan,prepared,rawOutput,result);
  });
}
if (require.main === module) packageDesktop().then(value=>process.stdout.write(`${JSON.stringify(value,null,2)}\n`)).catch(error=>{process.stderr.write(`[${error.code || 'packaging_failed'}] ${error.message}\n`);process.exitCode=1;});
module.exports = {packageDesktop,finalize,validatePackagedApplication};
