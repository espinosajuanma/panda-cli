package app

import (
	"io"

	doctordomain "github.com/espinosajuanma/panda-cli/internal/doctor"
)

type doctorReport = doctordomain.Report

func doctorCommand(args []string, out io.Writer) error {
	return doctordomain.Run(args, out)
}
