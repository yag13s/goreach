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
	"errors"
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

// blockingStorage never completes a Store on its own. If honorCtx is set it
// returns when the context is done, like an upload that respects
// cancellation; otherwise it hangs for good, like one that does not.
type blockingStorage struct {
	honorCtx bool
}

func (s blockingStorage) Store(ctx context.Context, _ []string, _ flush.Metadata) error {
	if s.honorCtx {
		<-ctx.Done()
		return ctx.Err()
	}
	select {}
}

// within fails the scenario unless fn returns an error wrapping want in less
// than limit.
func within(limit time.Duration, want error, what string, fn func() error) {
	start := time.Now()
	err := fn()
	if elapsed := time.Since(start); elapsed > limit {
		fatalf("%s took %v, want under %v", what, elapsed, limit)
	}
	if !errors.Is(err, want) {
		fatalf("%s returned %v, want an error wrapping %v", what, err, want)
	}
	fmt.Printf("%s: %v\n", what, err)
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

	case "emit-context":
		// A working Storage: EmitContext and StopContext behave like Emit and Stop.
		flush.Enable(cfg)
		check(flush.EmitContext(context.Background()))
		check(flush.StopContext(context.Background()))
		check(flush.EmitContext(context.Background()))
		check(flush.StopContext(context.Background()))

	case "emit-context-deadline":
		// The Storage blocks until cancelled: the context's deadline ends the flush.
		flush.Enable(flush.Config{Storage: blockingStorage{honorCtx: true}})
		within(2*time.Second, context.DeadlineExceeded, "EmitContext", func() error {
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			return flush.EmitContext(ctx)
		})
		// A context that is already done never starts a flush.
		within(2*time.Second, context.Canceled, "EmitContext (cancelled)", func() error {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return flush.EmitContext(ctx)
		})
		within(2*time.Second, context.DeadlineExceeded, "StopContext", func() error {
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			return flush.StopContext(ctx)
		})

	case "flush-timeout":
		// FlushTimeout bounds the flushes that take no context.
		errs := make(chan error, 16)
		flush.Enable(flush.Config{
			Storage:      blockingStorage{honorCtx: true},
			Interval:     10 * time.Millisecond,
			FlushTimeout: 50 * time.Millisecond,
			OnError: func(err error) {
				select {
				case errs <- err:
				default:
				}
			},
		})
		within(2*time.Second, context.DeadlineExceeded, "periodic flush", func() error {
			select {
			case err := <-errs:
				return err
			case <-time.After(5 * time.Second):
				return nil
			}
		})
		within(2*time.Second, context.DeadlineExceeded, "Emit", flush.Emit)
		within(2*time.Second, context.DeadlineExceeded, "Stop", flush.Stop)

	case "hung-storage":
		// The Storage ignores cancellation and never returns, so the periodic
		// flush is stuck holding the flush lock. Callers with a context must
		// still get control back.
		flush.Enable(flush.Config{
			Storage:  blockingStorage{},
			Interval: 10 * time.Millisecond,
		})
		time.Sleep(100 * time.Millisecond) // let the periodic flush start and hang
		within(2*time.Second, context.DeadlineExceeded, "EmitContext", func() error {
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			return flush.EmitContext(ctx)
		})
		within(2*time.Second, context.DeadlineExceeded, "StopContext", func() error {
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			return flush.StopContext(ctx)
		})

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
