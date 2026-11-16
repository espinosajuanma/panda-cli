package report

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/espinosajuanma/panda-cli/internal/jsonutil"
)

func RelationshipDisplayName(value any) string {
	relation, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	for _, field := range []string{"label", "name", "title", "id"} {
		if name := strings.TrimSpace(jsonutil.StringField(relation, field)); name != "" {
			return name
		}
	}
	return ""
}

func RelationshipDisplayNameWithCatalog(value any, catalogNames map[string]string) string {
	if id := jsonutil.RelationshipID(value); id != "" {
		if name := catalogNames[id]; name != "" {
			return name
		}
	}
	return RelationshipDisplayName(value)
}

func LabelsDisplayWithCatalog(value any, catalogNames map[string]string) string {
	items, _ := value.([]any)
	names := make([]string, 0, len(items))
	for _, item := range items {
		if name := RelationshipDisplayNameWithCatalog(item, catalogNames); name != "" {
			names = append(names, name)
		}
	}
	sort.Slice(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })
	return strings.Join(names, ", ")
}

func AssigneesDisplay(value any) string {
	items, _ := value.([]any)
	var names []string
	for _, item := range items {
		if name := RelationshipDisplayName(item); name != "" {
			names = append(names, name)
		}
	}
	sort.Slice(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })
	return strings.Join(names, ", ")
}

func DisplayStatus(status string) string {
	var spaced strings.Builder
	for i, char := range status {
		if i > 0 && char >= 'A' && char <= 'Z' {
			spaced.WriteByte(' ')
		}
		spaced.WriteRune(char)
	}
	return strings.ToUpper(spaced.String())
}

func TableCell(value any) string {
	if value == nil {
		return ""
	}
	return strings.Join(strings.Fields(fmt.Sprint(value)), " ")
}

func IsEmptyField(value any) bool {
	return value == nil || strings.TrimSpace(fmt.Sprint(value)) == ""
}

func TruncateCell(value string, width int) string {
	if width < 1 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}

func WrapValue(value string, width int) []string {
	if width < 1 {
		width = 1
	}
	words := strings.Fields(value)
	if len(words) == 0 {
		return nil
	}
	var lines []string
	line := ""
	for _, word := range words {
		for utf8.RuneCountInString(word) > width {
			if line != "" {
				lines = append(lines, line)
				line = ""
			}
			runes := []rune(word)
			lines = append(lines, string(runes[:width]))
			word = string(runes[width:])
		}
		if line == "" {
			line = word
		} else if utf8.RuneCountInString(line)+1+utf8.RuneCountInString(word) <= width {
			line += " " + word
		} else {
			lines = append(lines, line)
			line = word
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}
