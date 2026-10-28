import {createHash} from 'node:crypto';
import {readFile,readdir,lstat,mkdir,writeFile,realpath} from 'node:fs/promises';
import {resolve,join,dirname,isAbsolute,relative,sep} from 'node:path';
import {spawn} from 'node:child_process';
import {RuntimeError,SDK_VERSION,TOOL_NAMES,closed,object,safeID,type RuntimeConfig,type ExecuteRequest} from './contract.js';

const CONFIG_FIELDS=['executable','executable_sha256','home','source','provider_id','model','base_url','api_key_env_ref','configuration_sha256','tool_entry','plugin_package_root'];
const hex=(value:unknown):value is string=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
export function parseConfig(value:unknown):RuntimeConfig{
 closed(value,CONFIG_FIELDS);
 for(const name of CONFIG_FIELDS.filter(name=>name!=='api_key_env_ref'))if(typeof value[name]!=='string'||!value[name])throw new RuntimeError('configuration_invalid');
 for(const name of ['executable','home','source','tool_entry','plugin_package_root'])if(!isAbsolute(String(value[name])))throw new RuntimeError('configuration_invalid');
 if(!hex(value.executable_sha256)||!hex(value.configuration_sha256)||typeof value.provider_id!=='string'||!/^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$/.test(value.provider_id)||typeof value.model!=='string'||!/^[A-Za-z0-9][A-Za-z0-9._:/@+\-]{0,255}$/.test(value.model))throw new RuntimeError('configuration_invalid');
 if(value.api_key_env_ref!==undefined&&(typeof value.api_key_env_ref!=='string'||!/^[_A-Z][_A-Z0-9]{0,127}$/.test(value.api_key_env_ref)))throw new RuntimeError('configuration_invalid');
 let url:URL;try{url=new URL(String(value.base_url));}catch{throw new RuntimeError('configuration_invalid');}
 if(!['https:','http:'].includes(url.protocol)||url.username||url.password||url.search||url.hash||url.protocol==='http:'&&!['127.0.0.1','localhost','[::1]'].includes(url.hostname))throw new RuntimeError('configuration_invalid');
 const config=value as unknown as RuntimeConfig;if(dirname(dirname(resolve(config.home)))===dirname(resolve(config.home)))throw new RuntimeError('configuration_invalid');
 const child=relative(resolve(config.source),resolve(config.home));if(child===''||!child.startsWith('..'+sep)&&child!=='..'&&!isAbsolute(child))throw new RuntimeError('configuration_invalid');
 return config;
}
export function managedConfigPaths(platform:NodeJS.Platform=process.platform,env:NodeJS.ProcessEnv=process.env):string[]{
 const base=platform==='win32'?join(env.ProgramData||'C:\\ProgramData','opencode'):platform==='darwin'?'/Library/Application Support/opencode':'/etc/opencode';
 const paths=[join(base,'opencode.json'),join(base,'opencode.jsonc')];
 if(platform==='darwin'){paths.push('/Library/Managed Preferences/ai.opencode.managed.plist');}
 return paths;
}
export async function rejectManagedConfig(){
 for(const file of managedConfigPaths()){try{await lstat(file);throw new RuntimeError('managed_configuration');}catch(error){if((error as NodeJS.ErrnoException).code!=='ENOENT')throw error;}}
 if(process.platform==='darwin'){
  // User-scoped MDM preferences are system authority and cannot be overridden.
  const root='/Library/Managed Preferences';for(const name of await readdir(root).catch(()=>[])){const file=join(root,name,'ai.opencode.managed.plist');try{await lstat(file);throw new RuntimeError('managed_configuration');}catch(error){if(!['ENOENT','ENOTDIR'].includes(String((error as NodeJS.ErrnoException).code)))throw error;}}
 }
}
export function generatedConfig(config:RuntimeConfig,environment:NodeJS.ProcessEnv=process.env):Record<string,unknown>{
 const permission={'*':'deny',...Object.fromEntries(TOOL_NAMES.map(name=>['suma_workspace_'+name,'allow']))};
 const apiKey=config.api_key_env_ref?environment[config.api_key_env_ref]:undefined;if(config.api_key_env_ref&&(!apiKey||apiKey.length>16384||/[\r\n]/.test(apiKey)))throw new RuntimeError('credential_missing');
 return {username:'suma',model:config.provider_id+'/'+config.model,small_model:config.provider_id+'/'+config.model,enabled_providers:[config.provider_id],provider:{[config.provider_id]:{npm:'@ai-sdk/openai-compatible',name:'Suma approved provider',options:{baseURL:config.base_url,...(apiKey?{apiKey}:{})},models:{[config.model]:{name:config.model}}}},default_agent:'suma',agent:{suma:{mode:'primary',model:config.provider_id+'/'+config.model,permission},...Object.fromEntries(['build','plan','general','explore','title','summary','compaction'].map(name=>[name,{disable:true}]))},permission,plugin:[],mcp:{},instructions:[],skills:{paths:[],urls:[]},command:{},autoupdate:false,share:'disabled',snapshot:false,lsp:false,formatter:false,compaction:{auto:false,prune:false}};
}
export function childEnvironment(config:RuntimeConfig,generated:unknown,request?:ExecuteRequest,environment:NodeJS.ProcessEnv=process.env):NodeJS.ProcessEnv{
 const xdg=join(config.home,'config'),tmp=join(config.home,'tmp');
 const content=structuredClone(generated);let approvedKey:string|undefined;
 if(config.api_key_env_ref&&object(content)&&object(content.provider)){const provider=content.provider[config.provider_id];if(object(provider)&&object(provider.options)&&typeof provider.options.apiKey==='string'){approvedKey=provider.options.apiKey;provider.options.apiKey='{env:SUMA_OPENCODE_APPROVED_PROVIDER_KEY}';}}
 const result:NodeJS.ProcessEnv={HOME:config.home,USERPROFILE:config.home,XDG_CONFIG_HOME:xdg,XDG_DATA_HOME:join(config.home,'data'),XDG_CACHE_HOME:join(config.home,'cache'),XDG_STATE_HOME:join(config.home,'state'),TEMP:tmp,TMP:tmp,TMPDIR:tmp,OPENCODE_CONFIG_DIR:join(xdg,'opencode'),OPENCODE_CONFIG_CONTENT:JSON.stringify(content),OPENCODE_CLIENT:'suma',OPENCODE_DISABLE_PROJECT_CONFIG:'true',OPENCODE_PURE:'true',OPENCODE_DISABLE_DEFAULT_PLUGINS:'true',OPENCODE_DISABLE_EXTERNAL_SKILLS:'true',OPENCODE_DISABLE_CLAUDE_CODE:'true',OPENCODE_DISABLE_AUTOUPDATE:'true',OPENCODE_DISABLE_MODELS_FETCH:'true',OPENCODE_DISABLE_LSP_DOWNLOAD:'true',OPENCODE_DISABLE_AUTOCOMPACT:'true',OPENCODE_DISABLE_PRUNE:'true',OPENCODE_EXPERIMENTAL_DISABLE_FILEWATCHER:'true',npm_config_offline:'true',npm_config_ignore_scripts:'true',npm_config_audit:'false',npm_config_fund:'false',npm_config_userconfig:join(config.home,'empty.npmrc'),npm_config_globalconfig:join(config.home,'empty-global.npmrc'),npm_config_cache:join(config.home,'npm-cache')};
 if(approvedKey)result.SUMA_OPENCODE_APPROVED_PROVIDER_KEY=approvedKey;
 // Only operating-system process mechanics are inherited. Provider, proxy,
 // plugin, npm authentication and Node startup variables never cross this edge.
 for(const name of ['SystemRoot','SYSTEMROOT','WINDIR','COMSPEC','PATHEXT','ProgramData'])if(environment[name])result[name]=environment[name];
 const osBin=environment.SystemRoot||environment.SYSTEMROOT;result.PATH=[dirname(config.executable),...(osBin?[join(osBin,'System32'),osBin]:[])].join(process.platform==='win32'?';':':');
 if(request){result.SUMA_OPENCODE_WORKSPACE_BRIDGE=JSON.stringify(request.workspace_bridge);result.SUMA_OPENCODE_REQUEST_ID=request.request_id;result.SUMA_OPENCODE_TOOL_FAILURE=join(config.home,'tool-failure.json');}
 return result;
}
export interface Asset{path:string;sha256:string;size_bytes:number}
export interface RuntimeManifest{schema_version:1;entry:string;entry_sha256:string;external_imports:string[];dependencies:Record<string,string>;assets:Asset[];opencode:{sdk_version:string;server_version:string;tool_entry:string;plugin_package_root:string}}
const contained=(root:string,path:string)=>{const rel=relative(root,path);return rel!==''&&!rel.startsWith('..'+sep)&&rel!=='..'&&!isAbsolute(rel);};
export async function regularFiles(root:string):Promise<string[]>{
 const result:string[]=[];async function walk(dir:string){for(const name of await readdir(dir)){const path=join(dir,name),stat=await lstat(path);if(stat.isSymbolicLink())throw new RuntimeError('asset_invalid');if(stat.isDirectory())await walk(path);else if(stat.isFile())result.push(path);else throw new RuntimeError('asset_invalid');if(result.length>32768)throw new RuntimeError('asset_limit');}}await walk(root);return result.sort();
}
export async function verifyAssets(config:RuntimeConfig):Promise<RuntimeManifest>{
 const root=dirname(config.tool_entry);const manifestFile=join(root,'suma-opencode.manifest.json');const stat=await lstat(manifestFile);if(!stat.isFile()||stat.isSymbolicLink()||stat.size>8*1024*1024)throw new RuntimeError('asset_invalid');
 const manifest:unknown=JSON.parse(await readFile(manifestFile,'utf8'));closed(manifest,['schema_version','entry','entry_sha256','external_imports','dependencies','assets','opencode']);
 if(manifest.schema_version!==1||manifest.entry!=='suma-opencode.bundle.mjs'||!hex(manifest.entry_sha256)||!Array.isArray(manifest.external_imports)||manifest.external_imports.some(x=>typeof x!=='string'||!x.startsWith('node:'))||!object(manifest.dependencies)||manifest.dependencies['@opencode-ai/sdk']!==SDK_VERSION||manifest.dependencies['@opencode-ai/plugin']!==SDK_VERSION||manifest.dependencies['@modelcontextprotocol/client']!=='2.4.0'||!Array.isArray(manifest.assets)||manifest.assets.length<4||manifest.assets.length>32768||!object(manifest.opencode))throw new RuntimeError('asset_invalid');
 closed(manifest.opencode,['sdk_version','server_version','tool_entry','plugin_package_root']);
 if(manifest.opencode.sdk_version!==SDK_VERSION||manifest.opencode.server_version!==SDK_VERSION||manifest.opencode.tool_entry!=='suma-workspace.tool.mjs'||manifest.opencode.plugin_package_root!=='opencode-plugin'||resolve(config.tool_entry)!==join(root,'suma-workspace.tool.mjs')||resolve(config.plugin_package_root)!==join(root,'opencode-plugin'))throw new RuntimeError('asset_invalid');
 const seen=new Set<string>();let total=0;
 for(const item of manifest.assets){closed(item,['path','sha256','size_bytes']);if(typeof item.path!=='string'||!item.path||/[\\:*?"<>|\p{Cc}]/u.test(item.path)||item.path.startsWith('/')||item.path.split('/').some(part=>!part||part==='.'||part==='..')||!hex(item.sha256)||!Number.isSafeInteger(item.size_bytes)||Number(item.size_bytes)<0||Number(item.size_bytes)>16*1024*1024||seen.has(item.path))throw new RuntimeError('asset_invalid');
  const path=resolve(root,item.path);if(!contained(root,path)||await realpath(path)!==path)throw new RuntimeError('asset_invalid');const stat=await lstat(path);if(!stat.isFile()||stat.isSymbolicLink()||stat.size!==item.size_bytes)throw new RuntimeError('asset_invalid');const bytes=await readFile(path);if(createHash('sha256').update(bytes).digest('hex')!==item.sha256)throw new RuntimeError('asset_invalid');seen.add(item.path);total+=bytes.length;if(total>256*1024*1024)throw new RuntimeError('asset_limit');
 }
 for(const required of ['suma-opencode.bundle.mjs','suma-workspace.tool.mjs','opencode-plugin/package.json','opencode-plugin/package-lock.json','opencode-plugin/node_modules/@opencode-ai/plugin/package.json'])if(!seen.has(required))throw new RuntimeError('asset_invalid');
 if((manifest.assets as unknown as Asset[]).find(x=>x.path===manifest.entry)?.sha256!==manifest.entry_sha256)throw new RuntimeError('asset_invalid');
 for(const path of await regularFiles(config.plugin_package_root)){const rel=relative(root,path).split(sep).join('/');if(!seen.has(rel))throw new RuntimeError('asset_invalid');}
 const plugin=JSON.parse(await readFile(join(config.plugin_package_root,'node_modules/@opencode-ai/plugin/package.json'),'utf8'));const lock=JSON.parse(await readFile(join(config.plugin_package_root,'package-lock.json'),'utf8'));
 if(plugin.name!=='@opencode-ai/plugin'||plugin.version!==SDK_VERSION||plugin.exports?.['./tool']?.import!=='./dist/tool.js'||lock.lockfileVersion!==3||lock.packages?.['']?.dependencies?.['@opencode-ai/plugin']!==SDK_VERSION||lock.packages?.['node_modules/@opencode-ai/plugin']?.version!==SDK_VERSION)throw new RuntimeError('asset_invalid');
 const visited=new Set<string>(),pending=['@opencode-ai/plugin'];
 while(pending.length){const name=pending.pop()!;if(visited.has(name))continue;if(!/^(@[a-zA-Z0-9_.-]+\/)?[a-zA-Z0-9_.-]+$/.test(name))throw new RuntimeError('asset_invalid');visited.add(name);const prefix='node_modules/'+name,packageFile=join(config.plugin_package_root,prefix,'package.json');const pkg=JSON.parse(await readFile(packageFile,'utf8'));const locked=lock.packages?.[prefix];
  if(pkg.name!==name||!locked||locked.version!==pkg.version||!object(locked)||JSON.stringify(locked.dependencies||{})!==JSON.stringify(pkg.dependencies||{}))throw new RuntimeError('asset_invalid');
  const packageFiles=(manifest.assets as unknown as Asset[]).filter(item=>item.path.startsWith('opencode-plugin/'+prefix+'/'));
  const types=typeof pkg.types==='string'?pkg.types.replace(/^\.\//,''):undefined;
  if(!packageFiles.some(item=>/\.(cjs|mjs|js)$/.test(item.path))&&!packageFiles.some(item=>/\.d\.(ts|cts|mts)$/.test(item.path))&&(!types||!seen.has('opencode-plugin/'+prefix+'/'+types)))throw new RuntimeError('asset_invalid');
  for(const dependency of Object.keys(pkg.dependencies||{}))pending.push(dependency);
 }
 if(!visited.has('@opencode-ai/sdk')||!seen.has('opencode-plugin/node_modules/@opencode-ai/plugin/dist/tool.js')||!seen.has('opencode-plugin/node_modules/@opencode-ai/sdk/dist/v2/client.js'))throw new RuntimeError('asset_invalid');
 return manifest as unknown as RuntimeManifest;
}
export async function verifyExecutable(config:RuntimeConfig){const stat=await lstat(config.executable);if(!stat.isFile()||stat.isSymbolicLink()||createHash('sha256').update(await readFile(config.executable)).digest('hex')!==config.executable_sha256)throw new RuntimeError('executable_changed');}
export async function prepareHome(config:RuntimeConfig,request:ExecuteRequest){
 await rejectManagedConfig();await verifyExecutable(config);const manifest=await verifyAssets(config);
 const source=await lstat(config.source);if(!source.isDirectory()||source.isSymbolicLink()||await realpath(config.source)!==resolve(config.source))throw new RuntimeError('workspace_source');
 try{const stat=await lstat(config.home);if(!stat.isDirectory()||stat.isSymbolicLink()||(await readdir(config.home)).length)throw new RuntimeError('home_not_empty');}catch(error){if((error as NodeJS.ErrnoException).code!=='ENOENT')throw error;await mkdir(config.home,{recursive:true});}
 const generated=generatedConfig(config);const env=childEnvironment(config,generated,request);
 for(const path of [env.TEMP!,env.XDG_DATA_HOME!,env.XDG_CACHE_HOME!,env.XDG_STATE_HOME!])await mkdir(path,{recursive:true});
 const target=env.OPENCODE_CONFIG_DIR!;await mkdir(target,{recursive:true});
 const assets=new Map(manifest.assets.map(item=>[item.path,item]));
 const copyVerified=async(file:string,out:string,name:string)=>{const item=assets.get(name),bytes=await readFile(file);if(!item||bytes.length!==item.size_bytes||createHash('sha256').update(bytes).digest('hex')!==item.sha256)throw new RuntimeError('asset_changed');await mkdir(dirname(out),{recursive:true});await writeFile(out,bytes,{flag:'wx'});};
 for(const file of await regularFiles(config.plugin_package_root)){const name=relative(config.plugin_package_root,file);await copyVerified(file,join(target,name),'opencode-plugin/'+name.split(sep).join('/'));}
 await mkdir(join(target,'tools'));await copyVerified(config.tool_entry,join(target,'tools','suma_workspace.js'),'suma-workspace.tool.mjs');
 await writeFile(join(config.home,'empty.npmrc'),'','utf8');await writeFile(join(config.home,'empty-global.npmrc'),'','utf8');
 return {generated,env};
}
export async function probeExecutable(config:RuntimeConfig,env:NodeJS.ProcessEnv):Promise<void>{
 await new Promise<void>((resolve,reject)=>{const child=spawn(config.executable,['--version'],{env,cwd:config.home,windowsHide:true,stdio:['ignore','pipe','pipe']});let output='',size=0;let timedOut=false;const timer=setTimeout(()=>{timedOut=true;child.kill();},5000);child.stdout.on('data',chunk=>{size+=chunk.length;if(size>32768)child.kill();else output+=String(chunk);});child.stderr.on('data',chunk=>{size+=chunk.length;if(size>32768)child.kill();});child.on('error',()=>{clearTimeout(timer);reject(new RuntimeError('executable_version'));});child.on('close',code=>{clearTimeout(timer);if(code!==0||timedOut||size>32768||output.trim()!==SDK_VERSION)reject(new RuntimeError('executable_version'));else resolve();});});
}
