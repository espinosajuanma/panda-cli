package app

import (
	"io"

	"github.com/espinosajuanma/panda-cli/internal/report"
)

func writePagedOutput(out io.Writer, render func(io.Writer) error) error {
	return report.Paged(out, render)
}

func shouldPageOutput(terminal bool, output []byte, width, height int) bool {
	return report.ShouldPage(terminal, output, width, height)
}
