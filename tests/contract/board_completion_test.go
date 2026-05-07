// The `cacheboard` predictor emits per-board descriptors:
//
//	<id>\t<name> (<type>, <project[s]>) — capped at 2 keys with +N
//	overflow for 3+ keys, and projects segment dropped entirely when
//	empty.
//
// The 100ms latency target is verified manually (CI runners are too
// noisy); this test asserts shape only.
package contract

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestCacheBoardPredictorDescriptors(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := emptyBaseURLConfig(t)
	cacheRoot := t.TempDir()

	// Three boards: single-project, two-project, four-project.
	primeBoardsCache(t, cacheRoot, "default", `[
		{"id":1,"name":"Single Project Board","type":"scrum","project_keys":["ENG"]},
		{"id":2,"name":"Two Project Board","type":"kanban","project_keys":["ENG","PLAT"]},
		{"id":3,"name":"Four Project Board","type":"scrum","project_keys":["ENG","PLAT","OPS","SRE"]}
	]`)

	c := exec.Command(bin, "--config", cfg, "--@complete=cacheboard", "--", "")
	c.Env = append(os.Environ(), "XDG_CACHE_HOME="+cacheRoot)
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("complete cacheboard: %v\n%s", err, out)
	}
	got := string(out)

	want := []string{
		"1\tSingle Project Board (scrum, ENG)",
		"2\tTwo Project Board (kanban, ENG, PLAT)",
		"3\tFour Project Board (scrum, ENG, PLAT +2)",
	}
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Fatalf("predictor output missing line %q\n--- got ---\n%s", w, got)
		}
	}
}

func TestCacheBoardPredictorEmptyProjectsDropsSegment(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := emptyBaseURLConfig(t)
	cacheRoot := t.TempDir()

	primeBoardsCache(t, cacheRoot, "default", `[
		{"id":7,"name":"No Project Board","type":"scrum","project_keys":[]}
	]`)

	c := exec.Command(bin, "--config", cfg, "--@complete=cacheboard", "--", "")
	c.Env = append(os.Environ(), "XDG_CACHE_HOME="+cacheRoot)
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("complete cacheboard: %v\n%s", err, out)
	}
	got := string(out)
	want := "7\tNo Project Board (scrum)"
	if !strings.Contains(got, want) {
		t.Fatalf("predictor output missing %q\n--- got ---\n%s", want, got)
	}
}

func TestCacheBoardPredictorEmptyCacheNoError(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := emptyBaseURLConfig(t)
	cacheRoot := t.TempDir()

	c := exec.Command(bin, "--config", cfg, "--@complete=cacheboard", "--", "")
	c.Env = append(os.Environ(), "XDG_CACHE_HOME="+cacheRoot)
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("complete cacheboard with empty cache should not error: %v\n%s", err, out)
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Fatalf("expected empty completion output for empty cache; got: %q", string(out))
	}
}
