'use strict';
const {spawnSync}=require('node:child_process');
const {mkdirSync,existsSync}=require('node:fs');
const {resolve,join}=require('node:path');
const root=resolve(__dirname,'../../..');const out=join(root,'.cache/desktop/control');mkdirSync(out,{recursive:true});
if(!existsSync(join(root,'apps/web/dist/index.html')))throw Error('Run pnpm build before desktop packaging');
const result=spawnSync('go',['build','-trimpath','-o',join(out,process.platform==='win32'?'suma-control.exe':'suma-control'),'./cmd/suma-control'],{cwd:root,stdio:'inherit',env:process.env});
if(result.error)throw result.error;if(result.status!==0)process.exit(result.status??1);
