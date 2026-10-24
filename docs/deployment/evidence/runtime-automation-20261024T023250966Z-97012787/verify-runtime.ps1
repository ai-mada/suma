#requires -Version 7.0
[CmdletBinding()]
param(
    [string]$RepositoryRoot = 'D:/codes/ai-dev-os/suma',
    [string]$Stamp = '20261024T023250966Z-97012787',
    [string]$AuditDirectory = 'D:/codes/ai-dev-os/suma/.cache/runtime-automation-delivery/20261024T023234178Z-cf10f2cc'
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
    $roots = @('cmd/suma-control', 'cmd/suma-runner', 'cmd/suma-deployer', 'cmd/suma', 'internal')
    $paths = @((Join-Path $repo 'go.mod'), (Join-Path $repo 'go.sum')) + @($roots | ForEach-Object { Get-ChildItem -LiteralPath (Join-Path $repo $_) -File -Recurse -Filter '*.go' | Where-Object Name -NotLike '*_test.go' | ForEach-Object FullName })
    @($paths | Sort-Object -Unique | ForEach-Object { $f = Get-Item -LiteralPath $_; [ordered]@{ path = [IO.Path]::GetRelativePath($repo, $f.FullName).Replace('\', '/'); size_bytes = $f.Length; sha256 = Get-Sha $f.FullName } })
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
Assert-True ($receipt.status -eq 'succeeded' -and $receipt.source_unchanged -and $buildProcess.exit_code -eq 0) 'Build did not succeed with unchanged runtime source.'
Assert-True ($sourceHash -eq $receipt.source_sha256) 'Current runtime source hash differs from receipt.'
Assert-True (($current | ConvertTo-Json -Depth 5 -Compress) -ceq ($source | ConvertTo-Json -Depth 5 -Compress)) 'Current runtime inventory differs from archived source inventory.'
Assert-True (($current | ConvertTo-Json -Depth 5 -Compress) -ceq ($before | ConvertTo-Json -Depth 5 -Compress)) 'Runtime source changed after the pre-build snapshot.'
Assert-True ((Get-Sha (Join-Path $repo 'scripts/build-runtime.ps1')) -eq $receipt.build_script_sha256) 'Build script hash mismatch.'
Assert-True ($receipt.targets.Count -eq 6 -and $receipt.commands.Count -eq 50) 'Expected six targets and 50 Go commands.'
$binaries = [Collections.Generic.List[object]]::new()
$targetSummary = [Collections.Generic.List[object]]::new()
$moduleUnion = @{}
foreach ($target in $receipt.targets) {
    Assert-True ($target.status -eq 'succeeded' -and $target.artifacts.Count -eq 4) ('Target incomplete: ' + $target.target)
    $directory = Join-Path $repo $target.directory
    $platform = Get-Content -Raw -LiteralPath (Join-Path $directory 'manifest.json') | ConvertFrom-Json
    Assert-True ($platform.stamp -eq $Stamp -and $platform.target -eq $target.target -and $platform.source_sha256 -eq $sourceHash) 'Platform receipt mismatch.'
    $parts = $target.target.Split('-')
    $platformModules = @{}
    [long]$platformBytes = 0
    foreach ($artifact in $target.artifacts) {
        $path = Join-Path $directory $artifact.name
        $file = Get-Item -LiteralPath $path
        $hash = Get-Sha $path
        Assert-True ($file.Length -eq $artifact.size_bytes -and $file.Length -gt 0 -and $hash -eq $artifact.sha256) ('Delivered artifact mismatch: ' + $path)
        $recordedArtifact = @($platform.artifacts | Where-Object name -EQ $artifact.name)
        Assert-True ($recordedArtifact.Count -eq 1 -and $recordedArtifact[0].sha256 -eq $hash) 'Target manifest artifact mismatch.'
        $buildInfo = [IO.File]::ReadAllText((Join-Path $directory $artifact.build_info_file))
        Assert-True ($buildInfo -ceq $artifact.build_info) ('Build info archive mismatch: ' + $path)
        foreach ($setting in @(('GOOS=' + $parts[0]), ('GOARCH=' + $parts[1]), 'CGO_ENABLED=0', '-trimpath=true')) {
            Assert-True ($buildInfo -match ('(?m)^\s*build\s+' + [regex]::Escape($setting) + '\s*$')) ('Build setting mismatch: ' + $setting)
        }
        foreach ($module in $artifact.linked_modules) {
            $key = $module | ConvertTo-Json -Depth 5 -Compress
            $moduleUnion[$key] = $true
            $platformModules[$key] = $true
        }
        $binaries.Add([ordered]@{ target = $target.target; name = $artifact.name; path = [IO.Path]::GetRelativePath($repo, $path).Replace('\', '/'); size_bytes = $file.Length; sha256 = $hash; build_info_matches = $true })
        $platformBytes += $file.Length
    }
    $targetSummary.Add([ordered]@{ target = $target.target; artifacts = 4; size_bytes = $platformBytes; linked_modules = $platformModules.Count })
}
Assert-True ($binaries.Count -eq 24) 'Expected 24 delivered binaries.'
foreach ($file in $history) {
    $path = Join-Path $repo $file.path
    Assert-True ((Test-Path -LiteralPath $path -PathType Leaf) -and (Get-Item -LiteralPath $path).Length -eq $file.size_bytes -and (Get-Sha $path) -eq $file.sha256) ('Historical file changed: ' + $file.path)
}
foreach ($oldStamp in $prep.existing_build_stamps) { Assert-True (Test-Path -LiteralPath (Join-Path $repo ('dist/runtime/builds/' + $oldStamp)) -PathType Container) ('Historical receipt directory missing: ' + $oldStamp) }
$oldLinked = @(Get-Content -Raw -LiteralPath (Join-Path $repo 'dist/runtime/builds/20261024T022519852Z-2a1cdc66/linked-modules.json') | ConvertFrom-Json)
$oldKeys = @($oldLinked | ForEach-Object { $_.module | ConvertTo-Json -Depth 5 -Compress } | Sort-Object)
$newKeys = @($linked | ForEach-Object { $_.module | ConvertTo-Json -Depth 5 -Compress } | Sort-Object)
Assert-True ($linked.Count -eq 36 -and $moduleUnion.Count -eq 36) 'Expected 36 linked third-party modules.'
Assert-True (($oldKeys -join "`n") -ceq ($newKeys -join "`n")) 'Linked module identities differ from previous automation delivery.'
Assert-True ((@($moduleUnion.Keys | Sort-Object) -join "`n") -ceq ($newKeys -join "`n")) 'Linked module union differs from individual binary receipts.'
$environment = [ordered]@{}
foreach ($target in @('User', 'Machine')) {
    $envItems = [Environment]::GetEnvironmentVariables($target)
    $canonical = @($envItems.Keys | Sort-Object | ForEach-Object { [ordered]@{ name = $_; value = $envItems[$_] } }) | ConvertTo-Json -Depth 4 -Compress
    $hash = Text-Sha $canonical
    $environment[$target] = [ordered]@{ before_sha256 = $prep.environment_sha256.$target; after_sha256 = $hash; unchanged = $hash -eq $prep.environment_sha256.$target }
    Assert-True ($environment[$target].unchanged) ('Environment changed: ' + $target)
}
$evidence = Join-Path $repo ('docs/deployment/evidence/runtime-automation-' + $Stamp)
Assert-True (-not (Test-Path -LiteralPath $evidence)) 'Evidence directory already exists; refusing overwrite.'
[void][IO.Directory]::CreateDirectory((Join-Path $evidence 'logs'))
foreach ($name in @('manifest.json', 'source-manifest.json', 'linked-modules.json')) { [IO.File]::Copy((Join-Path $receiptDirectory $name), (Join-Path $evidence $name), $false) }
foreach ($name in @('historical-files.json', 'source-before.json', 'preparation.json', 'build-process.json', 'build-process.stdout.txt', 'build-process.stderr.txt', 'verify-runtime.ps1', 'preidle-runtime-bundles.original.md')) { [IO.File]::Copy((Join-Path $audit $name), (Join-Path $evidence $name), $false) }
$outputs = [Collections.Generic.List[object]]::new()
$index = 0
foreach ($command in $receipt.commands) {
    $index++
    Assert-True ($command.exit_code -eq 0) 'A Go child command failed.'
    $stdoutPath = Join-Path $repo $command.stdout_log
    $stderrPath = Join-Path $repo $command.stderr_log
    $stdout = [IO.File]::ReadAllText($stdoutPath)
    $stderr = [IO.File]::ReadAllText($stderrPath)
    Assert-True ($stderr.Length -eq 0) 'A Go child command wrote stderr.'
    $stdoutCopy = 'logs/' + [IO.Path]::GetFileName($stdoutPath)
    $stderrCopy = 'logs/' + [IO.Path]::GetFileName($stderrPath)
    [IO.File]::Copy($stdoutPath, (Join-Path $evidence $stdoutCopy), $false)
    [IO.File]::Copy($stderrPath, (Join-Path $evidence $stderrCopy), $false)
    $outputs.Add([ordered]@{ index = $index; arguments = $command.arguments; exit_code = $command.exit_code; elapsed_seconds = $command.elapsed_seconds; original_stdout_log = $command.stdout_log; original_stderr_log = $command.stderr_log; archived_stdout_log = $stdoutCopy; archived_stderr_log = $stderrCopy; stdout_sha256 = Get-Sha $stdoutPath; stderr_sha256 = Get-Sha $stderrPath; stdout = $stdout; stderr = $stderr })
}
Assert-True ($outputs.Count -eq 50 -and @(Get-ChildItem -LiteralPath (Join-Path $evidence 'logs') -File).Count -eq 100) 'Expected 50 stdout/stderr pairs.'
Write-Json (Join-Path $evidence 'go-tool-output.json') @($outputs.ToArray())
Write-Json (Join-Path $evidence 'environment-verification.json') $environment
Write-Json (Join-Path $evidence 'linked-module-comparison.json') ([ordered]@{ previous_stamp = '20261024T022519852Z-2a1cdc66'; current_stamp = $Stamp; previous_module_count = $oldLinked.Count; current_module_count = $linked.Count; identities_versions_sums_replacements_unchanged = $true; added = @(); removed = @() })
$oldSource = @(Get-Content -Raw -LiteralPath (Join-Path $repo 'dist/runtime/builds/20261024T022519852Z-2a1cdc66/source-manifest.json') | ConvertFrom-Json)
$oldMap = @{}; foreach ($item in $oldSource) { $oldMap[$item.path] = $item }
$newMap = @{}; foreach ($item in $source) { $newMap[$item.path] = $item }
$added = @($source | Where-Object { -not $oldMap.ContainsKey($_.path) })
$changed = @($source | Where-Object { $oldMap.ContainsKey($_.path) -and $oldMap[$_.path].sha256 -ne $_.sha256 } | ForEach-Object { [ordered]@{ path = $_.path; previous_sha256 = $oldMap[$_.path].sha256; current_sha256 = $_.sha256 } })
$removed = @($oldSource | Where-Object { -not $newMap.ContainsKey($_.path) })
Assert-True ($source.Count -eq 93 -and $oldSource.Count -eq 93 -and $added.Count -eq 0 -and $removed.Count -eq 0 -and ($changed.path -join ',') -ceq 'internal/platform/run_stream.go') 'Expected exactly the frozen idle-EOF event-stream source change.'
Write-Json (Join-Path $evidence 'cut-difference.json') ([ordered]@{ previous_stamp = '20261024T022519852Z-2a1cdc66'; current_stamp = $Stamp; previous_source_sha256 = 'bce32d56befd2866de29bccf3b321a942df28eb4f9bbd12b39264fbe12aee4cc'; current_source_sha256 = $sourceHash; added = $added; changed = $changed; removed = $removed })
$verification = [ordered]@{
    schema_version = 1; recorded_at = [DateTime]::UtcNow.ToString('o'); stamp = $Stamp; command = $buildProcess.command; exit_code = $buildProcess.exit_code
    artifact_count = $binaries.Count; target_count = $receipt.targets.Count; tool_invocations = $receipt.commands.Count; raw_stdout_stderr_pairs = $outputs.Count
    source_scope = $receipt.source_scope; source_sha256 = $sourceHash; source_file_count = $source.Count; source_unchanged = $receipt.source_unchanged; current_sources_match = $true; prebuild_sources_match = $true
    history_files_unchanged = $history.Count; historical_build_stamps_preserved = $prep.existing_build_stamps; linked_module_count = $linked.Count; linked_module_identities_unchanged_from_previous_automation = $true
    user_machine_environment_unchanged = $true; target_summary = @($targetSummary.ToArray()); binaries = @($binaries.ToArray())
    verification = 'Actual cross-compilation, host Go build-info inspection, delivered file size/SHA-256 and unchanged runtime source, module identities, historical file hashes and user/machine environment hashes only. No target program execution, installation, signing, foreign-platform deployment or production acceptance.'
}
Write-Json (Join-Path $evidence 'verification.json') $verification
$hashes = @(Get-ChildItem -LiteralPath $evidence -File -Recurse | Sort-Object FullName | ForEach-Object { [ordered]@{ name = [IO.Path]::GetRelativePath($evidence, $_.FullName).Replace('\', '/'); size_bytes = $_.Length; sha256 = Get-Sha $_.FullName } })
Write-Json (Join-Path $evidence 'sha256.json') $hashes
[pscustomobject]@{ evidence_directory = $evidence; stamp = $Stamp; artifacts = $binaries.Count; source_files = $source.Count; source_sha256 = $sourceHash; historical_files_preserved = $history.Count; linked_modules = $linked.Count; target_summary = $verification.target_summary; windows_control = $binaries[0]; evidence_sha256 = @(Get-ChildItem -LiteralPath $evidence -File | Where-Object Name -In @('manifest.json', 'source-manifest.json', 'linked-modules.json', 'verification.json', 'sha256.json') | Sort-Object Name | ForEach-Object { [ordered]@{ name = $_.Name; sha256 = Get-Sha $_.FullName } }) } | ConvertTo-Json -Depth 8
