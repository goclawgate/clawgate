@echo off
:: clawgate installer for Windows (CMD)
:: Usage: curl -fsSL clawgate.org/install.cmd -o install.cmd && install.cmd && del install.cmd
:: Delegates to the PowerShell installer.
powershell -ExecutionPolicy Bypass -Command "irm clawgate.org/install.ps1 | iex"
