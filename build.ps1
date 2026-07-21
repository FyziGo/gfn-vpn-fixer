# build.ps1 — Build script for GFN VPN FIXER
# Requirements:
#   - Go 1.22+
#   - MinGW-w64 GCC (CGO_ENABLED=1, Fyne requires it)
#     Install via: winget install -e --id GnuWin32.Gcc
#     Or Chocolatey: choco install mingw
#     Or MSYS2:      pacman -S mingw-w64-ucrt-x86_64-gcc
#
# Usage:
#   .\build.ps1            # builds gfn-vpn-fixer.exe
#   .\build.ps1 -Release   # strips debug info + adds Windows manifest

param(
    [switch]$Release
)

$ErrorActionPreference = "Stop"

Write-Host "==> GFN VPN FIXER Build" -ForegroundColor Cyan

# Verify Go is available
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Error "Go not found in PATH. Install from https://go.dev/dl/"
    exit 1
}
Write-Host "    Go: $(go version)"

# Verify GCC / CGO (required by Fyne)
if (-not (Get-Command gcc -ErrorAction SilentlyContinue)) {
    Write-Error "GCC not found in PATH. Install MinGW-w64 (see script header for options)."
    exit 1
}
Write-Host "    GCC: $(gcc --version | Select-Object -First 1)"

# Fetch dependencies
Write-Host "==> go mod tidy" -ForegroundColor Cyan
go mod tidy

# Set build flags
$env:CGO_ENABLED = "1"
$env:GOOS       = "windows"
$env:GOARCH     = "amd64"

$ldflags = "-H windowsgui"   # Suppress the console window in Launcher mode
if ($Release) {
    $ldflags += " -s -w"     # Strip debug symbols for smaller binary
    Write-Host "    Release build (stripped)" -ForegroundColor Yellow
}

$output = "gfn-vpn-fixer.exe"

Write-Host "==> go build -o $output" -ForegroundColor Cyan
go build -v -ldflags $ldflags -o $output .

if ($LASTEXITCODE -eq 0) {
    $size = (Get-Item $output).Length / 1KB
    Write-Host ""
    Write-Host "  Build successful!" -ForegroundColor Green
    Write-Host "  Output : $((Get-Item $output).FullName)"
    Write-Host "  Size   : $([math]::Round($size, 1)) KB"
    Write-Host ""
    Write-Host "Usage:" -ForegroundColor Cyan
    Write-Host "  .\gfn-vpn-fixer.exe --setup   # Open GUI (run as Administrator)"
    Write-Host "  .\gfn-vpn-fixer.exe            # Headless launcher (run as Administrator)"
} else {
    Write-Error "Build FAILED (exit code $LASTEXITCODE)"
}
