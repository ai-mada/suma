import {test,type TestContext} from 'node:test';
import assert from 'node:assert/strict';
import {EventEmitter} from 'node:events';
import {PassThrough} from 'node:stream';
import {basename} from 'node:path';
import {syncBuiltinESMExports} from 'node:module';
import fsPromises from 'node:fs/promises';
import childProcess from 'node:child_process';
import {inspectAdapters} from '../src/catalog.js';

const validProbe='{"v":1,"type":"probe","runtime":"dsh","protocol_version":1,"plugin_version":"fixture-1"}';
const codebuddyHelp='-p --input-format <text|stream-json> --output-format <stream-json> --verbose';
type Reply={stdout?:string;stderr?:string;code?:number};

async function fixture(t:TestContext,reply:(id:string,args:string[])=>Reply,run:(calls:{id:string;args:string[]}[])=>Promise<void>,wrapper=false):Promise<void>{
 const calls:{id:string;args:string[]}[]=[];const previousPath=process.env.PATH;process.env.PATH='catalog-fixture-only';
 t.mock.method(fsPromises,'access',async (file:Parameters<typeof fsPromises.access>[0])=>{const name=basename(String(file));if(wrapper?name==='codebuddy.cmd':/^(codebuddy|dsh)(\.exe)?$/.test(name))return;throw Error('fixture absent');});
 t.mock.method(childProcess,'spawn',((file:string,args:string[],options:Record<string,unknown>)=>{
  assert.equal(options.shell,false);assert.equal(options.windowsHide,true);assert.deepEqual(options.stdio,['ignore','pipe','pipe']);
  const id=basename(file).replace(/\.exe$/,'');calls.push({id,args:[...args]});const result=reply(id,args);
  const stdout=new PassThrough(),stderr=new PassThrough();const child=Object.assign(new EventEmitter(),{stdout,stderr,kill:()=>true});
  queueMicrotask(()=>{if(result.stdout)stdout.write(result.stdout);if(result.stderr)stderr.write(result.stderr);stdout.end();stderr.end();child.emit('close',result.code??0);});
  return child;
 }) as unknown as typeof childProcess.spawn);syncBuiltinESMExports();
 try{await run(calls);}finally{if(previousPath===undefined)delete process.env.PATH;else process.env.PATH=previousPath;t.mock.restoreAll();syncBuiltinESMExports();}
}

test('team catalog observes CodeBuddy flags and the exact DSH profile probe without enabling execution',async t=>{
 await fixture(t,(id,args)=>args[0]==='--version'?{stdout:id+' fixture-version'}:id==='codebuddy'?{stdout:codebuddyHelp}:{stdout:validProbe,stderr:'profile probe diagnostic\n'},async calls=>{
  const items=await inspectAdapters('en-US');assert.deepEqual(items.map(x=>x.id),['codex','opencode','hermes','harness','codebuddy']);
  const cb=items.find(x=>x.id==='codebuddy')!,dsh=items.find(x=>x.id==='harness')!;
  assert.equal(cb.installed,true);assert.deepEqual(cb.operations,['stream_json_print']);assert.deepEqual(cb.verified,['version','help']);
  assert.equal(dsh.installed,true);assert.deepEqual(dsh.operations,['multica_stdio_v1']);assert.deepEqual(dsh.verified,['version','multica_profile_probe']);
  assert.deepEqual(calls,[{id:'dsh',args:['--version']},{id:'dsh',args:['--profile','multica','--probe']},{id:'codebuddy',args:['--version']},{id:'codebuddy',args:['--help']}]);
  assert.ok(items.every(x=>!Object.hasOwn(x,'execution_enabled')),'catalog observations must not mint execution authority');
 });
});

test('installed DSH without a compatible profile never claims stdio capability',async t=>{
 for(const [name,probe] of Object.entries({missing:{code:1,stderr:'profile not installed'},duplicate:{stdout:validProbe.replace('"v":1','"v":2,"v":1')},escapedDuplicate:{stdout:validProbe.replace('"v":1','"v":2,"\\u0076":1')},foreign:{stdout:validProbe.replace('"runtime":"dsh"','"runtime":"other"')},wrongProtocol:{stdout:validProbe.replace('"protocol_version":1','"protocol_version":2')},missingPlugin:{stdout:validProbe.replace(',"plugin_version":"fixture-1"','')},nonIntegral:{stdout:validProbe.replace('"v":1','"v":1.0')},trailing:{stdout:validProbe+'\n{}'}})){
  await t.test(name,async()=>fixture(t,(_id,args)=>args[0]==='--version'?{stdout:'fixture-version'}:args[0]==='--profile'?probe:{stdout:codebuddyHelp},async calls=>{
   const item=(await inspectAdapters('zh-CN')).find(x=>x.id==='harness')!;assert.equal(item.installed,true);assert.deepEqual(item.operations,[]);assert.ok(item.reason);assert.equal(item.display?.message_key,'native.catalog.requirements_mismatch');assert.equal(item.display?.locale,'zh-CN');assert.ok(!calls.some(x=>x.id==='dsh'&&x.args[0]==='--help'));
  }));
 }
});

test('CodeBuddy missing stream flags and Windows shell wrappers remain unavailable',async t=>{
 await fixture(t,(_id,args)=>args[0]==='--version'?{stdout:'fixture-version'}:args[0]==='--profile'?{stdout:validProbe}:{stdout:'--input-format --output-format'},async()=>{
  const item=(await inspectAdapters('en-US')).find(x=>x.id==='codebuddy')!;assert.equal(item.installed,true);assert.deepEqual(item.operations,[]);assert.equal(item.display?.message_key,'native.catalog.requirements_mismatch');
 });
 if(process.platform==='win32')await fixture(t,()=>{throw Error('shell wrapper must not be spawned');},async calls=>{
  const item=(await inspectAdapters('zh-CN')).find(x=>x.id==='codebuddy')!;assert.equal(item.installed,true);assert.deepEqual(item.operations,[]);assert.deepEqual(item.verified,[]);assert.equal(item.display?.message_key,'native.catalog.shell_wrapper');assert.deepEqual(calls,[]);
 },true);
});
