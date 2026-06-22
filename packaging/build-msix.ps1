<#
.SYNOPSIS
  Builds the PC Remote MSIX package (Microsoft Store / sideload).

.DESCRIPTION
  1. Compiles the store-tagged binary (no console; the tray is the UI).
  2. Stages pkg\  (pc-remote.exe + Package.appxmanifest + Assets\).
  3. Packs it into PCRemote.msix with makeappx.
  4. (Optional) signs it for LOCAL sideload testing with -Sign.

  For the Store you do NOT sign — Partner Center signs on ingestion. Signing is only
  to install the package on your own machine before submitting.

  Requires the Windows 10/11 SDK (makeappx.exe, and signtool.exe if -Sign). The
  script auto-locates them under "Windows Kits\10\bin" if they're not already on PATH.

.EXAMPLE
  # Just build the unsigned package (for upload to Partner Center)
  .\packaging\build-msix.ps1

.EXAMPLE
  # Build + sign with a self-signed cert for local sideload.
  # The cert Subject MUST equal Identity/Publisher in Package.appxmanifest.
  .\packaging\build-msix.ps1 -Sign -PfxPath .\teste.pfx -PfxPassword (Read-Host -AsSecureString)
#>
[CmdletBinding()]
param(
  [string]$OutMsix = "Controlinho.msix",
  [switch]$Sign,
  [string]$PfxPath,
  [System.Security.SecureString]$PfxPassword
)

$ErrorActionPreference = "Stop"
# Repo root = parent of this script's folder.
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

function Find-SdkTool([string]$name) {
  $cmd = Get-Command $name -ErrorAction SilentlyContinue
  if ($cmd) { return $cmd.Source }
  $bases = @(
    "${env:ProgramFiles(x86)}\Windows Kits\10\bin",
    "${env:ProgramFiles}\Windows Kits\10\bin"
  ) | Where-Object { $_ -and (Test-Path $_) }
  foreach ($b in $bases) {
    $hit = Get-ChildItem -Path $b -Recurse -Filter $name -ErrorAction SilentlyContinue |
      Where-Object { $_.FullName -match "\\x64\\" } |
      Sort-Object FullName -Descending | Select-Object -First 1
    if ($hit) { return $hit.FullName }
  }
  throw "$name not found. Install the Windows 10/11 SDK (includes makeappx/signtool)."
}

$makeappx = Find-SdkTool "makeappx.exe"
Write-Host "makeappx: $makeappx"

# 1. Build the store-tagged binary (no console window).
Write-Host "Building store binary (go build -tags store)..."
$env:GOOS = "windows"; $env:GOARCH = "amd64"
go build -tags store -ldflags "-s -w -H windowsgui" -o "$root\pkg\pc-remote.exe" .
if ($LASTEXITCODE -ne 0) { throw "go build failed" }

# 2. Stage the package layout: pkg\ { pc-remote.exe, AppxManifest.xml, Assets\ }.
#    makeappx requires the manifest's footprint name to be exactly AppxManifest.xml.
Copy-Item "$root\packaging\Package.appxmanifest" "$root\pkg\AppxManifest.xml" -Force
if (Test-Path "$root\pkg\Package.appxmanifest") { Remove-Item "$root\pkg\Package.appxmanifest" -Force }
if (Test-Path "$root\pkg\Assets") { Remove-Item "$root\pkg\Assets" -Recurse -Force }
Copy-Item "$root\packaging\Assets" "$root\pkg\Assets" -Recurse -Force

# 3. Pack.
$msixOut = Join-Path $root $OutMsix
if (Test-Path $msixOut) { Remove-Item $msixOut -Force }
Write-Host "Packing $OutMsix..."
& $makeappx pack /o /d "$root\pkg" /p $msixOut
if ($LASTEXITCODE -ne 0) { throw "makeappx pack failed" }
Write-Host "Built: $msixOut"

# 4. Optional local-test signing.
if ($Sign) {
  if (-not $PfxPath) { throw "-Sign requires -PfxPath (and -PfxPassword)" }
  $signtool = Find-SdkTool "signtool.exe"
  Write-Host "signtool: $signtool"
  if ($PfxPassword) {
    $bstr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($PfxPassword)
    $plain = [Runtime.InteropServices.Marshal]::PtrToStringAuto($bstr)
    & $signtool sign /fd SHA256 /a /f $PfxPath /p $plain $msixOut
  } else {
    & $signtool sign /fd SHA256 /a /f $PfxPath $msixOut
  }
  if ($LASTEXITCODE -ne 0) { throw "signtool failed" }
  Write-Host "Signed: $msixOut"
  Write-Host "Install locally with:  Add-AppxPackage -Path `"$msixOut`""
}
