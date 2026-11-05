'use strict';

const fs = require('node:fs');
const path = require('node:path');
const {pathToFileURL} = require('node:url');

const root = path.resolve(__dirname, '../../..');
const base = path.join(root, '.cache/tool-identity-management');
const output = path.join(base, 'web/out');
const web = path.join(root, 'apps/web');
const modules = path.join(web, 'node_modules');

async function main() {
  const vite = path.dirname(fs.realpathSync(require.resolve('vite/package.json', {paths: [web]})));
  const {build} = await import(pathToFileURL(path.join(vite, 'dist/node/index.js')).href);
  const dependencies = ['react', 'react-dom', 'react-i18next', 'i18next', '@xyflow/react'];
  await build({
    configFile: false,
    root: __dirname,
    cacheDir: path.join(base, 'web/vite-cache'),
    clearScreen: false,
    logLevel: 'warn',
    resolve: {
      alias: Object.fromEntries(dependencies.map(name => [name, path.join(modules, name)])),
      dedupe: ['react', 'react-dom'],
    },
    esbuild: {jsx: 'automatic'},
    define: {'process.env.NODE_ENV': '"production"'},
    build: {
      outDir: output,
      emptyOutDir: false,
      copyPublicDir: false,
      minify: false,
      sourcemap: false,
      target: 'chrome134',
      cssCodeSplit: false,
      reportCompressedSize: false,
      lib: {
        entry: path.join(__dirname, 'entry.tsx'),
        name: 'ToolIdentityManagementFixture',
        formats: ['iife'],
        fileName: () => 'fixture.js',
        cssFileName: 'fixture',
      },
      rollupOptions: {output: {inlineDynamicImports: true}},
    },
  });
  fs.copyFileSync(path.join(__dirname, 'index.html'), path.join(output, 'index.html'));
  console.log(path.relative(root, output));
}

main().catch(error => {
  console.error(error);
  process.exitCode = 1;
});
