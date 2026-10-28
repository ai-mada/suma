import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFile,mkdir,mkdtemp,writeFile,rm} from 'node:fs/promises';
import {resolve,join,dirname,sep} from 'node:path';
import {createHash} from 'node:crypto';
import {pathToFileURL} from 'node:url';
import {verifyAssets,type RuntimeManifest} from '../src/config.js';
import type {RuntimeConfig} from '../src/contract.js';
const dist=resolve('dist');
const base={tool_entry:join(dist,'suma-workspace.tool.mjs'),plugin_package_root:join(dist,'opencode-plugin')} as RuntimeConfig;
test('packaged assets include real importable pinned SDK/plugin packages and their actual dependency lock',async()=>{
 const manifest=await verifyAssets(base);assert.equal(manifest.opencode.sdk_version,'1.19.4');assert.equal(manifest.dependencies['@modelcontextprotocol/client'],'2.4.0');assert.ok(manifest.assets.length>100);
 const plugin=await import(pathToFileURL(join(base.plugin_package_root,'node_modules/@opencode-ai/plugin/dist/tool.js')).href);const sdk=await import(pathToFileURL(join(base.plugin_package_root,'node_modules/@opencode-ai/sdk/dist/v2/client.js')).href);assert.equal(typeof plugin.tool,'function');assert.equal(typeof sdk.createOpencodeClient,'function');
 const lock=JSON.parse(await readFile(join(base.plugin_package_root,'package-lock.json'),'utf8'));assert.equal(lock.packages['node_modules/effect'].version,'4.0.0-beta.83');assert.ok(manifest.assets.some(item=>item.path==='opencode-plugin/node_modules/effect/dist/Effect.js'));
});
test('a fake node_modules marker and self-described version cannot satisfy packaged dependency acceptance',async()=>{
 const cache=resolve('.cache/tests');await mkdir(cache,{recursive:true});const root=await mkdtemp(join(cache,'fake-assets-'));try{
  const originals=JSON.parse(await readFile(join(dist,'suma-opencode.manifest.json'),'utf8')) as RuntimeManifest;const manifest={...originals,assets:[]} as RuntimeManifest;
  for(const [name,content] of Object.entries({'suma-opencode.bundle.mjs':'fixture','suma-workspace.tool.mjs':'fixture','opencode-plugin/package.json':'{"type":"module","dependencies":{"@opencode-ai/plugin":"1.19.4"}}','opencode-plugin/package-lock.json':'{"lockfileVersion":3,"packages":{"":{"dependencies":{"@opencode-ai/plugin":"1.19.4"}},"node_modules/@opencode-ai/plugin":{"version":"1.19.4"}}}','opencode-plugin/node_modules/@opencode-ai/plugin/package.json':'{"name":"@opencode-ai/plugin","version":"1.19.4"}'})){
   await mkdir(dirname(join(root,name)),{recursive:true});await writeFile(join(root,name),content);manifest.assets.push({path:name,sha256:createHash('sha256').update(content).digest('hex'),size_bytes:Buffer.byteLength(content)});
  }
  manifest.entry_sha256=manifest.assets[0].sha256;await writeFile(join(root,'suma-opencode.manifest.json'),JSON.stringify(manifest));await assert.rejects(verifyAssets({...base,tool_entry:join(root,'suma-workspace.tool.mjs'),plugin_package_root:join(root,'opencode-plugin')}));
 }finally{if(dirname(resolve(root))!==cache||!root.startsWith(cache+sep))throw Error('unsafe test cleanup');await rm(root,{recursive:true,force:true});}
});
