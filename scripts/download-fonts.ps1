#!/usr/bin/env pwsh
<#
.SYNOPSIS
    Downloads free/open-source fonts into assets/fonts/ so they can be
    embedded in the FontOverride binary at build time.
.DESCRIPTION
    All fonts bundled here are open-source (OFL or CC-BY).
    Run this script once before `wails build` (or `make build`).
    Fonts are excluded from git — see .gitignore.
#>

$fontsDir = Join-Path $PSScriptRoot "..\assets\fonts"
New-Item -ItemType Directory -Force -Path $fontsDir | Out-Null

$fonts = @(
    @{
        Name = "OpenDyslexic-Regular.otf"
        Url  = "https://raw.githubusercontent.com/antijingoist/opendyslexic/master/compiled/OpenDyslexic-Regular.otf"
        Desc = "OpenDyslexic (CC-BY)"
    },
    @{
        Name = "OpenDyslexicMono-Regular.otf"
        Url  = "https://raw.githubusercontent.com/antijingoist/opendyslexic/master/compiled/OpenDyslexicMono-Regular.otf"
        Desc = "OpenDyslexic Mono (CC-BY)"
    }
)

foreach ($f in $fonts) {
    $dst = Join-Path $fontsDir $f.Name
    if (Test-Path $dst) {
        Write-Host "SKIP  $($f.Name) (already present)"
        continue
    }
    Write-Host "GET   $($f.Desc) ..."
    try {
        $wc = New-Object System.Net.WebClient
        $wc.DownloadFile($f.Url, $dst)
        $size = [math]::Round((Get-Item $dst).Length / 1024, 1)
        Write-Host "OK    $($f.Name) ($size KB)"
    }
    catch {
        Write-Host "FAIL  $($f.Name): $_" -ForegroundColor Red
    }
}

Write-Host ""
Write-Host "Done. Run 'make build' (or 'wails build') next."
