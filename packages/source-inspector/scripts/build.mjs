import {build} from 'esbuild';
import {fileURLToPath} from 'node:url';
import {builtinModules} from 'node:module';
import {readFile,writeFile} from 'node:fs/promises';
import {dirname,resolve,join} from 'node:path';
const root=fileURLToPath(new URL('../',import.meta.url));
const builtins=new Set(builtinModules.map(value=>value.replace(/^node:/,'')));
const result=await build({absWorkingDir:root,entryPoints:['src/cli.mjs'],outfile:'dist/source-inspector.bundle.mjs',bundle:true,platform:'node',format:'esm',target:'node22',packages:'bundle',legalComments:'eof',metafile:true,banner:{js:"import {createRequire as __sumaCreateRequire} from 'node:module'; const require=__sumaCreateRequire(import.meta.url);"},plugins:[{name:'fixed-node-builtins',setup(context){context.onResolve({filter:/.*/},args=>{const name=args.path.replace(/^node:/,'');if(builtins.has(name))return {path:'node:'+name,external:true}})}}]});
for(const output of Object.values(result.metafile.outputs))for(const imported of output.imports)if(imported.external&&!imported.path.startsWith('node:'))throw Error('Source inspector bundle has a non-Node external import');
// Copy legal notices from the exact dependency packages contributing bytes to
// this build. This build-time IO is never included in the production runtime.
const notices=new Map();
for(const input of Object.keys(result.metafile.inputs)){
 const file=resolve(root,input);if(!file.replaceAll('\\','/').includes('/node_modules/'))continue;
 for(let directory=dirname(file);dirname(directory)!==directory;directory=dirname(directory)){
  let pkg;try{pkg=JSON.parse(await readFile(join(directory,'package.json'),'utf8'))}catch{continue}
  if(typeof pkg.name!=='string'||typeof pkg.version!=='string')continue;
  const key=pkg.name+'@'+pkg.version;if(notices.has(key))break;let license='';
  for(const name of ['LICENSE','LICENSE.md','LICENSE.txt','LICENSE-MIT','license'])try{license=await readFile(join(directory,name),'utf8');break}catch{}
  if(!license)throw Error('Bundled dependency has no copied license: '+key);
  notices.set(key,`${key}\nDeclared license: ${String(pkg.license??'see notice')}\n\n${license.trim()}\n`);break;
 }
}
await writeFile(join(root,'dist','THIRD_PARTY_NOTICES.txt'),[...notices].sort(([a],[b])=>a.localeCompare(b)).map(([,text])=>text).join('\n--------------------\n\n'),'utf8');
