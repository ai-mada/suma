// Public wire types are generated from repositoryintegrationcontract's
// SourceDependencyReport. This module owns parsing, never permissions or edits.
import {createHash} from 'node:crypto';
import {posix} from 'node:path';
import semver from 'semver';
import {parseDocument} from 'yaml';
import {minimatch} from 'minimatch';
import {sourceInspectorContract as CONTRACT} from '../../contracts/src/source-inspector-runtime.mjs';

export const MAX_INPUT=CONTRACT.max_transport_bytes,MAX_OUTPUT=CONTRACT.limits.max_report_bytes;
const PROTOCOL=CONTRACT.protocol;
const LIMITS=CONTRACT.limits;
const sha=raw=>createHash('sha256').update(raw).digest('hex');
const canonical=value=>JSON.stringify(value,(_,v)=>v&&typeof v==='object'&&!Array.isArray(v)?Object.fromEntries(Object.keys(v).sort().map(k=>[k,v[k]])):v).replace(/[<>&\u2028\u2029]/g,c=>'\\u'+c.charCodeAt(0).toString(16).padStart(4,'0'));
const digest=value=>sha(canonical(value));
const compare=(a,b)=>Buffer.compare(Buffer.from(a,'utf8'),Buffer.from(b,'utf8'));
const object=value=>value!==null&&typeof value==='object'&&!Array.isArray(value);
const exact=(value,keys)=>object(value)&&Object.keys(value).length===keys.length&&keys.every(k=>Object.hasOwn(value,k));
const hash=value=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
const oid=value=>typeof value==='string'&&/^(?:[a-f0-9]{40}|[a-f0-9]{64})$/.test(value);
const integer=(value,max)=>Number.isSafeInteger(value)&&value>=0&&value<=max;
const name=value=>typeof value==='string'&&value.length<=214&&/^(?:@[a-z0-9][a-z0-9._-]*\/)?[a-z0-9][a-z0-9._-]*$/.test(value);
const safePath=(value,root=false)=>typeof value==='string'&&(root&&value===''||value.length>0&&Buffer.byteLength(value)<=1024&&posix.normalize(value)===value&&!value.startsWith('/')&&!/[\\:*?\[\]{}\p{Cc}\p{Cs}]/u.test(value)&&!value.split('/').some(part=>part==='.'||part==='..'||part.toLowerCase()==='.git'));
const relativeRoot=path=>{const root=posix.dirname(path);return root==='.'?'':root};
const under=(root,path)=>root===''||path.startsWith(root+'/');
const jsonPointer=value=>value.replaceAll('~','~0').replaceAll('/','~1');
const format=path=>({'package.json':'package-json','pnpm-workspace.yaml':'pnpm-workspace-yaml','pnpm-lock.yaml':'pnpm-lock-yaml','package-lock.json':'npm-lock-json'})[posix.basename(path)];
const invalid=()=>{throw Error('source_inspector_invalid_input')};

// JSON.parse remains the JSON parser. This bounded lexical walk only prevents
// duplicate keys from disappearing during that parse, including escaped names.
function parseJSON(text){
 const result=JSON.parse(text);let at=0,tokens=0;
 const whitespace=()=>{while(/\s/.test(text[at]??'')&&at<text.length)at++};
 const string=()=>{const start=at++;while(at<text.length){const c=text[at++];if(c==='\\'){at++;continue}if(c==='"'){const value=JSON.parse(text.slice(start,at));if(/[\p{Cs}]/u.test(value))invalid();return value}}invalid()};
 const walk=depth=>{if(depth>64||++tokens>200000)invalid();whitespace();const c=text[at];
  if(c==='"'){string();return}if(c==='{'||c==='['){at++;const end=c==='{'?'}':']',seen=new Set();whitespace();if(text[at]===end){at++;return}while(at<text.length){if(c==='{'){const key=string();if(seen.has(key))invalid();seen.add(key);whitespace();at++}walk(depth+1);whitespace();if(text[at++]===end)return;whitespace()}invalid()}
  while(at<text.length&&!/[\s,\]}]/.test(text[at]))at++;
 };walk(0);return result;
}

