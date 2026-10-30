//go:build aix || android || darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd || solaris

package app

import (
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

func readSelectorEscapeByte(in *os.File) (byte, bool, error) {
	for {
		ready, err := unix.Poll([]unix.PollFd{{Fd: int32(in.Fd()), Events: unix.POLLIN}}, 35)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return 0, false, err
		}
		if ready == 0 {
			return 0, false, nil
		}
		var one [1]byte
		if _, err := io.ReadFull(in, one[:]); err != nil {
			return 0, false, err
		}
		return one[0], true, nil
	}
}
