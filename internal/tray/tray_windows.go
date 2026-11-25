//go:build windows

package tray

import (
	"fmt"
	"os/exec"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"github.com/Mostakim2005/cloud-manager/internal/branding"
	"golang.org/x/sys/windows"
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	shell32  = windows.NewLazySystemDLL("shell32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procGetModuleHandleW         = kernel32.NewProc("GetModuleHandleW")
	procRegisterClassExW         = user32.NewProc("RegisterClassExW")
	procCreateWindowExW          = user32.NewProc("CreateWindowExW")
	procDefWindowProcW           = user32.NewProc("DefWindowProcW")
	procDestroyWindow            = user32.NewProc("DestroyWindow")
	procGetMessageW              = user32.NewProc("GetMessageW")
	procTranslateMessage         = user32.NewProc("TranslateMessage")
	procDispatchMessageW         = user32.NewProc("DispatchMessageW")
	procPostQuitMessage          = user32.NewProc("PostQuitMessage")
	procPostMessageW             = user32.NewProc("PostMessageW")
	procLoadIconW                = user32.NewProc("LoadIconW")
	procCreateIconFromResourceEx = user32.NewProc("CreateIconFromResourceEx")
	procDestroyIcon              = user32.NewProc("DestroyIcon")
	procGetSystemMetrics         = user32.NewProc("GetSystemMetrics")
	procCreatePopupMenu          = user32.NewProc("CreatePopupMenu")
	procAppendMenuW              = user32.NewProc("AppendMenuW")
	procDestroyMenu              = user32.NewProc("DestroyMenu")
	procTrackPopupMenu           = user32.NewProc("TrackPopupMenu")
	procSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	procGetCursorPos             = user32.NewProc("GetCursorPos")
	procShellNotifyIconW         = shell32.NewProc("Shell_NotifyIconW")
)

const (
	wmDestroy      = 0x0002
	wmCommand      = 0x0111
	wmUser         = 0x0400
	wmTrayIcon     = wmUser + 1
	wmLButtonDbl   = 0x0203
	wmRButtonUp    = 0x0205
	nimAdd         = 0x00000000
	nimDelete      = 0x00000002
	nifMessage     = 0x00000001
	nifIcon        = 0x00000002
	nifTip         = 0x00000004
	mfString       = 0x00000000
	mfSeparator    = 0x00000800
	tpmBottomAlign = 0x0020
	tpmLeftAlign   = 0x0000
	idiApplication = 32512
	smCXIcon       = 11
	smCXSmIcon     = 49
	iconVersion    = 0x00030000

	cmdOpenDashboard = 1001
	cmdToggleLAN     = 1002
	cmdQuit          = 1003
)

type notifyIconData struct {
	cbSize           uint32
	hWnd             uintptr
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            uintptr
	szTip            [128]uint16
}

type wndClassExW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type point struct {
	x int32
	y int32
}

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
}

// Callbacks wires tray menu actions to daemon controls.
type Callbacks struct {
	DashboardURL string
	OnToggleLAN  func()
	OnQuit       func()
}

// Controller manages the Windows System Tray icon without CGO.
type Controller struct {
	mu   sync.Mutex
	hwnd uintptr
}

var activeController *Controller
var activeCallbacks Callbacks

// OpenBrowser opens url in the user's default Windows browser.
func OpenBrowser(url string) error {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}

// Start creates the hidden Win32 tray message loop in a dedicated OS-locked goroutine.
func Start(cb Callbacks) *Controller {
	ctrl := &Controller{}
	activeController = ctrl
	activeCallbacks = cb

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		hInst, _, _ := procGetModuleHandleW.Call(0)
		className, _ := syscall.UTF16PtrFromString("CloudManagerTrayWindow")
		smIcon, lgIcon, customIcons, cleanupIcons := loadTrayIcons()
		defer cleanupIcons()
		_ = customIcons

		wc := wndClassExW{
			cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
			lpfnWndProc:   syscall.NewCallback(wndProc),
			hInstance:     hInst,
			hIcon:         lgIcon,
			hIconSm:       smIcon,
			lpszClassName: className,
		}
		procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

		hwnd, _, _ := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(className)), 0, 0, 0, 0, 0, 0, 0, hInst, 0)
		if hwnd == 0 {
			return
		}
		ctrl.mu.Lock()
		ctrl.hwnd = hwnd
		ctrl.mu.Unlock()

		var nid notifyIconData
		nid.cbSize = uint32(unsafe.Sizeof(nid))
		nid.hWnd = hwnd
		nid.uID = 1
		nid.uFlags = nifMessage | nifIcon | nifTip
		nid.uCallbackMessage = wmTrayIcon
		nid.hIcon = smIcon
		tip, _ := syscall.UTF16FromString("Cloud Manager (:59432)")
		copy(nid.szTip[:], tip)
		procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))

		var m msg
		for {
			ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if int32(ret) <= 0 {
				break
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
		}
		procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
	}()
	return ctrl
}

