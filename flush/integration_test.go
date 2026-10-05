//go:build unix

package flush_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// The tests in this file build testdata/flushapp with -cover and run it, so
// that Enable/Emit/Stop are exercised against real coverage instrumentation.

var (
	buildOnce sync.Once
	appPath   string
	buildErr  error
)

// flushApp returns the path of the instrumented test binary, building it on
// first use.
func flushApp(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping: builds an instrumented binary")
	}
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "goreach-flushapp-*")
		if err != nil {
			buildErr = err
			return
		}
		appPath = filepath.Join(dir, "flushapp")
		cmd := exec.Command("go", "build", "-cover", "-covermode=atomic", "-o", appPath, "./testdata/flushapp")
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = &buildError{err: err, out: string(out)}
		}
	})
	if buildErr != nil {
		t.Fatalf("build flushapp: %v", buildErr)
	}
	return appPath
}

type buildError struct {
	err error
	out string
}

func (e *buildError) Error() string { return e.err.Error() + "\n" + e.out }

func TestMain(m *testing.M) {
	code := m.Run()
	if appPath != "" {
		_ = os.RemoveAll(filepath.Dir(appPath))
	}
	os.Exit(code)
}

// storeCall is one Store call as reported by flushapp.
type storeCall struct {
	Files        []string `json:"files"`
	Service      string   `json:"service"`
	Version      string   `json:"version"`
	Pod          string   `json:"pod"`
	Hostname     string   `json:"hostname"`
	HasTimestamp bool     `json:"has_timestamp"`
}

// runApp runs a flushapp scenario and returns its output directory, the
// Store calls it reported, and its raw stdout.
func runApp(t *testing.T, scenario string, env ...string) (dir string, calls []storeCall, stdout string) {
	t.Helper()
	app := flushApp(t)
	dir = filepath.Join(t.TempDir(), "out")

	cmd := exec.Command(app, scenario, dir)
	// The runtime writes covmeta to GOCOVERDIR at startup; keep it away from dir.
	cmd.Env = append(os.Environ(), "GOCOVERDIR="+t.TempDir(), "POD_NAME=")
	cmd.Env = append(cmd.Env, env...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("flushapp %s: %v\nstderr: %s", scenario, err, stderr.String())
	}

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		var c storeCall
		if json.Unmarshal([]byte(line), &c) == nil && len(c.Files) > 0 {
			calls = append(calls, c)
		}
	}
	return dir, calls, string(out)
}

// requireCoverageData fails unless dir holds data go tool covdata can read.
func requireCoverageData(t *testing.T, dir string) {
	t.Helper()
	var meta, counters int
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read output dir: %v", err)
	}
	for _, e := range entries {
		switch {
		case strings.HasPrefix(e.Name(), "covmeta."):
			meta++
		case strings.HasPrefix(e.Name(), "covcounters."):
			counters++
		}
	}
	if meta == 0 || counters == 0 {
		t.Fatalf("output dir has %d covmeta and %d covcounters files, want at least one of each", meta, counters)
	}
	out, err := exec.Command("go", "tool", "covdata", "percent", "-i="+dir).CombinedOutput()
	if err != nil {
		t.Fatalf("go tool covdata percent: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "coverage:") {
		t.Errorf("unexpected covdata output: %s", out)
	}
}

func TestIntegration_EmitAndStop(t *testing.T) {
	dir, calls, _ := runApp(t, "emit")

	// One Emit while enabled plus the final flush in Stop. Emit before
	// Enable, Emit after Stop and the second Stop must not store anything.
	if len(calls) != 2 {
		t.Fatalf("got %d Store calls, want 2: %+v", len(calls), calls)
	}
	hostname, _ := os.Hostname()
	for i, c := range calls {
		if c.Service != "flushapp" || c.Version != "v1" {
			t.Errorf("call %d: service/version = %q/%q, want flushapp/v1 (second Enable must be ignored)", i, c.Service, c.Version)
		}
		if !c.HasTimestamp {
			t.Errorf("call %d: timestamp not set", i)
		}
		if c.Hostname != hostname {
			t.Errorf("call %d: hostname = %q, want %q", i, c.Hostname, hostname)
		}
		if c.Pod != hostname {
			t.Errorf("call %d: pod = %q, want hostname %q when POD_NAME is empty", i, c.Pod, hostname)
		}
	}
	requireCoverageData(t, dir)
}

func TestIntegration_PodNameFromEnv(t *testing.T) {
	_, calls, _ := runApp(t, "emit", "POD_NAME=pod-7")
	if len(calls) == 0 {
		t.Fatal("no Store calls")
	}
	for i, c := range calls {
		if c.Pod != "pod-7" {
			t.Errorf("call %d: pod = %q, want pod-7", i, c.Pod)
		}
	}
}

func TestIntegration_PeriodicFlush(t *testing.T) {
	dir, calls, _ := runApp(t, "interval")
	// 200ms at a 10ms interval, plus the final flush. Leave plenty of slack
	// for slow machines; the point is that the ticker flushes at all.
	if len(calls) < 3 {
		t.Errorf("got %d Store calls, want several periodic flushes plus the final one", len(calls))
	}
	requireCoverageData(t, dir)
}

func TestIntegration_SignalFlush(t *testing.T) {
	dir, calls, _ := runApp(t, "signal")
	if len(calls) != 1 {
		t.Fatalf("got %d Store calls, want exactly 1 from SIGUSR1", len(calls))
	}
	requireCoverageData(t, dir)
}

func TestIntegration_DefaultStorageUsesGOCOVERDIR(t *testing.T) {
	coverDir := t.TempDir()
	runApp(t, "default-storage", "GOCOVERDIR="+coverDir)
	requireCoverageData(t, coverDir)
}

func TestIntegration_OnError(t *testing.T) {
	_, _, stdout := runApp(t, "on-error")
	if !strings.Contains(stdout, "goreach/flush: store: storage unavailable") {
		t.Errorf("OnError got %q, want the wrapped storage error", stdout)
	}
}
