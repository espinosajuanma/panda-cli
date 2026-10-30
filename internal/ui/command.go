package ui

import (
	"errors"
	"strings"
	"unicode"
)

// SplitCommand parses quoted command words without invoking a shell.
func SplitCommand(value string) ([]string, error) {
	var words []string
	var word strings.Builder
	var quote rune
	escaped, started := false, false
	characters := []rune(value)
	for index, char := range characters {
		if escaped {
			word.WriteRune(char)
			escaped, started = false, true
			continue
		}
		if char == '\\' && quote != '\'' {
			if index == len(characters)-1 {
				escaped, started = true, true
				continue
			}
			next := characters[index+1]
			if unicode.IsSpace(next) || next == '\\' || next == quote || quote == 0 && (next == '\'' || next == '"') {
				escaped, started = true, true
				continue
			}
			word.WriteRune(char)
			started = true
			continue
		}
		if quote != 0 {
			if char == quote {
				quote = 0
			} else {
				word.WriteRune(char)
			}
			continue
		}
		if char == '\'' || char == '"' {
			quote, started = char, true
			continue
		}
		if char == ' ' || char == '\t' || char == '\n' || char == '\r' {
			if started {
				words = append(words, word.String())
				word.Reset()
				started = false
			}
			continue
		}
		word.WriteRune(char)
		started = true
	}
	if escaped {
		return nil, errors.New("command ends with an incomplete escape")
	}
	if quote != 0 {
		return nil, errors.New("command contains an unterminated quote")
	}
	if started {
		words = append(words, word.String())
	}
	return words, nil
}
