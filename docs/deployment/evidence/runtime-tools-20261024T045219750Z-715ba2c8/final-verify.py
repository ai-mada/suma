from pathlib import Path
import json,hashlib,datetime,re,shutil,struct
root=Path('D:/codes/ai-dev-os/suma');stamp='20261024T045219750Z-715ba2c8';e=root/'docs/deployment/evidence'/('runtime-tools-'+stamp);audit=root/'.cache/runtime-tools-delivery/20261024T045212595Z-8cc5bad7';doc=root/'docs/deployment/tool-runtime-bundles.md'
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
read=lambda name:json.loads((e/name).read_text(encoding='utf-8-sig'))
v=read('verification.json');build=read('manifest.json');sources=read('source-manifest.json');history=read('historical-files.json');binding=read('go-test-binding.json')
assert len(sources)==116 and len(v['binaries'])==24 and len(build['commands'])==50
assert hashlib.sha256(json.dumps(sources,ensure_ascii=False,separators=(',',':')).encode()).hexdigest()==v['source_sha256']
for x in sources:
 p=root/x['path'];assert p.stat().st_size==x['size_bytes'] and sha(p)==x['sha256'],x['path']
for x in v['binaries']:
 p=root/x['path'];assert p.stat().st_size==x['size_bytes'] and sha(p)==x['sha256'],x['path']
 b=p.read_bytes()[:4096];target=x['target'];arch=target.split('-')[1]
 if target.startswith('windows'):
  off=struct.unpack_from('<I',b,0x3c)[0];assert b[:2]==b'MZ' and b[off:off+4]==b'PE\0\0';assert struct.unpack_from('<H',b,off+4)[0]==({'amd64':0x8664,'arm64':0xaa64}[arch])
 elif target.startswith('linux'):assert b[:6]==b'\x7fELF\x02\x01' and struct.unpack_from('<H',b,18)[0]==({'amd64':62,'arm64':183}[arch])
 else:assert b[:4]==bytes.fromhex('cffaedfe') and struct.unpack_from('<I',b,4)[0]==({'amd64':0x01000007,'arm64':0x0100000c}[arch])
for name in ['manifest.json','source-manifest.json','linked-modules.json']:
 assert sha(e/name)==sha(root/'dist/runtime/builds'/stamp/name),name
for c in build['commands']:
 assert c['exit_code']==0
 for key in ['stdout_log','stderr_log']:assert sha(root/c[key])==sha(e/'logs'/Path(c[key]).name),c[key]
for x in read('sha256.json'):assert sha(e/x['name'])==x['sha256'],x['name']
unchanged=[];protected=[];docchanges=[]
for x in history:
 p=root/x['path'];assert p.is_file(),x['path'];same=p.stat().st_size==x['size_bytes'] and sha(p)==x['sha256'];locked=x['path'].startswith(('dist/runtime/','docs/deployment/evidence/'))
 if locked:assert same,x['path'];protected.append(x['path'])
 if same:unchanged.append(x['path'])
 else:
  assert x['path'].startswith('docs/deployment/') and x['path'].endswith('.md'),x['path']
  docchanges.append({'path':x['path'],'previous_sha256':x['sha256'],'current_sha256':sha(p),'previous_size_bytes':x['size_bytes'],'current_size_bytes':p.stat().st_size,'scope':'Authorized current deployment guide update by root, distinct from immutable receipts and artifacts.'})
for x in read('preparation.json')['existing_build_stamps']:assert (root/'dist/runtime/builds'/x).is_dir(),x
now=datetime.datetime.now(datetime.timezone.utc).isoformat()
report={'schema_version':1,'recorded_at':now,'prebuild_snapshot_files':len(history),'all_snapshot_files_unchanged':len(unchanged),'immutable_artifact_and_evidence_files_checked':len(protected),'immutable_files_unchanged':True,'current_document_updates':docchanges,'scope':'Final integration-aware read of the original immutable prebuild inventory. Current deployment guide updates are separately listed; no original manifest was rewritten.'}
(e/'historical-preservation-final.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
assert sha(e/'go-test-receipt.original.json')==binding['source_receipt_sha256']
assert binding['runtime_files_match_final_test_snapshot'] and binding['actual_pass_events']==479 and binding['actual_failed_events']==binding['actual_skipped_test_cases']==0
assert sha(root/binding['raw_source'])==binding['raw_sha256']
text=doc.read_text(encoding='utf-8');assert not re.search(r'(?m)[ \t]+$',text)
links=[x.split('#',1)[0] for x in re.findall(r'\]\(([^)]+)\)',text) if not x.startswith('https:')]
pending={'final-verification.json','sha256-final.json'}
for link in links:
 p=doc.parent/link
 if p.name not in pending:assert p.is_file(),link
copied=e/'final-verify.py';assert not copied.exists();shutil.copyfile(Path(__file__),copied)
windows=[x for x in v['binaries'] if x['target']=='windows-amd64']
final={'schema_version':1,'recorded_at':now,'stage':'T-P4-004','stamp':stamp,'source_sha256':v['source_sha256'],'runtime_source_files_rechecked':len(sources),'runtime_go_files_rechecked':sum(x['path'].endswith('.go') for x in sources),'current_runtime_sources_match':True,'historical_files_rechecked':len(history),'historical_files_unchanged':len(unchanged),'immutable_historical_files_unchanged':len(protected),'current_document_updates':docchanges,'historical_build_receipt_directories':len(read('preparation.json')['existing_build_stamps']),'artifacts_rechecked':len(v['binaries']),'independent_go_metadata_checks':24,'original_receipt_copies_identical':3,'original_stdout_stderr_pairs_identical':50,'initial_evidence_files_rechecked':len(read('sha256.json')),'documentation':'docs/deployment/tool-runtime-bundles.md','documentation_sha256':sha(doc),'documentation_links_valid_except_pending_final_files':True,'documentation_link_count':len(links),'go_test_runtime_source_binding':True,'passed_test_events':479,'failed_test_events':0,'skipped_test_cases':0,'test_snapshot_files':187,'windows_binaries':windows,'no_target_program_started_or_stopped_by_delivery_agent':True,'no_binary_copied_to_documentation':True,'scope':'Offline cross-compilation, independent host metadata/header/size/SHA reads, original outputs and receipts, source/test binding and history preservation only. No tests rerun or target programs launched; TS assets and full platform acceptance are separate.'}
(e/'final-verification.json').write_text(json.dumps(final,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
files=[{'name':p.relative_to(e).as_posix(),'size_bytes':p.stat().st_size,'sha256':sha(p)} for p in sorted(e.rglob('*')) if p.is_file() and p.name!='sha256-final.json']
(e/'sha256-final.json').write_text(json.dumps(files,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
for x in files:
 p=e/x['name'];assert p.stat().st_size==x['size_bytes'] and sha(p)==x['sha256'],x['name']
for link in links:assert (doc.parent/link).is_file(),link
assert not any(p.suffix.lower() in ['.exe','.dll'] for p in e.rglob('*') if p.is_file())
print(json.dumps({'status':'verified','stamp':stamp,'source_sha256':v['source_sha256'],'runtime_input_files':len(sources),'evidence_files':len(files)+1,'protected_historical_files':len(protected),'all_history_unchanged':len(unchanged),'current_doc_changes':docchanges,'document_sha256':sha(doc),'final_verification_sha256':sha(e/'final-verification.json'),'final_manifest_sha256':sha(e/'sha256-final.json'),'windows':windows},ensure_ascii=True))
