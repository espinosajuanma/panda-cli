package task

import (
	"strings"

	"github.com/espinosajuanma/panda-cli/internal/jsonutil"
)

func EffortFromMap(values map[string]any) (Effort, bool) {
	complexity := strings.ToLower(strings.TrimSpace(jsonutil.StringField(values, "complexity")))
	effort, ok := ComplexityEffort[complexity]
	return effort, ok
}
