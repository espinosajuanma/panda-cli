package report

import (
	"bytes"
	"io"

	"github.com/espinosajuanma/panda-cli/internal/ui"
)

func Paged(out io.Writer, render func(io.Writer) error) error {
	if !ui.IsTerminal(out) {
		return render(out)
	}
	var output bytes.Buffer
	if err := render(&output); err != nil {
		return err
	}
	width, height, sizeErr := ui.TerminalSize(out)
	if sizeErr == nil && !ShouldPage(true, output.Bytes(), width, height) {
		_, err := io.Copy(out, &output)
		return err
	}
	return ui.PageOutput(out, output.Bytes())
}

func ShouldPage(terminal bool, output []byte, width, height int) bool {
	return terminal && !ui.OutputFitsScreen(output, width, height)
}
