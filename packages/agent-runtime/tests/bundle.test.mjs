import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {createHash} from 'node:crypto';
import {spawnSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';
test('the built native entry is a closed ESM bundle with a matching non-secret manifest',async()=>{
 const directory=new URL('../dist/',import.meta.url),manifest=JSON.parse(await readFile(new URL('suma-agent.manifest.json',directory),'utf8')),bytes=await readFile(new URL('suma-agent.bundle.mjs',directory));
 assert.equal(manifest.schema_version,1);assert.equal(manifest.entry,'suma-agent.bundle.mjs');assert.equal(manifest.entry_sha256,createHash('sha256').update(bytes).digest('hex'));assert.equal(manifest.dependencies['@modelcontextprotocol/client'],'2.4.0');assert.ok(manifest.external_imports.every(name=>name.startsWith('node:')));assert.ok(!JSON.stringify(manifest).match(/[A-Z]:[\\/]|token|secret/));
 const help=spawnSync(process.execPath,[fileURLToPath(new URL('suma-agent.bundle.mjs',directory)),'--help'],{encoding:'utf8',env:{...process.env,SUMA_MODEL:undefined,SUMA_MODEL_BASE_URL:undefined,NODE_PATH:''},windowsHide:true});assert.equal(help.status,0,help.stderr);assert.match(help.stdout,/JSONL/);assert.ok(!help.stdout.includes('started'));
});
