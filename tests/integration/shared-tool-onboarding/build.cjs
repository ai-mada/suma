'use strict';
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const {pathToFileURL} = require('node:url');
const root = path.resolve(__dirname, '../../..');
const base = path.join(root, '.cache/shared-tool-onboarding-219/actual');
const out = path.join(base, 'out');
const sha = value => crypto.createHash('sha256').update(value).digest('hex');

async function main() {
  const manifest = JSON.parse(fs.readFileSync(path.join(base, 'source-manifest-final.json'), 'utf8'));
  for (const source of manifest.sources) {
    if (sha(fs.readFileSync(path.join(root, source.path))) !== source.sha256) throw Error('Frozen source changed: ' + source.path);
  }
  const vite = path.dirname(fs.realpathSync(require.resolve('vite/package.json', {paths: [path.join(root, 'apps/web')]})));
  const modules = path.join(root, 'apps/web/node_modules');
  const {build} = await import(pathToFileURL(path.join(vite, 'dist/node/index.js')).href);
  const inputs = new Map();
  await build({
    configFile: false, root: __dirname, clearScreen: false, logLevel: 'warn',
    resolve: {alias: Object.fromEntries(['react', 'react-dom', 'react-i18next', 'i18next', '@xyflow/react'].map(name => [name, path.join(modules, name)])), dedupe: ['react', 'react-dom']},
    esbuild: {jsx: 'automatic'}, define: {'process.env.NODE_ENV': '"production"'},
    plugins: [{name: 'retain-production-input-hashes', transform(_code, id) {
      const file = id.split('?')[0];
      if (!file.includes('/node_modules/') && fs.existsSync(file) && fs.statSync(file).isFile() && (file.replaceAll('\\', '/').includes('/apps/web/src/') || file.replaceAll('\\', '/').includes('/packages/contracts/src/'))) {
        inputs.set(file, sha(fs.readFileSync(file)));
      }
    }}],
    build: {outDir: out, emptyOutDir: false, copyPublicDir: false, minify: false, sourcemap: false, target: 'chrome134', cssCodeSplit: false, reportCompressedSize: false,
      lib: {entry: path.join(__dirname, 'entry.tsx'), name: 'SharedToolOnboarding219', formats: ['iife'], fileName: () => 'fixture.js', cssFileName: 'fixture'},
      rollupOptions: {output: {inlineDynamicImports: true}}}
  });
  for (const [file, digest] of inputs) if (sha(fs.readFileSync(file)) !== digest) throw Error('Build source changed: ' + file);
  fs.copyFileSync(path.join(__dirname, 'index.html'), path.join(out, 'index.html'));
  fs.writeFileSync(path.join(base, 'build-inputs.json'), JSON.stringify({sources: [...inputs].map(([file, sha256]) => ({path: path.relative(root, file).replaceAll('\\', '/'), sha256})), source_changes_during_build: 0}, null, 2) + '\n');
  fs.writeFileSync(path.join(base, 'build-assets.json'), JSON.stringify(['index.html', 'fixture.js', 'fixture.css'].map(file => ({path: path.relative(root, path.join(out, file)).replaceAll('\\', '/'), sha256: sha(fs.readFileSync(path.join(out, file)))})), null, 2) + '\n');
  console.log('Built isolated production shared-tool onboarding components; active preview assets unchanged.');
}
main().catch(error => {console.error(error); process.exitCode = 1;});
