package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"

	"golang.org/x/term"
)

type selectorOption struct {
	value string
	label string
}

func isInteractiveTerminal(in io.Reader, out io.Writer) bool {
	inFile, inOK := in.(*os.File)
	outFile, outOK := out.(*os.File)
	return inOK && outOK && term.IsTerminal(int(inFile.Fd())) && term.IsTerminal(int(outFile.Fd()))
}

func selectSingleOption(in io.Reader, out io.Writer, prompt string, options []selectorOption, selected string) (string, error) {
	inFile, outFile, ok := selectorFiles(in, out)
	if !ok {
		return "", errors.New("interactive selector requires a terminal")
	}
	initial := 0
	for index, option := range options {
		if option.value == selected {
			initial = index
			break
		}
	}
	return runSelector(inFile, outFile, prompt, options, initial, nil, false)
}

func selectMultipleOptions(in io.Reader, out io.Writer, prompt string, options []selectorOption, selected map[string]bool) (map[string]bool, error) {
	inFile, outFile, ok := selectorFiles(in, out)
	if !ok {
		return nil, errors.New("interactive selector requires a terminal")
	}
	choices := make([]selectorOption, 0, len(options)+1)
	choices = append(choices, selectorOption{value: "", label: "None (clear all)"})
	choices = append(choices, options...)
	selectedIndexes := make(map[string]bool, len(selected))
	for key, value := range selected {
		selectedIndexes[key] = value
	}
	_, err := runSelector(inFile, outFile, prompt, choices, 0, selectedIndexes, true)
	return selectedIndexes, err
}

func selectorFiles(in io.Reader, out io.Writer) (*os.File, *os.File, bool) {
	inFile, inOK := in.(*os.File)
	outFile, outOK := out.(*os.File)
	if !inOK || !outOK || !term.IsTerminal(int(inFile.Fd())) || !term.IsTerminal(int(outFile.Fd())) {
		return nil, nil, false
	}
	return inFile, outFile, true
}

func runSelector(in, out *os.File, prompt string, options []selectorOption, initial int, selected map[string]bool, multiple bool) (string, error) {
	state, err := term.MakeRaw(int(in.Fd()))
	if err != nil {
		return "", err
	}
	defer term.Restore(int(in.Fd()), state)
	filter := ""
	cursor := initial
	printedLines := 0
	for {
		matches := filterSelectorOptions(options, filter)
		if cursor >= len(matches) {
			cursor = max(0, len(matches)-1)
		}
		printedLines = renderSelector(out, prompt, options, matches, cursor, filter, selected, multiple, printedLines)
		key, err := readSelectorKey(in)
		if err != nil {
			return "", err
		}
		switch key {
		case selectorUp:
			if len(matches) > 0 {
				cursor = (cursor - 1 + len(matches)) % len(matches)
			}
		case selectorDown:
			if len(matches) > 0 {
				cursor = (cursor + 1) % len(matches)
			}
		case selectorEnter:
			if multiple {
				finishSelector(out, printedLines, selectedSummary(options, selected))
				return "", nil
			}
			if len(matches) > 0 {
				choice := options[matches[cursor]]
				finishSelector(out, printedLines, choice.label)
				return choice.value, nil
			}
		case selectorSpace:
			if multiple && len(matches) > 0 {
				choice := options[matches[cursor]]
				if choice.value == "" {
					for key := range selected {
						selected[key] = false
					}
				} else {
					selected[choice.value] = !selected[choice.value]
				}
			}
		case selectorBackspace:
			if len(filter) > 0 {
				filter = filter[:len(filter)-1]
				cursor = 0
			}
		case selectorEscape:
			filter = ""
			cursor = 0
		case selectorCancel:
			finishSelector(out, printedLines, "Canceled")
			return "", errors.New("selection canceled")
		default:
			if unicode.IsPrint(key) && key != ' ' {
				filter += string(key)
				cursor = 0
			}
		}
	}
}

const (
	selectorUp        rune = 0x100
	selectorDown      rune = 0x101
	selectorEnter     rune = 0x102
	selectorSpace     rune = 0x103
	selectorBackspace rune = 0x104
	selectorEscape    rune = 0x105
	selectorCancel    rune = 0x106
)

func readSelectorKey(in *os.File) (rune, error) {
	var one [1]byte
	if _, err := io.ReadFull(in, one[:]); err != nil {
		return 0, err
	}
	switch one[0] {
	case '\r', '\n':
		return rune(selectorEnter), nil
	case ' ':
		return rune(selectorSpace), nil
	case 0x7f, 0x08:
		return rune(selectorBackspace), nil
	case 0x03:
		return rune(selectorCancel), nil
	case 0x1b:
		var sequence [2]byte
		if _, err := io.ReadFull(in, sequence[:1]); err != nil {
			return rune(selectorEscape), nil
		}
		if sequence[0] != '[' {
			return rune(selectorEscape), nil
		}
		if _, err := io.ReadFull(in, sequence[1:]); err != nil {
			return rune(selectorEscape), nil
		}
		switch sequence[1] {
		case 'A':
			return rune(selectorUp), nil
		case 'B':
			return rune(selectorDown), nil
		default:
			return rune(selectorEscape), nil
		}
	default:
		return rune(one[0]), nil
	}
}