// Stop tears down the Windows System Tray icon.
func (c *Controller) Stop() {
	if c == nil {
		return
	}
	c.mu.Lock()
	hwnd := c.hwnd
	c.hwnd = 0
	c.mu.Unlock()
	if hwnd != 0 {
		procPostMessageW.Call(hwnd, wmDestroy, 0, 0)
	}
}

func wndProc(hwnd uintptr, uMsg uint32, wParam, lParam uintptr) uintptr {
	switch uMsg {
	case wmTrayIcon:
		switch lParam {
		case wmLButtonDbl:
			if activeCallbacks.DashboardURL != "" {
				_ = OpenBrowser(activeCallbacks.DashboardURL)
			}
		case wmRButtonUp:
			showContextMenu(hwnd)
		}
		return 0
	case wmCommand:
		switch int(wParam & 0xffff) {
		case cmdOpenDashboard:
			if activeCallbacks.DashboardURL != "" {
				_ = OpenBrowser(activeCallbacks.DashboardURL)
			}
		case cmdToggleLAN:
			if activeCallbacks.OnToggleLAN != nil {
				go activeCallbacks.OnToggleLAN()
			}
		case cmdQuit:
			if activeCallbacks.OnQuit != nil {
				go activeCallbacks.OnQuit()
			}
			procDestroyWindow.Call(hwnd)
		}
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(uMsg), wParam, lParam)
	return ret
}

func showContextMenu(hwnd uintptr) {
	hMenu, _, _ := procCreatePopupMenu.Call()
	if hMenu == 0 {
		return
	}
	defer procDestroyMenu.Call(hMenu)

	lblOpen, _ := syscall.UTF16PtrFromString("Open Cloud Manager UI")
	lblLAN, _ := syscall.UTF16PtrFromString("Toggle LAN QR Drop-Zone (:53317)")
	lblQuit, _ := syscall.UTF16PtrFromString(fmt.Sprintf("Quit Cloud Manager"))

	procAppendMenuW.Call(hMenu, mfString, cmdOpenDashboard, uintptr(unsafe.Pointer(lblOpen)))
	procAppendMenuW.Call(hMenu, mfString, cmdToggleLAN, uintptr(unsafe.Pointer(lblLAN)))
	procAppendMenuW.Call(hMenu, mfSeparator, 0, 0)
	procAppendMenuW.Call(hMenu, mfString, cmdQuit, uintptr(unsafe.Pointer(lblQuit)))

	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWindow.Call(hwnd)
	procTrackPopupMenu.Call(hMenu, tpmBottomAlign|tpmLeftAlign, uintptr(pt.x), uintptr(pt.y), 0, hwnd, 0)
}

func loadTrayIcons() (smIcon, lgIcon uintptr, custom bool, cleanup func()) {
	smPx := metricClamped(smCXSmIcon, 32, 16, 64)
	lgPx := metricClamped(smCXIcon, 48, 32, 128)

	var destroyList []uintptr
	if ic := createPNGIcon(smPx); ic != 0 {
		smIcon = ic
		destroyList = append(destroyList, ic)
	}
	if ic := createPNGIcon(lgPx); ic != 0 {
		lgIcon = ic
		destroyList = append(destroyList, ic)
	}
	custom = smIcon != 0
	if smIcon == 0 {
		smIcon, _, _ = procLoadIconW.Call(0, uintptr(idiApplication))
	}
	if lgIcon == 0 {
		lgIcon = smIcon
	}
	return smIcon, lgIcon, custom, func() {
		for _, ic := range destroyList {
			procDestroyIcon.Call(ic)
		}
	}
}

func metricClamped(index, fallback, minVal, maxVal int) int {
	val, _, _ := procGetSystemMetrics.Call(uintptr(index))
	n := int(val)
	if n < minVal || n > maxVal {
		return fallback
	}
	return n
}

func createPNGIcon(size int) uintptr {
	pngBytes, err := branding.PNG(size)
	if err != nil || len(pngBytes) == 0 {
		return 0
	}
	hIcon, _, _ := procCreateIconFromResourceEx.Call(
		uintptr(unsafe.Pointer(&pngBytes[0])),
		uintptr(len(pngBytes)),
		1,
		uintptr(iconVersion),
		uintptr(size),
		uintptr(size),
		0,
	)
	runtime.KeepAlive(pngBytes)
	return hIcon
}
