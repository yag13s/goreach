package flush

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/coverage"
	"sync"
	"time"
)

// Config configures the coverage flush behavior.
type Config struct {
	// Storage is the destination for coverage data. If nil, LocalStorage using
	// GOCOVERDIR environment variable is used as a fallback.
	Storage Storage

	// ServiceName identifies the service producing coverage data.
	ServiceName string

	// BuildVersion is the build version or commit hash. Coverage data from
	// different build versions must not be merged (covmeta incompatibility).
	BuildVersion string

	// Interval sets the periodic flush interval. Zero disables periodic flush.
	Interval time.Duration

	// Clear resets coverage counters after each flush (atomic mode only).
	Clear bool
}

var (
	mu      sync.Mutex
	state   *flushState
	enabled bool
)

type flushState struct {
	cfg     Config
	stopCh  chan struct{}
	doneCh  chan struct{}
	flushMu sync.Mutex // serializes doFlush to prevent concurrent write+clear races

	sigMu     sync.Mutex
	sigCh     chan os.Signal
	sigStopCh chan struct{} // stop channel for current signal goroutine
}

// Enable activates coverage flushing with the given configuration.
// If the binary was not built with -cover, Enable is a no-op.
func Enable(cfg Config) {
	if !coverageAvailable() {
		return
	}

	mu.Lock()
	defer mu.Unlock()

	if enabled {
		return
	}

	if cfg.Storage == nil {
		dir := os.Getenv("GOCOVERDIR")
		if dir == "" {
			dir = filepath.Join(os.TempDir(), "goreach-coverage")
		}
		cfg.Storage = LocalStorage{Dir: dir}
	}

	s := &flushState{
		cfg:    cfg,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
	state = s
	enabled = true

	if cfg.Interval > 0 {
		go s.periodicFlush()
	} else {
		close(s.doneCh)
	}
}

// Stop performs a final flush and stops periodic flushing.
// It returns the error from the final flush, if any.
// It should be called via defer after Enable.
func Stop() error {
	mu.Lock()
	s := state
	if !enabled || s == nil {
		mu.Unlock()
		return nil
	}
	enabled = false
	state = nil
	mu.Unlock()

	close(s.stopCh)
	<-s.doneCh

	s.sigMu.Lock()
	if s.sigCh != nil {
		signal.Stop(s.sigCh)
	}
	s.sigMu.Unlock()

	// Final flush
	return s.doFlush()
}

// Emit performs an immediate coverage data flush.
func Emit() error {
	mu.Lock()
	s := state
	if !enabled || s == nil {
		mu.Unlock()
		return nil
	}
	mu.Unlock()

	return s.doFlush()
}

// HandleSignal registers signal-based flush triggers.
// When any of the specified signals is received, a flush is performed.
// Calling HandleSignal again replaces the previous signal handler.
func HandleSignal(sigs ...os.Signal) {
	mu.Lock()
	s := state
	if !enabled || s == nil {
		mu.Unlock()
		return
	}
	mu.Unlock()

	s.sigMu.Lock()
	defer s.sigMu.Unlock()

	// Clean up previous signal handler if any.
	if s.sigCh != nil {
		signal.Stop(s.sigCh)
		close(s.sigStopCh)
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, sigs...)
	s.sigCh = ch
	sigStop := make(chan struct{})
	s.sigStopCh = sigStop

	go func() {
		for {
			select {
			case <-ch:
				_ = s.doFlush()
			case <-s.stopCh:
				return
			case <-sigStop:
				return
			}
		}
	}()
}

func (s *flushState) periodicFlush() {
	defer close(s.doneCh)
	ticker := time.NewTicker(s.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			_ = s.doFlush()
		case <-s.stopCh:
			return
		}
	}
}

func (s *flushState) doFlush() error {
	// Serialize flushes to prevent concurrent write+clear races.
	s.flushMu.Lock()
	defer s.flushMu.Unlock()

	tmpDir, err := os.MkdirTemp("", "goreach-flush-*")
	if err != nil {
		return fmt.Errorf("goreach/flush: create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write coverage meta and counters to temp dir
	if err := coverage.WriteMetaDir(tmpDir); err != nil {
		return fmt.Errorf("goreach/flush: write meta: %w", err)
	}
	if err := coverage.WriteCountersDir(tmpDir); err != nil {
		return fmt.Errorf("goreach/flush: write counters: %w", err)
	}

	// Collect files written
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return fmt.Errorf("goreach/flush: read temp dir: %w", err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() {
			files = append(files, filepath.Join(tmpDir, e.Name()))
		}
	}
	if len(files) == 0 {
		return nil
	}

	hostname, _ := os.Hostname()
	podName := os.Getenv("POD_NAME")
	if podName == "" {
		podName = hostname
	}
	if podName == "" {
		podName = "unknown"
	}
	meta := Metadata{
		Timestamp:    time.Now(),
		Hostname:     hostname,
		PodName:      podName,
		BuildVersion: s.cfg.BuildVersion,
		ServiceName:  s.cfg.ServiceName,
	}

	if err := s.cfg.Storage.Store(context.Background(), files, meta); err != nil {
		return fmt.Errorf("goreach/flush: store: %w", err)
	}

	if s.cfg.Clear {
		if err := coverage.ClearCounters(); err != nil {
			return fmt.Errorf("goreach/flush: clear counters: %w", err)
		}
	}

	return nil
}

// coverageAvailable checks if coverage instrumentation is present.
func coverageAvailable() bool {
	// Try writing meta to a temp dir. If the binary was not built with -cover,
	// WriteMetaDir returns an error.
	tmpDir, err := os.MkdirTemp("", "goreach-check-*")
	if err != nil {
		return false
	}
	defer os.RemoveAll(tmpDir)
	err = coverage.WriteMetaDir(tmpDir)
	return err == nil
}
