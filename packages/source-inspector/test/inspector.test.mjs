import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {inspectSourceInput} from '../src/index.mjs';

const sha=raw=>createHash('sha256').update(raw).digest('hex');
const canonical=value=>JSON.stringify(value,(_,v)=>v&&typeof v==='object'&&!Array.isArray(v)?Object.fromEntries(Object.keys(v).sort().map(key=>[key,v[key]])):v).replace(/[<>&\u2028\u2029]/g,c=>'\\u'+c.charCodeAt(0).toString(16).padStart(4,'0'));
function fixture(){
 const data=[['package.json',JSON.stringify({name:'fixture-root',version:'1.0.0',workspaces:['packages/*']}),'package-json'],['packages/a/package.json',JSON.stringify({name:'@fixture/a',version:'1.0.0',dependencies:{'@fixture/b':'workspace:^2.0.0','alias-b':'workspace:@fixture/b@*','relative-b':'workspace:../b','bad-b':'workspace:@fixture/b@^9.0.0','external-lib':'^3.2.0','secret-lib':'https://user:private-source-token@registry.invalid/pkg'},devDependencies:{'@fixture/b':'workspace:*'},peerDependencies:{'peer-lib':'^4.0.0'},optionalDependencies:{'@fixture/b':'workspace:~'}}),'package-json'],['packages/b/package.json',JSON.stringify({name:'@fixture/b',version:'2.1.0'}),'package-json'],['pnpm-workspace.yaml',"packages:\n  - 'packages/*'\n",'pnpm-workspace-yaml']];
 const manifests=data.map(([path,content,format])=>({path,blob_oid:'b'.repeat(40),sha256:sha(content),size_bytes:Buffer.byteLength(content),content_base64:Buffer.from(content).toString('base64'),format}));
 const source={protocol:'suma.repository-source-inspection.v1',inspection_request_sha256:'a'.repeat(64),snapshot_sha256:'',exact_commit:'c'.repeat(40),tree_oid:'d'.repeat(40),scopes:[{relative_root:'',tree_oid:'d'.repeat(40)}],manifests,coverage:{state:'complete',tree_entries:6,manifest_count:manifests.length,content_bytes:manifests.reduce((sum,file)=>sum+file.size_bytes,0),skipped_entries:0,reasons:[]},limits:{max_manifests:200,max_tree_entries:10000,max_file_bytes:256<<10,max_content_bytes:8<<20,max_report_bytes:1<<20}};source.snapshot_sha256=sha(canonical(source));return {schema_version:1,sources:[source]};
}

test('fixed npm/pnpm source discovers bounded workspace facts, precise ranges and redacts unsupported locators',()=>{
 const input=fixture(),report=inspectSourceInput(input);
 assert.equal(report.schema_version,1);assert.equal(report.parser_id,'suma-node-workspace');assert.equal(report.packages.length,3);assert.ok(report.packages.every(pkg=>pkg.workspace_member));assert.equal(report.files.length,4);assert.equal(report.coverage.external_lock_resolution,false);assert.equal(report.coverage.runtime_dependencies,false);assert.equal(report.coverage.other_languages,false);
 const from=report.packages.find(pkg=>pkg.name==='@fixture/a'),target=report.packages.find(pkg=>pkg.name==='@fixture/b');assert.ok(from&&target);
 const choices=report.dependencies.filter(edge=>edge.from_package_id===from.id&&edge.section==='dependencies');
 for(const key of ['@fixture/b','alias-b','relative-b']){const edge=choices.find(edge=>edge.name===key);assert.equal(edge.range_state,'valid');assert.equal(edge.compatibility,'compatible');assert.deepEqual(edge.target_package_ids,[target.id]);assert.ok(edge.workspace)}
 assert.equal(choices.find(edge=>edge.name==='bad-b').compatibility,'incompatible');assert.equal(choices.find(edge=>edge.name==='external-lib').compatibility,'unknown');assert.deepEqual(choices.find(edge=>edge.name==='external-lib').target_package_ids,[]);
 const secret=choices.find(edge=>edge.name==='secret-lib');assert.equal(secret.range_redacted,true);assert.equal(secret.raw_range,'');assert.equal(secret.range_state,'unsupported');assert.equal(secret.raw_range_sha256,sha('https://user:private-source-token@registry.invalid/pkg'));assert.ok(!JSON.stringify(report).includes('private-source-token'));assert.ok(report.dependencies.some(edge=>edge.pointer==='/dependencies/@fixture~1b'));
 assert.deepEqual(inspectSourceInput(input),report);const tampered=structuredClone(input);tampered.sources[0].manifests[0].content_base64=Buffer.from('{}').toString('base64');assert.throws(()=>inspectSourceInput(tampered),/source_inspector_invalid_input/);
});
