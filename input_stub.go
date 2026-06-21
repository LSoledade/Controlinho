//go:build !windows

package main

import (
	"log"
	"syscall"
)

// hiddenProcAttr is a no-op on non-Windows (no hidden-window concept).
func hiddenProcAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{} }

// hasConsole: dev builds always "have a console" for QR printing convenience.
func hasConsole() bool { return true }

// Non-Windows stubs so the project builds and vets on Linux/macOS during
// development. Input injection is Windows-only (see input_windows.go); here the
// commands are accepted but do nothing except log once-per-kind for visibility.

func mouseMoveRelative(dx, dy int) {}
func mouseMoveAbsolute(x, y int)   {}
func mouseClick(button string)     { log.Printf("input(stub): mouse_click %q", button) }
func mouseButton(button string, b bool) {
	log.Printf("input(stub): mouse_button %q down=%v", button, b)
}
func mouseScroll(delta int) {}
func pressKey(vk uint16)    { log.Printf("input(stub): key vk=0x%X", vk) }
func tapChord(vks []uint16) { log.Printf("input(stub): chord %v", vks) }
func typeText(s string)     { log.Printf("input(stub): type %q", s) }
func monitorOff()           { log.Printf("input(stub): monitor_off") }

// vkFor mirrors the Windows resolver enough to keep the dispatcher happy.
func vkFor(token string) (uint16, bool) {
	if len(token) == 1 {
		c := token[0]
		if c >= 0x20 && c <= 0x7E {
			return uint16(c), true
		}
	}
	return 0, true // accept named keys in dev so shortcuts don't error
}
