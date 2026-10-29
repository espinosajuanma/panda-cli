package ui

import (
	"io"
	"os"
	"regexp"
	"strings"

	"golang.org/x/term"
)

var (
	markdownBold = regexp.MustCompile(`\*\*(.+?)\*\*`)
	markdownCode = regexp.MustCompile("`([^`]+)`")
)

func IsTerminal(out io.Writer) bool {
	file, ok := out.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

func ColorText(out io.Writer, style, value string) string {
	file, ok := out.(*os.File)
	if !ok || os.Getenv("NO_COLOR") != "" || !term.IsTerminal(int(file.Fd())) {
		return value
	}
	return "\x1b[" + style + "m" + value + "\x1b[0m"
}

func HighlightMarkdown(out io.Writer, markdown string) string {
	if !IsTerminal(out) || os.Getenv("NO_COLOR") != "" {
		return markdown
	}
	lines := strings.Split(markdown, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			lines[i] = ColorText(out, "1;36", line)
			continue
		}
		line = markdownBold.ReplaceAllStringFunc(line, func(match string) string {
			return ColorText(out, "1", strings.TrimSuffix(strings.TrimPrefix(match, "**"), "**"))
		})
		line = markdownCode.ReplaceAllStringFunc(line, func(match string) string {
			return ColorText(out, "36", strings.TrimSuffix(strings.TrimPrefix(match, "`"), "`"))
		})
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}
