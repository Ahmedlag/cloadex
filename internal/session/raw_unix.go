//go:build darwin || linux

package session

import (
	"os"
	"syscall"
	"unsafe"
)

func enableRawInput(f *os.File) (func(), error) {
	fd := int(f.Fd())
	state, err := getTermios(fd)
	if err != nil {
		return nil, err
	}

	raw := *state
	raw.Lflag &^= syscall.ECHO | syscall.ICANON | syscall.IEXTEN
	raw.Iflag &^= syscall.ICRNL | syscall.IXON
	raw.Cc[syscall.VMIN] = 1
	raw.Cc[syscall.VTIME] = 0

	if err := setTermios(fd, &raw); err != nil {
		return nil, err
	}

	return func() {
		_ = setTermios(fd, state)
	}, nil
}

func getTermios(fd int) (*syscall.Termios, error) {
	var state syscall.Termios
	_, _, errno := syscall.Syscall6(
		syscall.SYS_IOCTL,
		uintptr(fd),
		ioctlReadTermios,
		uintptr(unsafe.Pointer(&state)),
		0, 0, 0,
	)
	if errno != 0 {
		return nil, errno
	}
	return &state, nil
}

func setTermios(fd int, state *syscall.Termios) error {
	_, _, errno := syscall.Syscall6(
		syscall.SYS_IOCTL,
		uintptr(fd),
		ioctlWriteTermios,
		uintptr(unsafe.Pointer(state)),
		0, 0, 0,
	)
	if errno != 0 {
		return errno
	}
	return nil
}
