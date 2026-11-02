'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto');
const claude=require('../src/local-claude-sdk-assets.cjs'),opencode=require('../src/local-opencode-assets.cjs');
const sha=value=>crypto.createHash('sha256').update(value).digest('hex');
function fixture(root,kind){
 const runtime=path.join(root,kind+'-runtime'),host=kind==='claude'?path.join(runtime,'host'):runtime;
 const files=kind==='claude'?{'suma-claude.bundle.mjs':'// owned test payload','licenses/SDK.txt':'Owned metadata fixture, not a vendor license','third-party-licenses.json':JSON.stringify([{name:'@anthropic-ai/claude-agent-sdk',version:'0.3.314',license:'owned fixture',notices:['licenses/SDK.txt']}])}:{'suma-opencode.bundle.mjs':'// owned test payload','suma-workspace.tool.mjs':'// owned tool payload','opencode-plugin/package.json':'{}','opencode-plugin/package-lock.json':'{}','opencode-plugin/node_modules/@opencode-ai/plugin/package.json':'{}'};
 for(const [name,body]of Object.entries(files)){const file=path.join(host,name);fs.mkdirSync(path.dirname(file),{recursive:true});fs.writeFileSync(file,body);}
 const assets=Object.entries(files).map(([name,body])=>({path:name,sha256:sha(body),size_bytes:Buffer.byteLength(body)})),entry=Object.keys(files)[0],manifest=path.join(host,kind==='claude'?'suma-claude-runtime.json':'suma-opencode.manifest.json');
 const value=kind==='claude'?{schema_version:1,protocol:'claude-sdk-v1',sdk_version:'0.3.314',cli_version:'2.1.314',entry,assets}:{schema_version:1,entry,entry_sha256:sha(files[entry]),external_imports:['node:fs'],dependencies:{'@opencode-ai/sdk':'1.19.4','@opencode-ai/plugin':'1.19.4','@modelcontextprotocol/client':'2.4.0'},assets,opencode:{sdk_version:'1.19.4',server_version:'1.19.4',tool_entry:'suma-workspace.tool.mjs',plugin_package_root:'opencode-plugin'}};
 fs.writeFileSync(manifest,JSON.stringify(value));return {runtime,host,entry:path.join(host,entry),manifest,files};
}
test('203 SDK resource boundaries distinguish missing files, invalid bytes and unavailable trusted Node without exposing raw errors',()=>{
 const root=path.resolve(__dirname,'../../../.cache/tool-preparation-203/desktop/tmp');fs.mkdirSync(root,{recursive:true});const directory=fs.mkdtempSync(path.join(root,'resources-'));
 for(const kind of ['claude','opencode']){
  const provider=kind==='claude'?{validate:claude.validateClaudeSDKAssets,resolve:claude.resolveClaudeSDKAssets}:{validate:opencode.validateOpenCodeAssets,resolve:opencode.resolveOpenCodeAssets};
  const reason=expected=>error=>error.reason===expected;
  assert.throws(()=>provider.validate(path.join(directory,'absent-'+kind)),reason('sdk_assets_missing'));
  const target=path.join(directory,kind);fs.mkdirSync(target);const f=fixture(target,kind);assert.ok(provider.validate(f.host));
  const entry=fs.readFileSync(f.entry);fs.renameSync(f.entry,path.join(target,'preserved-entry'));assert.throws(()=>provider.validate(f.host),reason('sdk_assets_missing'));fs.renameSync(path.join(target,'preserved-entry'),f.entry);
  fs.appendFileSync(f.entry,'changed');assert.throws(()=>provider.validate(f.host),reason('sdk_assets_invalid'));fs.writeFileSync(f.entry,entry);
  const manifest=fs.readFileSync(f.manifest);fs.writeFileSync(f.manifest,'{ malformed secret/path data');assert.throws(()=>provider.validate(f.host),reason('sdk_assets_invalid'));fs.writeFileSync(f.manifest,manifest);
  const nodeName=process.platform==='win32'?'node.exe':'node',metadata={kind:kind==='claude'?'suma-claude-sdk-desktop-runtime':'suma-opencode-desktop-runtime',target:process.platform+'-'+process.arch,sdk_version:kind==='claude'?'0.3.314':'1.19.4',cli_version:'2.1.314',node:{file:nodeName,sha256:'a'.repeat(64)},runtime_manifest_sha256:sha(manifest),node_license_sha256:sha('Owned Node license fixture')};
  fs.writeFileSync(path.join(f.runtime,'desktop-runtime.json'),JSON.stringify(metadata));fs.writeFileSync(path.join(f.runtime,'NODE_LICENSE.txt'),'Owned Node license fixture');
  assert.throws(()=>provider.resolve({binary:path.join(target,'control'),environment:{PATH:''}}),reason('node_runtime_unavailable'));
  fs.writeFileSync(path.join(f.runtime,nodeName),'not a native Node executable');metadata.node.sha256=sha('not a native Node executable');fs.writeFileSync(path.join(f.runtime,'desktop-runtime.json'),JSON.stringify(metadata));assert.throws(()=>provider.resolve({binary:path.join(target,'control'),environment:{PATH:''}}),reason('node_runtime_unavailable'));
 }
});
