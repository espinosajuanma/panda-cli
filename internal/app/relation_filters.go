package app

import (
	"errors"
	"fmt"
	"strings"
)

type relationFilter struct {
	active bool
	none   bool
	ids    map[string]bool
	names  map[string]bool
}

func relationSelectorsNeedMembers(selectors []string) bool {
	for _, selector := range selectors {
		if !isNoneSelector(selector) {
			return true
		}
	}
	return false
}

func resolveRelationFilter(wsMembers []map[string]any, membersAvailable bool, selectors []string, userEmail, field string) (relationFilter, error) {
	if len(selectors) == 0 {
		return relationFilter{}, nil
	}
	filter := relationFilter{active: true, ids: make(map[string]bool), names: make(map[string]bool)}
	for _, selector := range selectors {
		if isNoneSelector(selector) {
			if len(selectors) != 1 {
				return relationFilter{}, fmt.Errorf("--%s none must be used alone", field)
			}
			filter.none = true
			return filter, nil
		}
	}
	for _, selector := range selectors {
		selector = strings.TrimSpace(selector)
		if strings.EqualFold(selector, "me") || strings.EqualFold(selector, "@me") {
			if strings.TrimSpace(userEmail) == "" {
				return relationFilter{}, fmt.Errorf("--%s me requires a saved login email; run `panda login`", field)
			}
			if !membersAvailable {
				return relationFilter{}, fmt.Errorf("--%s me requires synced project members; run `panda pull` first", field)
			}
			ids := make(map[string]map[string]any)
			for _, member := range wsMembers {
				user := memberUser(member)
				if strings.EqualFold(strings.TrimSpace(stringField(user, "email")), strings.TrimSpace(userEmail)) {
					if id := relationshipID(user); id != "" {
						ids[id] = member
					}
				}
			}
			if len(ids) == 0 {
				return relationFilter{}, fmt.Errorf("login email %q was not found in synced project members; run `panda pull` to refresh members", userEmail)
			}
			for id, member := range ids {
				addMemberToRelationFilter(&filter, id, member)
			}
			continue
		}
		if !membersAvailable {
			filter.names[strings.ToLower(selector)] = true
			continue
		}
		matches := make(map[string]map[string]any)
		for _, member := range wsMembers {
			user := memberUser(member)
			id := relationshipID(user)
			if id == "" {
				continue
			}
			if memberMatchesSelector(member, selector) {
				matches[id] = member
			}
		}
		if len(matches) == 0 {
			return relationFilter{}, fmt.Errorf("project member %q not found in synced members; run `panda pull` to refresh members", selector)
		}
		if len(matches) > 1 {
			return relationFilter{}, fmt.Errorf("project member %q is ambiguous; use an email address or ID", selector)
		}
		for id, member := range matches {
			addMemberToRelationFilter(&filter, id, member)
		}
	}
	return filter, nil
}

func memberMatchesSelector(member map[string]any, selector string) bool {
	user := memberUser(member)
	for _, field := range []string{"label", "name", "email", "id"} {
		if value := strings.TrimSpace(stringField(user, field)); value != "" && strings.EqualFold(value, selector) {
			return true
		}
	}
	return false
}

func addMemberToRelationFilter(filter *relationFilter, id string, member map[string]any) {
	filter.ids[strings.ToLower(id)] = true
	user := memberUser(member)
	for _, field := range []string{"label", "name", "email", "id"} {
		if value := strings.TrimSpace(stringField(user, field)); value != "" {
			filter.names[strings.ToLower(value)] = true
		}
	}
}

func (filter relationFilter) matches(value any) bool {
	if !filter.active {
		return true
	}
	values := relationFilterValues(value)
	if filter.none {
		return len(values) == 0
	}
	for _, candidate := range values {
		key := strings.ToLower(candidate)
		if filter.ids[key] || filter.names[key] {
			return true
		}
	}
	return false
}

func relationFilterValues(value any) []string {
	var values []string
	var add func(any)
	add = func(item any) {
		switch item := item.(type) {
		case string:
			if value := strings.TrimSpace(item); value != "" {
				values = append(values, value)
			}
		case []any:
			for _, nested := range item {
				add(nested)
			}
		case map[string]any:
			for _, field := range []string{"id", "label", "name", "email"} {
				if value := strings.TrimSpace(stringField(item, field)); value != "" {
					values = append(values, value)
				}
			}
			if user, ok := item["user"]; ok {
				add(user)
			}
		}
	}
	add(value)
	return values
}

func appendRelationSelectors(target *[]string, value, flag string) error {
	added := 0
	for _, part := range strings.Split(value, ",") {
		selector := strings.TrimSpace(part)
		if selector == "" {
			continue
		}
		*target = append(*target, selector)
		added++
	}
	if added == 0 {
		return errors.New(flag + " requires one or more comma-separated members")
	}
	return nil
}
