---
name: keep-awake
description: Keep a Windows machine awake during long or overnight runs (agent loops, builds, downloads) while letting the monitors sleep, and schedule an automatic reminder to revert the setting. Use when the user wants to stop the computer sleeping but still blank the displays, or asks to "keep the PC awake", "don't let it sleep", or "force the monitors off".
---

# Keep Awake (Windows)

Keeps the **system** from sleeping while leaving the **monitor** timeout intact, so a long-running task (e.g. an overnight agent loop) survives but the screens still power down. Optionally turns the displays off immediately and schedules a reminder to put normal sleep behavior back.

This targets the **Windows host** power configuration (`powercfg` + Task Scheduler). If you are running inside WSL, invoke the scripts through `powershell.exe` (see below).

## Scripts

- `scripts/keep-awake.ps1` — disable system sleep on AC, optionally blank monitors, and schedule a revert reminder.
- `scripts/revert-keepawake.ps1` — restore the normal AC sleep timeout (used by the scheduled reminder, or run manually).

## Usage

### From Windows (PowerShell)

```powershell
# Keep awake, blank the monitors now, remind to revert at 8 AM (default)
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\keep-awake.ps1 -SleepMonitors

# Pick a reminder time (24h HH:mm); fires today if still in the future, else tomorrow
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\keep-awake.ps1 -RemindAt 06:30

# Keep awake but don't schedule any reminder
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\keep-awake.ps1 -NoReminder

# Also prevent sleep on battery
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\keep-awake.ps1 -IncludeBattery
```

### From WSL

```bash
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$(wslpath -w scripts/keep-awake.ps1)" -SleepMonitors
```

### Reverting manually

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\revert-keepawake.ps1 -Minutes 5 -Quiet
```

## Behavior notes

- Only the **AC (plugged-in)** profile is changed by default; battery settings are untouched unless `-IncludeBattery` is passed.
- `keep-awake.ps1` reads the **current** AC sleep timeout before changing it and bakes that value into the revert reminder, so revert restores whatever was there before (falling back to 5 minutes if it was already "never").
- The reminder is a one-time Task Scheduler entry named **"Claude - Revert keep-awake"** that pops a Yes/No dialog and reverts on confirmation. Re-running `keep-awake.ps1` replaces any existing reminder.
- Blanking the monitors uses the `SC_MONITORPOWER` broadcast; any mouse/keyboard input wakes them again, which is expected.
- Changes are **persistent** until reverted — they are not tied to the task finishing. The scheduled reminder is the safety net.
