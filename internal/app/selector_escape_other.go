//go:build !aix && !android && !darwin && !dragonfly && !freebsd && !illumos && !linux && !netbsd && !openbsd && !solaris

package app

import (
	"io"
	"os"
)

func readSelectorEscapeByte(in *os.File) (byte, bool, error) {
	var one [1]byte
	if _, err := io.ReadFull(in, one[:]); err != nil {
		return 0, false, err
	}
	return one[0], true, nil
}
