'use strict';
const fs = require('node:fs'), path = require('node:path'), crypto = require('node:crypto');
const {pathToFileURL} = require('node:url');
const root = path.resolve(__dirname, '../../..'), base = path.join(root, '.cache/qoder-tool-preparation-221/actual');
const digest = file => crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');
async function main() {
  const frozen = JSON.parse(fs.readFileSync(path.join(base, 'source-manifest.json'), 'utf8'));
  for (const source of frozen.sources) if (digest(path.join(root, source.path)) !== source.sha256) throw Error('Frozen source changed: ' + source.path);
  const vite = path.dirname(fs.realpathSync(require.resolve('vite/package.json', {paths: [path.join(root, 'apps/web')]})));
  const {build} = await import(pathToFileURL(path.join(vite, 'dist/node/index.js')).href);
  const modules = path.join(root, 'apps/web/node_modules'), out = path.join(base, 'out'), inputs = new Map();
  await build({configFile: false, root: __dirname, clearScreen: false, logLevel: 'warn',
    resolve: {alias: Object.fromEntries(['react', 'react-dom', 'react-i18next', 'i18next', '@xyflow/react'].map(name => [name, path.join(modules, name)])), dedupe: ['react', 'react-dom']},
    esbuild: {jsx: 'automatic'}, define: {'process.env.NODE_ENV': '"production"'},
    plugins: [{name: 'retain-input-hashes', transform(_code, id) {
      const file = id.split('?')[0], normal = file.replaceAll('\\', '/');
      if (!normal.includes('/node_modules/') && (normal.includes('/apps/web/src/') || normal.includes('/packages/contracts/src/')) && fs.existsSync(file) && fs.statSync(file).isFile()) inputs.set(file, digest(file));
    }}],
    build: {outDir: out, emptyOutDir: false, copyPublicDir: false, minify: false, sourcemap: false, target: 'chrome134', cssCodeSplit: false, reportCompressedSize: false,
      lib: {entry: path.join(__dirname, 'entry.tsx'), name: 'QoderToolPreparation221', formats: ['iife'], fileName: () => 'fixture.js', cssFileName: 'fixture'}, rollupOptions: {output: {inlineDynamicImports: true}}}});
  for (const [file, sha256] of inputs) if (digest(file) !== sha256) throw Error('Source changed during build: ' + file);
  fs.copyFileSync(path.join(__dirname, 'index.html'), path.join(out, 'index.html'));
  fs.writeFileSync(path.join(base, 'build-inputs.json'), JSON.stringify({sources: [...inputs].map(([file, sha256]) => ({path: path.relative(root, file).replaceAll('\\', '/'), sha256})), source_changes_during_build: 0}, null, 2) + '\n');
  fs.writeFileSync(path.join(base, 'build-assets.json'), JSON.stringify(['index.html', 'fixture.js', 'fixture.css'].map(file => ({path: path.relative(root, path.join(out, file)).replaceAll('\\', '/'), sha256: digest(path.join(out, file))})), null, 2) + '\n');
  console.log('Built isolated production ToolSetupGuide and Qoder tool preparation; active previews untouched.');
}
main().catch(error => {console.error(error); process.exitCode = 1;});
