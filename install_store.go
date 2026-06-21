//go:build windows && store

package main

// Auto-start for the MSIX (Microsoft Store) build — replaces the schtasks logon
// task, which is forbidden for packaged apps.
//
// Auto-start is declared as the `windows.startupTask` extension in
// Package.appxmanifest (TaskId "PCRemoteStartup", Enabled="false") and toggled at
// runtime through the WinRT StartupTask API — this is the doc's "Opção A".
//
// Go has no in-box WinRT projection. Rather than hand-roll the COM/IInspectable
// async call chain via syscall (very long, and the IIDs must be byte-exact), we
// drive the API from PowerShell, which ships a WinRT projection. WinRT resolves
// StartupTask.GetAsync against the *current package's* identity, so the call must
// run with our package identity — a child process spawned by a packaged full-trust
// app inherits it, so powershell.exe launched from here qualifies.
//
// IMPORTANT: this only does anything meaningful when the binary runs from the
// installed MSIX (it has package identity then). Running the `-tags store` build as
// a bare .exe reports "no package" and the toggle no-ops — that's expected; ship it
// packaged. Marked for on-device verification (see packaging/README.md). If WinRT
// proves flaky, falling back to "Opção B" is trivial: make these two functions
// no-ops (autoStartEnabled→false, setAutoStart→nil) and drop the tray item; the
// user then toggles it in Configurações → Apps → Inicialização.

import (
	"encoding/base64"
	"fmt"
	"os/exec"
	"strings"
	"unicode/utf16"
)

// startupTaskID must match the TaskId in Package.appxmanifest.
const startupTaskID = "PCRemoteStartup"

// psPreamble loads the WinRT type and resolves our StartupTask. Await() bridges
// the WinRT IAsyncOperation<T> to a waitable .NET Task via the AsTask extension
// (the `-like 'IAsyncOperation*1'` filter picks the single-arg generic overload
// without needing a literal backtick in the script).
//
// The first two lines force-load System.Runtime.WindowsRuntime — without it
// [System.WindowsRuntimeSystemExtensions] (which provides AsTask) isn't resolvable
// in Windows PowerShell 5.1, and Await throws "cannot find type". This was found by
// on-device testing under package identity (Invoke-CommandInDesktopPackage).
const psPreamble = `$ErrorActionPreference='Stop'
[void][System.Runtime.InteropServices.WindowsRuntime.WindowsRuntimeMarshal]
try { Add-Type -AssemblyName System.Runtime.WindowsRuntime -ErrorAction Stop } catch {}
function Await($op,$rt){
  $m=[System.WindowsRuntimeSystemExtensions].GetMethods()|
    Where-Object {$_.Name -eq 'AsTask' -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -like 'IAsyncOperation*1'}|
    Select-Object -First 1
  $t=$m.MakeGenericMethod($rt).Invoke($null,@($op)); $t.Wait(-1)|Out-Null; $t.Result
}
[Windows.ApplicationModel.StartupTask,Windows.ApplicationModel,ContentType=WindowsRuntime]|Out-Null
$task=Await ([Windows.ApplicationModel.StartupTask]::GetAsync('` + startupTaskID + `')) ([Windows.ApplicationModel.StartupTask])
`

// runStartupTaskPS runs the preamble plus action and returns the trimmed stdout
// (which the action prints with [Console]::Out.Write).
func runStartupTaskPS(action string) (string, error) {
	script := psPreamble + action
	c := exec.Command("powershell", "-NoProfile", "-NonInteractive",
		"-ExecutionPolicy", "Bypass", "-EncodedCommand", encodePS(script))
	c.SysProcAttr = hiddenProcAttr()
	out, err := c.CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		return s, fmt.Errorf("StartupTask via PowerShell: %v: %s", err, s)
	}
	return s, nil
}

// encodePS encodes a script as UTF-16LE base64 for powershell -EncodedCommand,
// sidestepping all shell quoting of the (multi-line) script.
func encodePS(script string) string {
	u := utf16.Encode([]rune(script))
	b := make([]byte, len(u)*2)
	for i, r := range u {
		b[i*2] = byte(r)
		b[i*2+1] = byte(r >> 8)
	}
	return base64.StdEncoding.EncodeToString(b)
}

// startupEnabled is true for the StartupTaskState values that mean "will run at
// logon" (Enabled / EnabledByPolicy).
func startupEnabled(state string) bool {
	return strings.EqualFold(state, "Enabled") || strings.EqualFold(state, "EnabledByPolicy")
}

// autoStartEnabled reports whether the MSIX StartupTask is currently enabled.
func autoStartEnabled() bool {
	out, err := runStartupTaskPS(`[Console]::Out.Write($task.State.ToString())`)
	if err != nil {
		return false
	}
	return startupEnabled(out)
}

// setAutoStart enables or disables the StartupTask. Enabling can be refused by the
// OS/user (DisabledByUser/DisabledByPolicy) — RequestEnableAsync returns the
// resulting state, which we surface as an error so the tray reflects reality.
func setAutoStart(enable bool) error {
	if enable {
		out, err := runStartupTaskPS(
			`$s=Await ($task.RequestEnableAsync()) ([Windows.ApplicationModel.StartupTaskState]); [Console]::Out.Write($s.ToString())`)
		if err != nil {
			return err
		}
		if !startupEnabled(out) {
			return fmt.Errorf("o Windows manteve o início automático desligado (estado: %s) — ative em Configurações → Apps → Inicialização", out)
		}
		return nil
	}
	_, err := runStartupTaskPS(`$task.Disable()`)
	return err
}
