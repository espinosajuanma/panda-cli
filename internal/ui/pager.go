package ui

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/term"
)

var ansiEscapeSequence = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)

func OutputFitsScreen(output []byte, width, height int) bool {
	if width <= 0 || height <= 0 {
		return false
	}
	text := strings.ReplaceAll(string(output), "\r", "")
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	rows := 0
	for _, line := range lines {
		visible := ansiEscapeSequence.ReplaceAllString(line, "")
		columns := utf8.RuneCountInString(visible)
		if columns == 0 {
			rows++
		} else {
			rows += (columns + width - 1) / width
		}
		if rows > height {
			return false
		}
	}
	return true
}

func PageOutput(out io.Writer, content []byte) error {
	pager, parseErr := SplitCommand(os.Getenv("PAGER"))
	if parseErr != nil {
		_, writeErr := io.Copy(out, bytes.NewReader(content))
		return writeErr
	}
	if len(pager) == 0 {
		pager = []string{"less", "-FRX"}
	}
	pager = ConfigureLessPager(pager)
	command := exec.Command(pager[0], pager[1:]...)
	stdin, err := command.StdinPipe()
	if err != nil {
		_, writeErr := io.Copy(out, bytes.NewReader(content))
		return writeErr
	}
	command.Stdout = out
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		_ = stdin.Close()
		_, writeErr := io.Copy(out, bytes.NewReader(content))
		return writeErr
	}
	inputDone := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(stdin, bytes.NewReader(content))
		closeErr := stdin.Close()
		if copyErr == nil {
			copyErr = closeErr
		}
		inputDone <- copyErr
	}()
	waitErr := command.Wait()
	inputErr := <-inputDone
	if waitErr != nil {
		return fmt.Errorf("run pager: %w", waitErr)
	}
	// A pager normally closes stdin early when the user quits before the end.
	_ = inputErr
	return nil
}

func ConfigureLessPager(pager []string) []string {
	if len(pager) == 0 || !strings.EqualFold(filepath.Base(pager[0]), "less") {
		return pager
	}
	if !HasLessOption(pager[1:], "Rr", "--RAW-CONTROL-CHARS") {
		pager = append(pager, "-R")
	}
	if !HasLessOption(pager[1:], "X", "--NO-INIT") {
		pager = append(pager, "-X")
	}
	return pager
}

func HasLessOption(arguments []string, shortOptions, longOption string) bool {
	for _, argument := range arguments {
		if strings.EqualFold(argument, longOption) {
			return true
		}
		if strings.HasPrefix(argument, "-") && !strings.HasPrefix(argument, "--") && strings.ContainsAny(argument[1:], shortOptions) {
			return true
		}
	}
	return false
}

func TerminalSize(out io.Writer) (int, int, error) {
	file, ok := out.(*os.File)
	if !ok {
		return 0, 0, fmt.Errorf("output is not a terminal file")
	}
	return term.GetSize(int(file.Fd()))
}
