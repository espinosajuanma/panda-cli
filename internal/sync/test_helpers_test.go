package sync

import "encoding/json"

func marshalForTest(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}
