'use strict';
const fs=require('node:fs'),path=require('node:path'),{pathToFileURL}=require('node:url'),{sha}=require('./fixtures.cjs');
const root=path.resolve(__dirname,'../../..'),base=path.join(root,'.cache/real-cli-onboarding-226/web-history-recovery'),out=path.join(base,'out'),hash=file=>sha(fs.readFileSync(file));
async function main(){
 if(fs.existsSync(path.join(out,'fixture.js')))throw Error('Existing history variant retained; do not overwrite its bundle');
 const modules=path.join(root,'apps/web/node_modules'),vite=path.dirname(fs.realpathSync(require.resolve('vite/package.json',{paths:[path.join(root,'apps/web')]}))),inputs=new Map(),{build}=await import(pathToFileURL(path.join(vite,'dist/node/index.js')).href);
 await build({configFile:false,root:__dirname,clearScreen:false,logLevel:'warn',resolve:{alias:Object.fromEntries(['react','react-dom','react-i18next','i18next','@xyflow/react'].map(name=>[name,path.join(modules,name)])),dedupe:['react','react-dom']},esbuild:{jsx:'automatic'},define:{'process.env.NODE_ENV':'"production"'},plugins:[{name:'frozen-history-inputs',transform(_code,id){const file=id.split('?')[0],normal=file.replaceAll('\\','/');if(!normal.includes('/node_modules/')&&(normal.includes('/apps/web/src/')||normal.includes('/packages/contracts/src/')||normal.includes('/tests/integration/'))&&fs.existsSync(file)&&fs.statSync(file).isFile())inputs.set(file,hash(file));}}],build:{outDir:out,emptyOutDir:false,copyPublicDir:false,minify:false,sourcemap:false,target:'chrome134',cssCodeSplit:false,reportCompressedSize:false,lib:{entry:path.join(__dirname,'entry.tsx'),name:'ProbeHistory226',formats:['iife'],fileName:()=> 'fixture.js',cssFileName:'fixture'},rollupOptions:{output:{inlineDynamicImports:true}}}});
 for(const [file,sha256]of inputs)if(hash(file)!==sha256)throw Error('Source changed during history build: '+file);
 fs.copyFileSync(path.join(__dirname,'index.html'),path.join(out,'index.html'));
 fs.writeFileSync(path.join(base,'build-inputs.json'),JSON.stringify({sources:[...inputs].map(([file,sha256])=>({path:path.relative(root,file).replaceAll('\\','/'),sha256})),source_changes_during_build:0},null,2)+'\n');
 fs.writeFileSync(path.join(base,'build-assets.json'),JSON.stringify(['index.html','fixture.js','fixture.css'].map(file=>({path:path.relative(root,path.join(out,file)).replaceAll('\\','/'),sha256:hash(path.join(out,file))})),null,2)+'\n');
 console.log('Built only the isolated history regression variant; original representative bundle retained.');
}
main().catch(error=>{console.error(error.stack||String(error));process.exitCode=1;});
