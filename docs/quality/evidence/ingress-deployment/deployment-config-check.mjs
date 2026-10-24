import assert from 'node:assert/strict';
import {readFileSync,writeFileSync,mkdirSync} from 'node:fs';
import {resolve,join} from 'node:path';
import {createRequire} from 'node:module';
import {spawnSync} from 'node:child_process';

const require=createRequire(import.meta.url);
const yaml=require(resolve('node_modules/.pnpm/js-yaml@4.3.2/node_modules/js-yaml'));
const Ajv=require(resolve('node_modules/.pnpm/ajv@8.20.0/node_modules/ajv'));
const evidence=resolve('.cache/e2e/ingress-stage');
const env={...process.env,COMPOSE_DISABLE_ENV_FILE:'1',SUMA_PORT:'4310',
 SUMA_CONTROL_ENV_FILE:join(evidence,'deployment-absent-control.env'),
 SUMA_RUNNER_ENV_FILE:join(evidence,'deployment-absent-runner.env'),
 SUMA_KNOWLEDGE_ENV_FILE:join(evidence,'deployment-absent-knowledge.env'),
 SUMA_SETUP_TOKEN_FILE:join(evidence,'deployment-fixture-setup-token'),
 SUMA_DB_DSN_FILE:join(evidence,'deployment-fixture-database-dsn'),
 SUMA_INGRESS_MASTER_KEY_FILE:join(evidence,'deployment-fixture-ingress-master-key'),
 SUMA_RUNNER_TOKEN_FILE:join(evidence,'deployment-fixture-runner-token'),
 SUMA_KNOWLEDGE_TOKEN_FILE:join(evidence,'deployment-fixture-knowledge-token'),
 SUMA_RUNNER_PROJECT:'validation-project',SUMA_RUNNER_ID:'validation-worker',
 SUMA_RUNNER_WORKSPACE:process.cwd(),SUMA_RUNNER_ALLOWLIST:resolve('deploy/runner/commands.example.json')};
for(const key of ['SUMA_SETUP_TOKEN','SUMA_DB_DSN','SUMA_INGRESS_MASTER_KEY','SUMA_RUNNER_TOKEN','SUMA_KNOWLEDGE_TOKEN'])delete env[key];
const cases=[
 ['sqlite',[],'sqlite'],
 ['sqlite-optional',['runner','knowledge'],'sqlite'],
 ['oceanbase',['oceanbase'],'oceanbase'],
 ['oceanbase-optional',['runner','knowledge','oceanbase'],'oceanbase'],
 ['postgres',['postgres'],'postgres'],
 ['postgres-optional',['runner','knowledge','postgres'],'postgres']
];
for(const [name,overlays,driver] of cases){
 const args=['compose','-f','deploy/compose.yaml',...overlays.flatMap(value=>['-f',`deploy/compose.${value}.yaml`]),'config','--format','json'];
 const result=spawnSync('docker',args,{encoding:'utf8',env,timeout:20000});
 writeFileSync(join(evidence,`deployment-compose-${name}.stderr.log`),result.stderr??'');
 assert.equal(result.error,undefined);assert.equal(result.status,0,result.stderr);
 const config=JSON.parse(result.stdout);const control=config.services.control;
 assert.equal(control.environment.SUMA_DB_DRIVER,driver);
 assert.equal(control.environment.SUMA_INGRESS_MASTER_KEY,undefined);
 const secrets=control.secrets.map(value=>value.source);
 assert.ok(secrets.includes('setup_token'));
 assert.ok(control.volumes.some(value=>value.type==='volume'&&value.target==='/var/lib/suma'));
 if(driver==='sqlite'){
  assert.equal(control.environment.SUMA_INGRESS_MASTER_KEY_FILE,undefined);
  assert.ok(!secrets.includes('ingress_master_key'));
 }else{
  assert.equal(control.environment.SUMA_INGRESS_MASTER_KEY_FILE,'/run/secrets/ingress_master_key');
  assert.equal(control.environment.SUMA_DB_DSN_FILE,'/run/secrets/database_dsn');
  assert.ok(secrets.includes('database_dsn'));assert.ok(secrets.includes('ingress_master_key'));
  assert.equal(resolve(config.secrets.ingress_master_key.file),resolve(env.SUMA_INGRESS_MASTER_KEY_FILE));
 }
 if(overlays.includes('knowledge'))assert.equal(config.services.knowledge.ports,undefined);
 if(overlays.includes('runner'))assert.ok(config.services.runner.environment.SUMA_INGRESS_MASTER_KEY===undefined);
 writeFileSync(join(evidence,`deployment-compose-${name}.json`),JSON.stringify(config,null,2)+'\n');
 console.log(`Compose ${name}: parsed and checked secret references; no engine used.`);
}

const chart=resolve('deploy/helm/suma');
const schema=JSON.parse(readFileSync(join(chart,'values.schema.json'),'utf8'));
const values=yaml.load(readFileSync(join(chart,'values.yaml'),'utf8'));
const ajv=new Ajv({allErrors:true});
ajv.addMetaSchema(require(resolve('node_modules/.pnpm/ajv@8.20.0/node_modules/ajv/dist/refs/json-schema-draft-07.json')),'https://json-schema.org/draft-07/schema#');
const validate=ajv.compile(schema);
for(const driver of ['sqlite','oceanbase','postgres']){
 const candidate=structuredClone(values);candidate.database.driver=driver;
 assert.equal(validate(candidate),true,JSON.stringify(validate.errors));
 console.log(`Helm values schema: ${driver} accepted.`);
}
for(const [name,mutate] of [
 ['inline master key',value=>{value.ingressMasterKey.value='FORBIDDEN_INLINE_FIXTURE';}],
 ['empty Secret key',value=>{value.ingressMasterKey.key='';}],
 ['unsupported database',value=>{value.database.driver='unsupported';}],
 ['missing shared database Secret',value=>{value.database.driver='postgres';value.database.existingSecret='';}]
]){
 const candidate=structuredClone(values);mutate(candidate);assert.equal(validate(candidate),false,name);
 console.log(`Helm values schema: ${name} rejected.`);
}
const helm=spawnSync('helm',['version','--short'],{encoding:'utf8',timeout:5000});
assert.equal(helm.error?.code,'ENOENT');
console.log('Helm executable unavailable: no Helm lint/render or Kubernetes validation performed.');
console.log('This validates configuration and schema only; no database, image, container, or cluster acceptance.');
