// Sample server to try goreach end-to-end.
//
// Build and run:
//
//	go build -cover -covermode=atomic -o sampleserver ./testdata/sampleserver
//	mkdir -p /tmp/goreach-demo
//	GOCOVERDIR=/tmp/goreach-demo ./sampleserver
//
// In another terminal, send some requests:
//
//	curl http://localhost:8080/hello?name=Alice
//	curl http://localhost:8080/calc?op=add&a=3&b=4
//
// Then stop the server (Ctrl+C) and analyze:
//
//	go run ./cmd/goreach analyze -coverdir /tmp/goreach-demo -pretty
//	go run ./cmd/goreach summary -coverdir /tmp/goreach-demo
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /hello", handleHello)
	mux.HandleFunc("GET /calc", handleCalc)
	mux.HandleFunc("GET /health", handleHealth)

	srv := &http.Server{Addr: ":8080", Handler: mux}

	// Graceful shutdown on SIGINT/SIGTERM so that Go writes covcounters on exit.
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("shutting down ...")
		srv.Shutdown(context.Background())
	}()

	log.Println("sampleserver listening on :8080")
	log.Println("Try:")
	log.Println("  curl http://localhost:8080/hello?name=Alice")
	log.Println("  curl http://localhost:8080/calc?op=add&a=3&b=4")
	log.Println("  curl http://localhost:8080/health")
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func handleHello(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		name = "World"
	}
	greeting := greet(name)
	json.NewEncoder(w).Encode(map[string]string{"message": greeting})
}

func handleCalc(w http.ResponseWriter, r *http.Request) {
	op := r.URL.Query().Get("op")
	a, _ := strconv.Atoi(r.URL.Query().Get("a"))
	b, _ := strconv.Atoi(r.URL.Query().Get("b"))

	result, err := calc(op, a, b)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	json.NewEncoder(w).Encode(formatCalcResult(op, a, b, result))
}

func formatCalcResult(op string, a, b, result int) map[string]any {
	m := map[string]any{"op": op, "a": a, "b": b, "result": result}
	if result < 0 {
		m["warning"] = "negative result"
	}
	return m
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// --- business logic (coverage targets) ---

func greet(name string) string {
	if name == "admin" {
		return "Welcome back, administrator!"
	}
	if name == "guest" {
		return "Hello, guest! Please sign in."
	}
	return fmt.Sprintf("Hello, %s!", name)
}

func calc(op string, a, b int) (int, error) {
	switch op {
	case "add":
		return a + b, nil
	case "sub":
		return a - b, nil
	case "mul":
		return a * b, nil
	case "div":
		if b == 0 {
			return 0, fmt.Errorf("division by zero")
		}
		return a / b, nil
	default:
		return 0, fmt.Errorf("unknown op: %s", op)
	}
}

// unusedFeature is intentionally never called to demonstrate unreached code detection.
func unusedFeature() string {
	return "this code path is never reached in normal operation"
}

// unusedValidation is another function that is never called.
func unusedValidation(input string) error {
	if len(input) == 0 {
		return fmt.Errorf("input must not be empty")
	}
	if len(input) > 1000 {
		return fmt.Errorf("input too long: %d chars", len(input))
	}
	return nil
}
