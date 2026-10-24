#requires -Version 7.0
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$repo = 'D:/codes/ai-dev-os/suma'
$audit = Join-Path $repo ('.cache/runtime-ingress-delivery/' + [DateTime]::UtcNow.ToString('yyyyMMddTHHmmssfffZ') + '-' + [Guid]::NewGuid().ToString('N').Substring(0,8))
$utf8 = [Text.UTF8Encoding]::new($false)
[void][IO.Directory]::CreateDirectory((Join-Path $audit 'tmp'))
function Get-Sha([string]$Path) { (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant() }
function Text-Sha([string]$Text) { [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($utf8.GetBytes($Text))).ToLowerInvariant() }
function Write-Json([string]$Path, [object]$Value) { [IO.File]::WriteAllText($Path, ($Value | ConvertTo-Json -Depth 18), $utf8) }
$roots = @('cmd/suma-control','cmd/suma-runner','cmd/suma-deployer','cmd/suma','internal')
$sourcePaths = @((Join-Path $repo 'go.mod'),(Join-Path $repo 'go.sum')) + @($roots | ForEach-Object { Get-ChildItem -LiteralPath (Join-Path $repo $_) -File -Recurse -Filter '*.go' | Where-Object Name -NotLike '*_test.go' | ForEach-Object FullName })
$source = @($sourcePaths | Sort-Object -Unique | ForEach-Object { $f = Get-Item -LiteralPath $_; [ordered]@{ path = [IO.Path]::GetRelativePath($repo,$f.FullName).Replace('\','/'); size_bytes = $f.Length; sha256 = Get-Sha $f.FullName } })
$freeze = Text-Sha ($source | ConvertTo-Json -Depth 5 -Compress)
Write-Json (Join-Path $audit 'source-before.json') $source
$history = @(@('dist/runtime','docs/deployment') | ForEach-Object { Get-ChildItem -LiteralPath (Join-Path $repo $_) -File -Recurse } | Sort-Object FullName -Unique | ForEach-Object { [ordered]@{ path = [IO.Path]::GetRelativePath($repo,$_.FullName).Replace('\','/'); size_bytes = $_.Length; sha256 = Get-Sha $_.FullName } })
Write-Json (Join-Path $audit 'historical-files.json') $history
$environment = [ordered]@{}
foreach ($target in @('User','Machine')) { $envItems = [Environment]::GetEnvironmentVariables($target); $canonical = @($envItems.Keys | Sort-Object | ForEach-Object { [ordered]@{ name = $_; value = $envItems[$_] } }) | ConvertTo-Json -Depth 4 -Compress; $environment[$target] = Text-Sha $canonical }
$existing = @(Get-ChildItem -LiteralPath (Join-Path $repo 'dist/runtime/builds') -Directory | ForEach-Object Name | Sort-Object)
Write-Json (Join-Path $audit 'preparation.json') ([ordered]@{ recorded_at = [DateTime]::UtcNow.ToString('o'); repository = $repo; audit_directory = $audit; source_sha256 = $freeze; source_file_count = $source.Count; historical_file_count = $history.Count; existing_build_stamps = $existing; environment_sha256 = $environment; build_script_sha256 = Get-Sha (Join-Path $repo 'scripts/build-runtime.ps1'); process_environment_overrides = [ordered]@{ GOMAXPROCS='2'; TEMP=(Join-Path $audit 'tmp'); TMP=(Join-Path $audit 'tmp') } })
Write-Output ('PREPARED ' + (@{ audit_directory=$audit; source_sha256=$freeze; source_file_count=$source.Count; historical_file_count=$history.Count } | ConvertTo-Json -Compress))
$start = [Diagnostics.ProcessStartInfo]::new()
$start.FileName = (Get-Command pwsh -CommandType Application | Select-Object -First 1).Source
$start.WorkingDirectory = $repo
$start.UseShellExecute = $false
$start.CreateNoWindow = $true
$start.RedirectStandardOutput = $true
$start.RedirectStandardError = $true
foreach ($argument in @('-NoLogo','-NoProfile','-File',(Join-Path $repo 'scripts/build-runtime.ps1'),'-All')) { $start.ArgumentList.Add($argument) }
$start.Environment['GOMAXPROCS'] = '2'
$start.Environment['TEMP'] = Join-Path $audit 'tmp'
$start.Environment['TMP'] = Join-Path $audit 'tmp'
$process = [Diagnostics.Process]::new()
$process.StartInfo = $start
$started = [DateTime]::UtcNow
try { if (-not $process.Start()) { throw 'Build process failed to start.' }; $readOut = $process.StandardOutput.ReadToEndAsync(); $readErr = $process.StandardError.ReadToEndAsync(); $process.WaitForExit(); $stdout = $readOut.GetAwaiter().GetResult(); $stderr = $readErr.GetAwaiter().GetResult(); $code = $process.ExitCode } finally { $process.Dispose() }
[IO.File]::WriteAllText((Join-Path $audit 'build-process.stdout.txt'),$stdout,$utf8)
[IO.File]::WriteAllText((Join-Path $audit 'build-process.stderr.txt'),$stderr,$utf8)
$newStamps = @(Get-ChildItem -LiteralPath (Join-Path $repo 'dist/runtime/builds') -Directory | Where-Object { $_.Name -notin $existing } | ForEach-Object Name)
Write-Json (Join-Path $audit 'build-process.json') ([ordered]@{ command = 'pwsh -NoLogo -NoProfile -File ./scripts/build-runtime.ps1 -All'; started_at=$started.ToString('o'); completed_at=[DateTime]::UtcNow.ToString('o'); exit_code=$code; process_local_GOMAXPROCS='2'; new_stamps=$newStamps; stdout_file='build-process.stdout.txt'; stderr_file='build-process.stderr.txt' })
Write-Output $stdout
if ($stderr.Length -gt 0) { [Console]::Error.WriteLine($stderr) }
Write-Output ('COMPLETED ' + (@{ audit_directory=$audit; exit_code=$code; source_sha256=$freeze; new_stamps=$newStamps } | ConvertTo-Json -Compress))
exit $code
