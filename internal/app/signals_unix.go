//go:build unix

package app

import (
	"os"
	"syscall"
)

func cancellationSignals() []os.Signal { return []os.Signal{os.Interrupt, syscall.SIGTERM} }
