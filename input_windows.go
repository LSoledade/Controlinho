//go:build windows

package main

import (
	"strings"
	"syscall"
	"unicode/utf8"
)

// Windows input via direct user32.dll syscalls — no CGO / GCC needed.
// Uses mouse_event and keybd_event (the same subsystem robotgo drives under the hood).
// These are "deprecated" in the docs but fully functional on every supported Windows build,
// and avoid the fiddly INPUT-union struct alignment that SendInput demands.

var (
	user32 = syscall.NewLazyDLL("user32.dll")

	procMouseEv    = user32.NewProc("mouse_event")
	procKeybdEv    = user32.NewProc("keybd_event")
	procGetSysMet  = user32.NewProc("GetSystemMetrics")
	procMapVk      = user32.NewProc("MapVirtualKeyW")
	procSendMsg    = user32.NewProc("SendMessageW")
	procSendMsgTmo = user32.NewProc("SendMessageTimeoutW")
	procGetConWnd  = syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleWindow")
)

// mouse_event flags
const (
	mMove     = 0x0001
	mLeftD    = 0x0002
	mLeftU    = 0x0004
	mRightD   = 0x0008
	mRightU   = 0x0010
	mMiddleD  = 0x0020
	mMiddleU  = 0x0040
	mWheel    = 0x0800
	mAbsolute = 0x8000
)

// keybd_event flags
const (
	kExtended = 0x0001 // KEYEVENTF_EXTENDEDKEY — scan code prefixed by 0xE0
	kDown     = 0x0000
	kUp       = 0x0002
	kUnicode  = 0x0004
	kScan     = 0x0008
)

// extendedVK lists the virtual keys whose scan code carries the 0xE0 prefix and
// therefore need KEYEVENTF_EXTENDEDKEY. Without the flag, MapVirtualKey returns
// the numeric-keypad scan code, so e.g. the arrow keys would be interpreted as
// numpad digits when Num Lock is on. Source: Win32 "About Keyboard Input"
// (extended-key flag) + the Consumer-page scan-code table (media/volume/browser).
var extendedVK = map[uint16]bool{
	0x21: true, 0x22: true, // PageUp, PageDown
	0x23: true, 0x24: true, // End, Home
	0x25: true, 0x26: true, 0x27: true, 0x28: true, // arrows
	0x2D: true, 0x2E: true, // Insert, Delete
	0x2C: true, // PrintScreen
	0x90: true, // NumLock
	0xA3: true, 0xA5: true, // right Ctrl, right Alt
	0x5B: true, 0x5C: true, // left/right Win (GUI)
	0xAD: true, 0xAE: true, 0xAF: true, // volume mute/down/up
	0xB0: true, 0xB1: true, 0xB2: true, 0xB3: true, // media next/prev/stop/play
	0xA6: true, 0xA7: true, // browser back/forward
}

// GetSystemMetrics indices
const (
	smCXScreen = 0
	smCYScreen = 1
)

// MapVirtualKey map types
const (
	mapVkToSc = 0
)

// Window messages for monitor power control
const (
	wmSysCommand   = 0x0112
	scMonitorPower = 0xF170
	scMonitorOff   = 2
	hwndBroadcast  = 0xFFFF
	smtoAbortIfHung = 0x0008
)

// hiddenProcAttr makes spawned power commands run without flashing a console window.
func hiddenProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{HideWindow: true}
}

// hasConsole reports whether a console window is attached. A hidden
// Task-Scheduler launch (windowsgui build) has none, so we skip QR/auto-open.
func hasConsole() bool {
	h, _, _ := procGetConWnd.Call()
	return h != 0
}

func screenW() int {
	r, _, _ := procGetSysMet.Call(smCXScreen)
	return int(r)
}

func screenH() int {
	r, _, _ := procGetSysMet.Call(smCYScreen)
	return int(r)
}

func mapVirtualKey(vk uint16) uint16 {
	r, _, _ := procMapVk.Call(uintptr(vk), mapVkToSc)
	return uint16(r)
}

// --- Mouse ---

// mouseMoveRelative moves the cursor by (dx, dy) pixels.
func mouseMoveRelative(dx, dy int) {
	procMouseEv.Call(mMove, uintptr(dx), uintptr(dy), 0, 0)
}

// mouseMoveAbsolute moves the cursor to (x, y) in pixels.
func mouseMoveAbsolute(x, y int) {
	nx := uintptr((x * 65535) / max(screenW()-1, 1))
	ny := uintptr((y * 65535) / max(screenH()-1, 1))
	procMouseEv.Call(mMove|mAbsolute, nx, ny, 0, 0)
}

// mouseButtonFlags returns the (down, up) mouse_event flags for a named button.
func mouseButtonFlags(button string) (down, up uintptr, ok bool) {
	switch button {
	case "left", "":
		return mLeftD, mLeftU, true
	case "right":
		return mRightD, mRightU, true
	case "middle":
		return mMiddleD, mMiddleU, true
	default:
		return 0, 0, false
	}
}

// mouseClick presses and releases a button.
func mouseClick(button string) {
	down, up, ok := mouseButtonFlags(button)
	if !ok {
		return
	}
	procMouseEv.Call(down, 0, 0, 0, 0)
	procMouseEv.Call(up, 0, 0, 0, 0)
}