func filterSelectorOptions(options []selectorOption, filter string) []int {
	indices := make([]int, 0, len(options))
	filter = strings.ToLower(filter)
	for index, option := range options {
		if filter == "" || strings.Contains(strings.ToLower(option.label), filter) {
			indices = append(indices, index)
		}
	}
	return indices
}

func renderSelector(out *os.File, prompt string, options []selectorOption, matches []int, cursor int, filter string, selected map[string]bool, multiple bool, previousLines int) int {
	if previousLines > 0 {
		fmt.Fprintf(out, "\x1b[%dA\r\x1b[J", previousLines)
	}
	fmt.Fprintf(out, "%s %s\r\n", colorText(out, "1;32", "?"), prompt)
	lineCount := 1
	if filter != "" {
		fmt.Fprintf(out, "  Filter: %s\r\n", filter)
		lineCount++
	}
	maxRows := 10
	if _, height, err := term.GetSize(int(out.Fd())); err == nil && height > 6 {
		maxRows = min(maxRows, height-6)
	}
	start := max(0, cursor-maxRows/2)
	if start+maxRows > len(matches) {
		start = max(0, len(matches)-maxRows)
	}
	end := min(len(matches), start+maxRows)
	if start > 0 {
		fmt.Fprint(out, "  …\r\n")
		lineCount++
	}
	for visibleIndex := start; visibleIndex < end; visibleIndex++ {
		optionIndex := matches[visibleIndex]
		option := options[optionIndex]
		cursorMarker := "  "
		if visibleIndex == cursor {
			cursorMarker = colorText(out, "1;36", "›") + " "
		}
		if multiple {
			mark := " "
			if option.value == "" {
				if len(selected) == 0 || noSelectedOptions(selected) {
					mark = "✓"
				}
			} else if selected[option.value] {
				mark = "✓"
			}
			if mark == "✓" {
				mark = colorText(out, "1;32", "✓")
			}
			fmt.Fprintf(out, "%s[%s] %s\r\n", cursorMarker, mark, option.label)
		} else {
			fmt.Fprintf(out, "%s%s\r\n", cursorMarker, option.label)
		}
		lineCount++
	}
	if end < len(matches) {
		fmt.Fprint(out, "  …\r\n")
		lineCount++
	}
	if len(matches) == 0 {
		fmt.Fprint(out, "  No matches.\r\n")
		lineCount++
	}
	if multiple {
		fmt.Fprint(out, "  ↑/↓ move  type to filter  space toggle  enter confirm\r\n")
	} else {
		fmt.Fprint(out, "  ↑/↓ move  type to filter  enter select\r\n")
	}
	lineCount++
	return lineCount
}

func noSelectedOptions(selected map[string]bool) bool {
	for _, isSelected := range selected {
		if isSelected {
			return false
		}
	}
	return true
}

func selectedSummary(options []selectorOption, selected map[string]bool) string {
	var labels []string
	for _, option := range options {
		if option.value != "" && selected[option.value] {
			labels = append(labels, option.label)
		}
	}
	if len(labels) == 0 {
		return "No labels"
	}
	return strings.Join(labels, ", ")
}

func finishSelector(out *os.File, lines int, selection string) {
	if lines > 0 {
		fmt.Fprintf(out, "\x1b[%dA\r\x1b[J", lines)
	}
	fmt.Fprintf(out, "%s %s\r\n", colorText(out, "1;32", "✓"), selection)
}

func readSingleKey(in io.Reader, out io.Writer) (rune, error) {
	inFile, outFile, ok := selectorFiles(in, out)
	if !ok {
		return 0, errors.New("single-key input requires a terminal")
	}
	fmt.Fprintf(out, "%s Edit description? (press e to open, any other key to skip) ", colorText(out, "1;32", "?"))
	state, err := term.MakeRaw(int(inFile.Fd()))
	if err != nil {
		fmt.Fprintln(out)
		return 0, err
	}
	key, readErr := readSelectorKey(inFile)
	restoreErr := term.Restore(int(inFile.Fd()), state)
	fmt.Fprintln(out)
	if readErr != nil {
		return 0, readErr
	}
	if restoreErr != nil {
		return 0, restoreErr
	}
	_ = outFile
	return key, nil
}
