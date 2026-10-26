//go:build windows

// Command imeset reads and sets the IME open state of the foreground window
// from another process, to check whether IMC_SETOPENSTATUS takes effect in
// Windows Terminal.
//
// Usage: imeset get | off | on | keyoff | keyon | keytoggle
package main

import (
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32 = syscall.NewLazyDLL("user32.dll")
	imm32  = syscall.NewLazyDLL("imm32.dll")

	pGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	pGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	pGetGUIThreadInfo         = user32.NewProc("GetGUIThreadInfo")
	pSendMessageTimeoutW      = user32.NewProc("SendMessageTimeoutW")
	pSendInput                = user32.NewProc("SendInput")
	pImmGetDefaultIMEWnd      = imm32.NewProc("ImmGetDefaultIMEWnd")
)

const (
	wmIMEControl     = 0x0283
	imcGetOpenStatus = 0x0005
	imcSetOpenStatus = 0x0006
	smtoAbortIfHung  = 0x0002
	vkKanji          = 0x19
	vkIMEOn          = 0x16
	vkIMEOff         = 0x1A
	keyeventfKeyUp   = 0x0002
	inputKeyboard    = 1
)

type guiThreadInfo struct {
	cbSize                                                        uint32
	flags                                                         uint32
	hwndActive, hwndFocus, hwndCapture, hwndMenu, hwndMove, hwndC uintptr
	rcCaret                                                       [4]int32
}

// keyInput mirrors INPUT with a KEYBDINPUT on 64-bit Windows (40 bytes).
type keyInput struct {
	typ         uint32
	_           uint32
	wVk, wScan  uint16
	dwFlags     uint32
	time        uint32
	_           uint32
	dwExtraInfo uintptr
	_           [8]byte
}

func target() uintptr {
	fg, _, _ := pGetForegroundWindow.Call()
	var pid uint32
	tid, _, _ := pGetWindowThreadProcessId.Call(fg, uintptr(unsafe.Pointer(&pid)))
	gti := guiThreadInfo{cbSize: uint32(unsafe.Sizeof(guiThreadInfo{}))}
	if r, _, _ := pGetGUIThreadInfo.Call(tid, uintptr(unsafe.Pointer(&gti))); r != 0 && gti.hwndFocus != 0 {
		return gti.hwndFocus
	}
	return fg
}

func control(cmd, arg uintptr) (uintptr, bool) {
	ime, _, _ := pImmGetDefaultIMEWnd.Call(target())
	var val uintptr
	r, _, _ := pSendMessageTimeoutW.Call(ime, wmIMEControl, cmd, arg, smtoAbortIfHung, 200, uintptr(unsafe.Pointer(&val)))
	return val, r != 0
}

func press(vk uint16) {
	in := []keyInput{
		{typ: inputKeyboard, wVk: vk},
		{typ: inputKeyboard, wVk: vk, dwFlags: keyeventfKeyUp},
	}
	pSendInput.Call(uintptr(len(in)), uintptr(unsafe.Pointer(&in[0])), unsafe.Sizeof(in[0]))
}

func state() string {
	v, ok := control(imcGetOpenStatus, 0)
	if !ok {
		return "unknown (SendMessageTimeout failed)"
	}
	if v != 0 {
		return "On"
	}
	return "Off"
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: imeset get|off|on|keyoff|keyon|keytoggle")
		os.Exit(2)
	}
	fmt.Println("before:", state())
	switch os.Args[1] {
	case "get":
		return
	case "off":
		_, ok := control(imcSetOpenStatus, 0)
		fmt.Println("IMC_SETOPENSTATUS 0 sent, ok =", ok)
	case "on":
		_, ok := control(imcSetOpenStatus, 1)
		fmt.Println("IMC_SETOPENSTATUS 1 sent, ok =", ok)
	case "keyoff":
		press(vkIMEOff)
	case "keyon":
		press(vkIMEOn)
	case "keytoggle":
		press(vkKanji)
	default:
		fmt.Fprintln(os.Stderr, "unknown command")
		os.Exit(2)
	}
	time.Sleep(100 * time.Millisecond)
	fmt.Println("after: ", state())
}
