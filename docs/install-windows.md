# Installing clawgate on Windows

## Quick Install (recommended)

**PowerShell:**

```powershell
irm clawgate.org/install.ps1 | iex
```

**Command Prompt:**

```cmd
curl -fsSL clawgate.org/install.cmd -o install.cmd && install.cmd && del install.cmd
```

This downloads `clawgate.exe` to `%USERPROFILE%\.clawgate\bin\`, adds
it to your PATH, and you're ready to go. Restart your terminal
afterwards.

## Manual Download

1. Go to [clawgate.org](https://clawgate.org/#install) or the
   [GitHub Releases](https://github.com/goclawgate/clawgate/releases) page
2. Download `clawgate-windows-amd64.exe`
3. Create a folder: `%USERPROFILE%\.clawgate\bin\`
4. Save the file as `clawgate.exe` in that folder

## Add to PATH (manual installs only)

If you used the quick install, PATH was set up automatically. For manual
installs, add the folder to your PATH:

### Using PowerShell

```powershell
$binPath = "$env:USERPROFILE\.clawgate\bin"
$currentPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($currentPath -notlike "*$binPath*") {
    [Environment]::SetEnvironmentVariable("Path", "$currentPath;$binPath", "User")
    Write-Host "Added $binPath to PATH. Restart your terminal to apply."
}
```

### Using System Settings (GUI)

1. Press **Win + R**, type `sysdm.cpl`, press Enter
2. Go to **Advanced** tab > **Environment Variables**
3. Under **User variables**, select **Path** and click **Edit**
4. Click **New** and add: `%USERPROFILE%\.clawgate\bin`
5. Click **OK** on all dialogs
6. Close and reopen your terminal

## Verify

Open a **new** terminal (Command Prompt, PowerShell, or Windows
Terminal) and run:

```
clawgate --help
```

If you get `'clawgate' is not recognized`, make sure you restarted
the terminal after adding to PATH.

## Windows Defender / SmartScreen

Windows may flag the binary the first time you run it because it's
unsigned. You'll see a "Windows protected your PC" dialog.

1. Click **More info**
2. Click **Run anyway**

This only happens once. The binary is a standard Go executable with no
external dependencies.

## Login (ChatGPT mode)

```
clawgate login
```

Follow the on-screen instructions to authenticate with your ChatGPT
account.

## Start the proxy

```
# ChatGPT mode (default)
clawgate

# API key mode
clawgate --mode=api --apiKey=sk-xxx

# Custom port
clawgate --mode=api --apiKey=sk-xxx --port=9000
```

## Connect Claude Code

In a separate terminal:

```
set ANTHROPIC_BASE_URL=http://localhost:8082
claude
```

Or in PowerShell:

```powershell
$env:ANTHROPIC_BASE_URL = "http://localhost:8082"
claude
```

Or as a one-liner (PowerShell):

```powershell
$env:ANTHROPIC_BASE_URL="http://localhost:8082"; claude
```

## Run on startup (optional)

### Task Scheduler

1. Press **Win + R**, type `taskschd.msc`, press Enter
2. Click **Create Basic Task** in the right panel
3. Name: `clawgate`, click Next
4. Trigger: **When I log on**, click Next
5. Action: **Start a program**, click Next
6. Program: `%USERPROFILE%\.clawgate\bin\clawgate.exe`
7. Arguments: `--mode=api --apiKey=sk-xxx`
8. Click Finish

### PowerShell startup script

Add to your PowerShell profile (`$PROFILE`):

```powershell
# Start clawgate in the background on shell startup
if (-not (Get-Process clawgate -ErrorAction SilentlyContinue)) {
    Start-Process -WindowStyle Hidden "$env:USERPROFILE\.clawgate\bin\clawgate.exe" `
        -ArgumentList "--mode=api","--apiKey=sk-xxx"
}
```

## Uninstall

### PowerShell

```powershell
# Remove the binary and credentials
Remove-Item -Recurse -Force "$env:USERPROFILE\.clawgate"

# Remove from PATH
$binPath = "$env:USERPROFILE\.clawgate\bin"
$currentPath = [Environment]::GetEnvironmentVariable("Path", "User")
$newPath = ($currentPath -split ";" | Where-Object { $_ -ne $binPath }) -join ";"
[Environment]::SetEnvironmentVariable("Path", $newPath, "User")
```

### Manual

1. Delete the folder `%USERPROFILE%\.clawgate\`
2. Remove `%USERPROFILE%\.clawgate\bin` from PATH (System Settings > Environment Variables)
3. Delete the Task Scheduler entry if you created one
