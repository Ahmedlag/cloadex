//go:build windows

package session

import (
	"errors"
	"os"
)

func enableRawInput(f *os.File) (func(), error) {
	return nil, errors.New("raw input unsupported")
}
