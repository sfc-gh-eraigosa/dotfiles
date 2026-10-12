@echo off
rem Start the dev stack: the one WSL distro (k3s node, ollama, sshd) + claude-remote.
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0Apps\scripts\Start-DevStack.ps1" %*
