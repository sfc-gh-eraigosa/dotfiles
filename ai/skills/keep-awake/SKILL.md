---
name: keep-awake
description: Keep a computer (Windows or macOS) awake during long or overnight runs (agent loops, builds, downloads) while letting the display sleep, and optionally schedule an automatic revert/reminder. Use when the user wants to stop the machine sleeping but still blank the screens, or asks to "keep the PC/Mac awake", "don't let it sleep", "caffeinate", or "force the monitors off".
---

# Keep Awake (Windows + macOS)

Keeps the **system** from sleeping while leaving the **display** free to sleep, so a long-running task (e.g. an overnight agent loop) survives but the screens still power down. Optionally turns the displays off immediately.

Pick the script for the host OS:

| OS | Keep awake | Stop / revert | Mechanism |
|----|-----------|---------------|-----------|
| Windows | `scripts/keep-awake.ps1` | `scripts/revert-keepawake.ps1` | `powercfg` setting + Task Scheduler reminder |
| macOS | `scripts/keep-awake.sh` | `scripts/revert-keepawake.sh` | `caffeinate` background process |

**Key model difference:** Windows changes a *persistent* power setting, so it must be reverted (a scheduled reminder is created as a safety net). macOS uses `caffeinate`, a *process-based* assertion — nothing persists, so "reverting" just means stopping that process (or letting a timeout / watched PID end it).

## Windows

Targets the Windows host power configuration. If running inside WSL, invoke through `powershell.exe`.

```powershell
# Keep awake, blank the monitors now, remind to revert at 8 AM (default)
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\keep-awake.ps1 -SleepMonitors

# Pick a reminder time (24h HH:mm); fires today if still ahead, else tomorrow
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\keep-awake.ps1 -RemindAt 06:30

# Keep awake but schedule no reminder; also cover battery
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\keep-awake.ps1 -NoReminder -IncludeBattery

# Revert manually
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\revert-keepawake.ps1 -Minutes 5 -Quiet
```

From WSL:

```bash
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$(wslpath -w scripts/keep-awake.ps1)" -SleepMonitors
```

### Windows notes
- Only the **AC (plugged-in)** profile changes by default; battery is untouched unless `-IncludeBattery`.
- `keep-awake.ps1` reads the **current** AC sleep timeout first and bakes it into the revert reminder, so revert restores whatever was there before (fallback 5 min). It prints the active state, e.g. `Keep-awake ON (AC): system sleep = never; monitors still sleep after 5 min.`
- The reminder is a one-time Task Scheduler entry **"Claude - Revert keep-awake"** that pops a Yes/No dialog. Re-running replaces it.
- Blanking monitors uses the `SC_MONITORPOWER` broadcast; any input wakes them, which is expected.
- The setting is **persistent** until reverted — not tied to the task finishing.

## macOS

```bash
# Keep awake until stopped, and put the display to sleep now
./scripts/keep-awake.sh --sleep-display

# Auto-release at a clock time, or after a duration
./scripts/keep-awake.sh --until 06:30
./scripts/keep-awake.sh --minutes 480

# Stay awake only while a specific process (e.g. your loop) is alive
./scripts/keep-awake.sh --wait-pid 12345

# Stop manually
./scripts/revert-keepawake.sh
```

### macOS notes
- Uses `caffeinate -i -s`: `-i` prevents idle system sleep, `-s` prevents system sleep on AC. `-d` is deliberately **omitted** so the display can still sleep.
- The PID is tracked in `$TMPDIR/keep-awake.pid`; re-running while active is a no-op that reports the existing session.
- It prints the active state, e.g. `Keep-awake ON: system sleep prevented; display still sleeps after 10 min (caffeinate pid 4321; stays until revert-keepawake.sh).`
- Nothing persists: stopping the process, reaching the `--minutes`/`--until` timeout, the watched PID exiting, or a reboot all restore normal sleep.
- `--sleep-display` uses `pmset displaysleepnow`; any input wakes the display again.
