//go:build !windows

package main

import "fmt"

// Auto-start install is Windows-only (Task Scheduler + firewall). These stubs
// keep the project building and vetting on Linux/macOS during development.

func installSelf(bool) error { return fmt.Errorf("--install só é suportado no Windows") }
func uninstallSelf() error   { return fmt.Errorf("--uninstall só é suportado no Windows") }
func taskInstalled() bool    { return false }
func addFirewallRule() error { return nil }
