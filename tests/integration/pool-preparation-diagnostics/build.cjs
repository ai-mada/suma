'use strict';
// Build the existing production entry in a separate stage directory; older
// accepted preparation/scope screenshots and JavaScript remain byte-identical.
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto'),{pathToFileURL}=require('node:url');
const root=path.resolve(__dirname,'../../..'),source=path.join(__dirname,'../pool-config-preparation'),out=path.join(root,'.cache/pool-preparation-diagnostics-211/web/out'),sha=file=>crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');
(async()=>{
 const before=['pool-config-preparation-209','pool-config-scope-210'].flatMap(stage=>['index.html','fixture.js','fixture.css'].map(name=>{const file=path.join(root,'.cache',stage,'web/out',name);return{path:path.relative(root,file),sha256:sha(file)};}));
 const evidence=path.join(root,'.cache/pool-preparation-diagnostics-211/integration');fs.mkdirSync(evidence,{recursive:true});fs.writeFileSync(path.join(evidence,'original-assets.json'),JSON.stringify(before,null,2)+'\n');
 const vite=path.dirname(fs.realpathSync(require.resolve('vite/package.json',{paths:[path.join(root,'apps/web')]}))),modules=path.join(root,'apps/web/node_modules'),{build}=await import(pathToFileURL(path.join(vite,'dist/node/index.js')).href);
 await build({configFile:false,root:source,clearScreen:false,logLevel:'error',resolve:{alias:{react:path.join(modules,'react'),'react-dom':path.join(modules,'react-dom'),'react-i18next':path.join(modules,'react-i18next'),i18next:path.join(modules,'i18next'),'@xyflow/react':path.join(modules,'@xyflow/react')},dedupe:['react','react-dom']},esbuild:{jsx:'automatic'},define:{'process.env.NODE_ENV':'"production"'},build:{outDir:out,emptyOutDir:false,copyPublicDir:false,minify:false,sourcemap:false,target:'chrome134',cssCodeSplit:false,reportCompressedSize:false,lib:{entry:path.join(source,'entry.tsx'),name:'PoolPreparationDiagnostics211',formats:['iife'],fileName:()=> 'fixture.js',cssFileName:'fixture'},rollupOptions:{output:{inlineDynamicImports:true}}}});
 fs.copyFileSync(path.join(source,'index.html'),path.join(out,'index.html'));for(const file of before)if(sha(path.join(root,file.path))!==file.sha256)throw Error('Accepted preparation assets changed');console.log('Built isolated preparation diagnostic fixture; T209/T210 assets preserved.');
})().catch(error=>{console.error(error);process.exitCode=1;});
