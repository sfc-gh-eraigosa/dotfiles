@echo off
rem Self-elevating launcher for install-wslgiga1.ps1: UAC prompt, ExecutionPolicy Bypass for this one run,
rem window kept open (-NoExit) so the Autologon steps can be read. Extra arguments pass through, e.g. -WhatIf.
setlocal
set "PSEXE=%SystemRoot%\System32\WindowsPowerShell\v1.0\powershell.exe"
set "SCRIPT=%~dp0install-wslgiga1.ps1"
set "PASS=%*"
"%PSEXE%" -NoProfile -ExecutionPolicy Bypass -Command "Start-Process -Verb RunAs -FilePath $env:PSEXE -ArgumentList ('-NoProfile -ExecutionPolicy Bypass -NoExit -File \"{0}\" {1}' -f $env:SCRIPT, $env:PASS)"
if errorlevel 1 pause