function sources(input){
 if(!exact(input,['schema_version','sources'])||input.schema_version!==1||!Array.isArray(input.sources)||input.sources.length<1||input.sources.length>8)invalid();
 const seen=new Set();let totalBytes=0;
 return input.sources.map(source=>{
  if(!exact(source,['protocol','inspection_request_sha256','snapshot_sha256','exact_commit','tree_oid','scopes','manifests','coverage','limits'])||source.protocol!==PROTOCOL||!hash(source.inspection_request_sha256)||!hash(source.snapshot_sha256)||!oid(source.exact_commit)||!oid(source.tree_oid)||seen.has(source.snapshot_sha256)||!Array.isArray(source.scopes)||source.scopes.length<1||source.scopes.length>16||!Array.isArray(source.manifests)||source.manifests.length>200||canonical(source.limits)!==canonical(LIMITS)||digest({...source,snapshot_sha256:''})!==source.snapshot_sha256)invalid();seen.add(source.snapshot_sha256);
  let previous=null;const roots=[];for(const scope of source.scopes){if(!exact(scope,['relative_root','tree_oid'])||!safePath(scope.relative_root,true)||!oid(scope.tree_oid)||scope.tree_oid.length!==source.exact_commit.length||previous!==null&&compare(previous,scope.relative_root)>=0||roots.some(root=>root===''||scope.relative_root.startsWith(root+'/')))invalid();previous=scope.relative_root;roots.push(scope.relative_root)}
  const coverage=source.coverage;if(!exact(coverage,['state','tree_entries','manifest_count','content_bytes','skipped_entries','reasons'])||!['complete','partial'].includes(coverage.state)||!integer(coverage.tree_entries,10000)||!integer(coverage.manifest_count,200)||coverage.manifest_count!==source.manifests.length||!integer(coverage.content_bytes,8<<20)||!integer(coverage.skipped_entries,10000)||!Array.isArray(coverage.reasons)||coverage.reasons.some(code=>typeof code!=='string'||!/^[-a-z_]{1,80}$/.test(code))||coverage.state==='complete'&&(coverage.skipped_entries!==0||coverage.reasons.length))invalid();
  let bytes=0;previous=null;const files=source.manifests.map(file=>{
   if(!exact(file,['path','blob_oid','sha256','size_bytes','content_base64','format'])||!safePath(file.path)||!source.scopes.some(scope=>under(scope.relative_root,file.path))||previous!==null&&compare(previous,file.path)>=0||!oid(file.blob_oid)||file.blob_oid.length!==source.exact_commit.length||!hash(file.sha256)||!integer(file.size_bytes,256<<10)||format(file.path)!==file.format||typeof file.content_base64!=='string')invalid();previous=file.path;
   const raw=Buffer.from(file.content_base64,'base64');if(raw.length!==file.size_bytes||raw.toString('base64')!==file.content_base64||sha(raw)!==file.sha256)invalid();bytes+=raw.length;totalBytes+=raw.length;if(bytes>8<<20||totalBytes>8<<20)invalid();
   return {...file,raw};
  });if(bytes!==coverage.content_bytes)invalid();return {...source,files};
 });
}

