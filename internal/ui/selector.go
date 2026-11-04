package ui

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/term"
)

type selectorOption struct {
	value string
	label string
}

var ErrSelectionCanceled = errors.New("selection canceled")
var ErrSelectionEscaped = errors.New("selection canceled with escape")

func descriptionEditorRequested(key rune) bool {
	return key == 'e' || key == 'E'
}

func isInteractiveTerminal(in io.Reader, out io.Writer) bool {
	inFile, inOK := in.(*os.File)
	outFile, outOK := out.(*os.File)
	return inOK && outOK && term.IsTerminal(int(inFile.Fd())) && term.IsTerminal(int(outFile.Fd()))
}

func selectSingleOption(in io.Reader, out io.Writer, prompt string, options []selectorOption, selected string) (string, error) {
	return selectSingleOptionWithEscapeCancel(in, out, prompt, options, selected, false)
}

func selectSingleOptionWithEscapeCancel(in io.Reader, out io.Writer, prompt string, options []selectorOption, selected string, escapeCancels bool) (string, error) {
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
	return runSelector(inFile, outFile, prompt, options, initial, nil, false, escapeCancels)
}

func selectMultipleOptions(in io.Reader, out io.Writer, prompt string, options []selectorOption, selected map[string]bool) (map[string]bool, error) {
	return selectMultipleOptionsWithEscapeCancel(in, out, prompt, options, selected, false)
}

