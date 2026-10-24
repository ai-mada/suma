"""Read-only installed Codex version/help discovery through the delivered Runner.

No model request, login, task execution, platform credential or control API.
"""
from pathlib import Path
import datetime,hashlib,json,os,re,subprocess,sys

ROOT=Path.cwd().resolve()
STAGE=ROOT/'.cache/e2e/tool-stage'
ARCHIVE=ROOT/'docs/quality/evidence/tool-runtime'
runtime_manifest=ROOT/'docs/deployment/evidence/runtime-tools-20261024T045219750Z-715ba2c8/final-verification.json'
delivery=json.loads(runtime_manifest.read_text(encoding='utf-8-sig'))
runner_record=next(x for x in delivery['windows_binaries'] if x['name']=='suma-runner.exe')
runner=Path(runner_record['absolute_path'])
digest=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
assert digest(runner)==runner_record['sha256']=='12c8681ec9d7e1ba325252e264e5d7aaa7d73d9c053cc3d6c3466292b44f56cb','delivered_runner_hash_mismatch'

# Get-Command resolves the native .exe explicitly; npm shell shims are not run.
lookup=subprocess.run(['powershell.exe','-NoLogo','-NoProfile','-NonInteractive','-Command',"Get-Command codex.exe -All -ErrorAction SilentlyContinue | Where-Object { $_.CommandType -eq 'Application' } | Select-Object -First 1 -ExpandProperty Source"],cwd=ROOT,capture_output=True,text=True,encoding='utf-8',creationflags=getattr(subprocess,'CREATE_NO_WINDOW',0),timeout=15)
assert lookup.returncode==0,'native_codex_lookup_failed'
native_path=lookup.stdout.strip()
receipt={'stage':'T-P4-004','recorded_at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'scope':'Actual installed native Codex read-only version and exec --help probes through final delivered Runner --describe-tools. No model/authentication/task execution, login or network API invocation requested.','runner':{'path':str(runner),'sha256':digest(runner),'matches_public_delivery_manifest':True},'source':'Get-Command codex.exe; existing installed OpenAI Codex app native executable, not npm shell wrappers','native_codex_path':native_path,'private_context_loaded':False,'platform_tokens_used':False,'model_api_key_used':False,'platform_api_or_worker_started':False,'allowed_internal_cli_probes':[['--version'],['exec','--help']],'network_traffic_capture_performed':False,'scope_limits':['Installed/protocol-probed does not mean model authentication or health','No billed model request or coding task was run','No login/session resume was attempted','No OS-level network capture was performed']}
before_archive={p.name:digest(p) for p in ARCHIVE.iterdir() if p.is_file()}
if not native_path:
    receipt.update(available=False,reason='No installed native codex.exe was found; no installation attempted.')
    raw=b'';stderr=b''
else:
    native=Path(native_path).resolve()
    installed_root=Path(os.environ['LOCALAPPDATA'])/'OpenAI/Codex/bin'
    if not native.is_relative_to(installed_root.resolve()) or not native.is_file():
        receipt.update(available=False,reason='Resolved executable is outside the known installed OpenAI Codex app path; not executed.')
        raw=b'';stderr=b''
    else:
        before=digest(native)
        spec={'codex-readonly':{'executable':str(native),'adapter':'codex','protocol':'codex-jsonl-v1','args':['exec','--json','--sandbox','read-only','-'],'input':'goal','read_only':True}}
        config=STAGE/'installed-codex.commands.local.json'
        config.write_text(json.dumps(spec,indent=2)+'\n',encoding='utf-8')
        env={k:v for k,v in os.environ.items() if not k.startswith('SUMA_') and k not in ('OPENAI_API_KEY','OPENAI_ACCESS_TOKEN','CODEX_API_KEY')}
        command=[str(runner),'--describe-tools','--commands',str(config),'--allow-exec','--workspace',str(ROOT)]
        completed=subprocess.run(command,cwd=ROOT,env=env,capture_output=True,timeout=30,creationflags=getattr(subprocess,'CREATE_NO_WINDOW',0))
        raw=completed.stdout;stderr=completed.stderr
        after=digest(native)
        receipt.update(runner_describe_exit=completed.returncode,native_executable_sha256_before=before,native_executable_sha256_after=after,native_executable_unchanged=before==after,runner_arguments=['--describe-tools','--commands','[local command config; not archived]','--allow-exec','--workspace','[repository]'],local_config_archived=False,stderr_bytes=len(stderr),stderr_sha256=hashlib.sha256(stderr).hexdigest(),raw_process_stderr_archived=False)
        if completed.returncode==0:
            described=json.loads(raw.decode('utf-8-sig'))
            assert set(described)=={'execution_enabled','items'} and len(described['items'])==1,'unexpected_describe_shape'
            item=described['items'][0];profile=item['profile']
            assert set(item)=={'profile','available','reason'} and set(profile)=={'command_id','adapter','protocol','profile_sha256','executable_sha256','tool_version','read_only'},'unexpected_nonpublic_fields'
            assert profile['command_id']=='codex-readonly' and profile['adapter']=='codex' and profile['protocol']=='codex-jsonl-v1' and profile['read_only'] is True,'wrong_profile_identity'
            assert re.fullmatch(r'[a-f0-9]{64}',profile['profile_sha256']),'invalid_profile_digest'
            assert profile['executable_sha256']==before,'executable_profile_digest_mismatch'
            receipt.update(available=bool(item['available']) and before==after,profile=profile,reason=item['reason'],local_execution_opt_in_only=bool(described['execution_enabled']),model_authentication_verified=False,model_health_verified=False,model_task_execution_verified=False)
        else:
            receipt.update(available=False,reason='Delivered Runner describe failed; installation or model execution was not attempted.')

for label,value in [('describe',raw),('stderr',stderr)]:
    for pattern in [rb'-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----',rb'\bsk-[A-Za-z0-9_-]{20,}',rb'\bgh[pousr]_[A-Za-z0-9]{20,}',rb'Bearer\s+[A-Za-z0-9_.-]{12,}']:
        assert not re.search(pattern,value),'sensitive_output_pattern_detected_'+label

after_archive={p.name:digest(p) for p in ARCHIVE.iterdir() if p.is_file()}
assert before_archive==after_archive,'original_runtime_archive_changed'
receipt['original_runtime_archive_preserved_files']=len(before_archive)
receipt['original_runtime_archive_unchanged']=True
receipt['original_runtime_archive_manifest_sha256']=digest(ARCHIVE/'files-sha256.json')
(STAGE/'installed-codex-describe.json').write_bytes(raw)
receipt['describe_file']={'file':'installed-codex-describe.json','bytes':len(raw),'sha256':hashlib.sha256(raw).hexdigest()}
(STAGE/'installed-codex-verification.json').write_text(json.dumps(receipt,indent=2)+'\n',encoding='utf-8')
print(json.dumps({key:receipt.get(key) for key in ['available','native_codex_path','native_executable_sha256_before','runner_describe_exit','profile','reason','original_runtime_archive_unchanged']},ensure_ascii=True))
