package task

import "github.com/espinosajuanma/panda-cli/internal/jsonutil"

type Task struct {
	values map[string]any
}

func FromMap(values map[string]any) Task {
	if values == nil {
		values = make(map[string]any)
	}
	return Task{values: values}
}

func (task Task) Map() map[string]any {
	return task.values
}

func (task Task) Clone() Task {
	return Task{values: jsonutil.CloneMap(task.values)}
}

func (task Task) Field(name string) any {
	return jsonutil.ValueAt(task.values, name)
}

func (task Task) StringField(name string) string {
	return jsonutil.StringField(task.values, name)
}

func (task Task) Set(name string, value any) {
	if task.values == nil {
		task.values = make(map[string]any)
	}
	task.values[name] = value
}

func (task Task) Delete(name string) {
	delete(task.values, name)
}
