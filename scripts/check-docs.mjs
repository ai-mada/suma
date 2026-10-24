import fs from 'node:fs';
import path from 'node:path';
const root=path.resolve('docs');
const files=[];function walk(dir){for(const item of fs.readdirSync(dir,{withFileTypes:true})){const p=path.join(dir,item.name);if(item.isDirectory())walk(p);else if(p.endsWith('.md'))files.push(p);}}walk(root);
const missing=[];for(const file of files){const text=fs.readFileSync(file,'utf8');for(const match of text.matchAll(/\]\(([^)]+)\)/g)){const ref=match[1].split('#')[0];if(!ref||/^(https?:|mailto:)/.test(ref))continue;if(!fs.existsSync(path.resolve(path.dirname(file),decodeURIComponent(ref))))missing.push({file:path.relative(root,file),reference:ref});}}
if(missing.length){process.stderr.write(JSON.stringify(missing,null,2)+'\n');process.exitCode=1;}else console.log(`Checked ${files.length} Markdown documents: local links resolve.`);
const spec=path.join(root,'api/openapi.json');if(fs.existsSync(spec)){const api=JSON.parse(fs.readFileSync(spec,'utf8'));const refs=[];function visit(v){if(!v||typeof v!=='object')return;if(v.$ref)refs.push(v.$ref);Object.values(v).forEach(visit);}visit(api);for(const ref of refs){let v=api;for(const key of ref.replace(/^#\//,'').split('/'))v=v?.[key];if(!v)throw new Error('Unresolved API reference '+ref);}console.log(`Checked ${Object.keys(api.paths).length} API paths and ${refs.length} schema references.`);}
