package app

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

func normalizeMemberSelector(value string) string {
	folded, _, err := transform.String(transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC), value)
	if err != nil {
		folded = value
	}
	var normalized strings.Builder
	for _, char := range strings.ToLower(folded) {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			normalized.WriteRune(char)
		}
	}
	return normalized.String()
}

func matchingProjectMembers(members []map[string]any, selector string) []map[string]any {
	query := normalizeMemberSelector(selector)
	if query == "" {
		return nil
	}
	var exact []map[string]any
	for _, member := range members {
		if memberHasNormalizedValue(member, query) {
			exact = appendUniqueProjectMember(exact, member)
		}
	}
	if len(exact) > 0 {
		return sortProjectMembersByName(exact)
	}
	pattern := regexp.MustCompile(regexp.QuoteMeta(query))
	var partial []map[string]any
	for _, member := range members {
		if memberHasNormalizedPattern(member, pattern) {
			partial = appendUniqueProjectMember(partial, member)
		}
	}
	return sortProjectMembersByName(partial)
}

func memberHasNormalizedValue(member map[string]any, value string) bool {
	for _, candidate := range projectMemberMatchValues(member) {
		normalized := normalizeMemberSelector(candidate)
		if normalized == value {
			return true
		}
	}
	return false
}

func memberHasNormalizedPattern(member map[string]any, pattern *regexp.Regexp) bool {
	for _, candidate := range projectMemberMatchValues(member) {
		if pattern.MatchString(normalizeMemberSelector(candidate)) {
			return true
		}
	}
	return false
}

func projectMemberMatchValues(member map[string]any) []string {
	user := memberUser(member)
	values := make([]string, 0, 4)
	for _, field := range []string{"label", "name", "email", "id"} {
		if value := strings.TrimSpace(stringField(user, field)); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func appendUniqueProjectMember(members []map[string]any, member map[string]any) []map[string]any {
	key := relationshipID(memberUser(member))
	if key == "" {
		key = normalizeMemberSelector(memberDisplayName(member))
	}
	for _, existing := range members {
		existingKey := relationshipID(memberUser(existing))
		if existingKey == "" {
			existingKey = normalizeMemberSelector(memberDisplayName(existing))
		}
		if key == existingKey {
			return members
		}
	}
	return append(members, member)
}

func sortProjectMembersByName(members []map[string]any) []map[string]any {
	sort.SliceStable(members, func(i, j int) bool {
		return strings.ToLower(memberDisplayName(members[i])) < strings.ToLower(memberDisplayName(members[j]))
	})
	return members
}

func resolveAssignableProjectMember(eligible, all []map[string]any, selector string) (map[string]any, error) {
	matches := matchingProjectMembers(eligible, selector)
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return nil, ambiguousProjectMemberError(selector, matches)
	}
	if allMatches := matchingProjectMembers(all, selector); len(allMatches) > 0 {
		return nil, fmt.Errorf("project member %q is not eligible for assignment; eligible roles are %s", selector, strings.Join(assignableTeamRoles, ", "))
	}
	return nil, fmt.Errorf("project member %q not found; run `panda pull` to refresh members", selector)
}

func ambiguousProjectMemberError(selector string, matches []map[string]any) error {
	labels := make([]string, 0, len(matches))
	for _, member := range matches {
		user := memberUser(member)
		label := memberDisplayName(member)
		if email := strings.TrimSpace(stringField(user, "email")); email != "" && !strings.EqualFold(label, email) {
			label += " <" + email + ">"
		}
		labels = append(labels, label)
	}
	return fmt.Errorf("project member %q is ambiguous; matches %s; use a full name, email, or ID", selector, strings.Join(labels, ", "))
}
