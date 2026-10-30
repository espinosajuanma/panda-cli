package ui

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/term"
)

func Prompt(in io.Reader, out io.Writer, message string) (string, error) {
	if message != "" {
		fmt.Fprintf(out, "%s %s", ColorText(out, "1;32", "?"), message)
	}
	var line strings.Builder
	var one [1]byte
	for {
		n, err := in.Read(one[:])
		if n == 1 {
			if one[0] == '\n' {
				break
			}
			if one[0] != '\r' {
				line.WriteByte(one[0])
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", err
		}
	}
	return strings.TrimSpace(line.String()), nil
}

func OpenEditor(path string) error {
	commandLine := os.Getenv("EDITOR")
	if strings.TrimSpace(commandLine) == "" {
		commandLine = os.Getenv("VISUAL")
	}
	editor, err := SplitCommand(commandLine)
	if err != nil {
		return fmt.Errorf("parse editor command: %w", err)
	}
	if len(editor) == 0 {
		return errors.New("EDITOR and VISUAL are not set")
	}
	args := append(editor[1:], path)
	command := exec.Command(editor[0], args...)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	return command.Run()
}

func EditTemporaryNote(in io.Reader) (string, error) {
	dir, err := os.MkdirTemp("", "panda-note-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "note.md")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		return "", err
	}
	if file, ok := in.(*os.File); ok && !term.IsTerminal(int(file.Fd())) {
		return "", errors.New("add note text as an argument when running without an interactive terminal")
	}
	if err := OpenEditor(path); err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	return string(data), err
}
