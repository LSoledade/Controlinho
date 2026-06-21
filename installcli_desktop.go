//go:build !store

package main

// CLI install commands for the desktop .exe build. The MSIX (store) build has no
// such flags — the Microsoft Store installs/uninstalls and manages auto-start — so
// these are compiled out there (see installcli_store.go), keeping main() identical
// across builds.

import (
	"flag"
	"log"
)

var (
	flagInstall   *bool
	flagUninstall *bool
	flagSetupFW   *bool
)

// registerInstallFlags declares -install/-uninstall/-setupfw. Call before flag.Parse.
func registerInstallFlags() {
	flagInstall = flag.Bool("install", false, "register auto-start at logon + open the firewall, then start")
	flagUninstall = flag.Bool("uninstall", false, "remove the auto-start task and firewall rule")
	flagSetupFW = flag.Bool("setupfw", false, "(internal) add the firewall rule then exit — used for UAC elevation")
}

// runInstallFlags executes whichever install command was requested and returns
// true when it handled one (so main() should exit).
func runInstallFlags() bool {
	switch {
	case *flagSetupFW:
		// Internal: the elevated child spawned by -install adds the firewall rule and exits.
		if err := addFirewallRule(); err != nil {
			log.Fatalf("firewall: %v", err)
		}
		return true
	case *flagInstall:
		if err := installSelf(true); err != nil {
			log.Fatalf("install: %v", err)
		}
		log.Printf("pronto. No celular: abra http://SEU-IP:8080 e siga o passo a passo (ou veja a bandeja do sistema).")
		return true
	case *flagUninstall:
		if err := uninstallSelf(); err != nil {
			log.Fatalf("uninstall: %v", err)
		}
		return true
	}
	return false
}
