import {build} from 'esbuild';
import {builtinModules} from 'node:module';
import {createHash} from 'node:crypto';
import {readFile,writeFile,mkdir,realpath,readdir,lstat} from 'node:fs/promises';
import {dirname,resolve,join,isAbsolute,relative,sep} from 'node:path';
import {fileURLToPath} from 'node:url';

const root=fileURLToPath(new URL('../',import.meta.url)),configured=process.env.SUMA_CLAUDE_RUNTIME_OUT_DIR;
if(configured&&!isAbsolute(configured))throw Error('Claude runtime output must be absolute');
const out=configured||join(root,'dist','runtime'),entry='suma-claude.bundle.mjs',manifestName='suma-claude-runtime.json';
const sha=bytes=>createHash('sha256').update(bytes).digest('hex'),builtins=new Set(builtinModules.map(name=>name.replace(/^node:/,'')));
await mkdir(out,{recursive:true});
async function files(directory){const result=[];for(const name of await readdir(directory)){const file=join(directory,name),stat=await lstat(file);if(stat.isSymbolicLink())throw Error('Runtime output contains a link');if(stat.isDirectory())result.push(...await files(file));else if(stat.isFile())result.push(file);else throw Error('Runtime output contains a non-file');}return result;}
const existing=await files(out);
if(existing.length){const old=JSON.parse(await readFile(join(out,manifestName),'utf8')),known=new Map(old.assets.map(item=>[item.path,item]));for(const file of existing){const name=relative(out,file).split(sep).join('/');if(name===manifestName)continue;const item=known.get(name),bytes=await readFile(file);if(!item||item.sha256!==sha(bytes)||item.size_bytes!==bytes.length)throw Error('Refusing to overwrite an unrecognized runtime asset');}}
const result=await build({absWorkingDir:root,entryPoints:['src/cli.ts'],outfile:join(out,entry),bundle:true,format:'esm',platform:'node',target:'node22',metafile:true,sourcemap:false,legalComments:'inline',treeShaking:true,banner:{js:"import { createRequire as __sumaCreateRequire } from 'node:module'; const require = __sumaCreateRequire(import.meta.url);"},plugins:[{name:'fixed-node-builtins',setup(context){context.onResolve({filter:/.*/},args=>{const name=args.path.replace(/^node:/,'');if(builtins.has(name))return{path:'node:'+name,external:true};});}}]});
for(const output of Object.values(result.metafile.outputs))for(const imported of output.imports)if(imported.external&&(!imported.path.startsWith('node:')||!builtins.has(imported.path.slice(5))))throw Error('Claude bundle contains a non-Node external import');
// Retain actual notices for every published dependency contributing bytes.
// The SDK is licensed under Anthropic terms; never relabel it as MIT/Apache.
const packages=new Map();for(const input of Object.keys(result.metafile.inputs)){
 const file=isAbsolute(input)?input:resolve(root,input);if(!file.replaceAll('\\','/').includes('/node_modules/'))continue;
 for(let dir=dirname(await realpath(file));dirname(dir)!==dir;dir=dirname(dir)){try{const bytes=await readFile(join(dir,'package.json')),pkg=JSON.parse(bytes);if(typeof pkg.name==='string'&&typeof pkg.version==='string'){const old=packages.get(pkg.name);if(old&&old.version!==pkg.version)throw Error('Conflicting dependency versions');packages.set(pkg.name,{...pkg,directory:dir});break;}}catch(error){if(error.code!=='ENOENT')throw error;}}
}
if(packages.get('@anthropic-ai/claude-agent-sdk')?.version!=='0.3.314')throw Error('Pinned Claude SDK missing from bundle');
const notices=[];await mkdir(join(out,'licenses'),{recursive:true});
for(const [name,pkg]of [...packages].sort(([a],[b])=>a.localeCompare(b))){const names=(await readdir(pkg.directory)).filter(file=>/^(?:license|licence|copying|notice)(?:\.|$)/i.test(file)).sort();const record={name,version:pkg.version,license:pkg.license||'UNSPECIFIED',notices:[]};for(const file of names){const stat=await lstat(join(pkg.directory,file));if(!stat.isFile()||stat.isSymbolicLink())continue;const target='licenses/'+name.replace(/[^a-zA-Z0-9.-]/g,'_')+'-'+pkg.version+'-'+file;await writeFile(join(out,target),await readFile(join(pkg.directory,file)));record.notices.push(target);}if(!record.notices.length)throw Error('Dependency license notice missing: '+name);notices.push(record);}
await writeFile(join(out,'third-party-licenses.json'),JSON.stringify(notices,null,2)+'\n');
const assets=[];for(const file of await files(out)){if(file===join(out,manifestName))continue;const bytes=await readFile(file);if(bytes.length>32*1024*1024)throw Error('Runtime asset too large');assets.push({path:relative(out,file).split(sep).join('/'),sha256:sha(bytes),size_bytes:bytes.length});}
if(assets.length>32768||assets.reduce((sum,item)=>sum+item.size_bytes,0)>256*1024*1024)throw Error('Runtime assets exceed bounds');
const manifest={schema_version:1,protocol:'claude-sdk-v1',sdk_version:'0.3.314',cli_version:'2.1.314',entry,assets:assets.sort((a,b)=>a.path.localeCompare(b.path))};
await writeFile(join(out,manifestName),JSON.stringify(manifest,null,2)+'\n');
process.stdout.write('Pinned Claude SDK host, complete assets and license notices written.\n');
