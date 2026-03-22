#Requires -RunAsAdministrator
<#
.SYNOPSIS
    Installer / Uninstaller for FontOverride.

.PARAMETER Uninstall
    Remove FontOverride from the system (shortcuts, installed files, registry).

.EXAMPLE
    # Install
    .\install.ps1

.EXAMPLE
    # Uninstall
    .\install.ps1 -Uninstall
#>
param(
    [switch]$Uninstall
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# ── Constants ────────────────────────────────────────────────────────────────
$AppName      = "FontOverride"
$ExeName      = "FontOverride.exe"
$InstallDir   = Join-Path $env:LOCALAPPDATA "Programs\$AppName"
$StartMenuDir = Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs\$AppName"
$DesktopLnk   = Join-Path ([Environment]::GetFolderPath("Desktop")) "$AppName.lnk"
$StartMenuLnk = Join-Path $StartMenuDir "$AppName.lnk"
$UninstallLnk = Join-Path $StartMenuDir "Uninstall $AppName.lnk"

# Locate the source executable. Wails puts it in build\bin\ — fall back to
# the script directory for cases where the user has moved the exe manually.
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Definition
$SourceExe = Join-Path $ScriptDir "build\bin\$ExeName"
if (-not (Test-Path $SourceExe)) {
    $SourceExe = Join-Path $ScriptDir $ExeName
}

function Write-Step([string]$msg) { Write-Host "  • $msg" -ForegroundColor Cyan }
function Write-Ok([string]$msg)   { Write-Host "  ✓ $msg" -ForegroundColor Green }
function Write-Warn([string]$msg) { Write-Host "  ! $msg" -ForegroundColor Yellow }

function New-Shortcut([string]$lnkPath, [string]$target, [string]$desc, [string]$args = "") {
    $shell = New-Object -ComObject WScript.Shell
    $s = $shell.CreateShortcut($lnkPath)
    $s.TargetPath       = $target
    $s.Arguments        = $args
    $s.Description      = $desc
    $s.WorkingDirectory = Split-Path $target
    $s.Save()
    [System.Runtime.InteropServices.Marshal]::ReleaseComObject($shell) | Out-Null
}

# ── Uninstall ────────────────────────────────────────────────────────────────
if ($Uninstall) {
    Write-Host "`nUninstalling $AppName...`n" -ForegroundColor Magenta

    Write-Step "Removing shortcuts"
    foreach ($lnk in $DesktopLnk, $StartMenuLnk, $UninstallLnk) {
        if (Test-Path $lnk) { Remove-Item $lnk -Force; Write-Ok "Removed $lnk" }
    }
    if (Test-Path $StartMenuDir) {
        $remaining = Get-ChildItem $StartMenuDir -ErrorAction SilentlyContinue
        if (-not $remaining) { Remove-Item $StartMenuDir -Force }
    }

    Write-Step "Removing installed files"
    if (Test-Path $InstallDir) {
        Remove-Item $InstallDir -Recurse -Force
        Write-Ok "Removed $InstallDir"
    }

    Write-Step "Cleaning registry (font substitutes + Chrome policy)"
    # These are also removed on app shutdown, but clean up here too for robustness.
    $fontSubsPath = "HKCU:\Software\Microsoft\Windows NT\CurrentVersion\FontSubstitutes"
    $commonFonts  = @("Arial","Courier New","Helvetica","Times New Roman","Verdana",
                      "Tahoma","Segoe UI","Microsoft Sans Serif","MS Sans Serif","MS Serif")
    if (Test-Path $fontSubsPath) {
        foreach ($name in $commonFonts) {
            Remove-ItemProperty -Path $fontSubsPath -Name $name -ErrorAction SilentlyContinue
        }
    }
    foreach ($path in @(
        "HKLM:\SOFTWARE\Policies\Google\Chrome\ExtensionInstallForcelist",
        "HKLM:\SOFTWARE\Policies\Google\Chrome",
        "HKLM:\SOFTWARE\Policies\Google"
    )) {
        if (Test-Path $path) {
            $vals = (Get-Item $path).Property
            if (-not $vals) { Remove-Item $path -Force -ErrorAction SilentlyContinue }
        }
    }

    Write-Host "`n$AppName has been uninstalled.`n" -ForegroundColor Green
    return
}

# ── Install ──────────────────────────────────────────────────────────────────
Write-Host "`nInstalling $AppName...`n" -ForegroundColor Magenta

if (-not (Test-Path $SourceExe)) {
    Write-Host "ERROR: $ExeName not found next to this script ($ScriptDir)." -ForegroundColor Red
    Write-Host "       Build the app first:  wails build -platform windows/amd64" -ForegroundColor Red
    exit 1
}

Write-Step "Creating install directory"
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
Write-Ok $InstallDir

Write-Step "Copying executable"
Copy-Item $SourceExe (Join-Path $InstallDir $ExeName) -Force
Write-Ok "$InstallDir\$ExeName"

Write-Step "Creating Start Menu folder"
New-Item -ItemType Directory -Force -Path $StartMenuDir | Out-Null

Write-Step "Creating shortcuts"
$installedExe = Join-Path $InstallDir $ExeName

New-Shortcut $DesktopLnk   $installedExe "$AppName — system-wide font override"
Write-Ok "Desktop: $DesktopLnk"

New-Shortcut $StartMenuLnk $installedExe "$AppName"
Write-Ok "Start Menu: $StartMenuLnk"

# Uninstall shortcut runs this script with -Uninstall via PowerShell.
$psExe    = (Get-Command powershell.exe).Source
$uninstPs = Join-Path $InstallDir "Uninstall.ps1"
Copy-Item (Join-Path $ScriptDir "install.ps1") $uninstPs -Force
New-Shortcut $UninstallLnk $psExe "Uninstall $AppName" `
    "-NoProfile -ExecutionPolicy Bypass -File `"$uninstPs`" -Uninstall"
Write-Ok "Start Menu: $UninstallLnk"

Write-Host "`n$AppName installed successfully!" -ForegroundColor Green
Write-Host ""
Write-Host "  Launch: double-click the Desktop or Start Menu shortcut." -ForegroundColor White
Write-Host "  The app will request administrator rights (needed to apply Chrome font policy)." -ForegroundColor DarkGray
Write-Host "  First launch: restart Chrome once to activate the font override extension." -ForegroundColor DarkGray
Write-Host "  When you close the app, all settings are cleaned up automatically." -ForegroundColor DarkGray
Write-Host ""
