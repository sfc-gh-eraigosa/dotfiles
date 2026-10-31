@echo off
rem Self-elevating launcher for install-wslgiga1.ps1: UAC prompt, ExecutionPolicy Bypass for this one run,
rem window kept open (-NoExit) so the Autologon steps can be read.
rem Takes no arguments, or only -WhatIf: arbitrary %* text (quotes, &) cannot be forwarded safely through
rem cmd and PowerShell. For other options (-User, -AppExe) run the .ps1 from an elevated PowerShell.
setlocal
set "PASS="
if "%~1"=="" goto :run
if /i not "%~1"=="-WhatIf" goto :usage
if not "%~2"=="" goto :usage
set "PASS=-WhatIf"
:run
set "PSEXE=%SystemRoot%\System32\WindowsPowerShell\v1.0\powershell.exe"
set "SCRIPT=%~dp0install-wslgiga1.ps1"
"%PSEXE%" -NoProfile -ExecutionPolicy Bypass -Command "Start-Process -Verb RunAs -FilePath $env:PSEXE -ArgumentList ('-NoProfile -ExecutionPolicy Bypass -NoExit -File \"{0}\" {1}' -f $env:SCRIPT, $env:PASS)"
if errorlevel 1 pause
exit /b 0
:usage
echo Usage: install-wslgiga1.cmd [-WhatIf]
echo For -User or -AppExe run install-wslgiga1.ps1 from an elevated PowerShell.
pause
exit /b 2
