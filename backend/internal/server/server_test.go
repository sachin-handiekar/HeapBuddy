package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// sampleDump is the shared fixture used across the server tests (also referenced
// from api_test.go).
const sampleDump = "../../sample-hprof/sample.hprof"

func TestHealthz(t *testing.T) {
	srv := New()
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("healthz = %d %q, want 200 \"ok\"", rec.Code, rec.Body.String())
	}
}

// TestSweepStaleTempFiles removes heapbuddy-*.hprof left behind by a crashed run
// while leaving unrelated files in the temp dir untouched.
func TestSweepStaleTempFiles(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, "heapbuddy-stale.hprof")
	keep := filepath.Join(dir, "notes.txt")
	for _, f := range []string{stale, keep} {
		if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	New(WithTempDir(dir)).sweepStaleTempFiles()

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale temp dump should have been swept; stat err = %v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("unrelated file should be kept; stat err = %v", err)
	}
}

// TestServeGracefulShutdown checks that cancelling the context drains and stops
// the server, returning nil (not an error).
func TestServeGracefulShutdown(t *testing.T) {
	s := New(WithTempDir(t.TempDir()))
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- s.serve(ctx, "127.0.0.1:0", func() {}) }()

	time.Sleep(50 * time.Millisecond) // let the listener bind
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("graceful shutdown returned %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down within 5s of cancellation")
	}
}
