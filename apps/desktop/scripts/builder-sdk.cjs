'use strict';
const fs = require('node:fs');
const path = require('node:path');
const {failure,assertBuildAllowed} = require('./build-plan.cjs');
const {validateTarget,assertPlainPath} = require('./provenance.cjs');
const {assertScratch} = require('./prepare.cjs');

function electronVersion(plan) {
  return JSON.parse(fs.readFileSync(require.resolve('electron/package.json',{paths:[path.join(plan.root,'apps/desktop')]}),'utf8')).version;
}
function validateElectron(plan) {
  if (!plan.electron_dist) return;
  try {
    // pnpm's dependency directory can itself be a link; validate the resolved distribution.
    const distribution = fs.realpathSync(plan.electron_dist);
    assertPlainPath(distribution,plan.language);
    const binary = path.join(distribution,plan.platform === 'win32' ? 'electron.exe' : plan.platform === 'darwin' ? 'Electron.app/Contents/MacOS/Electron' : 'electron');
    validateTarget(plan,binary);
    if (fs.readFileSync(path.join(distribution,'version'),'utf8').trim().replace(/^v/,'') !== electronVersion(plan)) throw Error('version');
  } catch { throw failure('electron_target',plan.target,plan.language); }
}
function builderConfig(plan,rawOutput) {
  const prefix = plan.artifact_prefix;
  return {
    extends:null, appId:'dev.suma.workbench', productName:'Suma', electronVersion:electronVersion(plan),
    ...(plan.electron_dist ? {electronDist:fs.realpathSync(plan.electron_dist)} : {}),
    directories:{output:rawOutput,buildResources:path.join(plan.root,'apps/desktop/scripts')},
    files:['src/**/*','package.json'], asar:true, npmRebuild:false, nodeGypRebuild:false, publish:null,
    artifactName:`${prefix}-portable.\${ext}`, forceCodeSigning:plan.signing === 'required',
    extraResources:[
      {from:plan.resources.control,to:`control/${path.basename(plan.resources.control)}`},
      {from:plan.resources.web,to:'web',filter:['**/*']},
      {from:plan.resources.source_inspector,to:'source-inspector',filter:['node','node.exe','source-inspector.bundle.mjs','NODE_LICENSE.txt','THIRD_PARTY_NOTICES.txt','manifest.json']},
    ],
    win:{target:plan.platform === 'win32' ? plan.targets : [],signExecutable:plan.signing !== 'off'},
    mac:{target:plan.platform === 'darwin' ? plan.targets : [],category:'public.app-category.developer-tools',hardenedRuntime:true,entitlements:path.join(__dirname,'entitlements.mac.plist'),entitlementsInherit:path.join(__dirname,'entitlements.mac.plist'),notarize:false,...(plan.signing === 'off' ? {identity:null} : {})},
    linux:{target:plan.platform === 'linux' ? plan.targets : [],category:'Development',executableName:'suma',packageName:'suma',...(plan.maintainer ? {maintainer:plan.maintainer} : {})},
    nsis:{artifactName:`${prefix}-installer.\${ext}`,oneClick:false,perMachine:false,selectPerMachineByDefault:false,allowElevation:false,allowToChangeInstallationDirectory:true,packElevateHelper:false,deleteAppDataOnUninstall:false,displayLanguageSelector:true,installerLanguages:['zh_CN','en_US'],multiLanguageInstaller:true,differentialPackage:false,runAfterFinish:false},
    portable:{artifactName:`${prefix}-portable.\${ext}`},
    dmg:{artifactName:`${prefix}-installer.\${ext}`},
    appImage:{artifactName:`${prefix}-installer.\${ext}`},
    deb:{artifactName:`${prefix}-installer.\${ext}`},
  };
}
async function buildWithSDK(plan,rawOutput) {
  assertBuildAllowed(plan);
  assertScratch(plan);
  validateElectron(plan);
  const sdk = require('electron-builder');
  const platform = {win32:sdk.Platform.WINDOWS,darwin:sdk.Platform.MAC,linux:sdk.Platform.LINUX}[plan.platform];
  const directories = [];
  const config = builderConfig(plan,rawOutput);
  config.afterPack = async context => { directories.push(context.appOutDir); };
  const artifacts = await sdk.build({projectDir:path.join(plan.root,'apps/desktop'),targets:platform.createTarget(plan.targets,sdk.Arch[plan.arch]),publish:'never',config});
  return {artifacts,directories};
}
if (require.main === module) {
  const input = JSON.parse(fs.readFileSync(process.argv[2],'utf8'));
  buildWithSDK(input.plan,input.rawOutput).then(result=>fs.writeFileSync(process.argv[3],JSON.stringify(result))).catch(error=>{process.stderr.write(`${error.message}\n`);process.exitCode=1;});
}
module.exports = {builderConfig,buildWithSDK,validateElectron};
