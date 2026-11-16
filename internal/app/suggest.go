package app

import (
	"fmt"
	"sort"
	"strings"
)

func unknownCommandError(command string) error {
	candidates := append(rootCommandNames(), "help")
	if match := closestMatch(command, candidates); match != "" {
		return fmt.Errorf("unknown command %q; did you mean %q?", command, match)
	}
	return fmt.Errorf("unknown command %q; run `panda --help` for available commands", command)
}

func suggestionError(kind, input string, candidates []string) error {
	match := closestMatch(input, candidates)
	if match == "" {
		return nil
	}
	return fmt.Errorf("unknown %s %q; did you mean %q?", kind, input, match)
}

func closestMatch(input string, candidates []string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	if input == "" {
		return ""
	}
	sorted := append([]string(nil), candidates...)
	sort.Slice(sorted, func(i, j int) bool {
		return strings.ToLower(sorted[i]) < strings.ToLower(sorted[j])
	})
	inputRunes := []rune(input)
	best := suggestionCandidate{}
	for _, candidate := range sorted {
		normalized := strings.ToLower(strings.TrimSpace(candidate))
		if normalized == "" {
			continue
		}
		if normalized == input {
			return candidate
		}
		distance := levenshteinDistance(input, normalized)
		prefix := len(inputRunes) >= 3 && strings.HasPrefix(normalized, input)
		if !prefix && distance > 2 {
			continue
		}
		score := distance
		if prefix {
			score = 0
		}
		normalizedRunes := []rune(normalized)
		current := suggestionCandidate{
			raw:          candidate,
			normalized:   normalized,
			score:        score,
			firstRune:    normalizedRunes[0] == inputRunes[0],
			prefixLength: commonPrefixLength(inputRunes, normalizedRunes),
		}
		if betterSuggestion(current, best) {
			best = current
		}
	}
	return best.raw
}

type suggestionCandidate struct {
	raw          string
	normalized   string
	score        int
	firstRune    bool
	prefixLength int
}

func betterSuggestion(candidate, best suggestionCandidate) bool {
	if best.raw == "" {
		return true
	}
	if candidate.score != best.score {
		return candidate.score < best.score
	}
	if candidate.firstRune != best.firstRune {
		return candidate.firstRune
	}
	if candidate.prefixLength != best.prefixLength {
		return candidate.prefixLength > best.prefixLength
	}
	if len(candidate.normalized) != len(best.normalized) {
		return len(candidate.normalized) < len(best.normalized)
	}
	return candidate.normalized < best.normalized
}

func commonPrefixLength(a, b []rune) int {
	length := min(len(a), len(b))
	for index := 0; index < length; index++ {
		if a[index] != b[index] {
			return index
		}
	}
	return length
}

func levenshteinDistance(a, b string) int {
	ar, br := []rune(a), []rune(b)
	previous := make([]int, len(br)+1)
	for index := range previous {
		previous[index] = index
	}
	for i := 1; i <= len(ar); i++ {
		current := make([]int, len(br)+1)
		current[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 0
			if ar[i-1] != br[j-1] {
				cost = 1
			}
			current[j] = min(previous[j]+1, current[j-1]+1, previous[j-1]+cost)
		}
		previous = current
	}
	return previous[len(br)]
}
