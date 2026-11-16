package task

import "github.com/espinosajuanma/panda-cli/internal/jsonutil"

func MergeFields(local, remote, baseline Task) (Task, bool) {
	merged := remote.Clone()
	changed := false
	for _, field := range EditableFields {
		localValue := local.Field(field)
		baselineValue := baseline.Field(field)
		remoteValue := remote.Field(field)
		if !jsonutil.Same(localValue, baselineValue) && !jsonutil.Same(localValue, remoteValue) {
			merged.Set(field, localValue)
			changed = true
		}
	}
	return merged, changed
}

func MetadataConflicts(local, baseline, remote Task) []string {
	var conflicts []string
	for _, field := range EditableFields {
		localValue := local.Field(field)
		baselineValue := baseline.Field(field)
		remoteValue := remote.Field(field)
		if !jsonutil.Same(localValue, baselineValue) && !jsonutil.Same(remoteValue, baselineValue) && !jsonutil.Same(localValue, remoteValue) {
			conflicts = append(conflicts, field)
		}
	}
	return conflicts
}

func AdvanceBaseline(baseline, remote Task, conflicts []string, descriptionConflict bool) Task {
	advanced := remote.Clone()
	for _, field := range conflicts {
		if value, exists := baseline.values[field]; exists {
			advanced.Set(field, value)
		} else {
			advanced.Delete(field)
		}
	}
	if descriptionConflict {
		advanced.Set("description", baseline.StringField("description"))
	}
	return advanced
}

func SameVersion(remote, baseline Task) bool {
	remoteVersion, remoteHasVersion := remote.values["version"]
	baselineVersion, baselineHasVersion := baseline.values["version"]
	return remoteHasVersion && baselineHasVersion && remoteVersion != nil && baselineVersion != nil &&
		jsonutil.Same(remoteVersion, baselineVersion) &&
		jsonutil.Same(remote.Field("labels"), baseline.Field("labels")) &&
		jsonutil.Same(remote.Field("package"), baseline.Field("package")) &&
		jsonutil.Same(remote.Field("files"), baseline.Field("files"))
}
