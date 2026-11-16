package task

import (
	"fmt"

	"github.com/espinosajuanma/panda-cli/internal/jsonutil"
)

func Label(values map[string]any) string {
	if number := values["number"]; number != nil {
		return fmt.Sprint(number)
	}
	if id := jsonutil.StringField(values, "id"); id != "" {
		return id
	}
	if localID := jsonutil.StringField(values, "localId"); localID != "" {
		return localID
	}
	return "local task"
}
