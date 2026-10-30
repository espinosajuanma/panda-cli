package app

import (
	"io"
	"os"

	"github.com/espinosajuanma/panda-cli/internal/ui"
)

type selectorOption struct {
	value string
	label string
}

var errSelectionCanceled = ui.ErrSelectionCanceled
var errSelectionEscaped = ui.ErrSelectionEscaped

const (
	selectorUp        = ui.SelectorUp
	selectorDown      = ui.SelectorDown
	selectorEnter     = ui.SelectorEnter
	selectorSpace     = ui.SelectorSpace
	selectorBackspace = ui.SelectorBackspace
	selectorEscape    = ui.SelectorEscape
	selectorCancel    = ui.SelectorCancel
)

func descriptionEditorRequested(key rune) bool { return ui.DescriptionEditorRequested(key) }
func isInteractiveTerminal(in io.Reader, out io.Writer) bool {
	return ui.IsInteractiveTerminal(in, out)
}
func readSingleKey(in io.Reader, out io.Writer) (rune, error) { return ui.ReadSingleKey(in, out) }
func readEstimateAction(in io.Reader, out io.Writer) (rune, error) {
	return ui.ReadEstimateAction(in, out)
}
func readSelectorKey(in *os.File) (rune, error) { return ui.ReadSelectorKey(in) }

func selectSingleOption(in io.Reader, out io.Writer, prompt string, options []selectorOption, selected string) (string, error) {
	return ui.SelectSingleOption(in, out, prompt, toUIChoices(options), selected)
}

func selectSingleOptionWithEscapeCancel(in io.Reader, out io.Writer, prompt string, options []selectorOption, selected string, escapeCancels bool) (string, error) {
	return ui.SelectSingleOptionWithEscapeCancel(in, out, prompt, toUIChoices(options), selected, escapeCancels)
}

func selectMultipleOptions(in io.Reader, out io.Writer, prompt string, options []selectorOption, selected map[string]bool) (map[string]bool, error) {
	return ui.SelectMultipleOptions(in, out, prompt, toUIChoices(options), selected)
}

func filterSelectorOptions(options []selectorOption, filter string) []int {
	return ui.FilterSelectorOptions(toUIChoices(options), filter)
}

func renderSelector(out *os.File, prompt string, options []selectorOption, matches []int, cursor int, filter string, selected map[string]bool, multiple bool, previousLines int) int {
	return ui.RenderSelector(out, prompt, toUIChoices(options), matches, cursor, filter, selected, multiple, previousLines)
}

func toUIChoices(options []selectorOption) []ui.Choice {
	choices := make([]ui.Choice, len(options))
	for index, option := range options {
		choices[index] = ui.Choice{Value: option.value, Label: option.label}
	}
	return choices
}
