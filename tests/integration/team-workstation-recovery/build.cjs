'use strict';
const fs=require('node:fs'),path=require('node:path'),{pathToFileURL}=require('node:url');
const root=path.resolve(__dirname,'../../..'),out=path.join(root,'.cache/team-workstation-recovery-218/web-actual/out');
(async()=>{
 const vite=path.dirname(fs.realpathSync(require.resolve('vite/package.json',{paths:[path.join(root,'apps/web')]}))),modules=path.join(root,'apps/web/node_modules');
 const {build}=await import(pathToFileURL(path.join(vite,'dist/node/index.js')).href);
 await build({configFile:false,root:__dirname,clearScreen:false,logLevel:'warn',resolve:{alias:Object.fromEntries(['react','react-dom','react-i18next','i18next','@xyflow/react'].map(name=>[name,path.join(modules,name)])),dedupe:['react','react-dom']},esbuild:{jsx:'automatic'},define:{'process.env.NODE_ENV':'"production"'},build:{outDir:out,emptyOutDir:false,copyPublicDir:false,minify:false,sourcemap:false,target:'chrome134',cssCodeSplit:false,reportCompressedSize:false,lib:{entry:path.join(__dirname,'entry.tsx'),name:'TeamWorkstationRecovery218',formats:['iife'],fileName:()=> 'fixture.js',cssFileName:'fixture'},rollupOptions:{output:{inlineDynamicImports:true}}}});
 fs.copyFileSync(path.join(__dirname,'index.html'),path.join(out,'index.html'));
 console.log('Built isolated production ToolWorkspace and PoolWorkspace. Active previews and prior stage assets are unchanged.');
})().catch(error=>{console.error(error);process.exitCode=1;});
