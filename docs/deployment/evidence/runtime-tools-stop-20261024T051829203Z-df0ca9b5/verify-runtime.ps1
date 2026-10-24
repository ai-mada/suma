#requires -Version 7.0
[CmdletBinding()]
param(
  [string]$RepositoryRoot = 'D:/codes/ai-dev-os/suma',
  [Parameter(Mandatory)][string]$Stamp,
  [Parameter(Mandatory)][string]$AuditDirectory
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$repo = (Resolve-Path -LiteralPath $RepositoryRoot).Path
$audit = (Resolve-Path -LiteralPath $AuditDirectory).Path
$utf8 = [Text.UTF8Encoding]::new($false)
function Assert-True([bool]$Value, [string]$Message) { if (-not $Value) { throw $Message } }
function Get-Sha([string]$Path) { (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant() }
function Text-Sha([string]$Text) { [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($utf8.GetBytes($Text))).ToLowerInvariant() }
function Write-Json([string]$Path, [object]$Value) { [IO.File]::WriteAllText($Path, ($Value | ConvertTo-Json -Depth 18), $utf8) }
function Get-Sources {
  $roots = @('cmd/suma-control','cmd/suma-runner','cmd/suma-deployer','cmd/suma','internal')
  $paths = @((Join-Path $repo 'go.mod'),(Join-Path $repo 'go.sum')) + @($roots | ForEach-Object { Get-ChildItem -LiteralPath (Join-Path $repo $_) -File -Recurse -Filter '*.go' | Where-Object Name -NotLike '*_test.go' | ForEach-Object FullName })
  @($paths | Sort-Object -Unique | ForEach-Object { $f = Get-Item -LiteralPath $_; [ordered]@{ path=[IO.Path]::GetRelativePath($repo,$f.FullName).Replace('\','/'); size_bytes=$f.Length; sha256=Get-Sha $f.FullName } })
}
function Get-BinaryFormat([string]$Path, [string]$Target) {
  $stream = [IO.File]::OpenRead($Path)
  try { $header = [byte[]]::new(4096); $read = $stream.Read($header,0,$header.Length) } finally { $stream.Dispose() }
  Assert-True ($read -gt 64) 'Short executable header.'
  $parts = $Target.Split('-')
  if ($parts[0] -eq 'windows') {
    Assert-True ($header[0] -eq 0x4d -and $header[1] -eq 0x5a) 'Missing DOS executable header.'
    $pe = [BitConverter]::ToInt32($header,0x3c)
    Assert-True ($pe -gt 0 -and $pe+26 -lt $read -and [BitConverter]::ToUInt32($header,$pe) -eq 0x00004550) 'Missing PE signature.'
    $machine = [BitConverter]::ToUInt16($header,$pe+4)
    $expected = if ($parts[1] -eq 'amd64') { 0x8664 } else { 0xaa64 }
    Assert-True ($machine -eq $expected -and [BitConverter]::ToUInt16($header,$pe+24) -eq 0x20b) 'Wrong PE64 machine.'
    return [ordered]@{ format='PE32+'; architecture=$parts[1]; machine=('0x{0:x4}' -f $machine); target_matches=$true }
  }
  if ($parts[0] -eq 'linux') {
    Assert-True ($header[0] -eq 0x7f -and $header[1] -eq 0x45 -and $header[2] -eq 0x4c -and $header[3] -eq 0x46 -and $header[4] -eq 2 -and $header[5] -eq 1) 'Missing little-endian ELF64 header.'
    $machine = [BitConverter]::ToUInt16($header,18)
    $expected = if ($parts[1] -eq 'amd64') { 62 } else { 183 }
    Assert-True ($machine -eq $expected) 'Wrong ELF64 machine.'
    return [ordered]@{ format='ELF64'; architecture=$parts[1]; machine=$machine; target_matches=$true }
  }
  Assert-True ([BitConverter]::ToUInt32($header,0) -eq [uint32]4277009103) 'Missing Mach-O64 signature.'
  $machine = [BitConverter]::ToUInt32($header,4)
  $expected = if ($parts[1] -eq 'amd64') { 0x01000007 } else { 0x0100000c }
  Assert-True ($machine -eq $expected) 'Wrong Mach-O64 CPU.'
  return [ordered]@{ format='Mach-O64'; architecture=$parts[1]; machine=('0x{0:x8}' -f $machine); target_matches=$true }
}
$receiptDirectory = Join-Path $repo ('dist/runtime/builds/' + $Stamp)
$receipt = Get-Content -Raw -LiteralPath (Join-Path $receiptDirectory 'manifest.json') | ConvertFrom-Json
$source = @(Get-Content -Raw -LiteralPath (Join-Path $receiptDirectory 'source-manifest.json') | ConvertFrom-Json)
$linked = @(Get-Content -Raw -LiteralPath (Join-Path $receiptDirectory 'linked-modules.json') | ConvertFrom-Json)
$before = @(Get-Content -Raw -LiteralPath (Join-Path $audit 'source-before.json') | ConvertFrom-Json)
$history = @(Get-Content -Raw -LiteralPath (Join-Path $audit 'historical-files.json') | ConvertFrom-Json)
$prep = Get-Content -Raw -LiteralPath (Join-Path $audit 'preparation.json') | ConvertFrom-Json
$buildProcess = Get-Content -Raw -LiteralPath (Join-Path $audit 'build-process.json') | ConvertFrom-Json
$current = @(Get-Sources)
$sourceHash = Text-Sha ($current | ConvertTo-Json -Depth 5 -Compress)
Assert-True ($receipt.status -eq 'succeeded' -and $receipt.source_unchanged -and $buildProcess.exit_code -eq 0) 'Build/source freeze failure.'
Assert-True ($sourceHash -eq $receipt.source_sha256 -and $sourceHash -eq $prep.source_sha256) 'Frozen source hash mismatch.'
Assert-True (($current | ConvertTo-Json -Depth 5 -Compress) -ceq ($source | ConvertTo-Json -Depth 5 -Compress)) 'Current source manifest mismatch.'
Assert-True (($current | ConvertTo-Json -Depth 5 -Compress) -ceq ($before | ConvertTo-Json -Depth 5 -Compress)) 'Prebuild source manifest mismatch.'
Assert-True ((Get-Sha (Join-Path $repo 'scripts/build-runtime.ps1')) -eq $receipt.build_script_sha256) 'Original build script hash mismatch.'
Assert-True ($receipt.targets.Count -eq 6 -and $receipt.commands.Count -eq 50) 'Expected six targets and 50 Go calls.'
$deliveryEvidence = Join-Path $repo ('docs/deployment/evidence/runtime-tools-stop-' + $Stamp)
Assert-True (-not (Test-Path -LiteralPath $deliveryEvidence)) 'Evidence cut already exists; refusing overwrite.'
$evidence = Join-Path $audit ('verification-stage-' + [Guid]::NewGuid().ToString('N').Substring(0,8))
[void][IO.Directory]::CreateDirectory((Join-Path $evidence 'logs'))
[void][IO.Directory]::CreateDirectory((Join-Path $evidence 'independent-buildinfo'))
foreach ($name in @('manifest.json','source-manifest.json','linked-modules.json')) { [IO.File]::Copy((Join-Path $receiptDirectory $name),(Join-Path $evidence $name),$false) }
foreach ($name in @('historical-files.json','source-before.json','preparation.json','build-process.json','build-process.stdout.txt','build-process.stderr.txt','verify-runtime.ps1')) { [IO.File]::Copy((Join-Path $audit $name),(Join-Path $evidence $name),$false) }
[IO.File]::Copy((Join-Path $repo 'scripts/build-runtime.ps1'),(Join-Path $evidence 'build-runtime.original.ps1'),$false)
[IO.File]::Copy((Join-Path $repo '.cache/runtime-tools-stop-build.ps1'),(Join-Path $evidence 'invoke-build.ps1'),$false)
$go = (Get-Command go -CommandType Application | Select-Object -First 1).Source
$binaries = [Collections.Generic.List[object]]::new()
$targetSummary = [Collections.Generic.List[object]]::new()
$independent = [Collections.Generic.List[object]]::new()
$moduleUnion = @{}
foreach ($target in $receipt.targets) {
  Assert-True ($target.status -eq 'succeeded' -and $target.artifacts.Count -eq 4) ('Incomplete target: ' + $target.target)
  $directory = Join-Path $repo $target.directory
  $platform = Get-Content -Raw -LiteralPath (Join-Path $directory 'manifest.json') | ConvertFrom-Json
  Assert-True ($platform.stamp -eq $Stamp -and $platform.target -eq $target.target -and $platform.source_sha256 -eq $sourceHash) 'Target manifest mismatch.'
  $parts = $target.target.Split('-')
  $platformModules = @{}
  [long]$platformBytes = 0
  foreach ($artifact in $target.artifacts) {
    $path = Join-Path $directory $artifact.name
    $file = Get-Item -LiteralPath $path
    $hash = Get-Sha $path
    Assert-True ($file.Length -eq $artifact.size_bytes -and $file.Length -gt 0 -and $hash -eq $artifact.sha256) ('Delivered artifact differs: ' + $path)
    $recordedArtifact = @($platform.artifacts | Where-Object name -EQ $artifact.name)
    Assert-True ($recordedArtifact.Count -eq 1 -and $recordedArtifact[0].sha256 -eq $hash) 'Target artifact receipt mismatch.'
    $archivedInfo = [IO.File]::ReadAllText((Join-Path $directory $artifact.build_info_file))
    Assert-True ($archivedInfo -ceq $artifact.build_info) 'Archived binary info differs.'
    $start = [Diagnostics.ProcessStartInfo]::new()
    $start.FileName=$go; $start.WorkingDirectory=$repo; $start.UseShellExecute=$false; $start.CreateNoWindow=$true; $start.RedirectStandardOutput=$true; $start.RedirectStandardError=$true
    foreach ($argument in @('version','-m',$path)) { $start.ArgumentList.Add($argument) }
    foreach ($name in @('TEMP','TMP','TMPDIR','GOTMPDIR')) { $start.Environment[$name] = Join-Path $audit 'tmp' }
    foreach ($entry in @{GOCACHE=(Join-Path $repo '.cache/go-build');GOMODCACHE=(Join-Path $repo '.cache/go-mod-runtime');GOMAXPROCS='2';GOTOOLCHAIN='local';GOENV='off';GOWORK='off';GOPROXY='off';GOSUMDB='off';GOVCS='*:off';TEST_TELEMETRY_DIR=(Join-Path $repo ('.cache/runtime-build/'+$Stamp+'/telemetry'))}.GetEnumerator()) { $start.Environment[$entry.Key]=$entry.Value }
    $process=[Diagnostics.Process]::new(); $process.StartInfo=$start
    try { Assert-True $process.Start() 'Go metadata child start failed.'; $readOut=$process.StandardOutput.ReadToEndAsync(); $readErr=$process.StandardError.ReadToEndAsync(); $process.WaitForExit(); $stdout=$readOut.GetAwaiter().GetResult(); $stderr=$readErr.GetAwaiter().GetResult(); $code=$process.ExitCode } finally { $process.Dispose() }
    Assert-True ($code -eq 0 -and $stderr.Length -eq 0) 'Independent metadata inspection failed.'
    $infoPrefix = $target.target + '-' + $artifact.name
    [IO.File]::WriteAllText((Join-Path $evidence ('independent-buildinfo/'+$infoPrefix+'.stdout.txt')),$stdout,$utf8)
    [IO.File]::WriteAllText((Join-Path $evidence ('independent-buildinfo/'+$infoPrefix+'.stderr.txt')),$stderr,$utf8)
    $originalLines = @($archivedInfo -split '\r?\n' | Select-Object -Skip 1) -join "`n"
    $currentLines = @($stdout -split '\r?\n' | Select-Object -Skip 1) -join "`n"
    Assert-True ($originalLines -ceq $currentLines) 'Actual build metadata differs from original receipt.'
    foreach ($setting in @(('GOOS='+$parts[0]),('GOARCH='+$parts[1]),'CGO_ENABLED=0','-trimpath=true')) { Assert-True ($stdout -match ('(?m)^\s*build\s+'+[regex]::Escape($setting)+'\s*$')) ('Metadata setting mismatch: '+$setting) }
    $binaryFormat = Get-BinaryFormat $path $target.target
    foreach ($module in $artifact.linked_modules) { $key=$module|ConvertTo-Json -Depth 5 -Compress; $moduleUnion[$key]=$true; $platformModules[$key]=$true }
    $binaries.Add([ordered]@{target=$target.target;name=$artifact.name;path=[IO.Path]::GetRelativePath($repo,$path).Replace('\','/');absolute_path=$path;size_bytes=$file.Length;sha256=$hash;binary_format=$binaryFormat;independent_go_build_info_matches=$true})
    $independent.Add([ordered]@{arguments=@('version','-m',$path);exit_code=$code;stdout_file=('independent-buildinfo/'+$infoPrefix+'.stdout.txt');stderr_file=('independent-buildinfo/'+$infoPrefix+'.stderr.txt');build_metadata_matches_original=$true})
    $platformBytes += $file.Length
  }
  $targetSummary.Add([ordered]@{target=$target.target;artifacts=4;size_bytes=$platformBytes;linked_modules=$platformModules.Count;format=$binaryFormat.format})
}
Assert-True ($binaries.Count -eq 24 -and $independent.Count -eq 24) 'Expected 24 actual binaries and independent metadata reads.'
$historyUnchanged=0
$immutableHistory=0
$currentDocumentChanges=[Collections.Generic.List[object]]::new()
foreach ($file in $history) {
  $path=Join-Path $repo $file.path
  Assert-True (Test-Path -LiteralPath $path -PathType Leaf) ('Historical file missing: '+$file.path)
  $same=(Get-Item -LiteralPath $path).Length -eq $file.size_bytes -and (Get-Sha $path) -eq $file.sha256
  $immutable=$file.path.StartsWith('dist/runtime/') -or $file.path.StartsWith('docs/deployment/evidence/')
  if($immutable){$immutableHistory++;Assert-True $same ('Immutable historical file changed: '+$file.path)}
  if($same){$historyUnchanged++}else{
    Assert-True ($file.path.StartsWith('docs/deployment/') -and $file.path.EndsWith('.md')) ('Unexpected historical change: '+$file.path)
    $currentDocumentChanges.Add([ordered]@{path=$file.path;previous_size_bytes=$file.size_bytes;previous_sha256=$file.sha256;current_size_bytes=(Get-Item -LiteralPath $path).Length;current_sha256=Get-Sha $path;scope='Current deployment guide integration update by root; original immutable evidence and artifacts remain locked.'})
  }
}
Write-Json (Join-Path $evidence 'historical-preservation.json') ([ordered]@{recorded_at=[DateTime]::UtcNow.ToString('o');prebuild_snapshot_files=$history.Count;immutable_artifact_and_receipt_files_checked=$immutableHistory;immutable_files_unchanged=$true;all_snapshot_files_unchanged=$historyUnchanged;current_document_updates=@($currentDocumentChanges.ToArray());scope='Preserve the original prebuild snapshot. Only current Markdown deployment guides may differ during root integration; prior artifacts/evidence must remain byte-identical.'})
foreach ($oldStamp in $prep.existing_build_stamps) { Assert-True (Test-Path -LiteralPath (Join-Path $repo ('dist/runtime/builds/'+$oldStamp)) -PathType Container) 'Historical receipt directory absent.' }
$oldStamp='20261024T045219750Z-715ba2c8'
$oldLinked=@(Get-Content -Raw -LiteralPath (Join-Path $repo ('dist/runtime/builds/'+$oldStamp+'/linked-modules.json'))|ConvertFrom-Json)
$oldKeys=@($oldLinked|ForEach-Object{$_.module|ConvertTo-Json -Depth 5 -Compress}|Sort-Object)
$newKeys=@($linked|ForEach-Object{$_.module|ConvertTo-Json -Depth 5 -Compress}|Sort-Object)
$moduleSame=($oldKeys -join "`n") -ceq ($newKeys -join "`n")
Assert-True ((@($moduleUnion.Keys|Sort-Object) -join "`n") -ceq ($newKeys -join "`n")) 'Linked module union mismatch.'
$environment=[ordered]@{}
foreach ($target in @('User','Machine')) { $envItems=[Environment]::GetEnvironmentVariables($target); $canonical=@($envItems.Keys|Sort-Object|ForEach-Object{[ordered]@{name=$_;value=$envItems[$_]}})|ConvertTo-Json -Depth 4 -Compress; $hash=Text-Sha $canonical; $environment[$target]=[ordered]@{before_sha256=$prep.environment_sha256.$target;after_sha256=$hash;unchanged=$hash -eq $prep.environment_sha256.$target}; Assert-True $environment[$target].unchanged 'User/machine environment changed.' }
$outputs=[Collections.Generic.List[object]]::new(); $index=0
foreach ($command in $receipt.commands) {
  $index++; Assert-True ($command.exit_code -eq 0) 'Original Go call failed.'
  $stdoutPath=Join-Path $repo $command.stdout_log; $stderrPath=Join-Path $repo $command.stderr_log
  $stdout=[IO.File]::ReadAllText($stdoutPath);$stderr=[IO.File]::ReadAllText($stderrPath)
  Assert-True ($stderr.Length -eq 0) 'Original Go call wrote stderr.'
  $stdoutCopy='logs/'+[IO.Path]::GetFileName($stdoutPath);$stderrCopy='logs/'+[IO.Path]::GetFileName($stderrPath)
  [IO.File]::Copy($stdoutPath,(Join-Path $evidence $stdoutCopy),$false);[IO.File]::Copy($stderrPath,(Join-Path $evidence $stderrCopy),$false)
  $outputs.Add([ordered]@{index=$index;arguments=$command.arguments;exit_code=$command.exit_code;elapsed_seconds=$command.elapsed_seconds;original_stdout_log=$command.stdout_log;original_stderr_log=$command.stderr_log;archived_stdout_log=$stdoutCopy;archived_stderr_log=$stderrCopy;stdout_sha256=Get-Sha $stdoutPath;stderr_sha256=Get-Sha $stderrPath;stdout=$stdout;stderr=$stderr})
}
Write-Json (Join-Path $evidence 'go-tool-output.json') @($outputs.ToArray())
Write-Json (Join-Path $evidence 'independent-go-inspection.json') @($independent.ToArray())
Write-Json (Join-Path $evidence 'environment-verification.json') $environment
Write-Json (Join-Path $evidence 'linked-module-comparison.json') ([ordered]@{previous_stamp=$oldStamp;current_stamp=$Stamp;previous_module_count=$oldLinked.Count;current_module_count=$linked.Count;identities_versions_sums_replacements_unchanged=$moduleSame;entire_linked_manifest_byte_equal=(Get-Sha (Join-Path $repo ('dist/runtime/builds/'+$oldStamp+'/linked-modules.json'))) -eq (Get-Sha (Join-Path $receiptDirectory 'linked-modules.json'));added=@($newKeys|Where-Object{$_ -notin $oldKeys});removed=@($oldKeys|Where-Object{$_ -notin $newKeys})})
$oldSource=@(Get-Content -Raw -LiteralPath (Join-Path $repo ('dist/runtime/builds/'+$oldStamp+'/source-manifest.json'))|ConvertFrom-Json)
$oldMap=@{};foreach($item in $oldSource){$oldMap[$item.path]=$item}
$newMap=@{};foreach($item in $source){$newMap[$item.path]=$item}
$added=@($source|Where-Object{-not $oldMap.ContainsKey($_.path)})
$changed=@($source|Where-Object{$oldMap.ContainsKey($_.path) -and $oldMap[$_.path].sha256 -ne $_.sha256}|ForEach-Object{[ordered]@{path=$_.path;previous_sha256=$oldMap[$_.path].sha256;current_sha256=$_.sha256}})
$removed=@($oldSource|Where-Object{-not $newMap.ContainsKey($_.path)})
Write-Json (Join-Path $evidence 'cut-difference.json') ([ordered]@{previous_stamp=$oldStamp;current_stamp=$Stamp;previous_source_file_count=$oldSource.Count;current_source_file_count=$source.Count;previous_source_sha256='498300c9adffa8327982faa25da7d745d1a9ecb08f62265691d5673deacb318e';current_source_sha256=$sourceHash;added=$added;changed=$changed;removed=$removed})
$verification=[ordered]@{schema_version=1;recorded_at=[DateTime]::UtcNow.ToString('o');stamp=$Stamp;command=$buildProcess.command;exit_code=$buildProcess.exit_code;source_scope=$receipt.source_scope;source_sha256=$sourceHash;source_file_count=$source.Count;source_unchanged=$receipt.source_unchanged;current_sources_match=$true;prebuild_sources_match=$true;artifact_count=$binaries.Count;target_count=$receipt.targets.Count;original_go_tool_calls=$receipt.commands.Count;original_raw_stdout_stderr_pairs=$outputs.Count;independent_go_tool_calls=$independent.Count;historical_files_checked=$history.Count;historical_files_unchanged=$historyUnchanged;immutable_historical_files_unchanged=$immutableHistory;current_document_updates=@($currentDocumentChanges.ToArray());historical_build_stamps_preserved=$prep.existing_build_stamps;linked_module_count=$linked.Count;linked_module_identities_unchanged_from_previous_tool_cut=$moduleSame;user_machine_environment_unchanged=$true;target_summary=@($targetSummary.ToArray());binaries=@($binaries.ToArray());no_target_program_executed_by_delivery=$true;verification='Actual offline cross-compilation, independent host Go build-info inspection, executable headers, size/SHA-256, frozen runtime source, dependency identities, historical file hashes and user/machine environment hashes. No target program execution, installation, signing, foreign-system or production acceptance.'}
Write-Json (Join-Path $evidence 'verification.json') $verification
$hashes=@(Get-ChildItem -LiteralPath $evidence -File -Recurse|Sort-Object FullName|ForEach-Object{[ordered]@{name=[IO.Path]::GetRelativePath($evidence,$_.FullName).Replace('\','/');size_bytes=$_.Length;sha256=Get-Sha $_.FullName}})
Write-Json (Join-Path $evidence 'sha256.json') $hashes
[IO.Directory]::Move($evidence,$deliveryEvidence)
$evidence=$deliveryEvidence
[pscustomobject]@{evidence_directory=$evidence;stamp=$Stamp;artifacts=$binaries.Count;source_files=$source.Count;source_sha256=$sourceHash;historical_files_preserved=$historyUnchanged;immutable_files_preserved=$immutableHistory;linked_modules=$linked.Count;target_summary=$verification.target_summary;windows_control=$binaries[0];source_difference=@{added=$added.Count;changed=$changed.Count;removed=$removed.Count};evidence_sha256=@(Get-ChildItem -LiteralPath $evidence -File|Where-Object Name -In @('manifest.json','source-manifest.json','linked-modules.json','verification.json','sha256.json')|Sort-Object Name|ForEach-Object{[ordered]@{name=$_.Name;sha256=Get-Sha $_.FullName}})}|ConvertTo-Json -Depth 8
