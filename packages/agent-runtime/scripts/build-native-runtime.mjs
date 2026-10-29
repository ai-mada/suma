import {build,version as esbuildVersion} from 'esbuild';
import {builtinModules} from 'node:module';
import {createHash} from 'node:crypto';
import {readFile,writeFile,mkdir} from 'node:fs/promises';
import {dirname,resolve,join,isAbsolute} from 'node:path';
import {fileURLToPath} from 'node:url';

const root=fileURLToPath(new URL('../',import.meta.url));
// Verification and packaging can isolate outputs without replacing a running
// Runner's installed bundle. The override is local to the build process.
const configuredOutput=process.env.SUMA_NATIVE_RUNTIME_OUT_DIR;
if(configuredOutput&&!isAbsolute(configuredOutput))throw Error('Native runtime output directory must be absolute');
const outputDirectory=configuredOutput||join(root,'dist'),output=join(outputDirectory,'suma-agent.bundle.mjs');
const builtins=new Set(builtinModules.map(name=>name.replace(/^node:/,'')));
await mkdir(dirname(output),{recursive:true});
const result=await build({absWorkingDir:root,entryPoints:['src/cli.ts'],outfile:output,bundle:true,format:'esm',platform:'node',target:'node22',metafile:true,sourcemap:false,legalComments:'inline',treeShaking:true,
 banner:{js:"import { createRequire as __sumaCreateRequire } from 'node:module'; const require = __sumaCreateRequire(import.meta.url);"},
 plugins:[{name:'fixed-node-builtins',setup(context){context.onResolve({filter:/.*/},args=>{const name=args.path.replace(/^node:/,'');if(builtins.has(name))return {path:'node:'+name,external:true};});}}]});
const external=new Set(['node:module']);
for(const item of Object.values(result.metafile.outputs))for(const imported of item.imports){if(!imported.external||!imported.path.startsWith('node:')||!builtins.has(imported.path.slice(5)))throw Error('Native bundle has a non-Node external import');external.add(imported.path);}
// Record the exact published packages actually contributing source bytes,
// rather than ranges from a package manifest or absolute build-machine paths.
const dependencies={esbuild:esbuildVersion};
for(const input of Object.keys(result.metafile.inputs)){
 const path=isAbsolute(input)?input:resolve(root,input);if(!path.replaceAll('\\','/').includes('/node_modules/'))continue;
 for(let directory=dirname(path);dirname(directory)!==directory;directory=dirname(directory)){
  try{const pkg=JSON.parse(await readFile(join(directory,'package.json'),'utf8'));if(typeof pkg.name==='string'&&typeof pkg.version==='string'){if(dependencies[pkg.name]&&dependencies[pkg.name]!==pkg.version)throw Error('Conflicting native bundle dependency versions');dependencies[pkg.name]=pkg.version;break;}}catch(error){if(error?.code!=='ENOENT')throw error;}
 }
}
if(dependencies['@modelcontextprotocol/client']!=='2.4.0')throw Error('Native bundle did not close over the fixed official MCP client');
const bytes=await readFile(output),manifest={schema_version:1,entry:'suma-agent.bundle.mjs',entry_sha256:createHash('sha256').update(bytes).digest('hex'),external_imports:[...external].sort(),dependencies:Object.fromEntries(Object.entries(dependencies).sort(([a],[b])=>a.localeCompare(b)))};
await writeFile(join(outputDirectory,'suma-agent.manifest.json'),JSON.stringify(manifest,null,2)+'\n','utf8');
process.stdout.write('Native runtime bundle and schema-version-1 manifest written.\n');
