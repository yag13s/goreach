// Command flushapp exercises the flush SDK from a binary built with -cover.
// It is built and run by the tests in package flush.
//
// Usage: flushapp <scenario> <output-dir>
//
// Every Store call is reported on stdout as one JSON line.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/yag13s/goreach/flush"
)

// loggingStorage stores files locally and reports each call on stdout.
type loggingStorage struct {
	local flush.LocalStorage
}

func (s loggingStorage) Store(ctx context.Context, files []string, meta flush.Metadata) error {
	names := make([]string, 0, len(files))
	for _, f := range files {
		names = append(names, filepath.Base(f))
	}
	line, err := json.Marshal(map[string]any{
		"files":         names,
		"service":       meta.ServiceName,
		"version":       meta.BuildVersion,
		"pod":           meta.PodName,
		"hostname":      meta.Hostname,
		"has_timestamp": !meta.Timestamp.IsZero(),
	})
	if err != nil {
		return err
	}
	fmt.Println(string(line))
	return s.local.Store(ctx, files, meta)
}

// failingStorage always fails, to exercise OnError.
type failingStorage struct{}

func (failingStorage) Store(context.Context, []string, flush.Metadata) error {
	return fmt.Errorf("storage unavailable")
}

func main() {
	if len(os.Args) != 3 {
		fatalf("usage: flushapp <scenario> <output-dir>")
	}
	scenario, dir := os.Args[1], os.Args[2]

	cfg := flush.Config{
		Storage:      loggingStorage{local: flush.LocalStorage{Dir: dir}},
		ServiceName:  "flushapp",
		BuildVersion: "v1",
	}

	switch scenario {
	case "emit":
		// Emit before Enable and after Stop must be no-ops.
		check(flush.Emit())
		flush.Enable(cfg)
		flush.Enable(flush.Config{Storage: failingStorage{}}) // ignored: already enabled
		check(flush.Emit())
		check(flush.Stop())
		check(flush.Emit())
		check(flush.Stop())

	case "interval":
		cfg.Interval = 10 * time.Millisecond
		cfg.Clear = true
		flush.Enable(cfg)
		time.Sleep(200 * time.Millisecond)
		check(flush.Stop())

	case "signal":
		flush.Enable(cfg)
		flush.HandleSignal(syscall.SIGUSR2)
		flush.HandleSignal(syscall.SIGUSR1) // replaces the SIGUSR2 handler
		check(syscall.Kill(os.Getpid(), syscall.SIGUSR1))
		time.Sleep(200 * time.Millisecond)
		// No final flush here: the test asserts the signal alone stored data.
		os.Exit(0)

	case "default-storage":
		// No Storage: falls back to LocalStorage at $GOCOVERDIR.
		flush.Enable(flush.Config{})
		check(flush.Stop())

	case "on-error":
		errs := make(chan error, 16)
		flush.Enable(flush.Config{
			Storage:  failingStorage{},
			Interval: 10 * time.Millisecond,
			OnError: func(err error) {
				select {
				case errs <- err:
				default:
				}
			},
		})
		select {
		case err := <-errs:
			fmt.Println(err)
		case <-time.After(5 * time.Second):
			fatalf("OnError was not called")
		}
		if err := flush.Stop(); err == nil {
			fatalf("Stop returned nil, want the final flush's storage error")
		}

	default:
		fatalf("unknown scenario %q", scenario)
	}
}

func check(err error) {
	if err != nil {
		fatalf("%v", err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "flushapp: "+format+"\n", args...)
	os.Exit(1)
}
