package flush

import (
	"syscall"
	"testing"
)

// Note: Full integration tests for Enable/Emit/Stop require a binary built
// with -cover flag. These tests exercise the no-op paths and state machine
// behavior when coverage instrumentation is not present.

func TestStop_NotEnabled(t *testing.T) {
	// Stop on an un-enabled flush should be a no-op and return nil.
	if err := Stop(); err != nil {
		t.Errorf("Stop() = %v, want nil", err)
	}
}

func TestStop_DoubleStop(t *testing.T) {
	// Double stop should not panic.
	if err := Stop(); err != nil {
		t.Errorf("first Stop() = %v, want nil", err)
	}
	if err := Stop(); err != nil {
		t.Errorf("second Stop() = %v, want nil", err)
	}
}

func TestEmit_NotEnabled(t *testing.T) {
	if err := Emit(); err != nil {
		t.Errorf("Emit() = %v, want nil", err)
	}
}

func TestHandleSignal_NotEnabled(t *testing.T) {
	// HandleSignal on an un-enabled flush should not panic.
	HandleSignal(syscall.SIGUSR1)
}

func TestEnable_NoCoverage(t *testing.T) {
	// Enable without -cover build is a no-op.
	Enable(Config{})
	defer Stop()

	// State should remain disabled.
	mu.Lock()
	e := enabled
	mu.Unlock()
	if e {
		t.Error("expected enabled=false when coverage is not available")
	}
}

func TestEnable_DoubleEnable_NoCoverage(t *testing.T) {
	// Double enable without coverage should not panic.
	Enable(Config{})
	Enable(Config{})
}

func TestCoverageAvailable(t *testing.T) {
	// Without -cover build, coverageAvailable should return false.
	if coverageAvailable() {
		t.Skip("test binary built with -cover; skipping")
	}
}
