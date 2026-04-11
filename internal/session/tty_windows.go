//go:build windows

package session

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleMode = kernel32.NewProc("GetConsoleMode")
)

// IsInteractive reports whether the given file descriptor is a terminal.
func IsInteractive(f *os.File) bool {
	var mode uint32
	r1, _, _ := procGetConsoleMode.Call(f.Fd(), uintptr(unsafe.Pointer(&mode)))
	return r1 != 0
}
