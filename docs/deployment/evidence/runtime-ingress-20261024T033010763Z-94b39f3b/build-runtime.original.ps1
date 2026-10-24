#requires -Version 7.0
[CmdletBinding()]
param(
    [switch]$All,
    [string[]]$Targets = @(),
    [string]$ModuleCacheSeed = ''
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$taskRepository = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
$taskStamp = [DateTime]::UtcNow.ToString('yyyyMMddTHHmmssfffZ') + '-' + [Guid]::NewGuid().ToString('N').Substring(0, 8)
$taskCache = Join-Path $taskRepository '.cache'
$taskScratch = Join-Path $taskCache ('runtime-build/' + $taskStamp)
$taskLogs = Join-Path $taskScratch 'logs'
$taskTemp = Join-Path $taskScratch 'tmp'
$taskTelemetry = Join-Path $taskScratch 'telemetry'
$taskModuleCache = Join-Path $taskCache 'go-mod-runtime'
$taskBuildCache = Join-Path $taskCache 'go-build'
$taskDelivery = Join-Path $taskRepository 'dist/runtime'
$taskReceiptDirectory = Join-Path $taskDelivery ('builds/' + $taskStamp)
$taskUtf8 = [Text.UTF8Encoding]::new($false)
$taskCommands = [Collections.Generic.List[object]]::new()
$taskPlatformResults = [Collections.Generic.List[object]]::new()
$taskSupported = @('windows-amd64', 'windows-arm64', 'linux-amd64', 'linux-arm64', 'darwin-amd64', 'darwin-arm64')
$taskPrograms = @(
    [ordered]@{ name = 'suma-control'; package = './cmd/suma-control' },
    [ordered]@{ name = 'suma-runner'; package = './cmd/suma-runner' },
    [ordered]@{ name = 'suma-deployer'; package = './cmd/suma-deployer' },
    [ordered]@{ name = 'suma'; package = './cmd/suma' }
)
$taskSourceRoots = @($taskPrograms | ForEach-Object { $_.package.Substring(2) }) + @('internal')
$taskRecord = [ordered]@{
    schema_version = 1; stamp = $taskStamp; started_at = [DateTime]::UtcNow.ToString('o'); completed_at = $null
    status = 'in_progress'; repository = $taskRepository; toolchain = $null; host = $null
    programs = $taskPrograms
    source_scope = 'go.mod, go.sum, and non-test *.go files under ' + ($taskSourceRoots -join ', ') + '; no go:embed assets are currently used'
    source_sha256 = $null; source_unchanged = $false; targets = $taskPlatformResults; commands = $taskCommands
    module_cache_seed = $null; source_files = @(); linked_modules = @(); error = $null
    build_script_sha256 = (Get-FileHash -LiteralPath $PSCommandPath -Algorithm SHA256).Hash.ToLowerInvariant()
    build_settings = [ordered]@{ cgo_enabled = '0'; goamd64 = 'v1'; goarm64 = 'v8.0'; trimpath = $true; buildvcs = $false; module_mode = 'readonly'; dependency_network = 'off'; telemetry = 'off, repository-local directory' }
    verification = 'Cross-compilation and host Go build-info inspection only; not installation, launch, signing, deployment, or platform acceptance.'
}

function Write-JsonFile([string]$Path, [object]$Value) {
    [IO.File]::WriteAllText($Path, ($Value | ConvertTo-Json -Depth 18), $taskUtf8)
}

function Assert-Within([string]$Root, [string]$Path) {
    $rootFull = [IO.Path]::GetFullPath($Root).TrimEnd([IO.Path]::DirectorySeparatorChar) + [IO.Path]::DirectorySeparatorChar
    $pathFull = [IO.Path]::GetFullPath($Path)
    if (-not $pathFull.StartsWith($rootFull, [StringComparison]::OrdinalIgnoreCase)) {
        throw "Path is outside its exact build directory: $pathFull"
    }
}

function Invoke-GoChild([string[]]$Arguments, [hashtable]$Extra = @{}) {
    $index = $taskCommands.Count + 1
    $stdoutPath = Join-Path $taskLogs ('{0:D3}.stdout.txt' -f $index)
    $stderrPath = Join-Path $taskLogs ('{0:D3}.stderr.txt' -f $index)
    $start = [Diagnostics.ProcessStartInfo]::new()
    $start.FileName = $taskGo
    $start.WorkingDirectory = $taskRepository
    $start.UseShellExecute = $false
    $start.CreateNoWindow = $true
    $start.RedirectStandardOutput = $true
    $start.RedirectStandardError = $true
    foreach ($argument in $Arguments) { $start.ArgumentList.Add($argument) }
    foreach ($name in $taskEnvironment.Keys) { $start.Environment[$name] = $taskEnvironment[$name] }
    foreach ($name in $Extra.Keys) { $start.Environment[$name] = $Extra[$name] }
    $process = [Diagnostics.Process]::new()
    $process.StartInfo = $start
    $started = [DateTime]::UtcNow
    try {
        if (-not $process.Start()) { throw 'Go child process did not start.' }
        $readOut = $process.StandardOutput.ReadToEndAsync()
        $readErr = $process.StandardError.ReadToEndAsync()
        $process.WaitForExit()
        $stdout = $readOut.GetAwaiter().GetResult()
        $stderr = $readErr.GetAwaiter().GetResult()
        $code = $process.ExitCode
    } finally { $process.Dispose() }
    [IO.File]::WriteAllText($stdoutPath, $stdout, $taskUtf8)
    [IO.File]::WriteAllText($stderrPath, $stderr, $taskUtf8)
    $record = [ordered]@{
        arguments = $Arguments; exit_code = $code; elapsed_seconds = [Math]::Round(([DateTime]::UtcNow - $started).TotalSeconds, 3)
        stdout_log = [IO.Path]::GetRelativePath($taskRepository, $stdoutPath).Replace('\', '/')
        stderr_log = [IO.Path]::GetRelativePath($taskRepository, $stderrPath).Replace('\', '/')
    }
    $taskCommands.Add($record)
    return [pscustomobject]@{ ExitCode = $code; Stdout = $stdout; Stderr = $stderr; Record = $record }
}

function Require-GoSuccess([object]$Result, [string]$Action) {
    if ($Result.ExitCode -ne 0) {
        throw "$Action failed (exit $($Result.ExitCode)). $($Result.Stderr.Trim()) See $($Result.Record.stderr_log)"
    }
}

function Copy-ModuleSeed([string]$Source) {
    $sourceRoot = (Resolve-Path -LiteralPath $Source).Path
    if (-not (Test-Path -LiteralPath (Join-Path $sourceRoot 'cache/download') -PathType Container)) {
        throw 'ModuleCacheSeed must be an existing Go module cache containing cache/download.'
    }
    if ([IO.Path]::GetFullPath($sourceRoot) -eq [IO.Path]::GetFullPath($taskModuleCache)) { throw 'Module cache cannot seed itself.' }
    $pending = [Collections.Generic.Stack[string]]::new()
    $pending.Push($sourceRoot)
    $copied = 0
    $retained = 0
    while ($pending.Count -gt 0) {
        $directory = $pending.Pop()
        foreach ($item in (Get-ChildItem -LiteralPath $directory -Force)) {
            if ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) { throw "Linked module cache entry is unsupported: $($item.FullName)" }
            $relative = [IO.Path]::GetRelativePath($sourceRoot, $item.FullName)
            $destination = Join-Path $taskModuleCache $relative
            Assert-Within $taskModuleCache $destination
            if ($item.PSIsContainer) {
                [void][IO.Directory]::CreateDirectory($destination)
                $pending.Push($item.FullName)
            } elseif (-not $item.Name.EndsWith('.lock') -and -not $item.Name.EndsWith('.tmp')) {
                if ([IO.File]::Exists($destination)) { $retained++ } else { [IO.File]::Copy($item.FullName, $destination, $false); $copied++ }
            }
        }
    }
    return [ordered]@{ source = $sourceRoot; copied_files = $copied; retained_existing_files = $retained; overwrite = $false }
}

function Get-SourceInventory {
    $paths = [Collections.Generic.List[string]]::new()
    $paths.Add((Join-Path $taskRepository 'go.mod'))
    $paths.Add((Join-Path $taskRepository 'go.sum'))
    foreach ($root in $taskSourceRoots) {
        foreach ($directory in (Get-ChildItem -LiteralPath (Join-Path $taskRepository $root) -Directory -Recurse)) {
            if ($directory.Attributes -band [IO.FileAttributes]::ReparsePoint) { throw "Linked Go source directory is unsupported: $($directory.FullName)" }
        }
        foreach ($file in (Get-ChildItem -LiteralPath (Join-Path $taskRepository $root) -Filter '*.go' -File -Recurse)) {
            if ($file.Name.EndsWith('_test.go')) { continue }
            if ($file.Attributes -band [IO.FileAttributes]::ReparsePoint) { throw "Linked Go source is unsupported: $($file.FullName)" }
            if ([IO.File]::ReadAllText($file.FullName) -match '(?m)^\s*//go:embed\s') {
                throw 'go:embed was added; extend the explicit source inventory to cover its assets before building.'
            }
            $paths.Add($file.FullName)
        }
    }
    $entries = @($paths | Sort-Object -Unique | ForEach-Object {
        $info = Get-Item -LiteralPath $_
        [ordered]@{ path = [IO.Path]::GetRelativePath($taskRepository, $info.FullName).Replace('\', '/'); size_bytes = $info.Length; sha256 = (Get-FileHash -LiteralPath $info.FullName -Algorithm SHA256).Hash.ToLowerInvariant() }
    })
    return $entries
}

function Get-TextHash([string]$Text) {
    $hash = [Security.Cryptography.SHA256]::HashData($taskUtf8.GetBytes($Text))
    return [Convert]::ToHexString($hash).ToLowerInvariant()
}

function Read-BuildModules([string]$Text) {
    $modules = [Collections.Generic.List[object]]::new()
    $previous = $null
    foreach ($line in ($Text -split "`n")) {
        $parts = $line.Trim() -split '\s+'
        if ($parts.Count -ge 3 -and $parts[0] -eq 'dep') {
            $previous = [ordered]@{ path = $parts[1]; version = $parts[2]; sum = $(if ($parts.Count -ge 4) { $parts[3] } else { '' }); replace = $null }
            $modules.Add($previous)
        } elseif ($parts.Count -ge 2 -and $parts[0] -eq '=>') {
            if ($null -eq $previous) { throw 'Unexpected replacement without a linked dependency.' }
            $previous.replace = [ordered]@{ path = $parts[1]; version = $(if ($parts.Count -ge 3) { $parts[2] } else { '' }); sum = $(if ($parts.Count -ge 4) { $parts[3] } else { '' }) }
        }
    }
    return @($modules.ToArray())
}

try {
    if ($All -and $Targets.Count -gt 0) { throw 'Use either -All or -Targets, not both.' }
    foreach ($directory in @($taskScratch, $taskLogs, $taskTemp, $taskTelemetry, $taskModuleCache, $taskBuildCache, $taskReceiptDirectory)) { [void][IO.Directory]::CreateDirectory($directory) }
    # Go 1.24's telemetry shim honors this directory for both go and its child tools.
    [IO.File]::WriteAllText((Join-Path $taskTelemetry 'mode'), 'off', $taskUtf8)
    $taskGo = (Get-Command go -CommandType Application -ErrorAction Stop | Select-Object -First 1).Source
    $taskEnvironment = @{
        GOCACHE = $taskBuildCache; GOMODCACHE = $taskModuleCache; GOTMPDIR = $taskTemp; TEMP = $taskTemp; TMP = $taskTemp; TMPDIR = $taskTemp
        CGO_ENABLED = '0'; GOTOOLCHAIN = 'local'; GOWORK = 'off'; GOENV = 'off'; GOFLAGS = ''; GOEXPERIMENT = ''; GOCACHEPROG = ''
        TEST_TELEMETRY_DIR = $taskTelemetry; GO_TELEMETRY_CHILD = ''; GO_TELEMETRY_CHILD_UPLOAD = ''; GOAMD64 = 'v1'; GOARM64 = 'v8.0'
        GOPROXY = 'off'; GOSUMDB = 'off'; GOPRIVATE = ''; GONOPROXY = 'none'; GONOSUMDB = ''; GOINSECURE = ''; GOVCS = '*:off'; GOOS = ''; GOARCH = ''
    }
    $identity = Invoke-GoChild @('env', '-json', 'GOHOSTOS', 'GOHOSTARCH', 'GOVERSION')
    Require-GoSuccess $identity 'Go toolchain inspection'
    $hostInfo = $identity.Stdout | ConvertFrom-Json
    $taskRecord.toolchain = [ordered]@{ executable = $taskGo; version = $hostInfo.GOVERSION; automatic_download = $false }
    $taskRecord.host = [ordered]@{ os = $hostInfo.GOHOSTOS; arch = $hostInfo.GOHOSTARCH }
    $taskEnvironment.GOOS = $hostInfo.GOHOSTOS
    $taskEnvironment.GOARCH = $hostInfo.GOHOSTARCH
    $requested = @()
    if ($All) { $requested = $taskSupported } elseif ($Targets.Count -gt 0) { $requested = @($Targets | ForEach-Object { $_ -split ',' }) } else { $requested = @($hostInfo.GOHOSTOS + '-' + $hostInfo.GOHOSTARCH) }
    $selected = [Collections.Generic.List[string]]::new()
    foreach ($raw in $requested) {
        $target = $raw.Trim().ToLowerInvariant().Replace('/', '-')
        if ($target -notin $taskSupported) { throw "Unsupported target: $target. Use: $($taskSupported -join ', ')" }
        if (-not $selected.Contains($target)) { $selected.Add($target) }
    }
    if ($ModuleCacheSeed -ne '') {
        Write-Host 'Copying missing public module-cache files into the repository cache; existing files are retained.'
        $taskRecord.module_cache_seed = Copy-ModuleSeed $ModuleCacheSeed
    }
    $before = @(Get-SourceInventory)
    $sourceText = $before | ConvertTo-Json -Depth 5 -Compress
    $taskRecord.source_sha256 = Get-TextHash $sourceText
    $taskRecord.source_files = $before
    Write-JsonFile (Join-Path $taskReceiptDirectory 'source-manifest.json') $before
    $verify = Invoke-GoChild @('mod', 'verify')
    Require-GoSuccess $verify 'Offline module-cache verification'
    foreach ($target in $selected) {
        $parts = $target.Split('-')
        $stage = Join-Path $taskScratch ('targets/' + $target)
        [void][IO.Directory]::CreateDirectory($stage)
        $platform = [ordered]@{ target = $target; status = 'building'; directory = $null; artifacts = @(); error = $null }
        $taskPlatformResults.Add($platform)
        try {
            $artifacts = [Collections.Generic.List[object]]::new()
            foreach ($program in $taskPrograms) {
                $name = $program.name + $(if ($parts[0] -eq 'windows') { '.exe' } else { '' })
                $binary = Join-Path $stage $name
                Write-Host "Building $target / $($program.name)"
                $compiled = Invoke-GoChild @('build', '-trimpath', '-buildvcs=false', '-mod=readonly', '-o', $binary, $program.package) @{ GOOS = $parts[0]; GOARCH = $parts[1]; CGO_ENABLED = '0' }
                Require-GoSuccess $compiled "$target / $($program.name)"
                if (-not [IO.File]::Exists($binary) -or (Get-Item -LiteralPath $binary).Length -le 0) { throw 'Go reported success without a fresh nonempty binary.' }
                $buildInfo = Invoke-GoChild @('version', '-m', $binary)
                Require-GoSuccess $buildInfo "$target / $($program.name) build-info inspection"
                foreach ($setting in @(('GOOS=' + $parts[0]), ('GOARCH=' + $parts[1]), 'CGO_ENABLED=0')) {
                    if ($buildInfo.Stdout -notmatch ('(?m)^\s*build\s+' + [regex]::Escape($setting) + '\s*$')) { throw "Binary build information does not prove $setting" }
                }
                $infoName = $name + '.buildinfo.txt'
                [IO.File]::WriteAllText((Join-Path $stage $infoName), $buildInfo.Stdout, $taskUtf8)
                $artifacts.Add([ordered]@{
                    name = $name; package = $program.package; size_bytes = (Get-Item -LiteralPath $binary).Length
                    sha256 = (Get-FileHash -LiteralPath $binary -Algorithm SHA256).Hash.ToLowerInvariant()
                    toolchain = $hostInfo.GOVERSION; build_info_file = $infoName; build_info = $buildInfo.Stdout
                    linked_modules = @(Read-BuildModules $buildInfo.Stdout)
                })
            }
            $platform.artifacts = @($artifacts.ToArray())
            $platform.status = 'compiled'
        } catch { $platform.status = 'failed'; $platform.error = $_.Exception.Message; Write-Warning $platform.error }
    }
    $after = @(Get-SourceInventory)
    $taskRecord.source_unchanged = (Get-TextHash ($after | ConvertTo-Json -Depth 5 -Compress)) -eq $taskRecord.source_sha256
    if (-not $taskRecord.source_unchanged) { throw 'Runtime Go sources changed during this build; no platform is published. Staging and receipts are retained.' }
    $union = @{}
    foreach ($platform in $taskPlatformResults) {
        if ($platform.status -ne 'compiled') { continue }
        foreach ($artifact in $platform.artifacts) {
            foreach ($module in $artifact.linked_modules) {
                $moduleKey = ($module | ConvertTo-Json -Depth 5 -Compress)
                if (-not $union.ContainsKey($moduleKey)) { $union[$moduleKey] = [ordered]@{ module = $module; targets = [Collections.Generic.List[string]]::new(); binaries = [Collections.Generic.List[string]]::new() } }
                if (-not $union[$moduleKey].targets.Contains($platform.target)) { $union[$moduleKey].targets.Add($platform.target) }
                $label = $platform.target + '/' + $artifact.name
                if (-not $union[$moduleKey].binaries.Contains($label)) { $union[$moduleKey].binaries.Add($label) }
            }
        }
        $parent = Join-Path $taskDelivery $platform.target
        $destination = Join-Path $parent $taskStamp
        $stage = Join-Path $taskScratch ('targets/' + $platform.target)
        Assert-Within (Join-Path $taskScratch 'targets') $stage
        Assert-Within $parent $destination
        [void][IO.Directory]::CreateDirectory($parent)
        if ([IO.Directory]::Exists($destination)) { throw 'Unique delivery directory already exists; refusing overwrite.' }
        Write-JsonFile (Join-Path $stage 'manifest.json') ([ordered]@{ schema_version = 1; stamp = $taskStamp; target = $platform.target; status = 'succeeded'; source_sha256 = $taskRecord.source_sha256; source_manifest = '../../builds/' + $taskStamp + '/source-manifest.json'; toolchain = $taskRecord.toolchain; artifacts = $platform.artifacts; verification = $taskRecord.verification })
        [IO.Directory]::Move($stage, $destination)
        $platform.directory = [IO.Path]::GetRelativePath($taskRepository, $destination).Replace('\', '/')
        $platform.status = 'succeeded'
    }
    $taskRecord.linked_modules = @($union.Values | Sort-Object { $_.module.path }, { $_.module.version })
    Write-JsonFile (Join-Path $taskReceiptDirectory 'linked-modules.json') $taskRecord.linked_modules
    $failed = @($taskPlatformResults | Where-Object status -ne 'succeeded')
    $taskRecord.status = $(if ($failed.Count -eq 0) { 'succeeded' } else { 'partial_failure' })
    $taskRecord.completed_at = [DateTime]::UtcNow.ToString('o')
    Write-JsonFile (Join-Path $taskReceiptDirectory 'manifest.json') $taskRecord
    Write-Host "Runtime receipt: $taskReceiptDirectory"
    if ($failed.Count -gt 0) { exit 1 }
    exit 0
} catch {
    $taskRecord.status = 'failed'
    $taskRecord.error = $_.Exception.Message
    $taskRecord.completed_at = [DateTime]::UtcNow.ToString('o')
    if ([IO.Directory]::Exists($taskReceiptDirectory)) { Write-JsonFile (Join-Path $taskReceiptDirectory 'manifest.json') $taskRecord }
    [Console]::Error.WriteLine($taskRecord.error)
    exit 1
}
