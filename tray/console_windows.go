//go:build windows

package tray

import (
	"os"
	"syscall"
	"unsafe"

	"github.com/rs/zerolog/log"
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	user32           = syscall.NewLazyDLL("user32.dll")
	getConsoleWindow = kernel32.NewProc("GetConsoleWindow")
	showWindow       = user32.NewProc("ShowWindow")
	allocConsoleProc = kernel32.NewProc("AllocConsole")
	setStdHandle     = kernel32.NewProc("SetStdHandle")
)

const (
	SW_HIDE = 0
	SW_SHOW = 5

	STD_INPUT_HANDLE  = ^uintptr(0) - 10 + 1 // -10
	STD_OUTPUT_HANDLE = ^uintptr(0) - 11 + 1 // -11
	STD_ERROR_HANDLE  = ^uintptr(0) - 12 + 1 // -12
)

// consoleWindow stores the console window handle
var consoleWindow uintptr

func init() {
	// Get console window handle at startup
	consoleWindow, _, _ = getConsoleWindow.Call()
}

// InitConsole creates a new console window for GUI applications
// Call this at startup if built with -H windowsgui
func InitConsole() {
	// Check if we already have a console
	hwnd, _, _ := getConsoleWindow.Call()
	if hwnd != 0 {
		consoleWindow = hwnd
		// Even if console exists, ensure IO is redirected
		redirectConsoleIO()
		return
	}

	// Allocate a new console
	ret, _, _ := allocConsoleProc.Call()
	if ret == 0 {
		return
	}

	// Get new console window handle
	consoleWindow, _, _ = getConsoleWindow.Call()

	// Redirect stdout/stderr to new console
	redirectConsoleIO()
}

// redirectConsoleIO redirects stdout/stderr to the allocated console
func redirectConsoleIO() {
	// Open CONOUT$ for writing
	conout, err := syscall.Open("CONOUT$", syscall.O_RDWR, 0)
	if err != nil {
		return
	}

	// Redirect Go's os.Stdout and os.Stderr
	os.Stdout = os.NewFile(uintptr(conout), "/dev/stdout")
	os.Stderr = os.NewFile(uintptr(conout), "/dev/stderr")

	// Also set Windows standard handles
	setStdHandle.Call(STD_OUTPUT_HANDLE, uintptr(conout))
	setStdHandle.Call(STD_ERROR_HANDLE, uintptr(conout))
}

// HideConsole hides the console window
func HideConsole() {
	if consoleWindow != 0 {
		showWindow.Call(consoleWindow, SW_HIDE)
		log.Debug().Msg("Console window hidden")
	}
}

// ShowConsole shows the console window
func ShowConsole() {
	if consoleWindow != 0 {
		showWindow.Call(consoleWindow, SW_SHOW)
		log.Debug().Msg("Console window shown")
	}
}

// IsConsoleVisible checks if console is currently visible
func IsConsoleVisible() bool {
	if consoleWindow == 0 {
		return false
	}
	isVisible := user32.NewProc("IsWindowVisible")
	ret, _, _ := isVisible.Call(consoleWindow)
	return ret != 0
}


// DisableCloseButton disables the X button on console window
// User must use tray menu to hide console or exit
func DisableCloseButton() {
	if consoleWindow == 0 {
		return
	}

	getSystemMenu := user32.NewProc("GetSystemMenu")
	enableMenuItem := user32.NewProc("EnableMenuItem")

	const (
		MF_BYCOMMAND = 0x00000000
		MF_GRAYED    = 0x00000001
		MF_DISABLED  = 0x00000002
		SC_CLOSE     = 0xF060
	)

	// Get system menu
	hMenu, _, _ := getSystemMenu.Call(consoleWindow, 0)
	if hMenu != 0 {
		// Disable and gray out the close menu item
		enableMenuItem.Call(hMenu, SC_CLOSE, MF_BYCOMMAND|MF_DISABLED|MF_GRAYED)
		log.Debug().Msg("Console close button disabled")
	}
}

// SetConsoleTitle sets the console window title
func SetConsoleTitle(title string) {
	setConsoleTitleW := kernel32.NewProc("SetConsoleTitleW")
	titlePtr, _ := syscall.UTF16PtrFromString(title)
	setConsoleTitleW.Call(uintptr(unsafe.Pointer(titlePtr)))
}

