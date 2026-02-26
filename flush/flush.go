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
	cfg    Config
	stopCh chan struct{}
	doneCh chan struct{}
	sigCh  chan os.Signal
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
// It should be called via defer after Enable.
func Stop() {
	mu.Lock()
	s := state
	if !enabled || s == nil {
		mu.Unlock()
		return
	}
	enabled = false
	state = nil
	mu.Unlock()

	close(s.stopCh)
	<-s.doneCh

	if s.sigCh != nil {
		signal.Stop(s.sigCh)
	}

	// Final flush
	_ = doFlush(s.cfg)
}

// Flush performs an immediate coverage data flush.
func Flush() error {
	mu.Lock()
	s := state
	if !enabled || s == nil {
		mu.Unlock()
		return nil
	}
	cfg := s.cfg
	mu.Unlock()

	return doFlush(cfg)
}

// HandleSignal registers signal-based flush triggers.
// When any of the specified signals is received, a flush is performed.
func HandleSignal(sigs ...os.Signal) {
	mu.Lock()
	s := state
	if !enabled || s == nil {
		mu.Unlock()
		return
	}
	mu.Unlock()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, sigs...)
	s.sigCh = ch

	go func() {
		for {
			select {
			case <-ch:
				_ = doFlush(s.cfg)
			case <-s.stopCh:
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
			_ = doFlush(s.cfg)
		case <-s.stopCh:
			return
		}
	}
}

func doFlush(cfg Config) error {
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
	meta := Metadata{
		Timestamp:    time.Now(),
		Hostname:     hostname,
		PodName:      os.Getenv("POD_NAME"),
		BuildVersion: cfg.BuildVersion,
		ServiceName:  cfg.ServiceName,
	}

	if err := cfg.Storage.Store(context.Background(), files, meta); err != nil {
		return fmt.Errorf("goreach/flush: store: %w", err)
	}

	if cfg.Clear {
		_ = coverage.ClearCounters()
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
