package jsonutil

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
)

// CloneMap returns an independent JSON-shaped copy of value.
func CloneMap(value map[string]any) map[string]any {
	data, _ := json.Marshal(value)
	var clone map[string]any
	_ = json.Unmarshal(data, &clone)
	if clone == nil {
		clone = make(map[string]any)
	}
	return clone
}

// ValueAt reads a map field, treating a missing field as nil.
func ValueAt(values map[string]any, key string) any {
	return values[key]
}

// Same compares JSON-shaped values structurally.
func Same(left, right any) bool {
	return reflect.DeepEqual(left, right)
}

// StringField formats a present, non-nil map field.
func StringField(values map[string]any, key string) string {
	if values == nil || values[key] == nil {
		return ""
	}
	return StringValue(values[key])
}

// StringValue formats an optional JSON value.
func StringValue(value any) string {
	if value == nil {
		return ""
	}
	switch number := value.(type) {
	case float64:
		return strconv.FormatFloat(number, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(number), 'f', -1, 32)
	}
	return fmt.Sprint(value)
}

// RelationshipID extracts an ID from a Solutions relationship object.
func RelationshipID(value any) string {
	relation, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	return StringField(relation, "id")
}
