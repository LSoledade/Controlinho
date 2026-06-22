<#
.SYNOPSIS
  Verifies the MSIX StartupTask (auto-start) WITHOUT clicking the tray, by running
  the WinRT API under the installed package's identity. Exercises the same code path
  install_store.go uses at runtime, so it's a faithful check of "Opcao A".

.DESCRIPTION
  Requires the package installed first (run build-msix-local.ps1).
  Default: prints the current StartupTask state.
  -Enable / -Disable: flips it (then prints the resulting state).

  Uses Invoke-CommandInDesktopPackage to obtain package identity (no admin needed on
  most setups; if it errors, the equivalent manual check is the tray toggle ->
  Task Manager > Startup).

.EXAMPLE
  .\packaging\verify-startuptask.ps1
.EXAMPLE
  .\packaging\verify-startuptask.ps1 -Enable
  .\packaging\verify-startuptask.ps1 -Disable
#>
[CmdletBinding()]
param([switch]$Enable, [switch]$Disable)

$ErrorActionPreference = "Stop"

$pkg = Get-AppxPackage -Name "*Controlinho*" | Select-Object -First 1
if (-not $pkg) { throw "Controlinho is not installed. Run build-msix-local.ps1 first." }
$pfn = $pkg.PackageFamilyName

$out = Join-Path $env:TEMP "pcr_verify_startuptask.txt"
if (Test-Path $out) { Remove-Item $out -Force }

# Pick the action. Each writes the resulting State to $out for read-back.
$action = '$task.State.ToString() | Out-File -Encoding ascii $env:TEMP\pcr_verify_startuptask.txt'
if ($Enable)  { $action = '$s=Await ($task.RequestEnableAsync()) ([Windows.ApplicationModel.StartupTaskState]); $s.ToString() | Out-File -Encoding ascii $env:TEMP\pcr_verify_startuptask.txt' }
if ($Disable) { $action = '$task.Disable(); $task.State.ToString() | Out-File -Encoding ascii $env:TEMP\pcr_verify_startuptask.txt' }

# Single-quoted here-string => every $ is literal; __ACTION__ is substituted after.
# This block mirrors install_store.go's psPreamble (incl. the WindowsRuntime assembly
# load that PS 5.1 needs for [System.WindowsRuntimeSystemExtensions]).
$probeTemplate = @'
$ErrorActionPreference='Stop'
[void][System.Runtime.InteropServices.WindowsRuntime.WindowsRuntimeMarshal]
try { Add-Type -AssemblyName System.Runtime.WindowsRuntime -ErrorAction Stop } catch {}
function Await($op,$rt){ $m=([System.WindowsRuntimeSystemExtensions].GetMethods()|Where-Object{$_.Name -eq 'AsTask' -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -like 'IAsyncOperation*1'})[0]; $t=$m.MakeGenericMethod($rt).Invoke($null,@($op)); $t.Wait(-1)|Out-Null; $t.Result }
[Windows.ApplicationModel.StartupTask,Windows.ApplicationModel,ContentType=WindowsRuntime]|Out-Null
$task=Await ([Windows.ApplicationModel.StartupTask]::GetAsync('PCRemoteStartup')) ([Windows.ApplicationModel.StartupTask])
__ACTION__
'@
$probe = $probeTemplate -replace '__ACTION__', $action

$pf = Join-Path $env:TEMP "pcr_verify_probe.ps1"
Set-Content -Path $pf -Value $probe -Encoding ASCII

Write-Host "Running under package identity ($pfn)..."
Invoke-CommandInDesktopPackage -PackageFamilyName $pfn -AppId "PCRemote" -Command "powershell.exe" -Args "-NoProfile -NonInteractive -ExecutionPolicy Bypass -File `"$pf`""
Start-Sleep -Seconds 4
if (Test-Path $out) { Write-Host ("StartupTask state: " + (Get-Content $out -Raw).Trim()) }
else { Write-Host "No result (the WinRT call may have failed; try the tray toggle instead)." }
