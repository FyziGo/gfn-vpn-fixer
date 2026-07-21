//go:build windows

package main

import (
	"log"
	"os"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// isAdmin returns true if the current process is running with elevated (Administrator) privileges.
func isAdmin() bool {
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return false
	}
	defer token.Close()
	return token.IsElevated()
}

// requireAdmin checks for admin rights. If not elevated, it re-launches the
// current executable via ShellExecuteW with the "runas" verb (triggers UAC
// prompt) and exits the current (non-elevated) process. This makes autostart
// from HKCU\...\Run work seamlessly — the user only sees a UAC dialog.
func requireAdmin() {
	if isAdmin() {
		return
	}

	exe, err := os.Executable()
	if err != nil {
		log.Fatalf("requireAdmin: cannot determine executable path: %v", err)
	}

	// Pass along any command-line flags (e.g. --setup) to the elevated process.
	args := strings.Join(os.Args[1:], " ")

	verbPtr, _ := syscall.UTF16PtrFromString("runas")
	exePtr, _ := syscall.UTF16PtrFromString(exe)
	argPtr, _ := syscall.UTF16PtrFromString(args)

	// ShellExecuteW returns > 32 on success.
	shell32 := windows.NewLazySystemDLL("shell32.dll")
	shellExecute := shell32.NewProc("ShellExecuteW")

	ret, _, _ := shellExecute.Call(
		0,
		uintptr(unsafe.Pointer(verbPtr)),
		uintptr(unsafe.Pointer(exePtr)),
		uintptr(unsafe.Pointer(argPtr)),
		0,
		1, // SW_SHOWNORMAL
	)

	if ret <= 32 {
		// UAC was declined or ShellExecute failed — show a message and exit.
		title, _ := windows.UTF16PtrFromString(L.PrivilegeTitle)
		msg, _ := windows.UTF16PtrFromString(L.PrivilegeMessage)
		windows.MessageBox(0, msg, title, 0x00000010|0x00001000)
	}

	// Exit the non-elevated process (elevated copy is now running).
	os.Exit(0)
}
