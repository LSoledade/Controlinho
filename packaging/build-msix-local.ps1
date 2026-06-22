<#
.SYNOPSIS
  Registers the `store` build of PC Remote locally for TESTING -- in particular the
  WinRT StartupTask auto-start, which only works with package identity (it cannot be
  exercised from a bare .exe).

.DESCRIPTION
  Uses `Add-AppxPackage -Register` over the loose layout in pkg\: NO certificate and
  NO signing needed. Requires Developer Mode ON
  (Settings > Privacy and security > For developers).

  For the DISTRIBUTION artifact (Partner Center upload), use build-msix.ps1.

.EXAMPLE
  .\packaging\build-msix-local.ps1            # build + register + launch
.EXAMPLE
  .\packaging\build-msix-local.ps1 -NoLaunch  # do not launch at the end
#>
[CmdletBinding()]
param([switch]$NoLaunch)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

# Developer Mode (required for -Register of a loose layout).
$devKey = "HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\AppModelUnlock"
$devOn = (Get-ItemProperty -Path $devKey -Name AllowDevelopmentWithoutDevLicense -ErrorAction SilentlyContinue).AllowDevelopmentWithoutDevLicense -eq 1
if (-not $devOn) {
  Write-Warning "Developer Mode looks OFF -- 'Add-AppxPackage -Register' will likely fail."
  Write-Host "Enable it: Settings > Privacy and security > For developers > Developer Mode."
  Write-Host "Or (admin PowerShell): Set-ItemProperty '$devKey' AllowDevelopmentWithoutDevLicense 1"
  Write-Host "Trying anyway...`n"
}

# 1. Build the store binary (no console; the tray is the UI).
$env:GOOS = "windows"; $env:GOARCH = "amd64"
Write-Host "Building (go build -tags store)..."
go build -tags store -ldflags "-s -w -H windowsgui" -o "$root\pkg\pc-remote.exe" .
if ($LASTEXITCODE -ne 0) { throw "go build failed" }

# 2. Loose layout: pkg\ { pc-remote.exe, AppxManifest.xml, Assets\ }.
Copy-Item "$root\packaging\Package.appxmanifest" "$root\pkg\AppxManifest.xml" -Force
if (Test-Path "$root\pkg\Assets") { Remove-Item "$root\pkg\Assets" -Recurse -Force }
Copy-Item "$root\packaging\Assets" "$root\pkg\Assets" -Recurse -Force

# 3. Register (re-registers over an existing one).
Write-Host "Registering (Add-AppxPackage -Register)..."
Add-AppxPackage -Register "$root\pkg\AppxManifest.xml"

$pkg = Get-AppxPackage -Name "*Controlinho*" | Select-Object -First 1
if (-not $pkg) { throw "register failed: package not found" }
Write-Host "Installed: $($pkg.PackageFullName)"
$aumid = "$($pkg.PackageFamilyName)!PCRemote"
Write-Host "AppUserModelId: $aumid"

# 4. Launch in the interactive session.
if (-not $NoLaunch) {
  Write-Host "Launching the app..."
  Start-Process "shell:appsFolder\$aumid"
}

Write-Host ""
Write-Host "Check:"
Write-Host "  - System tray: 'PC Remote' icon."
Write-Host "  - Connect / PIN page: http://127.0.0.1:8080/qr"
Write-Host "  - StartupTask (Option A): in the tray, toggle 'Iniciar com o Windows',"
Write-Host "    then confirm in Task Manager > Startup (PC Remote: Enabled/Disabled)."
Write-Host ""
Write-Host "Remove:  Get-AppxPackage *Controlinho* | Remove-AppxPackage"
