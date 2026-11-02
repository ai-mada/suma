'use strict';
// Read the existing preparation entry, but preserve all T209 build evidence.
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto'),{pathToFileURL}=require('node:url');
const root=path.resolve(__dirname,'../../..'),source=path.join(__dirname,'../pool-config-preparation'),out=path.join(root,'.cache/pool-config-scope-210/web/out');
const sha=file=>crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');
(async()=>{
 const old=path.join(root,'.cache/pool-config-preparation-209/web/out'),before=['index.html','fixture.js','fixture.css'].map(name=>({path:path.relative(root,path.join(old,name)),sha256:sha(path.join(old,name))}));
 const evidence=path.join(root,'.cache/pool-config-scope-210/integration');fs.mkdirSync(evidence,{recursive:true});fs.writeFileSync(path.join(evidence,'original-209-assets.json'),JSON.stringify(before,null,2)+'\n');
 const vite=path.dirname(fs.realpathSync(require.resolve('vite/package.json',{paths:[path.join(root,'apps/web')]}))),modules=path.join(root,'apps/web/node_modules');const {build}=await import(pathToFileURL(path.join(vite,'dist/node/index.js')).href);
 await build({configFile:false,root:source,clearScreen:false,logLevel:'error',resolve:{alias:{react:path.join(modules,'react'),'react-dom':path.join(modules,'react-dom'),'react-i18next':path.join(modules,'react-i18next'),i18next:path.join(modules,'i18next'),'@xyflow/react':path.join(modules,'@xyflow/react')},dedupe:['react','react-dom']},esbuild:{jsx:'automatic'},define:{'process.env.NODE_ENV':'"production"'},build:{outDir:out,emptyOutDir:false,copyPublicDir:false,minify:false,sourcemap:false,target:'chrome134',cssCodeSplit:false,reportCompressedSize:false,lib:{entry:path.join(source,'entry.tsx'),name:'PoolConfigScope210',formats:['iife'],fileName:()=> 'fixture.js',cssFileName:'fixture'},rollupOptions:{output:{inlineDynamicImports:true}}}});
 fs.copyFileSync(path.join(source,'index.html'),path.join(out,'index.html'));for(const input of before)if(sha(path.join(root,input.path))!==input.sha256)throw Error('Original T209 evidence changed');console.log('Built one isolated scope-change fixture; original T209 assets preserved.');
})().catch(error=>{console.error(error);process.exitCode=1;});
