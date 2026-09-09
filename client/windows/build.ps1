param(
    [string]$Version = "0.1.0",
    [string]$OutputRoot = "release/windows"
)

$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "../..")).Path
$outputRootPath = [IO.Path]::GetFullPath((Join-Path $repoRoot $OutputRoot))
if (-not $outputRootPath.StartsWith($repoRoot + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
    throw "OutputRoot must stay inside the repository"
}
$stagePath = Join-Path $outputRootPath "ShareDisk"
$exePath = Join-Path $stagePath "ShareDisk.exe"
$zipPath = Join-Path $outputRootPath "ShareDisk_$Version`_windows_amd64.zip"

if (Test-Path -LiteralPath $stagePath) {
    Remove-Item -LiteralPath $stagePath -Recurse -Force
}
New-Item -ItemType Directory -Force (Join-Path $stagePath "migrations/sqlite") | Out-Null

$commit = (git -C $repoRoot rev-parse --short HEAD 2>$null)
if (-not $commit) { $commit = "unknown" }
$buildTime = [DateTime]::UtcNow.ToString("yyyy-MM-ddTHH:mm:ssZ")
$ldflags = "-H=windowsgui -s -w -X github.com/share-disk/share-disk/internal/version.Version=$Version -X github.com/share-disk/share-disk/internal/version.Commit=$commit -X github.com/share-disk/share-disk/internal/version.BuildTime=$buildTime"

Push-Location $repoRoot
try {
    $env:CGO_ENABLED = "0"
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    go build -trimpath -ldflags $ldflags -o $exePath ./client/windows/cmd/share-disk
    if ($LASTEXITCODE -ne 0) { throw "Windows client build failed" }
    Copy-Item -Path "client/ubuntu/migrations/sqlite/*.sql" -Destination (Join-Path $stagePath "migrations/sqlite")
    Copy-Item -LiteralPath "client/windows/README.md" -Destination (Join-Path $stagePath "README.md")
} finally {
    Pop-Location
}

if (Test-Path -LiteralPath $zipPath) {
    Remove-Item -LiteralPath $zipPath -Force
}
Compress-Archive -Path $stagePath -DestinationPath $zipPath -CompressionLevel Optimal
Write-Host "Built $exePath"
Write-Host "Packaged $zipPath"
