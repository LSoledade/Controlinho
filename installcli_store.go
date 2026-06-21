//go:build store

package main

// The MSIX (store) build exposes no install CLI: the Microsoft Store handles
// install/uninstall, and auto-start is the manifest's StartupTask (toggled from the
// tray via install_store.go). These no-ops keep main() identical across builds.

func registerInstallFlags() {}

func runInstallFlags() bool { return false }
