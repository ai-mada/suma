import {build,version as esbuildVersion} from 'esbuild';
import {builtinModules} from 'node:module';
import {createHash} from 'node:crypto';
import {readFile,writeFile,mkdir,readdir,lstat,realpath,copyFile,rm} from 'node:fs/promises';
import {dirname,resolve,join,isAbsolute,relative,sep} from 'node:path';
import {fileURLToPath} from 'node:url';

const root=fileURLToPath(new URL('../',import.meta.url)),dist=join(root,'dist'),payload=join(dist,'opencode-plugin');
const builtins=new Set(builtinModules.map(name=>name.replace(/^node:/,''))),dependencies={esbuild:esbuildVersion,'@suma/agent-runtime':'1.0.0'};
const dependencyVersions=new Map(Object.entries(dependencies).map(([name,version])=>[name,new Set([version])]));
function recordDependency(name,version){const versions=dependencyVersions.get(name)||new Set();versions.add(version);dependencyVersions.set(name,versions);}
const sha=bytes=>createHash('sha256').update(bytes).digest('hex');
await mkdir(dist,{recursive:true});
async function files(dir){const out=[];for(const name of await readdir(dir)){const path=join(dir,name),stat=await lstat(path);if(stat.isSymbolicLink())throw Error('Runtime asset contains a link');if(stat.isDirectory())out.push(...await files(path));else if(stat.isFile())out.push(path);else throw Error('Runtime asset is not a regular file');}return out.sort();}
// Only a previous, unchanged generated dependency payload is disposable. Keep
// its receipt outside the payload and refuse unique or modified files.
if(await lstat(payload).then(()=>true,error=>error.code==='ENOENT'?false:Promise.reject(error))){
 const previous=await readFile(join(dist,'suma-opencode.manifest.json'),'utf8'),manifest=JSON.parse(previous);const known=new Map(manifest.assets.map(item=>[item.path,item]));
 for(const file of await files(payload)){const name=relative(dist,file).split(sep).join('/'),bytes=await readFile(file),item=known.get(name);if(!item||item.sha256!==sha(bytes)||item.size_bytes!==bytes.length)throw Error('Refusing to replace unrecognized runtime payload files');}
 if(resolve(payload)!==join(resolve(dist),'opencode-plugin'))throw Error('Runtime payload cleanup escaped output');await mkdir(join(root,'.cache','build'),{recursive:true});await writeFile(join(root,'.cache','build','previous-manifest.json'),previous);await rm(payload,{recursive:true});
}
const external=new Set(['node:module']);
for(const [entry,outfile] of [['src/cli.ts','suma-opencode.bundle.mjs'],['src/suma_workspace.ts','suma-workspace.tool.mjs']]){
 const result=await build({absWorkingDir:root,entryPoints:[entry],outfile:join(dist,outfile),bundle:true,format:'esm',platform:'node',target:'node22',metafile:true,sourcemap:false,legalComments:'inline',treeShaking:true,banner:{js:"import { createRequire as __sumaCreateRequire } from 'node:module'; const require = __sumaCreateRequire(import.meta.url);"},plugins:[{name:'node-only-external',setup(context){context.onResolve({filter:/.*/},args=>{const name=args.path.replace(/^node:/,'');if(builtins.has(name))return {path:'node:'+name,external:true};});}}]});
 for(const output of Object.values(result.metafile.outputs))for(const imported of output.imports){if(!imported.external||!imported.path.startsWith('node:')||!builtins.has(imported.path.slice(5)))throw Error('OpenCode bundle has a non-Node external import');external.add(imported.path);}
 for(const input of Object.keys(result.metafile.inputs)){
  const path=isAbsolute(input)?input:resolve(root,input);if(!path.replaceAll('\\','/').includes('/node_modules/'))continue;
  for(let directory=dirname(path);dirname(directory)!==directory;directory=dirname(directory)){try{const pkg=JSON.parse(await readFile(join(directory,'package.json'),'utf8'));if(typeof pkg.name==='string'&&typeof pkg.version==='string'){recordDependency(pkg.name,pkg.version);break;}}catch(error){if(error.code!=='ENOENT')throw error;}}
 }
}
async function packageRoot(name,from){for(let dir=from;dirname(dir)!==dir;dir=dirname(dir)){const candidate=join(dir,'node_modules',...name.split('/'));try{const path=await realpath(candidate),pkg=JSON.parse(await readFile(join(path,'package.json'),'utf8'));if(pkg.name===name)return path;}catch(error){if(!['ENOENT','ENOTDIR'].includes(error.code))throw error;}}throw Error('Installed pinned dependency missing: '+name);}
const packages=new Map(),queue=[{name:'@opencode-ai/plugin',from:root}];
while(queue.length){const {name,from}=queue.shift(),path=await packageRoot(name,from),pkg=JSON.parse(await readFile(join(path,'package.json'),'utf8'));const existing=packages.get(name);if(existing){if(existing.pkg.version!==pkg.version)throw Error('Conflicting packaged dependency versions: '+name);continue;}packages.set(name,{path,pkg});recordDependency(name,pkg.version);for(const child of Object.keys(pkg.dependencies||{}))queue.push({name:child,from:path});}
const rootPackage={name:'@suma/opencode-config-assets',version:'1.0.0',private:true,type:'module',dependencies:{'@opencode-ai/plugin':'1.19.4'}},lock={name:rootPackage.name,version:rootPackage.version,lockfileVersion:3,requires:true,packages:{'':{name:rootPackage.name,version:rootPackage.version,dependencies:rootPackage.dependencies}}};
await mkdir(payload,{recursive:true});
for(const [name,{path,pkg}] of [...packages].sort(([a],[b])=>a.localeCompare(b))){
 const target=join(payload,'node_modules',...name.split('/'));await mkdir(target,{recursive:true});
 async function copy(source,destination){for(const name of await readdir(source)){if(name==='node_modules'||name==='.git')continue;const input=join(source,name),output=join(destination,name),stat=await lstat(input);if(stat.isSymbolicLink())throw Error('Installed package contains a link');if(stat.isDirectory()){await mkdir(output,{recursive:true});await copy(input,output);}else if(stat.isFile())await copyFile(input,output);else throw Error('Installed package contains a non-regular file');}}
 await copy(path,target);
 lock.packages['node_modules/'+name]={version:pkg.version,...(pkg.license?{license:pkg.license}:{}),...(pkg.dependencies?{dependencies:pkg.dependencies}:{}),...(pkg.optionalDependencies?{optionalDependencies:pkg.optionalDependencies}:{}),...(pkg.peerDependencies?{peerDependencies:pkg.peerDependencies}:{}),...(pkg.peerDependenciesMeta?{peerDependenciesMeta:pkg.peerDependenciesMeta}:{})};
}
await writeFile(join(payload,'package.json'),JSON.stringify(rootPackage,null,2)+'\n');await writeFile(join(payload,'package-lock.json'),JSON.stringify(lock,null,2)+'\n');
for(const [name,versions] of dependencyVersions){if(versions.size===1)dependencies[name]=[...versions][0];else {delete dependencies[name];for(const version of [...versions].sort())dependencies[name+'@'+version]=version;}}
for(const [name,want] of [['@opencode-ai/sdk','1.19.4'],['@opencode-ai/plugin','1.19.4'],['@modelcontextprotocol/client','2.4.0']])if(dependencies[name]!==want)throw Error('Runtime did not include pinned '+name);
const assets=[];for(const path of [join(dist,'suma-opencode.bundle.mjs'),join(dist,'suma-workspace.tool.mjs'),...await files(payload)]){const bytes=await readFile(path);assets.push({path:relative(dist,path).split(sep).join('/'),sha256:sha(bytes),size_bytes:bytes.length});}
if(assets.length>32768||assets.reduce((sum,item)=>sum+item.size_bytes,0)>256*1024*1024)throw Error('Packaged runtime assets exceed verification bounds');
const manifest={schema_version:1,entry:'suma-opencode.bundle.mjs',entry_sha256:assets.find(item=>item.path==='suma-opencode.bundle.mjs').sha256,external_imports:[...external].sort(),dependencies:Object.fromEntries(Object.entries(dependencies).sort(([a],[b])=>a.localeCompare(b))),assets:assets.sort((a,b)=>a.path.localeCompare(b.path)),opencode:{sdk_version:'1.19.4',server_version:'1.19.4',tool_entry:'suma-workspace.tool.mjs',plugin_package_root:'opencode-plugin'}};
await writeFile(join(dist,'suma-opencode.manifest.json'),JSON.stringify(manifest,null,2)+'\n');process.stdout.write('OpenCode SDK runtime, trusted tools and verified dependency payload written.\n');
