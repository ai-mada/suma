import {test} from 'node:test';
import assert from 'node:assert/strict';
import {mkdtemp,mkdir,writeFile,rm} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {spawnSync} from 'node:child_process';

const shell=process.env.SUMA_TEST_SH??'sh';
const script=resolve('deploy/entrypoint.sh');
const scratch=resolve('.cache/deploy-entrypoint-tests');
const cleanEnv={...process.env};
for(const name of ['SUMA_SETUP_TOKEN_FILE','SUMA_DB_DSN_FILE','SUMA_INGRESS_MASTER_KEY_FILE','SUMA_RUNNER_TOKEN_FILE','SUMA_KNOWLEDGE_TOKEN_FILE','SUMA_KNOWLEDGE_OB_URL_FILE','SUMA_EMBEDDING_API_KEY_FILE'])cleanEnv[name]='';
delete cleanEnv.SUMA_INGRESS_MASTER_KEY;

test('secret entrypoint exports literal file content without evaluating or printing it',async t=>{
 await mkdir(scratch,{recursive:true});const dir=await mkdtemp(join(scratch,'secret-'));t.after(()=>rm(dir,{recursive:true,force:true}));const file=join(dir,'setup-token');const token="fake-secret-'$()\\-only-for-test";await writeFile(file,token+'\n',{mode:0o600});
 const result=spawnSync(shell,[script,process.execPath,'-e','process.exit(process.env.SUMA_SETUP_TOKEN===process.env.SUMA_EXPECTED_SECRET?0:2)'],{encoding:'utf8',env:{...cleanEnv,SUMA_SETUP_TOKEN_FILE:file,SUMA_EXPECTED_SECRET:token},timeout:5000});
 assert.equal(result.error,undefined);assert.equal(result.status,0,result.stderr);assert.equal(result.stdout,'');assert.equal(result.stderr,'');
});

test('unreadable secret prevents the requested executable from starting',()=>{
 const result=spawnSync(shell,[script,process.execPath,'-e','process.stdout.write("EXECUTED")'],{encoding:'utf8',env:{...cleanEnv,SUMA_SETUP_TOKEN_FILE:join(scratch,'definitely-absent')},timeout:5000});
 assert.equal(result.error,undefined);assert.equal(result.status,1);assert.equal(result.stdout,'');assert.match(result.stderr,/SUMA_SETUP_TOKEN/);
});

test('ingress master key secret file is exported without output',async t=>{
 await mkdir(scratch,{recursive:true});const dir=await mkdtemp(join(scratch,'ingress-'));t.after(()=>rm(dir,{recursive:true,force:true}));const file=join(dir,'ingress-master-key');const key=Buffer.alloc(32,0x23).toString('base64');await writeFile(file,key+'\n',{mode:0o600});
 const result=spawnSync(shell,[script,process.execPath,'-e','process.exit(process.env.SUMA_INGRESS_MASTER_KEY===process.env.SUMA_EXPECTED_SECRET?0:2)'],{encoding:'utf8',env:{...cleanEnv,SUMA_INGRESS_MASTER_KEY_FILE:file,SUMA_EXPECTED_SECRET:key},timeout:5000});
 assert.equal(result.error,undefined);assert.equal(result.status,0,result.stderr);assert.equal(result.stdout,'');assert.equal(result.stderr,'');
});

test('unreadable ingress master key file prevents execution without leaking its path',()=>{
 const result=spawnSync(shell,[script,process.execPath,'-e','process.stdout.write("EXECUTED")'],{encoding:'utf8',env:{...cleanEnv,SUMA_INGRESS_MASTER_KEY_FILE:join(scratch,'absent-ingress-master-key')},timeout:5000});
 assert.equal(result.error,undefined);assert.equal(result.status,1);assert.equal(result.stdout,'');assert.match(result.stderr,/SUMA_INGRESS_MASTER_KEY/);assert.doesNotMatch(result.stderr,/absent-ingress-master-key/);
});

test('empty ingress master key file fails closed even if a direct key is present',async t=>{
 await mkdir(scratch,{recursive:true});const dir=await mkdtemp(join(scratch,'empty-ingress-'));t.after(()=>rm(dir,{recursive:true,force:true}));const file=join(dir,'ingress-master-key');await writeFile(file,'\n',{mode:0o600});
 const result=spawnSync(shell,[script,process.execPath,'-e','process.stdout.write("EXECUTED")'],{encoding:'utf8',env:{...cleanEnv,SUMA_INGRESS_MASTER_KEY_FILE:file,SUMA_INGRESS_MASTER_KEY:Buffer.alloc(32,0x23).toString('base64')},timeout:5000});
 assert.equal(result.error,undefined);assert.equal(result.status,1);assert.equal(result.stdout,'');assert.match(result.stderr,/SUMA_INGRESS_MASTER_KEY/);
});

test('ingress master key without a secret file preserves direct environment or SQLite auto provisioning',()=>{
 for(const configured of [false,true]){
  const env={...cleanEnv};if(configured)env.SUMA_INGRESS_MASTER_KEY=Buffer.alloc(32,0x23).toString('base64');
  const expected=configured?'process.env.SUMA_INGRESS_MASTER_KEY===process.env.SUMA_EXPECTED_SECRET':'process.env.SUMA_INGRESS_MASTER_KEY===undefined';
  const result=spawnSync(shell,[script,process.execPath,'-e',`process.exit(${expected}?0:2)`],{encoding:'utf8',env:{...env,SUMA_EXPECTED_SECRET:configured?env.SUMA_INGRESS_MASTER_KEY:''},timeout:5000});
  assert.equal(result.error,undefined);assert.equal(result.status,0,result.stderr);assert.equal(result.stdout,'');assert.equal(result.stderr,'');
 }
});
