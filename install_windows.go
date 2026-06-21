//go:build windows && !store

package main

// Self-install (desktop .exe build only — excluded from the MSIX `store` build).
// Fold what install.bat used to do into the binary so a single
// .exe can register itself to auto-start at logon and open the firewall.
//
//   pc-remote.exe -install     register the logon task + firewall rule, start now
//   pc-remote.exe -uninstall   remove the task + firewall rule
//
// The logon task runs as the current user (no admin needed) — essential, because
// input injection only works from the interactive session, never from a Windows
// service in Session 0. The firewall rule needs admin, so we try it directly and,
// if that's refused, re-launch ourselves elevated (UAC) to add just the rule.

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"
)

const taskName = "pc-remote"

var (
	shell32          = syscall.NewLazyDLL("shell32.dll")
	procShellExecute = shell32.NewProc("ShellExecuteW")
)

const swHide = 0

// runHidden runs a command without flashing a console window and returns a
// helpful error (including the command's output) if it fails.
func runHidden(name string, args ...string) error {
	c := exec.Command(name, args...)
	c.SysProcAttr = hiddenProcAttr()
	out, err := c.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %v: %s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// taskInstalled reports whether our logon task is currently registered.
func taskInstalled() bool {
	c := exec.Command("schtasks", "/query", "/tn", taskName)
	c.SysProcAttr = hiddenProcAttr()
	return c.Run() == nil
}

// addFirewallRule opens TCP 8080/8443 inbound. Needs admin; returns an error if
// not elevated. (-setupfw re-enters here from the elevated child process.)
func addFirewallRule() error {
	_ = runHidden("netsh", "advfirewall", "firewall", "delete", "rule", "name=pc-remote")
	return runHidden("netsh", "advfirewall", "firewall", "add", "rule",
		"name=pc-remote", "dir=in", "action=allow", "protocol=TCP", "localport=8080,8443")
}

// shellExecuteRunAs re-launches the given executable elevated (UAC), hidden.
// Returns nil only when ShellExecuteW reports success (> 32).
func shellExecuteRunAs(exe string, args ...string) error {
	verb, _ := syscall.UTF16PtrFromString("runas")
	file, _ := syscall.UTF16PtrFromString(exe)
	params, _ := syscall.UTF16PtrFromString(strings.Join(args, " "))
	r, _, callErr := procShellExecute.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(params)),
		0,
		uintptr(swHide),
	)
	if r <= 32 { // ShellExecuteW: > 32 == success
		return fmt.Errorf("ShellExecuteW falhou (código %d): %v", r, callErr)
	}
	return nil
}

// ensureFirewall adds the rule directly, falling back to an elevated re-launch
// (pc-remote.exe -setupfw) if we lack admin rights.
func ensureFirewall() {
	if err := addFirewallRule(); err == nil {
		log.Printf("[install] regra de firewall criada para as portas 8080/8443.")
		return
	}
	log.Printf("[install] abrindo o firewall com elevação (UAC)…")
	exe, err := os.Executable()
	if err == nil {
		if err = shellExecuteRunAs(exe, "-setupfw"); err == nil {
			log.Printf("[install] solicitação de firewall enviada (confirme o prompt do UAC).")
			return
		}
	}
	log.Printf("[install] AVISO: não foi possível abrir o firewall automaticamente.")
	log.Printf("[install]        Rode 'pc-remote.exe -install' como administrador,")
	log.Printf("[install]        ou libere as portas 8080 e 8443 manualmente no Firewall do Windows.")
}

// installSelf registers the logon task and firewall rule. When startNow is true
// (the CLI -install path) it also kicks off the task so the server runs without
// a log-off/on cycle; the tray toggle passes false (an instance is already up).
func installSelf(startNow bool) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolver o caminho do executável: %w", err)
	}

	// Re-register cleanly.
	_ = runHidden("schtasks", "/end", "/tn", taskName)
	_ = runHidden("schtasks", "/delete", "/tn", taskName, "/f")
	if err := runHidden("schtasks", "/create", "/tn", taskName,
		"/tr", `"`+exe+`"`, "/sc", "onlogon", "/rl", "limited", "/f"); err != nil {
		return fmt.Errorf("registrar a tarefa de logon: %w", err)
	}
	log.Printf("[install] tarefa \"%s\" criada (inicia no logon).", taskName)

	ensureFirewall()

	if startNow {
		if err := runHidden("schtasks", "/run", "/tn", taskName); err != nil {
			log.Printf("[install] não consegui iniciar agora (%v) — iniciará no próximo logon.", err)
		} else {
			log.Printf("[install] servidor iniciado.")
		}
	}
	return nil
}

// uninstallSelf removes the logon task and the firewall rule (best effort).
func uninstallSelf() error {
	_ = runHidden("schtasks", "/end", "/tn", taskName)
	if err := runHidden("schtasks", "/delete", "/tn", taskName, "/f"); err != nil {
		return fmt.Errorf("remover a tarefa: %w", err)
	}
	log.Printf("[uninstall] tarefa \"%s\" removida.", taskName)
	if err := runHidden("netsh", "advfirewall", "firewall", "delete", "rule", "name=pc-remote"); err != nil {
		log.Printf("[uninstall] não removi a regra de firewall (talvez precise de admin): %v", err)
	} else {
		log.Printf("[uninstall] regra de firewall removida.")
	}
	return nil
}

// --- auto-start abstraction (consumed by tray_windows.go) ---
//
// The tray's "Iniciar com o Windows" toggle calls these. The desktop build backs
// them with the logon Task Scheduler entry above; the MSIX build (install_store.go)
// swaps in a WinRT StartupTask implementation. Same signatures on both sides so the
// tray code is identical across builds.

// autoStartEnabled reports whether auto-start at logon is currently active.
func autoStartEnabled() bool { return taskInstalled() }

// setAutoStart enables or disables auto-start at logon. On enable we pass
// startNow=false: an instance (this one) is already running, so we only register.
func setAutoStart(enable bool) error {
	if enable {
		return installSelf(false)
	}
	return uninstallSelf()
}
