//go:build darwin

package session

import (
	"os"
	"syscall"
	"unsafe"
)

// IsInteractive reports whether the given file descriptor is a terminal.
func IsInteractive(f *os.File) bool {
	var termios syscall.Termios
	_, _, err := syscall.Syscall6(
		syscall.SYS_IOCTL,
		f.Fd(),
		syscall.TIOCGETA,
		uintptr(unsafe.Pointer(&termios)),
		0, 0, 0,
	)
	return err == 0
}