export function inspectSourceInput(input){
 const fixed=sources(input),packages=[],dependencies=[],files=[],diagnostics=[],reasons=new Set();let parsedFiles=0,workspaceDeclarations=false;
 const diagnostic=(source,path,pointer,code)=>{if(diagnostics.length>=2000)throw Error('source_inspector_report_limit');diagnostics.push({source_snapshot_sha256:source.snapshot_sha256,path,pointer,code});reasons.add(code)};
 for(const source of fixed){
  if(source.coverage.state!=='complete')reasons.add('source_coverage_partial');
  const docs=new Map(),definitions=[];
  const patterns=(values,owner,path,pointer)=>{workspaceDeclarations=true;if(!Array.isArray(values)||values.length>200||values.some(pattern=>typeof pattern!=='string'||pattern.length<1||pattern.length>256||/[\\:?\[\]{}()\p{Cc}\p{Cs}]/u.test(pattern)||pattern.startsWith('/')||pattern.split('/').includes('..'))){diagnostic(source,path,pointer,'unsupported_workspace_patterns');return []}return values.map(value=>({exclude:value.startsWith('!'),pattern:value.replace(/^!/,''),root:owner}));};
  for(const file of source.files){
   files.push({source_snapshot_sha256:source.snapshot_sha256,path:file.path,blob_oid:file.blob_oid,sha256:file.sha256,size_bytes:file.size_bytes,format:file.format});
   if(file.format==='pnpm-lock-yaml'||file.format==='npm-lock-json'){diagnostic(source,file.path,'','external_lock_not_evaluated');continue}
   let text;try{text=new TextDecoder('utf-8',{fatal:true}).decode(file.raw)}catch{diagnostic(source,file.path,'','invalid_utf8');continue}
   let doc;try{if(file.format==='package-json')doc=parseJSON(text);else {const yaml=parseDocument(text,{uniqueKeys:true});if(yaml.errors.length||yaml.warnings.length)throw Error();doc=yaml.toJS({maxAliasCount:0})};if(!object(doc))throw Error()}catch{diagnostic(source,file.path,'',file.format==='package-json'?'invalid_package_json':'invalid_workspace_yaml');continue}
   parsedFiles++;docs.set(file.path,doc);const root=relativeRoot(file.path);
   if(file.format==='pnpm-workspace-yaml'){definitions.push({root,patterns:patterns(doc.packages,root,file.path,'/packages'),explicit:true});continue}
   // Private workspace roots may omit a publishable package name. Their
   // workspace declaration still supplies scope for the actual child packages.
   if(Object.hasOwn(doc,'workspaces'))definitions.push({root,patterns:patterns(object(doc.workspaces)?doc.workspaces.packages:doc.workspaces,root,file.path,'/workspaces'),explicit:true});
   if(!name(doc.name)){diagnostic(source,file.path,'/name','package_name_unavailable');continue}
   const version=typeof doc.version==='string'&&doc.version.length<=256&&semver.valid(doc.version)?doc.version:'';
   if(!version)diagnostic(source,file.path,'/version','package_version_unavailable');
   packages.push({id:digest([source.snapshot_sha256,file.path,doc.name]),source_snapshot_sha256:source.snapshot_sha256,path:file.path,name:doc.name,version,root,workspace_member:false});
  }
  for(const scope of source.scopes)if(!definitions.some(def=>def.root===scope.relative_root))definitions.push({root:scope.relative_root,patterns:[],explicit:false});
  definitions.sort((a,b)=>b.root.length-a.root.length||Number(b.explicit)-Number(a.explicit));const owners=new Map();
  const local=packages.filter(pkg=>pkg.source_snapshot_sha256===source.snapshot_sha256);
  if(local.length===0)reasons.add('no_supported_package_manifest');
  for(const pkg of local){const owner=definitions.find(def=>pkg.root===def.root||under(def.root,pkg.root)&&def.patterns.some(p=>!p.exclude&&minimatch(pkg.root.slice(def.root?def.root.length+1:0),p.pattern,{nonegate:true,dot:false}))&&!def.patterns.some(p=>p.exclude&&minimatch(pkg.root.slice(def.root?def.root.length+1:0),p.pattern,{nonegate:true,dot:false})));if(owner){pkg.workspace_member=true;owners.set(pkg.id,owner)}}
  for(const pkg of local){const doc=docs.get(pkg.path);for(const section of ['dependencies','devDependencies','peerDependencies','optionalDependencies']){
   if(!Object.hasOwn(doc,section))continue;if(!object(doc[section])){diagnostic(source,pkg.path,'/'+section,'invalid_dependency_section');continue}
   const entries=Object.entries(doc[section]).sort(([a],[b])=>compare(a,b));
   for(const [dependency,raw] of entries){if(dependencies.length>=10000)throw Error('source_inspector_report_limit');const pointer='/'+section+'/'+jsonPointer(dependency);if(!name(dependency)||typeof raw!=='string'||raw.length>512){diagnostic(source,pkg.path,pointer,'unsupported_dependency_declaration');continue}
    let targetName=dependency,range=raw,rangeState='unknown',compatibility='unknown',targets=[],workspace=raw.startsWith('workspace:'),supported=true;
    if(workspace){range=raw.slice(10);if(range.startsWith('./')||range.startsWith('../')){const root=posix.normalize(posix.join(pkg.root,range));if(!safePath(root,true)){supported=false}else targets=local.filter(other=>other.root===root&&owners.get(other.id)===owners.get(pkg.id)&&owners.has(pkg.id))}else{const at=range.lastIndexOf('@');if(at>0){targetName=range.slice(0,at);range=range.slice(at+1);if(!name(targetName))supported=false}targets=local.filter(other=>other.name===targetName&&owners.has(pkg.id)&&owners.get(other.id)===owners.get(pkg.id))}
     if(supported){const relative=range.startsWith('./')||range.startsWith('../');if(relative)rangeState='valid';else if(['*','^','~'].includes(range)||semver.validRange(range))rangeState='valid';else rangeState='unsupported';if(targets.length===1&&rangeState==='valid'){if(relative)compatibility='compatible';else if(targets[0].version){const tested=['*','^','~'].includes(range)?range==='*'?'*':range+targets[0].version:range;compatibility=semver.satisfies(targets[0].version,tested)?'compatible':'incompatible'}}if(targets.length!==1)diagnostic(source,pkg.path,pointer,targets.length?'ambiguous_workspace_target':'workspace_target_unavailable')}
    }else if(raw.startsWith('file:')||raw.startsWith('link:')){const locator=raw.slice(raw.indexOf(':')+1);const root=posix.normalize(posix.join(pkg.root,locator));if(!locator.startsWith('/')&&safePath(root,true)&&!/[\\:\p{Cc}\p{Cs}]/u.test(locator)){targets=local.filter(other=>other.root===root);rangeState='valid'}else supported=false;
    }else if(semver.validRange(raw)){rangeState='valid'}else if(/^[0-9v~^*<>=|. +xX-]+$/.test(raw)){rangeState='invalid'}else supported=false;
    if(!supported||rangeState==='unsupported'){rangeState='unsupported';diagnostic(source,pkg.path,pointer,'unsupported_dependency_specifier')}
    if(workspace&&rangeState==='unknown')rangeState='unsupported';
    const redacted=!supported||rangeState==='unsupported'||/[\p{Cc}\p{Cs}]/u.test(raw)||raw.includes('://'),safeRaw=redacted?'':raw;
    dependencies.push({id:digest([source.snapshot_sha256,pkg.path,section,dependency]),source_snapshot_sha256:source.snapshot_sha256,from_package_id:pkg.id,section,name:dependency,raw_range:safeRaw,raw_range_sha256:sha(raw),range_redacted:redacted,workspace,target_package_ids:targets.map(target=>target.id).sort(),range_state:rangeState,compatibility,path:pkg.path,pointer});
   }
  }}
 }
 packages.sort((a,b)=>compare(a.id,b.id));dependencies.sort((a,b)=>compare(a.id,b.id));files.sort((a,b)=>compare(a.source_snapshot_sha256+'\0'+a.path,b.source_snapshot_sha256+'\0'+b.path));diagnostics.sort((a,b)=>compare(a.source_snapshot_sha256+'\0'+a.path+'\0'+a.pointer+'\0'+a.code,b.source_snapshot_sha256+'\0'+b.path+'\0'+b.pointer+'\0'+b.code));
 const report={schema_version:1,parser_id:CONTRACT.parser_id,parser_version:CONTRACT.parser_version,source_snapshots:fixed.map(source=>source.snapshot_sha256).sort(),packages,dependencies,files,coverage:{state:reasons.size?'partial':'complete',files:files.length,parsed_files:parsedFiles,packages:packages.length,dependencies:dependencies.length,workspace_declarations:workspaceDeclarations,external_lock_resolution:false,runtime_dependencies:false,other_languages:false,reasons:[...reasons].sort()},diagnostics};
 if(Buffer.byteLength(JSON.stringify(report))>MAX_OUTPUT)throw Error('source_inspector_report_limit');return report;
}
export function inspectSourceJSON(raw){return inspectSourceInput(parseJSON(raw))}
