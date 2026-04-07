# clawgate installer for Windows
# Usage: irm clawgate.org/install.ps1 | iex

$ErrorActionPreference = "Stop"

$repo = "goclawgate/clawgate"
$installDir = "$env:USERPROFILE\.clawgate\bin"
$binary = "clawgate.exe"
$asset = "clawgate-windows-amd64.exe"
$url = "https://github.com/$repo/releases/latest/download/$asset"

Write-Host ""
Write-Host "  clawgate installer"
Write-Host "  -------------------"
Write-Host "  OS:   windows"
Write-Host "  Arch: amd64"
Write-Host ""

# Download
Write-Host "  Downloading from GitHub..."
$tmpFile = [System.IO.Path]::GetTempFileName() + ".exe"

try {
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
    Invoke-WebRequest -Uri $url -OutFile $tmpFile -UseBasicParsing
} catch {
    Write-Host ""
    Write-Host "  Error: Download failed." -ForegroundColor Red
    Write-Host "  Check https://github.com/$repo/releases for available assets."
    if (Test-Path $tmpFile) { Remove-Item $tmpFile -Force }
    exit 1
}

# Install
if (-not (Test-Path $installDir)) {
    New-Item -ItemType Directory -Force -Path $installDir | Out-Null
}
Move-Item -Force $tmpFile "$installDir\$binary"
Write-Host "  Installed to $installDir\$binary"

# Add to PATH
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$installDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$userPath;$installDir", "User")
    Write-Host ""
    Write-Host "  Added $installDir to PATH."
    Write-Host "  Restart your terminal for the change to take effect."
}

# Done
Write-Host ""
Write-Host "  Installation complete!" -ForegroundColor Green
Write-Host ""
Write-Host "  Get started (open a new terminal first):"
Write-Host "    clawgate login          # authenticate with ChatGPT"
Write-Host "    clawgate                # start the proxy"
Write-Host '    $env:ANTHROPIC_BASE_URL="http://localhost:8082"; claude'
Write-Host ""
