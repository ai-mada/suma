#requires -Version 7.0
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$repo='D:/codes/ai-dev-os/suma'
$stamp='20261024T033010763Z-94b39f3b'
$audit=Join-Path $repo '.cache/runtime-ingress-delivery/20261024T033003962Z-b809f415'
$evidence=Join-Path $repo ('docs/deployment/evidence/runtime-ingress-'+$stamp)
$doc=Join-Path $repo 'docs/deployment/ingress-runtime-bundles.md'
$utf8=[Text.UTF8Encoding]::new($false)
function Assert-True([bool]$Value,[string]$Message){if(-not $Value){throw $Message}}
function Sha([string]$Path){(Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()}
function Write-Json([string]$Path,[object]$Value){Assert-True (-not(Test-Path -LiteralPath $Path)) ('Refusing existing evidence: '+$Path);[IO.File]::WriteAllText($Path,($Value|ConvertTo-Json -Depth 18),$utf8)}
$build=Get-Content -Raw -LiteralPath (Join-Path $evidence 'manifest.json')|ConvertFrom-Json
$verify=Get-Content -Raw -LiteralPath (Join-Path $evidence 'verification.json')|ConvertFrom-Json
$source=@(Get-Content -Raw -LiteralPath (Join-Path $evidence 'source-manifest.json')|ConvertFrom-Json)
$history=@(Get-Content -Raw -LiteralPath (Join-Path $evidence 'historical-files.json')|ConvertFrom-Json)
$goReceiptPath=Join-Path $repo 'docs/quality/evidence/ingress-go-verification.json'
$goReceipt=Get-Content -Raw -LiteralPath $goReceiptPath|ConvertFrom-Json
$goRawPath=Join-Path (Split-Path -Parent $goReceiptPath) $goReceipt.raw_file
Assert-True ((Sha $goRawPath) -eq $goReceipt.raw_sha256) 'Final Go raw evidence hash mismatch.'
$testSource=@{};foreach($file in $goReceipt.source_files){$testSource[$file.path]=$file.sha256}
$runtimeGo=@($source|Where-Object path -Like '*.go')
foreach($file in $runtimeGo){Assert-True ($testSource.ContainsKey($file.path) -and $testSource[$file.path] -eq $file.sha256) ('Test snapshot differs from runtime Go source: '+$file.path)}
$events=@([IO.File]::ReadLines($goRawPath)|ForEach-Object{$_|ConvertFrom-Json})
$passed=@($events|Where-Object{ $_.Action -eq 'pass' -and $_.PSObject.Properties['Test'] }).Count
$failed=@($events|Where-Object Action -EQ fail).Count
$skipped=@($events|Where-Object{ $_.Action -eq 'skip' -and $_.PSObject.Properties['Test'] }).Count
Assert-True ($passed -eq 385 -and $failed -eq 0 -and $skipped -eq 0 -and $goReceipt.test_exit -eq 0 -and $goReceipt.vet_exit -eq 0 -and $goReceipt.source_unchanged_since_start) 'Final Go evidence does not support stated results.'
Write-Json (Join-Path $evidence 'go-test-binding.json') ([ordered]@{schema_version=1;recorded_at=[DateTime]::UtcNow.ToString('o');runtime_source_sha256=$build.source_sha256;runtime_manifest_files=$source.Count;runtime_go_source_files=$runtimeGo.Count;runtime_go_files_match_final_test_snapshot=$true;test_snapshot_source_files=@($goReceipt.source_files).Count;source_receipt='docs/quality/evidence/ingress-go-verification.json';source_receipt_sha256=Sha $goReceiptPath;raw_file=('docs/quality/evidence/'+$goReceipt.raw_file);raw_sha256=Sha $goRawPath;actual_pass_events=$passed;actual_failed_events=$failed;actual_skipped_test_cases=$skipped;test_exit=$goReceipt.test_exit;vet_exit=$goReceipt.vet_exit;test_source_unchanged=$goReceipt.source_unchanged_since_start;not_in_test_source_manifest=@('go.mod','go.sum');scope='Read-only binding of the 99 runtime non-test Go files to the previously completed final Go test snapshot and raw result. Module-file hashes are covered by this runtime build receipt; final Go test receipt does not separately list those two files. No test rerun.'})
$stages=@(Get-ChildItem -LiteralPath $audit -Directory|Where-Object Name -Like 'verification-stage-*'|Select-Object -ExpandProperty FullName)
Write-Json (Join-Path $evidence 'verification-attempts.json') ([ordered]@{first_independent_attempt=[ordered]@{exit_code=1;error='Missing Mach-O64 signature.';cause='Audit PowerShell interpreted 0xfeedfacf as signed Int32 while the actual header was UInt32 4277009103. Runtime binaries and builder were not changed.';retained_partial_audit_directories=$stages};corrective_action='The independent audit script compares the explicit UInt32 value 4277009103. Actual Mach-O header bytes are CFFAEDFE.';final_independent_attempt=[ordered]@{exit_code=0;artifacts=24;source_unchanged=$true;receipts='verification.json and independent-go-inspection.json'};scope='Audit-script check correction only; the original 50-call cross-compilation succeeded once with unchanged runtime source.'})
foreach($file in $source){$path=Join-Path $repo $file.path;Assert-True ((Get-Item -LiteralPath $path).Length -eq $file.size_bytes -and (Sha $path) -eq $file.sha256) ('Current source changed: '+$file.path)}
foreach($file in $history){$path=Join-Path $repo $file.path;Assert-True ((Get-Item -LiteralPath $path).Length -eq $file.size_bytes -and (Sha $path) -eq $file.sha256) ('Historical file changed: '+$file.path)}
foreach($file in $verify.binaries){$path=Join-Path $repo $file.path;Assert-True ((Get-Item -LiteralPath $path).Length -eq $file.size_bytes -and (Sha $path) -eq $file.sha256) ('Binary changed: '+$file.path)}
$receiptCopies=0
foreach($name in @('manifest.json','source-manifest.json','linked-modules.json')){Assert-True ((Sha (Join-Path $evidence $name)) -eq (Sha (Join-Path $repo ('dist/runtime/builds/'+$stamp+'/'+$name)))) ('Receipt copy changed: '+$name);$receiptCopies++}
$rawPairs=0
foreach($command in $build.commands){foreach($key in @('stdout_log','stderr_log')){$original=Join-Path $repo $command.$key;$copy=Join-Path $evidence ('logs/'+[IO.Path]::GetFileName($original));Assert-True ((Sha $original) -eq (Sha $copy)) 'Original Go output copy changed.'};$rawPairs++}
$initialHashes=@(Get-Content -Raw -LiteralPath (Join-Path $evidence 'sha256.json')|ConvertFrom-Json)
foreach($file in $initialHashes){Assert-True ((Sha (Join-Path $evidence $file.name)) -eq $file.sha256) ('Initial archived evidence changed: '+$file.name)}
[IO.File]::Copy($PSCommandPath,(Join-Path $evidence 'final-check.ps1'),$false)
$docText=[IO.File]::ReadAllText($doc)
Assert-True ($docText -notmatch '(?m)[ \t]+\r?$') 'Document trailing whitespace.'
$linkPaths=@([regex]::Matches($docText,'\[[^\]]+\]\(([^)]+)\)')|ForEach-Object{$_.Groups[1].Value})
$pending=@('evidence/runtime-ingress-'+$stamp+'/final-verification.json','evidence/runtime-ingress-'+$stamp+'/sha256-final.json')
foreach($link in $linkPaths){if($link -in $pending){continue};Assert-True (Test-Path -LiteralPath (Join-Path (Split-Path -Parent $doc) $link) -PathType Leaf) ('Missing documentation link: '+$link)}
$win=@($verify.binaries|Where-Object{ $_.target -eq 'windows-amd64' -and $_.name -eq 'suma-control.exe' })[0]
Write-Json (Join-Path $evidence 'final-verification.json') ([ordered]@{schema_version=1;recorded_at=[DateTime]::UtcNow.ToString('o');stamp=$stamp;source_sha256=$build.source_sha256;runtime_source_files_rechecked=$source.Count;current_runtime_sources_match=$true;historical_files_rechecked=$history.Count;historical_files_unchanged=$true;historical_build_receipt_directories=11;artifacts_rechecked=@($verify.binaries).Count;independent_go_metadata_checks=24;original_receipt_copies_identical=$receiptCopies;original_stdout_stderr_pairs_identical=$rawPairs;initial_evidence_files_rechecked=$initialHashes.Count;documentation='docs/deployment/ingress-runtime-bundles.md';documentation_sha256=Sha $doc;documentation_links_valid=$true;documentation_link_count=$linkPaths.Count;go_test_runtime_source_binding=$true;passed_test_events=$passed;failed_test_events=$failed;skipped_test_cases=$skipped;windows_control_path=$win.absolute_path;windows_control_sha256=$win.sha256;no_target_program_started_or_stopped_by_delivery_agent=$true;no_binary_copied_to_documentation=$true;verification_scope='Offline cross-compilation, independent host Go metadata/header reads, source/file/receipt hashes, history preservation, documentation and existing final-test evidence binding. No new test execution or target runtime acceptance.'})
$allHashes=@(Get-ChildItem -LiteralPath $evidence -File -Recurse|Sort-Object FullName|ForEach-Object{[ordered]@{name=[IO.Path]::GetRelativePath($evidence,$_.FullName).Replace('\','/');size_bytes=$_.Length;sha256=Sha $_.FullName}})
Write-Json (Join-Path $evidence 'sha256-final.json') $allHashes
foreach($link in $linkPaths){Assert-True (Test-Path -LiteralPath (Join-Path (Split-Path -Parent $doc) $link) -PathType Leaf) ('Final missing documentation link: '+$link)}
foreach($file in $allHashes){Assert-True ((Get-Item -LiteralPath (Join-Path $evidence $file.name)).Length -eq $file.size_bytes -and (Sha (Join-Path $evidence $file.name)) -eq $file.sha256) ('Final evidence hash mismatch: '+$file.name)}
Assert-True (@(Get-ChildItem -LiteralPath $evidence -File -Recurse|Where-Object Extension -In @('.exe','.dll')).Count -eq 0) 'Unexpected binary in documentation.'
[ordered]@{status='verified';stamp=$stamp;source_sha256=$build.source_sha256;runtime_go_files_matched=$runtimeGo.Count;passed_test_events=$passed;failed_test_events=$failed;skipped_test_cases=$skipped;historical_files=$history.Count;binary_count=@($verify.binaries).Count;receipt_copies=$receiptCopies;raw_pairs=$rawPairs;documentation_links=$linkPaths.Count;final_evidence_files=@($allHashes).Count;final_sha256_manifest=Sha (Join-Path $evidence 'sha256-final.json');document_sha256=Sha $doc;windows_control=$win.absolute_path;windows_control_sha256=$win.sha256}|ConvertTo-Json -Depth 5
