param([string]$BuildScript = 'scripts/build-runtime.ps1')
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$taskScript = (Resolve-Path -LiteralPath $BuildScript).Path
$taskSource = Get-Content -LiteralPath $taskScript -Raw
$taskTokens = $null
$taskErrors = $null
[void][Management.Automation.Language.Parser]::ParseInput($taskSource, [ref]$taskTokens, [ref]$taskErrors)
if ($taskErrors.Count -ne 0) { throw ($taskErrors | Out-String) }
$taskPrefix = $taskSource.Substring(0, [regex]::Match($taskSource, '(?m)^try \{').Index)
$taskChecks = @'
$taskInventory = @(Get-SourceInventory)
$taskNames = @($taskPrograms | ForEach-Object name)
if (($taskNames -join ',') -ne 'suma-control,suma-runner,suma-deployer,suma') { throw 'Unexpected program set.' }
foreach ($taskProgram in $taskPrograms) {
    $taskEntry = $taskProgram.package.Substring(2) + '/main.go'
    if (@($taskInventory | Where-Object path -eq $taskEntry).Count -ne 1) { throw "Missing entry: $taskEntry" }
}
if (@($taskInventory | Where-Object { $_.path.StartsWith('internal/deployer/') }).Count -eq 0) { throw 'Missing provider implementation sources.' }
if (@($taskInventory | Where-Object { $_.path.StartsWith('internal/releasecontract/') }).Count -eq 0) { throw 'Missing shared release contract sources.' }
if (@($taskInventory | Where-Object { $_.path.EndsWith('_test.go') }).Count -ne 0) { throw 'Test file entered runtime source inventory.' }
if (@($taskInventory | Group-Object { $_.path } | Where-Object Count -gt 1).Count -ne 0) { throw 'Duplicate source paths.' }
foreach ($taskEntry in $taskInventory) {
    $taskActual = (Get-FileHash -LiteralPath (Join-Path $taskRepository $taskEntry.path) -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($taskActual -ne $taskEntry.sha256) { throw "Source digest mismatch: $($taskEntry.path)" }
}
[ordered]@{ syntax = 'passed'; programs = $taskNames; source_file_count = $taskInventory.Count; deployer_entry = 'cmd/suma-deployer/main.go'; shared_contract = 'internal/releasecontract'; result = 'passed' } | ConvertTo-Json -Depth 4
'@
$taskPrefix = $taskPrefix.Replace('$PSScriptRoot', "'" + (Join-Path (Get-Location) 'scripts').Replace("'", "''") + "'")
$taskPrefix = $taskPrefix.Replace('$PSCommandPath', "'" + $taskScript.Replace("'", "''") + "'")
& ([scriptblock]::Create($taskPrefix + "`n" + $taskChecks))
