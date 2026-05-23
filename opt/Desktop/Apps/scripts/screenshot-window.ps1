# ============================================================================
#  Captures a single window to a PNG in  Pictures\Screenshots  (DPI-aware).
#  Called by macos.ahk on Ctrl+Shift+C, passing the active window's handle.
#  Run standalone to capture the current foreground window.
# ============================================================================
param([long]$Hwnd = 0)

$ErrorActionPreference = 'Stop'

Add-Type @"
using System;
using System.Runtime.InteropServices;
public class WinShot {
  [DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern bool IsIconic(IntPtr h);
  [DllImport("dwmapi.dll")] public static extern int DwmGetWindowAttribute(IntPtr h, int a, out RECT r, int s);
  [DllImport("shcore.dll")] public static extern int SetProcessDpiAwareness(int v);
  [StructLayout(LayoutKind.Sequential)] public struct RECT { public int Left, Top, Right, Bottom; }
}
"@

# Per-monitor DPI aware -> coordinates are real (physical) pixels at 200% scaling.
try { [WinShot]::SetProcessDpiAwareness(2) | Out-Null } catch {}

Add-Type -AssemblyName System.Drawing
Add-Type -AssemblyName System.Windows.Forms

$h = if ($Hwnd -ne 0) { [IntPtr]::new($Hwnd) } else { [WinShot]::GetForegroundWindow() }
if ($h -eq [IntPtr]::Zero -or [WinShot]::IsIconic($h)) { exit 2 }

$r = New-Object WinShot+RECT
# DWMWA_EXTENDED_FRAME_BOUNDS = 9 -> the true visible bounds (skips invisible borders)
if ([WinShot]::DwmGetWindowAttribute($h, 9, [ref]$r, 16) -ne 0) {
    [WinShot]::GetWindowRect($h, [ref]$r) | Out-Null
}

$w  = $r.Right - $r.Left
$ht = $r.Bottom - $r.Top
if ($w -le 0 -or $ht -le 0) { exit 3 }

$bmp = New-Object System.Drawing.Bitmap($w, $ht)
$g = [System.Drawing.Graphics]::FromImage($bmp)
$g.CopyFromScreen($r.Left, $r.Top, 0, 0, $bmp.Size)
$g.Dispose()

$dir = Join-Path ([Environment]::GetFolderPath('MyPictures')) 'Screenshots'
if (-not (Test-Path $dir)) { New-Item -ItemType Directory -Path $dir -Force | Out-Null }
$file = Join-Path $dir ('Screenshot {0}.png' -f (Get-Date -Format 'yyyy-MM-dd HHmmss-fff'))
$bmp.Save($file, [System.Drawing.Imaging.ImageFormat]::Png)

# Also copy to clipboard (best effort, matches macOS-ish behaviour).
try { [System.Windows.Forms.Clipboard]::SetImage($bmp) } catch {}
$bmp.Dispose()

Write-Output $file
