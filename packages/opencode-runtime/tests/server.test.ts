import {test} from 'node:test';
import assert from 'node:assert/strict';
import {mkdir,mkdtemp,writeFile,readFile,rm} from 'node:fs/promises';
import {resolve,join,dirname,sep} from 'node:path';
import {createHash} from 'node:crypto';
import {startServer} from '../src/server.js';
import {childEnvironment,generatedConfig} from '../src/config.js';
import type {RuntimeConfig} from '../src/contract.js';

test('private server stop terminates and awaits only its owned Node fixture parent',async()=>{
 const cache=resolve('.cache/tests');await mkdir(cache,{recursive:true});const root=await mkdtemp(join(cache,'parent-close-'));
 let server:Awaited<ReturnType<typeof startServer>>|undefined;
 try{
  const source=join(root,'source');await mkdir(source);
  // This is a labelled Node process fixture, never the OpenCode executable.
  // It exercises only the production launcher/parent-close acknowledgement.
  await writeFile(join(source,'serve'),'process.stdout.write("opencode server listening on http://127.0.0.1:23456\\n");setInterval(()=>{},1000);\n');
  const config={executable:process.execPath,executable_sha256:createHash('sha256').update(await readFile(process.execPath)).digest('hex'),source,home:join(root,'home'),provider_id:'fixed',model:'fixed-model',base_url:'http://127.0.0.1:1/v1'} as RuntimeConfig;
  server=await startServer(config,childEnvironment(config,generatedConfig(config)),new AbortController().signal);
  const parentPID=server.parentPID;assert.ok(parentPID);assert.doesNotThrow(()=>process.kill(parentPID,0));
  await assert.doesNotReject(server.stop());assert.throws(()=>process.kill(parentPID,0),{code:'ESRCH'});await assert.doesNotReject(server.stop());
 }finally{if(server)await server.stop().catch(()=>{});if(dirname(resolve(root))!==cache||!root.startsWith(cache+sep))throw Error('unsafe test cleanup');await rm(root,{recursive:true,force:true});}
});
