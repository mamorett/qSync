package doctor

import (
	"runtime"
	"strings"
	"testing"

	"github.com/mamorett/qsync/internal/config"
)

func TestDoctor_OSXRsyncWarning(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS specific test")
	}
	cfg := config.Default()
	cfg.Transport.Rsync = "/usr/bin/rsync"
	report := Run("config.yaml", cfg, nil)
	var foundCheck *Check
	for i := range report.Checks {
		if report.Checks[i].Name == "rsync-binary" {
			foundCheck = &report.Checks[i]
			break
		}
	}
	if foundCheck == nil {
		t.Fatal("expected rsync-binary check in report")
	}
	if !foundCheck.OK {
		t.Fatalf("expected rsync-binary OK=true for /usr/bin/rsync fallback, got false")
	}
	if !foundCheck.Warning {
		t.Fatalf("expected rsync-binary Warning=true for /usr/bin/rsync fallback, got false")
	}
	if !strings.Contains(foundCheck.Detail, "warning") {
		t.Errorf("unexpected detail: %s", foundCheck.Detail)
	}
}
