//go:build !integration

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const testGlobal = `
warmingup:
  envfile: secrets.env
endpoints:
  mta: { protocol: smtp, host: mail.example.com, port: 587, starttls: true, username: w@example.com, password: "${WARMUPTEST_MTA}" }
  box: { protocol: imap, host: imap.example.net, imaps: true, username: a@example.net, password: "${WARMUPTEST_BOX}" }
`

const testDomain = `
domain: example.com
sender: { endpoint: mta, from: news@example.com }
targets: [box]
campaign: { start: 2026-12-01 08:00 }
`

func writeConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range map[string]string{
		"warmingup.yaml":      testGlobal,
		"secrets.env":         "WARMUPTEST_MTA=one\n",
		"domains.d/site.yaml": testDomain,
	} {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dir, "warmingup.yaml")
}

func TestPlanService(t *testing.T) {
	config := writeConfig(t)
	data := filepath.Join(t.TempDir(), "data")
	g := &globals{configFile: config, dataDir: data}

	if _, err := planService(g); err == nil || !strings.Contains(err.Error(), "WARMUPTEST_BOX") {
		t.Fatalf("an unresolvable secret must stop the installation, got %v", err)
	}
	t.Setenv("WARMUPTEST_BOX", "two")
	p, err := planService(g)
	if err != nil {
		t.Fatal(err)
	}
	if p.configFile != config || p.dataDir != data || p.domains != 1 || p.envFile != filepath.Join(filepath.Dir(config), "secrets.env") {
		t.Errorf("plan = %+v", p)
	}
	warnings := strings.Join(p.warnings, "\n")
	// BOX comes only from the installer's environment; MTA is in the envfile.
	if !strings.Contains(warnings, "WARMUPTEST_BOX") || strings.Contains(warnings, "WARMUPTEST_MTA") {
		t.Errorf("environment warning: %s", warnings)
	}
	if !strings.Contains(warnings, "datadir") {
		t.Errorf("a --datadir that other commands would miss must be reported: %s", warnings)
	}
	if _, err := os.Stat(data); !os.IsNotExist(err) {
		t.Error("planning must not create anything")
	}
}

func TestProtect(t *testing.T) {
	root := t.TempDir()
	programData := filepath.Join(root, "ProgramData")
	busy := filepath.Join(root, "busy")
	if err := os.MkdirAll(busy, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(busy, "other.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	ours := filepath.Join(programData, "warmingup")

	run := func(p *servicePlan) (calls []string, warnings []string) {
		warnings = protect(p, "svc", programData, func(path, _ string, readOnly bool) error {
			calls = append(calls, fmt.Sprintf("%s %t", path, readOnly))
			return nil
		})
		return calls, warnings
	}

	// Everything in one folder under ProgramData.
	calls, warnings := run(&servicePlan{dataDir: ours, configFile: filepath.Join(ours, "warmingup.yaml")})
	if !slices.Equal(calls, []string{ours + " false"}) || len(warnings) != 0 {
		t.Errorf("one folder: %v %v", calls, warnings)
	}
	// A separate configuration folder under ProgramData becomes read only.
	conf := filepath.Join(programData, "conf")
	calls, _ = run(&servicePlan{dataDir: ours, configFile: filepath.Join(conf, "warmingup.yaml")})
	if !slices.Equal(calls, []string{ours + " false", conf + " true"}) {
		t.Errorf("two folders: %v", calls)
	}
	// Folders holding other files are left alone; the secrets file is restricted.
	env := filepath.Join(busy, "warmingup.env")
	calls, warnings = run(&servicePlan{dataDir: busy, configFile: filepath.Join(busy, "warmingup.yaml"), envFile: env})
	if len(calls) != 0 || len(warnings) != 1 {
		t.Errorf("busy folder: %v %v", calls, warnings)
	}
	calls, warnings = run(&servicePlan{dataDir: ours, configFile: filepath.Join(busy, "warmingup.yaml"), envFile: env})
	if !slices.Equal(calls, []string{ours + " false", env + " true"}) || len(warnings) != 1 {
		t.Errorf("config elsewhere: %v %v", calls, warnings)
	}
	// ProgramData itself is never rewritten.
	if err := os.WriteFile(filepath.Join(programData+"-file"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if within(programData, programData) || !within(ours, programData) || within(programData+"-file", programData) {
		t.Error("within is wrong")
	}
}

func TestServiceLogsToFile(t *testing.T) {
	t.Setenv("WARMUPTEST_BOX", "two")
	data := t.TempDir()
	e, err := setup(t.Context(), &globals{configFile: writeConfig(t), dataDir: data, logToFile: true}, needSecrets)
	if err != nil {
		t.Fatal(err)
	}
	e.log.Info("hello from the service")
	e.Close()
	files, err := filepath.Glob(filepath.Join(data, "logs", "warmingup-*.log"))
	if err != nil || len(files) != 1 {
		t.Fatalf("log files = %v, %v", files, err)
	}
	if got, err := os.ReadFile(files[0]); err != nil || !strings.Contains(string(got), "hello from the service") {
		t.Errorf("log = %q, %v", got, err)
	}
}