// mouseButton holds (press=true) or releases (press=false) a button — used for
// click-and-drag, where the button stays down while the cursor moves.
func mouseButton(button string, press bool) {
	down, up, ok := mouseButtonFlags(button)
	if !ok {
		return
	}
	if press {
		procMouseEv.Call(down, 0, 0, 0, 0)
	} else {
		procMouseEv.Call(up, 0, 0, 0, 0)
	}
}

// mouseScroll scrolls by delta ticks. Positive = up, negative = down.
// One wheel notch is WHEEL_DELTA (120).
func mouseScroll(delta int) {
	procMouseEv.Call(mWheel, 0, 0, uintptr(int32(delta*120)), 0)
}

// --- Keyboard ---

// vkAliases maps friendly names to Win32 virtual-key codes.
var vkAliases = map[string]uint16{
	// modifiers
	"ctrl": 0x11, "control": 0x11,
	"alt": 0x12, "menu": 0x12,
	"shift": 0x10,
	"win": 0x5B, "super": 0x5B, "meta": 0x5B, "cmd": 0x5B, "windows": 0x5B,
	// editing / navigation
	"enter": 0x0D, "return": 0x0D,
	"esc": 0x1B, "escape": 0x1B,
	"tab": 0x09,
	"backspace": 0x08, "bs": 0x08, "back": 0x08,
	"space": 0x20,
	"del": 0x2E, "delete": 0x2E,
	"insert": 0x2D, "ins": 0x2D,
	"home": 0x24, "end": 0x23,
	"pageup": 0x21, "pgup": 0x21,
	"pagedown": 0x22, "pgdn": 0x22,
	"up": 0x26, "down": 0x28,
	"left": 0x25, "right": 0x27,
	"capslock": 0x14, "numlock": 0x90,
	"printscreen": 0x2C, "prtsc": 0x2C,
	"scrolllock": 0x91,
	// function keys
	"f1": 0x70, "f2": 0x71, "f3": 0x72, "f4": 0x73,
	"f5": 0x74, "f6": 0x75, "f7": 0x76, "f8": 0x77,
	"f9": 0x78, "f10": 0x79, "f11": 0x7A, "f12": 0x7B,
	// media
	"mediaplay": 0xB3, "media_play_pause": 0xB3, "playpause": 0xB3,
	"medianext": 0xB0, "media_next": 0xB0,
	"mediaprev": 0xB1, "media_prev": 0xB1,
	"mediastop": 0xB2, "media_stop": 0xB2,
	// volume
	"volumeup": 0xAF, "volume_up": 0xAF,
	"volumedown": 0xAE, "volume_down": 0xAE,
	"volumemute": 0xAD, "volume_mute": 0xAD,
	// browser
	"browserback": 0xA6, "browser_back": 0xA6,
	"browserforward": 0xA7, "browser_forward": 0xA7,
}

// vkFor resolves a token to a virtual-key code.
// A single printable ASCII char (a-z, 0-9, punctuation) maps to its own VK.
func vkFor(token string) (uint16, bool) {
	if token == "" {
		return 0, false
	}
	if v, ok := vkAliases[strings.ToLower(token)]; ok {
		return v, true
	}
	if len(token) == 1 {
		c := token[0]
		if c >= 0x20 && c <= 0x7E {
			return uint16(c), true
		}
	}
	return 0, false
}

// keyDown / keyUp send a single key transition, setting KEYEVENTF_EXTENDEDKEY
// for the keys that require it (arrows, nav cluster, media, right modifiers…).
func keyDown(vk uint16) {
	flags := uintptr(kDown)
	if extendedVK[vk] {
		flags |= kExtended
	}
	procKeybdEv.Call(uintptr(vk), uintptr(mapVirtualKey(vk)), flags, 0)
}

func keyUp(vk uint16) {
	flags := uintptr(kUp)
	if extendedVK[vk] {
		flags |= kExtended
	}
	procKeybdEv.Call(uintptr(vk), uintptr(mapVirtualKey(vk)), flags, 0)
}

// pressKey taps a single key.
func pressKey(vk uint16) {
	keyDown(vk)
	keyUp(vk)
}

// tapChord holds all keys down in order, then releases them in reverse —
// the correct way to send a multi-key combination.
func tapChord(vks []uint16) {
	if len(vks) == 0 {
		return
	}
	for _, vk := range vks {
		keyDown(vk)
	}
	for i := len(vks) - 1; i >= 0; i-- {
		keyUp(vks[i])
	}
}

// --- Text input ---

// typeText types a Unicode string via the KEYEVENTF_UNICODE path.
func typeText(s string) {
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		if r == utf8.RuneError {
			s = s[size:]
			continue
		}
		procKeybdEv.Call(0, uintptr(r), kUnicode|kScan, 0)
		procKeybdEv.Call(0, uintptr(r), kUnicode|kScan|kUp, 0)
		s = s[size:]
	}
}

// --- Monitor off ---

// monitorOff turns the display off. It first tries the console window; if there
// is none (e.g. launched by Task Scheduler hidden), it broadcasts the command.
func monitorOff() {
	hwnd, _, _ := procGetConWnd.Call()
	if hwnd != 0 {
		procSendMsg.Call(hwnd, wmSysCommand, scMonitorPower, scMonitorOff)
		return
	}
	procSendMsgTmo.Call(
		uintptr(hwndBroadcast),
		wmSysCommand,
		scMonitorPower,
		scMonitorOff,
		smtoAbortIfHung,
		1000,
		0,
	)
}
