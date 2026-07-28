param(
    [string]$Version = "1.0.9",
    [ValidateSet("all", "x86", "arm")]
    [string]$Platform = "all",
    [switch]$SkipTests
)

$ErrorActionPreference = "Stop"
$ProjectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$DistDir = Join-Path $ProjectRoot "dist"
$BinDir = Join-Path $ProjectRoot "bin"

function Resolve-Go {
    $projectGo = Join-Path $ProjectRoot ".tools\complete\go\bin\go.exe"
    if (Test-Path $projectGo) { return $projectGo }
    $fallbackGo = Join-Path $ProjectRoot ".tools\go\bin\go.exe"
    if (Test-Path $fallbackGo) { return $fallbackGo }
    $command = Get-Command go -ErrorAction SilentlyContinue
    if ($command) { return $command.Source }
    throw "Go 1.24+ was not found. Install Go or place a portable SDK in .tools/go."
}

function Resolve-Python {
    $command = Get-Command python -ErrorAction SilentlyContinue
    if ($command) { return $command.Source }
    throw "Python 3 was not found."
}

$Go = Resolve-Go
$Python = Resolve-Python

Push-Location $ProjectRoot
try {
    Write-Host "[1/5] Building WebUI"
    Push-Location (Join-Path $ProjectRoot "webui")
    try {
        npm.cmd ci
        npm.cmd run build
    } finally {
        Pop-Location
    }

    Write-Host "[2/5] Generating FNOS icons"
    & $Python (Join-Path $ProjectRoot "scripts\generate_icons.py")

    if (-not $SkipTests) {
        Write-Host "[3/5] Running Go tests"
        & $Go test ./...
        if ($LASTEXITCODE -ne 0) { throw "Go tests failed" }
    } else {
        Write-Host "[3/5] Tests skipped"
    }

    New-Item -ItemType Directory -Force -Path $DistDir, $BinDir | Out-Null
    $targets = @()
    if ($Platform -eq "all" -or $Platform -eq "x86") {
        $targets += @{ Platform = "x86"; GoArch = "amd64" }
    }
    if ($Platform -eq "all" -or $Platform -eq "arm") {
        $targets += @{ Platform = "arm"; GoArch = "arm64" }
    }

    Write-Host "[4/5] Cross-compiling Linux binaries"
    foreach ($target in $targets) {
        $binary = Join-Path $BinDir ("fndns-linux-" + $target.GoArch)
        $env:CGO_ENABLED = "0"
        $env:GOOS = "linux"
        $env:GOARCH = $target.GoArch
        & $Go build -trimpath -ldflags "-s -w -X main.version=$Version" -o $binary ./cmd/fndns
        if ($LASTEXITCODE -ne 0) { throw "Build failed for $($target.Platform)" }
        $target.Binary = $binary
    }
    Remove-Item Env:CGO_ENABLED, Env:GOOS, Env:GOARCH -ErrorAction SilentlyContinue

    Write-Host "[5/5] Packaging FNOS .fpk files"
    foreach ($target in $targets) {
        $output = Join-Path $DistDir ("com.fndns.manager_{0}_{1}.fpk" -f $Version, $target.Platform)
        & $Python (Join-Path $ProjectRoot "scripts\package_fpk.py") --root $ProjectRoot --binary $target.Binary --version $Version --platform $target.Platform --output $output
        if ($LASTEXITCODE -ne 0) { throw "Packaging failed for $($target.Platform)" }
    }
    $sourceOutput = Join-Path $DistDir ("com.fndns.manager_{0}_source.zip" -f $Version)
    & $Python (Join-Path $ProjectRoot "scripts\package_source.py") --root $ProjectRoot --output $sourceOutput
    if ($LASTEXITCODE -ne 0) { throw "Source packaging failed" }
    Write-Host "Build complete: $DistDir"
} finally {
    Pop-Location
}
