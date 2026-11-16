package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/espinosajuanma/panda-cli/internal/config"
)

func TestDoctorRunsBeforeNormalConfigurationLoading(t *testing.T) {
	setTestConfigHome(t)
	t.Setenv("PANDA_API_URL", "")
	configDir, err := config.Directory()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	err = Run([]string{"doctor", "--json"}, strings.NewReader(""), &output, &output)
	if err == nil {
		t.Fatal("doctor should report malformed configuration as a failure")
	}
	var report doctorReport
	if parseErr := json.Unmarshal([]byte(output.String()), &report); parseErr != nil {
		t.Fatalf("doctor did not emit JSON for malformed config: %v\n%s", parseErr, output.String())
	}
	if report.OK || !hasDoctorCode(report, "config.invalid") {
		t.Fatalf("malformed config report = %#v", report)
	}
}

func hasDoctorCode(report doctorReport, code string) bool {
	for _, finding := range report.Findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}
