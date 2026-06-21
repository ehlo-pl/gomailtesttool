//go:build integration

package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// TestSendMail runs gomailtest msgraph sendmail and verifies:
//   - the command exits with code 0
//   - a CSV log file is created in the temp directory
//
// Requires env vars: MSGRAPHTENANTID, MSGRAPHCLIENTID, MSGRAPHSECRET, MSGRAPHMAILBOX
// Run with: go test -tags integration -v ./tests/integration/
func TestSendMail(t *testing.T) {
	requiredEnv := []string{
		"MSGRAPHTENANTID",
		"MSGRAPHCLIENTID",
		"MSGRAPHSECRET",
		"MSGRAPHMAILBOX",
	}
	for _, e := range requiredEnv {
		if os.Getenv(e) == "" {
			t.Skipf("skipping: %s not set", e)
		}
	}

	// Resolve binary relative to this file's location (tests/integration/ → ../../bin/)
	binary := filepath.Join("..", "..", "bin", "gomailtest")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}

	cmd := exec.Command(binary, "msgraph", "sendmail", "--verbose")
	output, err := cmd.CombinedOutput()
	t.Logf("output:\n%s", output)
	if err != nil {
		t.Fatalf("sendmail failed: %v", err)
	}

	// Verify CSV log file was created (matches pattern written by msgraph protocol)
	date := time.Now().Format("2006-01-02")
	csvPath := filepath.Join(os.TempDir(), fmt.Sprintf("_msgraphtool_sendmail_%s.csv", date))
	if _, statErr := os.Stat(csvPath); os.IsNotExist(statErr) {
		t.Errorf("expected CSV log at %s, not found", csvPath)
	}
}
