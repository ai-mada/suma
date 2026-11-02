'use strict';
const fs=require('node:fs'),path=require('node:path'),{pathToFileURL}=require('node:url');
const root=path.resolve(__dirname,'../../..'),out=path.join(root,'.cache/tool-management-experience-206/web/out');
(async()=>{
 const vite=path.dirname(fs.realpathSync(require.resolve('vite/package.json',{paths:[path.join(root,'apps/web')]}))),modules=path.join(root,'apps/web/node_modules');
 const {build}=await import(pathToFileURL(path.join(vite,'dist/node/index.js')).href);
 await build({configFile:false,root:__dirname,clearScreen:false,logLevel:'warn',resolve:{alias:{react:path.join(modules,'react'),'react-dom':path.join(modules,'react-dom'),'react-i18next':path.join(modules,'react-i18next'),i18next:path.join(modules,'i18next'),'@xyflow/react':path.join(modules,'@xyflow/react')},dedupe:['react','react-dom']},esbuild:{jsx:'automatic'},define:{'process.env.NODE_ENV':'"production"'},build:{outDir:out,emptyOutDir:false,copyPublicDir:false,minify:false,sourcemap:false,target:'chrome134',cssCodeSplit:false,reportCompressedSize:false,lib:{entry:path.join(__dirname,'entry.tsx'),name:'ToolManagementExperience206',formats:['iife'],fileName:()=> 'fixture.js',cssFileName:'fixture'},rollupOptions:{output:{inlineDynamicImports:true}}}});
 fs.copyFileSync(path.join(__dirname,'index.html'),path.join(out,'index.html'));console.log('Built one isolated ToolWorkspace entry; active preview unchanged.');
})().catch(error=>{console.error(error);process.exitCode=1;});