func selectMultipleOptionsWithEscapeCancel(in io.Reader, out io.Writer, prompt string, options []selectorOption, selected map[string]bool, escapeCancels bool) (map[string]bool, error) {
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
	_, err := runSelector(inFile, outFile, prompt, choices, 0, selectedIndexes, true, escapeCancels)
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

func runSelector(in, out *os.File, prompt string, options []selectorOption, initial int, selected map[string]bool, multiple, escapeCancels bool) (string, error) {
	state, err := term.MakeRaw(int(in.Fd()))
	if err != nil {
		return "", err
	}
	defer term.Restore(int(in.Fd()), state)
	type selectorState struct {
		filter   string
		cursor   int
		selected map[string]bool
		frame    []string
		width    int
		height   int
		finished bool
	}
	current := &selectorState{cursor: initial, selected: selected}
	current.width, current.height = selectorDimensions(out)
	initialMatches := filterSelectorOptions(options, current.filter)
	if current.cursor >= len(initialMatches) {
		current.cursor = max(0, len(initialMatches)-1)
	}
	current.frame = renderSelectorAtSize(out, prompt, options, initialMatches, current.cursor, current.filter, current.selected, multiple, 0, current.width, current.height)

	var mutex sync.Mutex
	stopWatcher := make(chan struct{})
	watcherDone := make(chan struct{})
	ticker := time.NewTicker(150 * time.Millisecond)
	go func() {
		defer close(watcherDone)
		defer ticker.Stop()
		for {
			select {
			case <-stopWatcher:
				return
			case <-ticker.C:
				width, height := selectorDimensions(out)
				mutex.Lock()
				if current.finished {
					mutex.Unlock()
					return
				}
				if width != current.width || height != current.height {
					clearRows := selectorRowsForWidth(current.frame, width)
					current.width, current.height = width, height
					current.frame = renderSelectorAtSize(out, prompt, options, filterSelectorOptions(options, current.filter), current.cursor, current.filter, current.selected, multiple, clearRows, width, height)
				}
				mutex.Unlock()
			}
		}
	}()
	var stopOnce sync.Once
	stop := func() {
		mutex.Lock()
		current.finished = true
		mutex.Unlock()
		stopOnce.Do(func() { close(stopWatcher) })
		<-watcherDone
	}
	defer stop()

	for {
		key, err := readSelectorKey(in)
		if err != nil {
			stop()
			return "", err
		}
		mutex.Lock()
		width, height := selectorDimensions(out)
		clearRows := selectorRowsForWidth(current.frame, width)
		matches := filterSelectorOptions(options, current.filter)
		if current.cursor >= len(matches) {
			current.cursor = max(0, len(matches)-1)
		}
		var value string
		var selectionErr error
		finishedSelection := false
		switch key {
		case selectorUp:
			if len(matches) > 0 {
				current.cursor = (current.cursor - 1 + len(matches)) % len(matches)
			}
		case selectorDown:
			if len(matches) > 0 {
				current.cursor = (current.cursor + 1) % len(matches)
			}
		case selectorEnter:
			if multiple {
				value = selectedSummary(options, current.selected)
				finishedSelection = true
			}
			if !multiple && len(matches) > 0 {
				value = options[matches[current.cursor]].value
				finishedSelection = true
			}
		case selectorSpace:
			if multiple && len(matches) > 0 {
				choice := options[matches[current.cursor]]
				if choice.value == "" {
					for key := range current.selected {
						current.selected[key] = false
					}
				} else {
					current.selected[choice.value] = !current.selected[choice.value]
				}
			}
		case selectorBackspace:
			if len(current.filter) > 0 {
				runes := []rune(current.filter)
				current.filter = string(runes[:len(runes)-1])
				current.cursor = 0
			}
		case selectorEscape:
			if escapeCancels {
				value, selectionErr, finishedSelection = "Canceled", ErrSelectionEscaped, true
			} else {
				current.filter = ""
				current.cursor = 0
			}
		case selectorCancel:
			value, selectionErr, finishedSelection = "Canceled", ErrSelectionCanceled, true
		default:
			if unicode.IsPrint(key) && key != ' ' {
				current.filter += string(key)
				current.cursor = 0
			}
		}
		if finishedSelection {
			current.width, current.height = width, height
			finishSelector(out, clearRows, value)
			current.finished = true
			mutex.Unlock()
			stop()
			return value, selectionErr
		}
		current.width, current.height = width, height
		current.frame = renderSelectorAtSize(out, prompt, options, filterSelectorOptions(options, current.filter), current.cursor, current.filter, current.selected, multiple, clearRows, width, height)
		mutex.Unlock()
	}
}

func readTaskAction(in io.Reader, out io.Writer, prompt string, accepted ...rune) (rune, error) {
	inFile, _, ok := selectorFiles(in, out)
	if !ok {
		return 0, errors.New("interactive task prompt requires a terminal")
	}
	fmt.Fprintf(out, "%s %s ", ColorText(out, "1;32", "?"), ColorText(out, "1", prompt))
	state, err := term.MakeRaw(int(inFile.Fd()))
	if err != nil {
		fmt.Fprint(out, "\r\n")
		return 0, err
	}
	defer term.Restore(int(inFile.Fd()), state)
	for {
		key, readErr := readSelectorKey(inFile)
		if readErr != nil {
			fmt.Fprint(out, "\r\n")
			return 0, readErr
		}
		switch key {
		case rune(selectorEnter), rune(selectorCancel), 'd', 'D', 's', 'S', 'q', 'Q':
			fmt.Fprint(out, "\r\n")
			return key, nil
		}
		for _, acceptedKey := range accepted {
			if key == acceptedKey {
				fmt.Fprint(out, "\r\n")
				return key, nil
			}
		}
	}
}

func readEstimateAction(in io.Reader, out io.Writer) (rune, error) {
	return readTaskAction(in, out, "Enter choose complexity · d description · s skip · q quit")
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

const (
	SelectorUp        = selectorUp
	SelectorDown      = selectorDown
	SelectorEnter     = selectorEnter
	SelectorSpace     = selectorSpace
	SelectorBackspace = selectorBackspace
	SelectorEscape    = selectorEscape
	SelectorCancel    = selectorCancel
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
		second, available, err := readSelectorEscapeByte(in)
		if err != nil {
			return 0, err
		}
		if !available || second != '[' {
			return rune(selectorEscape), nil
		}
		third, available, err := readSelectorEscapeByte(in)
		if err != nil {
			return 0, err
		}
		if !available {
			return rune(selectorEscape), nil
		}
		switch third {
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
	width, height := selectorDimensions(out)
	frame := renderSelectorAtSize(out, prompt, options, matches, cursor, filter, selected, multiple, previousLines, width, height)
	return selectorRowsForWidth(frame, width)
}

func selectorDimensions(out *os.File) (int, int) {
	width, height, err := term.GetSize(int(out.Fd()))
	if err != nil || width < 1 || height < 1 {
		return 80, 24
	}
	return width, height
}

func renderSelectorAtSize(out *os.File, prompt string, options []selectorOption, matches []int, cursor int, filter string, selected map[string]bool, multiple bool, previousRows, width, height int) []string {
	if width < 1 {
		width = 80
	}
	if previousRows > 0 {
		fmt.Fprintf(out, "\x1b[%dA\r\x1b[J", previousRows)
	}
	frame := buildSelectorFrame(out, prompt, options, matches, cursor, filter, selected, multiple, width, height)
	for _, line := range frame {
		fmt.Fprintf(out, "%s\r\n", line)
	}
	return frame
}

func buildSelectorFrame(out *os.File, prompt string, options []selectorOption, matches []int, cursor int, filter string, selected map[string]bool, multiple bool, width, height int) []string {
	contentWidth := max(1, width-1)
	promptPrefix := ColorText(out, "1;32", "?") + " "
	promptWidth := selectorDisplayWidth(promptPrefix)
	lines := []string{promptPrefix + ColorText(out, "1", truncateSelectorText(prompt, max(0, contentWidth-promptWidth)))}
	if filter != "" {
		filterPrefix := "  Filter: "
		available := max(0, contentWidth-selectorDisplayWidth(filterPrefix))
		lines = append(lines, fitSelectorLine(filterPrefix+truncateSelectorTail(filter, available), contentWidth))
	}
	maxRows := 10
	if height > 6 {
		maxRows = min(maxRows, height-6)
	} else if height > 0 {
		maxRows = 1
	}
	start := max(0, cursor-maxRows/2)
	if start+maxRows > len(matches) {
		start = max(0, len(matches)-maxRows)
	}
	end := min(len(matches), start+maxRows)
	if start > 0 {
		lines = append(lines, truncateSelectorText("  …", contentWidth))
	}
	for visibleIndex := start; visibleIndex < end; visibleIndex++ {
		option := options[matches[visibleIndex]]
		cursorMarker := "  "
		if visibleIndex == cursor {
			cursorMarker = ColorText(out, "1;36", "›") + " "
		}
		prefix := cursorMarker
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
				mark = ColorText(out, "1;32", "✓")
			}
			prefix += "[" + mark + "] "
		}
		labelWidth := max(0, contentWidth-selectorDisplayWidth(prefix))
		lines = append(lines, fitSelectorLine(prefix+truncateSelectorText(option.label, labelWidth), contentWidth))
	}
	if end < len(matches) {
		lines = append(lines, truncateSelectorText("  …", contentWidth))
	}
	if len(matches) == 0 {
		lines = append(lines, truncateSelectorText("  No matches.", contentWidth))
	}
	help := "  ↑/↓ move  type to filter  enter select"
	if multiple {
		help = "  ↑/↓ move  type to filter  space toggle  enter confirm"
	}
	lines[0] = fitSelectorLine(lines[0], contentWidth)
	lines = append(lines, truncateSelectorText(help, contentWidth))
	return lines
}

func selectorDisplayWidth(value string) int {
	visible := ansiEscapeSequence.ReplaceAllString(value, "")
	return utf8.RuneCountInString(visible)
}

func selectorRowsForWidth(lines []string, width int) int {
	if width < 1 {
		width = 80
	}
	rows := 0
	for _, line := range lines {
		columns := selectorDisplayWidth(line)
		rows += max(1, (columns+width-1)/width)
	}
	return rows
}

func truncateSelectorText(value string, width int) string {
	if width <= 0 {
		return ""
	}
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}

func fitSelectorLine(value string, width int) string {
	if selectorDisplayWidth(value) <= width {
		return value
	}
	return truncateSelectorText(ansiEscapeSequence.ReplaceAllString(value, ""), width)
}

func truncateSelectorTail(value string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}
	return "…" + string(runes[len(runes)-(width-1):])
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
	fmt.Fprintf(out, "%s %s\r\n", ColorText(out, "1;32", "✓"), selection)
}

