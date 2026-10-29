package app

import (
	"bytes"
	"io"

	"github.com/espinosajuanma/panda-cli/internal/ui"
)

func writePagedOutput(out io.Writer, render func(io.Writer) error) error {
	if !isTerminal(out) {
		return render(out)
	}
	var output bytes.Buffer
	if err := render(&output); err != nil {
		return err
	}
	width, height, sizeErr := ui.TerminalSize(out)
	if sizeErr == nil && !shouldPageOutput(true, output.Bytes(), width, height) {
		_, err := io.Copy(out, &output)
		return err
	}
	return ui.PageOutput(out, output.Bytes())
}

func shouldPageOutput(terminal bool, output []byte, width, height int) bool {
	return terminal && !ui.OutputFitsScreen(output, width, height)
}
