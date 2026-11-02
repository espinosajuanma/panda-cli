package ui

import (
	"io"
	"os"

	"golang.org/x/sys/windows"
)

func readSelectorEscapeByte(in *os.File) (byte, bool, error) {
	handle := windows.Handle(in.Fd())
	fileType, err := windows.GetFileType(handle)
	if err != nil {
		return 0, false, err
	}
	if fileType == windows.FILE_TYPE_PIPE {
		var one [1]byte
		n, readErr := io.ReadFull(in, one[:])
		if readErr == io.EOF && n == 0 {
			return 0, false, nil
		}
		if readErr != nil {
			return 0, false, readErr
		}
		return one[0], true, nil
	}
	ready, err := windows.WaitForSingleObject(handle, 35)
	if err != nil {
		return 0, false, err
	}
	if ready == uint32(windows.WAIT_TIMEOUT) {
		return 0, false, nil
	}
	if ready != windows.WAIT_OBJECT_0 {
		return 0, false, io.ErrUnexpectedEOF
	}
	var one [1]byte
	if _, err := io.ReadFull(in, one[:]); err != nil {
		return 0, false, err
	}
	return one[0], true, nil
}