func readSingleKey(in io.Reader, out io.Writer) (rune, error) {
	inFile, _, ok := selectorFiles(in, out)
	if !ok {
		return 0, errors.New("single-key input requires a terminal")
	}
	fmt.Fprintf(out, "%s %s [(e) to launch editor, enter to skip] ", ColorText(out, "1;32", "?"), ColorText(out, "1", "Description"))
	state, err := term.MakeRaw(int(inFile.Fd()))
	if err != nil {
		fmt.Fprint(out, "\r\n")
		return 0, err
	}
	defer term.Restore(int(inFile.Fd()), state)
	for {
		key, readErr := readSelectorKey(inFile)
		if readErr != nil {
			fmt.Fprint(out, "\r\n")
			return 0, readErr
		}
		if key == 'e' || key == 'E' || key == rune(selectorEnter) {
			fmt.Fprint(out, "\r\n")
			return key, nil
		}
		if key == rune(selectorCancel) {
			fmt.Fprint(out, "\r\n")
			return 0, ErrSelectionCanceled
		}
	}
}

type Choice struct{ Value, Label string }

func DescriptionEditorRequested(key rune) bool               { return descriptionEditorRequested(key) }
func IsInteractiveTerminal(in io.Reader, out io.Writer) bool { return isInteractiveTerminal(in, out) }
func SelectSingleOption(in io.Reader, out io.Writer, prompt string, choices []Choice, selected string) (string, error) {
	return selectSingleOption(in, out, prompt, internalChoices(choices), selected)
}
func SelectSingleOptionWithEscapeCancel(in io.Reader, out io.Writer, prompt string, choices []Choice, selected string, escapeCancels bool) (string, error) {
	return selectSingleOptionWithEscapeCancel(in, out, prompt, internalChoices(choices), selected, escapeCancels)
}
func SelectMultipleOptions(in io.Reader, out io.Writer, prompt string, choices []Choice, selected map[string]bool) (map[string]bool, error) {
	return selectMultipleOptions(in, out, prompt, internalChoices(choices), selected)
}

