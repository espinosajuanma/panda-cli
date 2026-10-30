//go:build !unix

package app

import "os"

func cancellationSignals() []os.Signal { return []os.Signal{os.Interrupt} }
