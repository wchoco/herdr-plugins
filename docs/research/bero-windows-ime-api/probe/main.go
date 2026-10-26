//go:build windows

// Command probe reads (never sets) the IME open state of the foreground
// window from another process, the way an IME Agent would, and prints what
// each step returned. It exists to back docs/research/bero-windows-ime-api.md.
//
// Build: GOOS=windows GOARCH=amd64 go build -o probe.exe ./docs/research/bero-windows-ime-api/probe
// Run:   ./probe.exe [-n N] [-wait SECONDS]
package main

import (
	"flag"
	"fmt"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	imm32    = syscall.NewLazyDLL("imm32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	pGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	pGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	pGetGUIThreadInfo         = user32.NewProc("GetGUIThreadInfo")
	pGetClassNameW            = user32.NewProc("GetClassNameW")
	pSendMessageTimeoutW      = user32.NewProc("SendMessageTimeoutW")
	pGetKeyboardLayout        = user32.NewProc("GetKeyboardLayout")
	pGetAncestor              = user32.NewProc("GetAncestor")

	pImmGetDefaultIMEWnd = imm32.NewProc("ImmGetDefaultIMEWnd")
	pImmGetContext       = imm32.NewProc("ImmGetContext")
	pImmGetOpenStatus    = imm32.NewProc("ImmGetOpenStatus")
	pImmReleaseContext   = imm32.NewProc("ImmReleaseContext")

	pOpenProcess               = kernel32.NewProc("OpenProcess")
	pQueryFullProcessImageName = kernel32.NewProc("QueryFullProcessImageNameW")
	pCloseHandle               = kernel32.NewProc("CloseHandle")
)

const (
	wmIMEControl         = 0x0283
	imcGetConversionMode = 0x0001
	imcGetOpenStatus     = 0x0005
	smtoAbortIfHung      = 0x0002
	gaRoot               = 2
	processQueryLimited  = 0x1000
)

// guiThreadInfo mirrors GUITHREADINFO on 64-bit Windows.
type guiThreadInfo struct {
	cbSize        uint32
	flags         uint32
	hwndActive    uintptr
	hwndFocus     uintptr
	hwndCapture   uintptr
	hwndMenuOwner uintptr
	hwndMoveSize  uintptr
	hwndCaret     uintptr
	rcCaret       [4]int32
}

func className(h uintptr) string {
	if h == 0 {
		return "-"
	}
	buf := make([]uint16, 256)
	n, _, _ := pGetClassNameW.Call(h, uintptr(unsafe.Pointer(&buf[0])), 256)
	return syscall.UTF16ToString(buf[:n])
}

func threadPid(h uintptr) (tid, pid uint32) {
	t, _, _ := pGetWindowThreadProcessId.Call(h, uintptr(unsafe.Pointer(&pid)))
	return uint32(t), pid
}

func exeName(pid uint32) string {
	hp, _, _ := pOpenProcess.Call(processQueryLimited, 0, uintptr(pid))
	if hp == 0 {
		return "?"
	}
	defer pCloseHandle.Call(hp)
	buf := make([]uint16, 1024)
	size := uint32(len(buf))
	r, _, _ := pQueryFullProcessImageName.Call(hp, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r == 0 {
		return "?"
	}
	return syscall.UTF16ToString(buf[:size])
}

// imeControl sends WM_IME_CONTROL with a read-only command and a 200 ms timeout.
func imeControl(imeWnd, cmd uintptr) (val uintptr, ok bool, took time.Duration) {
	start := time.Now()
	r, _, _ := pSendMessageTimeoutW.Call(imeWnd, wmIMEControl, cmd, 0, smtoAbortIfHung, 200, uintptr(unsafe.Pointer(&val)))
	return val, r != 0, time.Since(start)
}

func describe(label string, h uintptr) {
	tid, pid := threadPid(h)
	imeWnd, _, _ := pImmGetDefaultIMEWnd.Call(h)
	fmt.Printf("  %-8s hwnd=%#x class=%q tid=%d pid=%d exe=%s\n", label, h, className(h), tid, pid, exeName(pid))
	hkl, _, _ := pGetKeyboardLayout.Call(uintptr(tid))
	fmt.Printf("           HKL=%#x  ImmGetDefaultIMEWnd=%#x class=%q\n", hkl, imeWnd, className(imeWnd))
	if imeWnd != 0 {
		open, ok, took := imeControl(imeWnd, imcGetOpenStatus)
		conv, ok2, took2 := imeControl(imeWnd, imcGetConversionMode)
		fmt.Printf("           IMC_GETOPENSTATUS=%d (ok=%v, %v)  IMC_GETCONVERSIONMODE=%#x (ok=%v, %v)\n", open, ok, took, conv, ok2, took2)
	}
	himc, _, _ := pImmGetContext.Call(h)
	if himc != 0 {
		st, _, _ := pImmGetOpenStatus.Call(himc)
		pImmReleaseContext.Call(h, himc)
		fmt.Printf("           ImmGetContext=%#x ImmGetOpenStatus=%d\n", himc, st)
	} else {
		fmt.Printf("           ImmGetContext=NULL (cross-process)\n")
	}
}

func main() {
	n := flag.Int("n", 1, "number of samples")
	wait := flag.Float64("wait", 0, "seconds between samples")
	flag.Parse()
	for i := 0; i < *n; i++ {
		if i > 0 {
			time.Sleep(time.Duration(*wait * float64(time.Second)))
		}
		fg, _, _ := pGetForegroundWindow.Call()
		fmt.Printf("sample %d at %s\n", i, time.Now().Format("15:04:05.000"))
		if fg == 0 {
			fmt.Println("  no foreground window")
			continue
		}
		describe("fg", fg)
		tid, _ := threadPid(fg)
		gti := guiThreadInfo{cbSize: uint32(unsafe.Sizeof(guiThreadInfo{}))}
		r, _, _ := pGetGUIThreadInfo.Call(uintptr(tid), uintptr(unsafe.Pointer(&gti)))
		if r != 0 && gti.hwndFocus != 0 && gti.hwndFocus != fg {
			root, _, _ := pGetAncestor.Call(gti.hwndFocus, gaRoot)
			describe("focus", gti.hwndFocus)
			fmt.Printf("           focus root=%#x\n", root)
		} else if r == 0 {
			fmt.Println("  GetGUIThreadInfo failed")
		} else {
			fmt.Println("  focus == fg (or none)")
		}
	}
}
