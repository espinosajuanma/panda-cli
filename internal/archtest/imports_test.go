package archtest

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const modulePath = "github.com/espinosajuanma/panda-cli"

func moduleRoot(t *testing.T) string {
	t.Helper()
	output, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		t.Fatalf("resolve module root: %v", err)
	}
	root := filepath.Dir(strings.TrimSpace(string(output)))
	if root == "." || root == "" {
		t.Fatalf("unexpected module root %q", root)
	}
	return root
}

func listImports(t *testing.T) map[string][]string {
	t.Helper()
	command := exec.Command("go", "list", "-f", `{{.ImportPath}}|{{join .Imports " "}}`, "./...")
	command.Dir = moduleRoot(t)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("list package imports: %v", err)
	}
	imports := make(map[string][]string)
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		pkg, pkgImports, ok := strings.Cut(line, "|")
		if !ok {
			t.Fatalf("unexpected go list line %q", line)
		}
		imports[pkg] = strings.Fields(pkgImports)
	}
	return imports
}

func TestLayerImports(t *testing.T) {
	layered := map[string]bool{
		"internal/catalog":   true,
		"internal/config":    true,
		"internal/content":   true,
		"internal/diff":      true,
		"internal/jsonutil":  true,
		"internal/solutions": true,
		"internal/task":      true,
		"internal/ui":        true,
		"internal/workspace": true,
	}
	imports := listImports(t)
	for pkg, pkgImports := range imports {
		short := strings.TrimPrefix(pkg, modulePath+"/")
		if layered[short] {
			for _, imported := range pkgImports {
				if imported == modulePath+"/internal/app" {
					t.Errorf("%s must not import internal/app", short)
				}
			}
		}
		if short != "cmd/panda" {
			continue
		}
		for _, imported := range pkgImports {
			if strings.HasPrefix(imported, modulePath+"/internal/") && imported != modulePath+"/internal/app" {
				t.Errorf("cmd/panda must only import internal/app, found %s", imported)
			}
		}
	}
}
