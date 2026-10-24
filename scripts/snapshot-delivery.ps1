$ErrorActionPreference = 'Stop'
$taskRepository = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$taskOutput = 'docs/quality/evidence/source-sha256.json'
$taskFiles = git -C $taskRepository ls-files --cached --others --exclude-standard
if ($LASTEXITCODE -ne 0) { throw 'Cannot inventory the delivery repository.' }
$taskEntries = @()
foreach ($taskFile in ($taskFiles | Sort-Object -Unique)) {
    if ($taskFile -eq $taskOutput) { continue }
    $taskAbsolute = Join-Path $taskRepository $taskFile
    if (Test-Path -LiteralPath $taskAbsolute -PathType Leaf) {
        $taskInfo = Get-Item -LiteralPath $taskAbsolute
        $taskEntries += @{
            file = $taskFile
            bytes = $taskInfo.Length
            sha256 = (Get-FileHash -LiteralPath $taskAbsolute -Algorithm SHA256).Hash.ToLowerInvariant()
        }
    }
}
@{
    generated_at = [DateTime]::UtcNow.ToString('o')
    scope = 'Tracked and non-ignored delivery source/docs. Self excluded. Binaries have separate manifests.'
    files = $taskEntries
} | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath (Join-Path $taskRepository $taskOutput) -Encoding utf8
Write-Output "Inventoried $($taskEntries.Count) delivery source and evidence files."