func SelectMultipleOptionsWithEscapeCancel(in io.Reader, out io.Writer, prompt string, choices []Choice, selected map[string]bool) (map[string]bool, error) {
	return selectMultipleOptionsWithEscapeCancel(in, out, prompt, internalChoices(choices), selected, true)
}
func ReadEstimateAction(in io.Reader, out io.Writer) (rune, error) {
	return readEstimateAction(in, out)
}

func ReadTaskAction(in io.Reader, out io.Writer, prompt string, accepted ...rune) (rune, error) {
	return readTaskAction(in, out, prompt, accepted...)
}
func ReadSingleKey(in io.Reader, out io.Writer) (rune, error) { return readSingleKey(in, out) }
func ReadSelectorKey(in *os.File) (rune, error)               { return readSelectorKey(in) }
func FilterSelectorOptions(choices []Choice, filter string) []int {
	return filterSelectorOptions(internalChoices(choices), filter)
}
func RenderSelector(out *os.File, prompt string, choices []Choice, matches []int, cursor int, filter string, selected map[string]bool, multiple bool, previousLines int) int {
	return renderSelector(out, prompt, internalChoices(choices), matches, cursor, filter, selected, multiple, previousLines)
}

func internalChoices(choices []Choice) []selectorOption {
	options := make([]selectorOption, len(choices))
	for index, choice := range choices {
		options[index] = selectorOption{value: choice.Value, label: choice.Label}
	}
	return options
}
