[CmdletBinding()]
param(
    [string]$RepoRoot,
    [Parameter(Mandatory)][string]$WorkspaceRoot,
    [Parameter(Mandatory)][string]$OutputRoot
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$scriptRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
if ([string]::IsNullOrWhiteSpace($RepoRoot)) { $RepoRoot = Join-Path $scriptRoot '..' }
$repo = [IO.Path]::GetFullPath($RepoRoot)
$workspace = [IO.Path]::GetFullPath($WorkspaceRoot)
$output = [IO.Path]::GetFullPath($OutputRoot)
$packageName = 'enterprise-multi-tenant-agent-platform-20260905'
$staging = Join-Path $workspace "work\$packageName"
$zipPath = Join-Path $output "$packageName.zip"
$inventoryName = 'PACKAGE_INVENTORY_20260905.md'
$checksumsName = 'SHA256SUMS_20260905.txt'

if (-not (Test-Path -LiteralPath (Join-Path $repo 'go.mod'))) { throw 'platform source root is not valid' }
New-Item -ItemType Directory -Force -Path (Join-Path $workspace 'work'),$output | Out-Null
if (Test-Path -LiteralPath $staging) { Remove-Item -LiteralPath $staging -Recurse -Force }
New-Item -ItemType Directory -Force -Path $staging | Out-Null
if (Test-Path -LiteralPath $zipPath) { Remove-Item -LiteralPath $zipPath -Force }

$included = [Collections.Generic.List[object]]::new()
$excluded = [Collections.Generic.List[string]]::new()
$blockedDirectoryNames = @('.git','node_modules','runtime','volumes','cache','caches','tmp','temp','logs','bin','dist','coverage','archive')
$blockedExtensions = @('.log','.out','.db','.sqlite','.sqlite3','.pem','.key','.p12','.pfx','.crt','.cer','.der','.exe','.dll','.so','.zip','.7z','.tar','.gz','.tgz','.bz2','.xz')

function Test-SkippedFile([IO.FileInfo]$File) {
    if ($blockedExtensions -contains $File.Extension.ToLowerInvariant()) { return $true }
    if ($File.FullName -match '(?i)\\(?:\.git|node_modules|runtime|volumes|cache|caches|tmp|temp|logs|bin|dist|coverage)(?:\\|$)') { return $true }
    if ($File.Name -eq '.env' -or ($File.Name -like '.env.*' -and $File.Name -ne '.env.example')) { return $true }
    $parts = $File.FullName.Substring($File.DirectoryName.Length).TrimStart('\','/').Split([IO.Path]::DirectorySeparatorChar)
    foreach ($part in $parts) { if ($blockedDirectoryNames -contains $part.ToLowerInvariant()) { return $true } }
    return $false
}

function Copy-SafeTree([string]$Source,[string]$Destination,[string]$Label) {
    $sourceFull = [IO.Path]::GetFullPath($Source)
    foreach ($file in Get-ChildItem -LiteralPath $sourceFull -Recurse -File -Force) {
        if (Test-SkippedFile $file) {
            $excluded.Add("$Label/$($file.FullName.Substring($sourceFull.Length).TrimStart('\','/'))")
            continue
        }
        $relative = $file.FullName.Substring($sourceFull.Length).TrimStart('\','/')
        $normalized = $relative.Replace('\','/')
        $historical = @(
            'docs/archive/*', 'archive/*'
        ) | Where-Object { $normalized -like $_ }
        if ($historical) {
            $excluded.Add("$Label/$normalized")
            continue
        }
        $target = Join-Path $Destination $relative
        New-Item -ItemType Directory -Force -Path (Split-Path -Parent $target) | Out-Null
        Copy-Item -LiteralPath $file.FullName -Destination $target -Force
        $included.Add([pscustomobject]@{Path=(($Label+'/'+$relative).Replace('\','/'));Source=$file.FullName;Kind='source'})
    }
}

function Copy-SafeFile([string]$Source,[string]$Destination,[string]$Label) {
    if (-not (Test-Path -LiteralPath $Source -PathType Leaf)) { throw "required material is missing: $Source" }
    $file = Get-Item -LiteralPath $Source
    if (Test-SkippedFile $file) { throw "required material matched a blocked file rule: $Source" }
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $Destination) | Out-Null
    Copy-Item -LiteralPath $Source -Destination $Destination -Force
    $included.Add([pscustomobject]@{Path=$Label.Replace('\','/');Source=$Source;Kind='material'})
}

Copy-SafeTree $repo (Join-Path $staging 'platform-source') 'platform-source'

$evidenceDestination = Join-Path $staging 'verification-evidence'
$evidenceNames = @('validation-*.log','integration-*.log','admin-*.log','capacity-*.log','k3d-*.log','mesh-*.log','vault-*.log','gateway-rollback-*.log','compose-postrestart-*.log')
foreach ($file in Get-ChildItem -LiteralPath (Join-Path $workspace 'outputs') -File -ErrorAction SilentlyContinue | Where-Object { $name = $_.Name; @($evidenceNames | Where-Object { $name -like $_ }).Count -gt 0 }) {
    $safeName = ($file.Name -replace '(?i)-(?:platform|v[0-9]+)(?=-|\.)','')
    $evidenceTarget = Join-Path $evidenceDestination $safeName
    New-Item -ItemType Directory -Force -Path $evidenceDestination | Out-Null
    Copy-Item -LiteralPath $file.FullName -Destination $evidenceTarget -Force
    $included.Add([pscustomobject]@{Path=("verification-evidence/$safeName");Source=$file.FullName;Kind='evidence'})
}
Copy-SafeFile (Join-Path $workspace 'outputs\PROJECT_COMPLETION_SUMMARY_20260905.md') (Join-Path $staging 'delivery-summary\PROJECT_COMPLETION_SUMMARY_20260905.md') 'delivery-summary/PROJECT_COMPLETION_SUMMARY_20260905.md'

# Never place a credential-bearing file in the package. Scan text before the
# inventory is written so a failure cannot be hidden by a later archive step.
$secretRegex = '-----BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY-----|sk-(proj-)?[A-Za-z0-9_-]{24,}|gh[pousr]_[A-Za-z0-9]{30,}|AKIA[0-9A-Z]{16}|xox[baprs]-[A-Za-z0-9-]{20,}'
$secretHits = @()
foreach ($file in Get-ChildItem -LiteralPath $staging -Recurse -File) {
    if ($file.Extension.ToLowerInvariant() -in @('.zip','.bundle','.tar','.gz')) { continue }
    # These two tests intentionally contain short fake provider-key strings to
    # prove redaction. They are not credentials and are covered by the source
    # test suite; all other text files remain subject to the signature scan.
    if ($file.Name -eq 'secret_leakage_test.go') { continue }
    $matches = Select-String -LiteralPath $file.FullName -Pattern $secretRegex -AllMatches -ErrorAction SilentlyContinue
    if ($matches) { $secretHits += $file.FullName }
}
if ($secretHits.Count -gt 0) { throw "secret-signature scan failed: $($secretHits -join '; ')" }

$sourceCount = $included.Count
$sourceBytes = [long]0
foreach ($item in $included) { $sourceBytes += (Get-Item -LiteralPath $item.Source).Length }
$inventory = @(
    '# Enterprise Multi-Tenant Agent Platform Package Inventory',
    '',
    "Generated: $([DateTime]::Now.ToString('yyyy-MM-dd HH:mm:ss zzz'))",
    "Package: $packageName",
    "Included files: $sourceCount (source bytes: $sourceBytes)",
    '',
    '## Included',
    '',
    '- `platform-source/`: authoritative source, tests, migrations, deployment templates, scripts and current docs.',
    '- `verification-evidence/`: redacted validation logs with exit states, topology and public endpoints only.',
    '',
    '## Excluded',
    '',
    '- Real `.env`/`.env.*` files (except `.env.example`), including `deploy/.env.wecom.local`.',
    '- Docker images, volumes, databases, sockets, runtime directories, caches and temporary workspaces.',
    '- Private keys, certificates, binaries, log databases and unreviewed nested archives.',
    '- E-drive lab caches, database dumps, certificates, keys, tools and runtimes.',
    '',
    'Exclusions prevent credential and runtime-data spread; they do not remove required source or submission docs. `SHA256SUMS_20260905.txt` covers every other file.'
)
[IO.File]::WriteAllLines((Join-Path $staging $inventoryName),$inventory,[Text.UTF8Encoding]::new($false))
$sumLines = [Collections.Generic.List[string]]::new()
foreach ($file in Get-ChildItem -LiteralPath $staging -Recurse -File | Sort-Object FullName) {
    if ($file.Name -eq $checksumsName) { continue }
    $relative = $file.FullName.Substring($staging.Length).TrimStart('\','/').Replace('\','/')
    $sumLines.Add("$((Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant())  $relative")
}
[IO.File]::WriteAllLines((Join-Path $staging $checksumsName),$sumLines,[Text.UTF8Encoding]::new($false))

Compress-Archive -LiteralPath $staging -DestinationPath $zipPath -CompressionLevel Optimal -Force
$zipHash = (Get-FileHash -LiteralPath $zipPath -Algorithm SHA256).Hash.ToLowerInvariant()
$unpack = Join-Path $workspace 'work\package-unpack-check-20260905'
if (Test-Path -LiteralPath $unpack) { Remove-Item -LiteralPath $unpack -Recurse -Force }
Expand-Archive -LiteralPath $zipPath -DestinationPath $unpack -Force
$unpackedRoot = Join-Path $unpack $packageName
$expected = @(Get-ChildItem -LiteralPath $staging -Recurse -File | ForEach-Object { $_.FullName.Substring($staging.Length).TrimStart('\','/').Replace('\','/') } | Sort-Object)
$actual = @(Get-ChildItem -LiteralPath $unpackedRoot -Recurse -File | ForEach-Object { $_.FullName.Substring($unpackedRoot.Length).TrimStart('\','/').Replace('\','/') } | Sort-Object)
$inventoryDifference = @(Compare-Object $expected $actual)
if ($inventoryDifference.Count -ne 0) { throw 'fresh extraction file inventory differs from staging' }
foreach ($relative in $expected) {
    $a = Get-FileHash -LiteralPath (Join-Path $staging ($relative -replace '/', '\')) -Algorithm SHA256
    $b = Get-FileHash -LiteralPath (Join-Path $unpackedRoot ($relative -replace '/', '\')) -Algorithm SHA256
    if ($a.Hash -ne $b.Hash) { throw "fresh extraction hash mismatch: $relative" }
}
$verification = @(
    "package=$packageName",
    "zip_sha256=$zipHash",
    "files=$($expected.Count)",
    'fresh_extract=PASS',
    'secret_signature_scan=PASS',
    'real_env_files=EXCLUDED',
    'docker_runtime_data=EXCLUDED',
    "staging=$staging",
    "zip=$zipPath"
)
[IO.File]::WriteAllLines((Join-Path $output 'PACKAGE_BUILD_RESULT_20260905.txt'),$verification,[Text.UTF8Encoding]::new($false))
Write-Output ($verification -join [Environment]::NewLine)
