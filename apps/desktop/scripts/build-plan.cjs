'use strict';
const fs = require('node:fs');
const path = require('node:path');

const messages = {
  option: ['无效、重复或缺少值的打包参数', 'Invalid, duplicate, or missing packaging option'],
  absolute_path: ['资源及输出路径必须为绝对路径', 'Resource and output paths must be absolute'],
  output_path: ['输出必须位于仓库 dist/desktop 或 .cache 的子目录', 'Output must be a child of repository dist/desktop or .cache'],
  native_host_required: ['安装包和便携包必须在目标操作系统构建', 'Installers and portable packages require the target operating system'],
  mac_host_required: ['macOS 应用目录也必须在 macOS 构建', 'macOS application directories also require a macOS host'],
  signing_unsupported: ['Linux 不支持此代码签名策略；请使用 off', 'Linux does not support this code-signing policy; use off'],
  signing_pipeline_unverified: ['签名后的控制程序来源校验链尚未实现；auto 和 required 当前不可用，请使用 off', 'Signed control provenance verification is not implemented; auto and required are currently unavailable, use off'],
  source_changed: ['构建期间源码发生变化，请重新构建', 'Source changed during preparation; build again'],
  unsafe_path: ['拒绝链接、特殊文件或越界路径', 'Linked, special, or escaping paths are not supported'],
  missing_web: ['网页目录必须含有非空 index.html', 'Web directory must contain a nonempty index.html'],
  private_web_data: ['网页资源包含疑似凭据、密钥或数据库文件', 'Web resources contain likely credentials, keys, or database files'],
  binary_target: ['控制程序格式或架构与目标不匹配', 'Control binary format or architecture does not match the target'],
  binary_manifest: ['控制程序来源清单缺失或与当前源码、版本、目标或摘要不匹配', 'Control provenance manifest is missing or does not match current source, version, target, or hash'],
  binary_version: ['控制程序产品版本未通过校验', 'Control binary product version could not be verified'],
  command_failed: ['构建子进程失败；请检查上述原始诊断', 'Build subprocess failed; inspect the preceding original diagnostics'],
  startup_smoke_failed: ['打包后的控制服务启动检查失败', 'Packaged control startup smoke check failed'],
  output_nonempty: ['输出目录已含文件；请使用新的输出子目录以保留既有产物', 'Output directory is not empty; use a new output child to preserve existing artifacts'],
  build_busy: ['同一目标已有准备或打包任务；请等待或核对锁文件', 'This target already has a preparation or packaging task; wait or inspect its lock file'],
  build_empty: ['构建未生成可核验的实际文件', 'Build produced no verifiable files'],
  electron_target: ['本地 Electron 版本、格式或架构不匹配；请提供目标发行目录', 'Local Electron version, format, or architecture does not match; provide the target distribution directory'],
  embed_inventory: ['Go 内嵌资源需要先纳入源码摘要范围', 'Go embedded resources must first be included in the source inventory'],
  maintainer_required: ['Debian 安装包需要真实维护者；请设置 --maintainer 或 SUMA_DESKTOP_MAINTAINER 为姓名 <邮箱>', 'Debian packages require a real maintainer; set --maintainer or SUMA_DESKTOP_MAINTAINER to Name <email>'],
  maintainer_invalid: ['维护者必须是有效的姓名 <邮箱>，不能包含控制字符', 'Maintainer must be a valid Name <email> without control characters'],
  source_inspector_node: ['源码观察器需要目标平台 Node 22+ 及其原始许可证；跨目标请提供 --node-binary 和 --node-license', 'Source inspection requires target Node 22+ and its original license; provide --node-binary and --node-license for a different target'],
  source_inspector_bundle: ['源码观察器及依赖许可资源缺失或摘要不匹配，请重新准备桌面资源', 'Source inspector or dependency notices are missing or their hashes differ; prepare desktop resources again'],
  opencode_runtime_node: ['OpenCode 运行时需要目标平台 Node 22+ 及其原始许可证；跨目标请提供 --node-binary 和 --node-license', 'OpenCode requires target Node 22+ and its original license; provide --node-binary and --node-license for a different target'],
  opencode_runtime_bundle: ['OpenCode SDK 资源或许可证缺失、版本不符或摘要变化，请重新准备桌面资源', 'OpenCode SDK assets or notices are missing, have a different version or changed hashes; prepare desktop resources again'],
  claude_sdk_runtime_node: ['Claude SDK 运行时需要目标平台 Node 22+ 及原始许可证', 'Claude SDK requires target Node 22+ and its original license'],
  claude_sdk_runtime_bundle: ['Claude SDK 宿主资源或许可证缺失、版本不符或摘要变化，请重新准备桌面资源', 'Claude SDK host assets or notices are missing, have a different version or changed hashes; prepare desktop resources again'],
};
function locale(value) { return value === 'en-US' || value === 'en' ? 'en-US' : 'zh-CN'; }
function failure(code, detail = '', language = 'zh-CN') {
  const error = new Error(`${(messages[code] || messages.command_failed)[locale(language) === 'en-US' ? 1 : 0]}${detail ? `: ${detail}` : ''}`);
  error.code = code;
  return error;
}
function within(parent, child) {
  const relative = path.relative(parent, child);
  return relative !== '' && !relative.startsWith(`..${path.sep}`) && relative !== '..' && !path.isAbsolute(relative);
}
function createPlan(argv = [], context = {}) {
  const root = path.resolve(context.root || path.resolve(__dirname, '../../..'));
  const env = context.env || process.env;
  const hostPlatform = context.hostPlatform || process.platform;
  const hostArch = context.hostArch || process.arch;
  const options = {};
  const boolean = new Set(['plan', 'help']);
  const names = new Set(['platform', 'arch', 'format', 'web-dir', 'control-binary', 'control-manifest', 'electron-dist', 'node-binary', 'node-license', 'out-dir', 'signing', 'language', 'maintainer', ...boolean]);
  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i];
    if (!arg.startsWith('--') || !names.has(arg.slice(2)) || Object.hasOwn(options, arg.slice(2))) throw failure('option', arg, options.language || env.SUMA_LANGUAGE);
    const name = arg.slice(2);
    if (boolean.has(name)) options[name] = true;
    else {
      const value = argv[++i];
      if (!value || value.startsWith('--') || /[\x00-\x1f]/.test(value)) throw failure('option', arg, options.language || env.SUMA_LANGUAGE);
      options[name] = value;
    }
  }
  const language = locale(options.language || env.SUMA_LANGUAGE);
  if (options.language && !['zh-CN','en-US'].includes(options.language)) throw failure('option','--language', language);
  const platform = options.platform || hostPlatform;
  const arch = options.arch || hostArch;
  const format = options.format || 'installer';
  const signing = options.signing || 'off';
  if (!['win32','darwin','linux'].includes(platform) || !['x64','arm64'].includes(arch) || !['installer','portable','dir','all'].includes(format) || !['off','auto','required'].includes(signing)) throw failure('option', '', language);
  if (options['control-manifest'] && !options['control-binary']) throw failure('option', '--control-manifest requires --control-binary', language);
  for (const name of ['web-dir','control-binary','control-manifest','electron-dist','node-binary','node-license','out-dir']) if (options[name] && !path.isAbsolute(options[name])) throw failure('absolute_path', `--${name}`, language);
  const target = `${platform}-${arch}`;
  const stage = path.join(root, '.cache/desktop', target);
  const output = path.resolve(options['out-dir'] || path.join(root, 'dist/desktop', target));
  if (!within(path.join(root,'dist/desktop'),output) && !within(path.join(root,'.cache'),output)) throw failure('output_path',output,language);
  if (output === stage || within(stage, output) || within(output, stage)) throw failure('output_path',output,language);
  const version = JSON.parse(fs.readFileSync(path.join(root, 'apps/desktop/package.json'), 'utf8')).version;
  const rootPackage = JSON.parse(fs.readFileSync(path.join(root,'package.json'),'utf8'));
  if (version !== rootPackage.version || !/^\d+\.\d+\.\d+$/.test(version)) throw failure('option','product version drift',language);
  const formats = {win32:{installer:['nsis'],portable:['portable']},darwin:{installer:['dmg'],portable:['zip']},linux:{installer:['AppImage','deb'],portable:['tar.gz']}}[platform];
  const targets = format === 'dir' ? ['dir'] : format === 'all' ? [...formats.installer,...formats.portable] : formats[format];
  const maintainer = options.maintainer || env.SUMA_DESKTOP_MAINTAINER || rootPackage.config?.desktopMaintainer || null;
  if (maintainer) {
    if (typeof maintainer !== 'string') throw failure('maintainer_invalid','',language);
    const match = /^([^<>\x00-\x1f]{1,100})\s*<([A-Za-z0-9.!#$%&'*+/=?^_`{|}~-]+)@([A-Za-z0-9.-]+)>$/.exec(maintainer);
    const domain = match?.[3].split('.');
    if (!match || !match[1].trim() || !domain || domain.length < 2 || domain.some(label=>!/^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?$/.test(label)) || !/^[A-Za-z]{2,63}$/.test(domain.at(-1))) throw failure('maintainer_invalid','',language);
  }
  const reasons = [];
  if (platform !== hostPlatform && format !== 'dir') reasons.push('native_host_required');
  if (platform === 'darwin' && hostPlatform !== 'darwin' && format === 'dir') reasons.push('mac_host_required');
  if (platform === 'linux' && signing !== 'off') reasons.push('signing_unsupported');
  if (platform !== 'linux' && signing !== 'off') reasons.push('signing_pipeline_unverified');
  if (platform === 'linux' && targets.includes('deb') && !maintainer) reasons.push('maintainer_required');
  return {
    schema_version:1, product:'suma', version, language, root, platform, arch, target, format, targets,
    host:{platform:hostPlatform,arch:hostArch}, goos:platform === 'win32' ? 'windows' : platform, goarch:arch === 'x64' ? 'amd64' : arch,
    plan_only:Boolean(options.plan), help:Boolean(options.help), output, stage,
    resources:{control:path.join(stage,'control',platform === 'win32' ? 'suma-control.exe' : 'suma-control'),web:path.join(stage,'web'),source_inspector:path.join(stage,'source-inspector'),opencode_runtime:path.join(stage,'control','opencode-runtime'),claude_sdk_runtime:path.join(stage,'control','claude-runtime')},
    web_source:options['web-dir'] ? path.resolve(options['web-dir']) : null,
    control_source:options['control-binary'] ? path.resolve(options['control-binary']) : null,
    control_manifest:options['control-binary'] ? path.resolve(options['control-manifest'] || `${options['control-binary']}.manifest.json`) : null,
    electron_dist:options['electron-dist'] ? path.resolve(options['electron-dist']) : null,
    node_source:options['node-binary'] ? path.resolve(options['node-binary']) : platform === hostPlatform && arch === hostArch ? process.execPath : null,
    node_license_source:options['node-license'] ? path.resolve(options['node-license']) : null,
    tmp:path.join(stage,'tmp'), cache:path.join(stage,'cache'), signing, supported_signing:['off'], maintainer, publish:'never',
    native_host_required:format !== 'dir' || platform === 'darwin', build_allowed:reasons.length === 0, reasons,
    limitations:['cross_os_dir_requires_sdk_and_target_electron_resources','signing_and_notarization_need_independent_verification','installation_and_gui_acceptance_not_proven'],
    artifact_prefix:`Suma-${version}-${platform}-${arch}`,
  };
}
function assertBuildAllowed(plan) { if (!plan.build_allowed) throw failure(plan.reasons[0],plan.target,plan.language); }
function childEnvironment(plan, parent = process.env) {
  const env = {...parent, GOOS:plan.goos, GOARCH:plan.goarch, CGO_ENABLED:'0', GOAMD64:'v1', GOARM64:'v8.0', GOFLAGS:'', GOTOOLCHAIN:'local', GOTELEMETRY:'off', GOWORK:'off', GOCACHE:path.join(plan.cache,'go-build'), GOMODCACHE:path.join(plan.cache,'go-mod'), GOTMPDIR:plan.tmp, TMPDIR:plan.tmp, TMP:plan.tmp, TEMP:plan.tmp, ELECTRON_CACHE:path.join(plan.cache,'electron'), ELECTRON_BUILDER_CACHE:path.join(plan.cache,'electron-builder'), SUMA_LANGUAGE:plan.language};
  if (plan.signing === 'off') {
    for (const name of Object.keys(env)) if (/^(?:WIN_)?CSC_/.test(name) || /^(?:APPLE_|ASC_)/.test(name)) delete env[name];
    env.CSC_IDENTITY_AUTO_DISCOVERY = 'false';
  }
  return env;
}
module.exports = {createPlan, assertBuildAllowed, childEnvironment, within, failure, locale};
