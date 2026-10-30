package app

import (
	"fmt"
	"io"
	"strings"
)

var mainCommandDetails = map[string][]string{
	"list": {
		"Status filters: --current, --staging, --released, --discarded, --all, --status <status,...>.",
		"Other filters: --no-complexity, --no-description, --no-package, --no-labels.",
	},
}

func printHelp(out io.Writer) error {
	return writeHelpOutput(out, renderMainHelp)
}

func renderMainHelp(out, colorTarget io.Writer) {
	printManHeader(out, colorTarget, "PANDA")
	writeManSection(out, colorTarget, "NAME", []string{"panda - synchronize Solutions tasks with a local task workspace"})
	writeManSection(out, colorTarget, "SYNOPSIS", []string{"panda [global options] <command> [arguments]"})
	writeManSection(out, colorTarget, "DESCRIPTION", []string{
		"Panda pulls Solutions tasks into Markdown and JSON files, lets you edit them locally, and publishes selected changes with push.",
		"Use command-specific help for detailed syntax and options: panda <command> --help or panda help <command>.",
	})
	writeSectionTitle(out, colorTarget, "COMMANDS")
	for _, command := range mainCommands {
		synopsis := "panda " + command.command
		if command.synopsis != "" {
			synopsis += " " + command.synopsis
		}
		writeWrapped(out, "    ", synopsis, 84)
		writeWrapped(out, "        ", command.description, 84)
		for _, detail := range mainCommandDetails[command.command] {
			writeWrapped(out, "        ", detail, 84)
		}
		fmt.Fprintln(out)
	}
	writeSectionTitle(out, colorTarget, "GLOBAL OPTIONS")
	writeManualOption(out, manualOption{"-h, --help", "Display the main manual. Append --help to a command for its manual page."})
	fmt.Fprintln(out)
	writeSectionTitle(out, colorTarget, "ENVIRONMENT")
	writeManualOption(out, manualOption{"EDITOR", "Select the editor used by edit, edit-note, and add-note."})
	writeManualOption(out, manualOption{"PAGER", "Select the pager used for long output; less is configured to leave its output visible after exit."})
	writeManualOption(out, manualOption{"PANDA_API_URL", "Override the configured Solutions API base URL."})
	fmt.Fprintln(out)
	writeManSection(out, colorTarget, "FILES", []string{"Configuration and task files are stored under the operating system's user config directory in panda-dashboard/. Run panda path to print the active workspace path."})
}

func printCommandHelp(command string, out io.Writer) error {
	command = strings.Join(strings.Fields(command), " ")
	page, ok := manualPages[command]
	if !ok {
		return fmt.Errorf("no help available for %q", command)
	}
	return writeHelpOutput(out, func(out, colorTarget io.Writer) {
		pageTitle := "PANDA-" + strings.ToUpper(strings.ReplaceAll(command, " ", "-"))
		printManHeader(out, colorTarget, pageTitle)
		writeManSection(out, colorTarget, "NAME", []string{page.name})
		writeManSection(out, colorTarget, "SYNOPSIS", page.synopsis)
		writeManSection(out, colorTarget, "DESCRIPTION", page.description)
		if len(page.arguments) > 0 {
			writeSectionTitle(out, colorTarget, "ARGUMENTS")
			for _, argument := range page.arguments {
				writeManualOption(out, argument)
			}
			fmt.Fprintln(out)
		}
		if len(page.options) > 0 {
			writeSectionTitle(out, colorTarget, "OPTIONS")
			for _, option := range page.options {
				writeManualOption(out, option)
			}
			fmt.Fprintln(out)
		}
		if len(page.examples) > 0 {
			writeManSection(out, colorTarget, "EXAMPLES", page.examples)
		}
		if len(page.seeAlso) > 0 {
			writeManSection(out, colorTarget, "SEE ALSO", []string{strings.Join(page.seeAlso, ", ")})
		}
	})
}

func commandHelpKey(args []string) string {
	if len(args) == 0 {
		return ""
	}
	switch args[0] {
	case "config":
		if len(args) > 1 {
			switch args[1] {
			case "get", "set":
				if len(args) > 2 && (args[2] == "project" || args[2] == "api-url") {
					return "config " + args[1] + " " + args[2]
				}
				return "config " + args[1]
			}
		}
	case "completion":
		if len(args) > 1 {
			switch args[1] {
			case "bash", "zsh", "fish":
				return "completion " + args[1]
			}
		}
	case "files":
		if len(args) > 1 && (args[1] == "upload" || args[1] == "download") {
			return "files " + args[1]
		}
	case "labels", "packages":
		if len(args) > 1 {
			switch args[1] {
			case "create", "edit":
				return args[0] + " " + args[1]
			case "delete", "remove":
				return args[0] + " delete"
			}
		}
	}
	return args[0]
}

func printManHeader(out, colorTarget io.Writer, title string) {
	title += "(1)"
	const width = 80
	middle := "Panda CLI Manual"
	gap := width - len(title)*2 - len(middle)
	if gap < 2 {
		gap = 2
	}
	leftGap, rightGap := gap/2, gap-gap/2
	fmt.Fprintf(out, "%s%s%s%s%s\n\n", colorText(colorTarget, "1;32", title), strings.Repeat(" ", leftGap), middle, strings.Repeat(" ", rightGap), colorText(colorTarget, "1;32", title))
}

func writeManSection(out, colorTarget io.Writer, section string, paragraphs []string) {
	if len(paragraphs) == 0 {
		return
	}
	writeSectionTitle(out, colorTarget, section)
	for _, paragraph := range paragraphs {
		writeWrapped(out, "    ", paragraph, 84)
	}
	fmt.Fprintln(out)
}

func writeSectionTitle(out, colorTarget io.Writer, title string) {
	fmt.Fprintln(out, colorText(colorTarget, "1;32", title))
}

func writeHelpOutput(out io.Writer, render func(io.Writer, io.Writer)) error {
	return writePagedOutput(out, func(rendered io.Writer) error {
		render(rendered, out)
		return nil
	})
}

func shouldPageHelp(terminal bool, output []byte, width, height int) bool {
	return shouldPageOutput(terminal, output, width, height)
}

func writeManualOption(out io.Writer, option manualOption) {
	const optionColumn, lineWidth = 32, 84
	lines := wrapWords(option.description, lineWidth-optionColumn)
	if len(option.name) < optionColumn-4 {
		fmt.Fprintf(out, "    %-*s%s\n", optionColumn-4, option.name, lines[0])
		for _, line := range lines[1:] {
			fmt.Fprintf(out, "    %-*s%s\n", optionColumn-4, "", line)
		}
		return
	}
	writeWrapped(out, "    ", option.name, lineWidth)
	writeWrapped(out, "        ", option.description, lineWidth)
}

func writeWrapped(out io.Writer, indent, text string, width int) {
	for _, line := range wrapWords(text, width-len(indent)) {
		fmt.Fprintln(out, indent+line)
	}
}

func wrapWords(text string, width int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}
	if width < 1 {
		width = 1
	}
	lines := []string{}
	line := words[0]
	for _, word := range words[1:] {
		if len(line)+1+len(word) > width {
			lines = append(lines, line)
			line = word
			continue
		}
		line += " " + word
	}
	return append(lines, line)
}
