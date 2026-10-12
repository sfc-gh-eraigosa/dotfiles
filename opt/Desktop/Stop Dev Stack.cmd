@echo off
rem Stop the dev stack: Docker Desktop if running, then wsl --shutdown.
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0Apps\scripts\Stop-DevStack.ps1" %*
