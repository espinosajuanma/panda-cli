package catalog

import (
	"fmt"
	"sort"
	"strings"

	"panda-dashboard/internal/jsonutil"
)

func MergeRecords(kind Kind, local, baseline, remote []map[string]any) ([]map[string]any, []map[string]any, []string) {
	local = DedupeRecords(local)
	baseline = DedupeRecords(baseline)
	remote = DedupeRecords(remote)
	fields := Fields(kind)
	localByID := make(map[string]map[string]any)
	remoteByID := make(map[string]map[string]any)
	for _, record := range local {
		if id := jsonutil.StringField(record, "id"); id != "" {
			localByID[id] = record
		}
	}
	for _, record := range remote {
		if id := jsonutil.StringField(record, "id"); id != "" {
			remoteByID[id] = record
		}
	}
	merged := make([]map[string]any, 0, len(local)+len(remote))
	advanced := make([]map[string]any, 0, len(remote)+len(baseline))
	var conflicts []string
	handledLocal := make(map[string]bool)
	handledRemote := make(map[string]bool)
	for _, base := range baseline {
		id := jsonutil.StringField(base, "id")
		localRecord, hasLocal := localByID[id]
		remoteRecord, hasRemote := remoteByID[id]
		switch {
		case !hasLocal && !hasRemote:
			continue
		case !hasLocal && hasRemote:
			if RecordChanged(kind, remoteRecord, base) {
				conflicts = append(conflicts, fmt.Sprintf("%s %q was deleted locally but changed remotely", ItemName(kind), jsonutil.StringField(base, NameField(kind))))
			}
			advanced = append(advanced, jsonutil.CloneMap(base))
			handledRemote[id] = true
		case hasLocal && !hasRemote:
			if RecordChanged(kind, localRecord, base) {
				conflicts = append(conflicts, fmt.Sprintf("%s %q was deleted remotely but changed locally", ItemName(kind), jsonutil.StringField(base, NameField(kind))))
				merged = append(merged, jsonutil.CloneMap(localRecord))
				advanced = append(advanced, jsonutil.CloneMap(base))
			}
			handledLocal[id] = true
		case hasLocal && hasRemote:
			updatedLocal := jsonutil.CloneMap(remoteRecord)
			updatedBaseline := jsonutil.CloneMap(remoteRecord)
			pending := false
			for _, field := range fields {
				localChanged := !jsonutil.Same(localRecord[field], base[field])
				remoteChanged := !jsonutil.Same(remoteRecord[field], base[field])
				if !localChanged {
					continue
				}
				localValue, remoteValue := localRecord[field], remoteRecord[field]
				if remoteChanged && !jsonutil.Same(localValue, remoteValue) {
					conflicts = append(conflicts, fmt.Sprintf("%s %q: %s", ItemName(kind), jsonutil.StringField(base, NameField(kind)), field))
				}
				if jsonutil.Same(localValue, remoteValue) {
					continue
				}
				setField(updatedLocal, field, localRecord)
				setField(updatedBaseline, field, base)
				pending = true
			}
			merged = append(merged, updatedLocal)
			if pending {
				advanced = append(advanced, updatedBaseline)
			} else {
				advanced = append(advanced, jsonutil.CloneMap(remoteRecord))
			}
			handledLocal[id], handledRemote[id] = true, true
		}
	}
	for _, record := range remote {
		id := jsonutil.StringField(record, "id")
		if id != "" && handledRemote[id] {
			continue
		}
		merged = append(merged, jsonutil.CloneMap(record))
		advanced = append(advanced, jsonutil.CloneMap(record))
		handledRemote[id] = true
	}
	for _, record := range local {
		if jsonutil.StringField(record, "localId") != "" {
			merged = append(merged, jsonutil.CloneMap(record))
			continue
		}
		id := jsonutil.StringField(record, "id")
		if remoteByID[id] != nil {
			continue
		}
		if id == "" || !handledLocal[id] {
			merged = append(merged, jsonutil.CloneMap(record))
		}
	}
	merged = DedupeRecords(merged)
	sort.SliceStable(merged, func(i, j int) bool {
		return strings.ToLower(jsonutil.StringField(merged[i], NameField(kind))) < strings.ToLower(jsonutil.StringField(merged[j], NameField(kind)))
	})
	sort.Strings(conflicts)
	return merged, advanced, conflicts
}

func RecordChanged(kind Kind, record, baseline map[string]any) bool {
	for _, field := range Fields(kind) {
		if !jsonutil.Same(record[field], baseline[field]) {
			return true
		}
	}
	return false
}

func DedupeRecords(records []map[string]any) []map[string]any {
	seen := make(map[string]bool, len(records))
	unique := make([]map[string]any, 0, len(records))
	for _, record := range records {
		id := jsonutil.StringField(record, "id")
		if id != "" && seen[id] {
			continue
		}
		if id != "" {
			seen[id] = true
		}
		unique = append(unique, record)
	}
	return unique
}

func setField(destination map[string]any, field string, source map[string]any) {
	if value, exists := source[field]; exists {
		destination[field] = value
	} else {
		delete(destination, field)
	}
}
