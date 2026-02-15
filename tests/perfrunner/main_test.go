package perfrunner_test

import (
	"os/exec"
	"strings"
	"testing"
)

func TestRunScenarioSmoke(t *testing.T) {
	cmd := exec.Command("go", "run", "../../cmd/perf-runner", "--scenarios", "small", "--timeout", "20s", "--repeat", "1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("perf-runner failed: %v\n%s", err, string(out))
	}
	if !strings.Contains(string(out), "small,1,ok,") {
		t.Fatalf("expected successful scenario output, got:\n%s", string(out))
	}
}
