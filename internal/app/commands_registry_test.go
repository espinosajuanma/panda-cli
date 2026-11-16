package app

import (
	"strings"
	"testing"
)

func TestCommandRegistryMatchesMainCommands(t *testing.T) {
	mainWorkspace := make(map[string]bool)
	for _, command := range mainCommands {
		fields := strings.Fields(command.command)
		if len(fields) == 0 || fields[0] == "help" {
			continue
		}
		if _, exists := mainWorkspace[fields[0]]; !exists {
			mainWorkspace[fields[0]] = command.workspace
		}
	}
	for name, needsWorkspace := range mainWorkspace {
		spec, ok := commandRegistry[name]
		if !ok {
			t.Errorf("command %q is listed in mainCommands but missing from commandRegistry", name)
			continue
		}
		if spec.needsWorkspace != needsWorkspace {
			t.Errorf("command %q workspace flag = %v, want %v", name, spec.needsWorkspace, needsWorkspace)
		}
		if spec.run == nil {
			t.Errorf("command %q has no handler", name)
		}
	}
	for name := range commandRegistry {
		if _, ok := mainWorkspace[name]; !ok {
			t.Errorf("command %q is registered but missing from mainCommands", name)
		}
	}
}

func TestEveryRegisteredCommandHasManual(t *testing.T) {
	for name := range commandRegistry {
		if _, ok := manualPages[name]; !ok {
			t.Errorf("command %q has no manual page", name)
		}
	}
}
