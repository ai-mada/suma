// Archive only a completed, sealed desktop delivery. tar preserves executable
// permissions and application-framework links across CI artifact transport.
import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {spawnSync} from 'node:child_process';
import {createHash} from 'node:crypto';
import {createRequire} from 'node:module';

const {assertPlainPath}=createRequire(import.meta.url)('../apps/desktop/scripts/provenance.cjs');

const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
const target=process.argv[2];
if(!/^(win32|darwin|linux)-(x64|arm64)$/.test(target??'')||process.argv.length!==3)throw Error('Usage: node scripts/archive-desktop.mjs <platform-arch>');
const directory=path.join(root,'dist/desktop',target);
assertPlainPath(directory,'en-US');
const source=JSON.parse(fs.readFileSync(path.join(directory,'manifest.json'),'utf8'));
const version=JSON.parse(fs.readFileSync(path.join(root,'package.json'),'utf8')).version;
if(source.schema_version!==1||source.product!=='suma'||source.product_version!==version||source.target!==target||source.verification?.packaging!=='sdk_completed')throw Error('A completed current desktop manifest is required');
const expected=new Set(['manifest.json','SHA256SUMS']);
const relative=name=>typeof name==='string'&&name.length>0&&!name.includes('\\')&&!path.posix.isAbsolute(name)&&!name.includes(':')&&!/[\x00-\x1f\x7f]/.test(name)&&name.split('/').every(part=>part&&part!=='.'&&part!=='..');
if(!Array.isArray(source.files)||!source.files.length)throw Error('The sealed delivery has no files');
for(const file of source.files){
  if(!relative(file.path)||expected.has(file.path))throw Error('Invalid or repeated delivery path');
  expected.add(file.path);
}
function walk(current){
  for(const entry of fs.readdirSync(current,{withFileTypes:true})){
    const filename=path.join(current,entry.name),name=path.relative(directory,filename).split(path.sep).join('/');
    if(entry.isDirectory())walk(filename);
    else if(!expected.delete(name))throw Error('Unsealed file in the desktop delivery');
    if(entry.isSymbolicLink()){
      const destination=fs.readlinkSync(filename),resolved=path.relative(directory,fs.realpathSync(filename));
      if(path.isAbsolute(destination)||resolved==='..'||resolved.startsWith('..'+path.sep)||path.isAbsolute(resolved))throw Error('Escaping application link');
    }else if(!entry.isDirectory()&&!entry.isFile())throw Error('Special file in the desktop delivery');
  }
}
if(fs.lstatSync(directory).isSymbolicLink())throw Error('A linked delivery root is not supported');
walk(directory);
if(expected.size)throw Error('A sealed delivery file is missing');
const output=path.join(root,'.cache/desktop-ci/artifacts');assertPlainPath(output,'en-US',true);fs.mkdirSync(output,{recursive:true});
const archive=path.join(output,target+'.tar.gz');
// Never overwrite another run's archive. CI workspaces are fresh by default.
if(fs.existsSync(archive)||fs.existsSync(archive+'.sha256'))throw Error('Archive already exists; preserve it before another archive operation');
const result=spawnSync('tar',['-czf',path.relative(root,archive),'-C',path.relative(root,path.dirname(directory)),target],{cwd:root,stdio:'inherit',windowsHide:true});
if(result.error||result.status!==0)throw Error('tar failed; no successful archive receipt was written');
const hash=createHash('sha256');for await(const bytes of fs.createReadStream(archive))hash.update(bytes);
fs.writeFileSync(archive+'.sha256',`${hash.digest('hex')}  ${path.basename(archive)}\n`,{flag:'wx'});
console.log(JSON.stringify({target,archive:path.relative(root,archive).split(path.sep).join('/'),packaging:source.verification.packaging,installation:false,signature:source.verification.signature,notarization:source.verification.notarization}));
