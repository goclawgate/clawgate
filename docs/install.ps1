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

# Verify checksum
$checksumUrl = "https://github.com/$repo/releases/latest/download/checksums.txt"
Write-Host "  Verifying checksum..."
try {
    $checksumData = (Invoke-WebRequest -Uri $checksumUrl -UseBasicParsing).Content
    $expectedLine = ($checksumData -split "`n") | Where-Object { $_ -match $asset }
    if ($expectedLine) {
        $expected = ($expectedLine -split "\s+")[0]
        $actual = (Get-FileHash -Path $tmpFile -Algorithm SHA256).Hash.ToLower()
        if ($actual -ne $expected) {
            Write-Host ""
            Write-Host "  ERROR: Checksum verification failed!" -ForegroundColor Red
            Write-Host "  Expected: $expected"
            Write-Host "  Actual:   $actual"
            Write-Host ""
            Write-Host "  The downloaded binary may have been tampered with." -ForegroundColor Red
            Write-Host "  Aborting installation."
            if (Test-Path $tmpFile) { Remove-Item $tmpFile -Force }
            exit 1
        }
        Write-Host "  Checksum OK ($expected)"
    } else {
        Write-Host "  Warning: No checksum found for $asset — skipping verification." -ForegroundColor Yellow
    }
} catch {
    Write-Host "  Warning: Could not download checksums.txt — skipping verification." -ForegroundColor Yellow
}

# Install
if (-not (Test-Path $installDir)) {
    New-Item -ItemType Directory -Force -Path $installDir | Out-Null
}
$dest = "$installDir\$binary"
$old  = "$dest.old"
# Clean up leftover .old from a previous upgrade
if (Test-Path $old) { Remove-Item $old -Force -ErrorAction SilentlyContinue }
if (Test-Path $dest) {
    try {
        Remove-Item $dest -Force
    } catch {
        # Binary is running — rename it out of the way (Windows allows this)
        Rename-Item $dest $old -Force
    }
}
Move-Item -Force $tmpFile $dest
Write-Host "  Installed to $dest"

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
