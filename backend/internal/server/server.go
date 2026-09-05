// Package server provides the HeapBuddy HTTP server: a small, self-hosted web UI
// for uploading an HPROF heap dump and viewing the analysis report in a browser.
//
// It is designed for trusted, on-prem use (a team running it inside their own
// network), so it deliberately ships without authentication. The only built-in
// guardrail is a configurable upload-size cap to avoid an accidental OOM.
package server

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/sachin-handiekar/HeapBuddy/internal/api"
)

// DefaultMaxUploadBytes is the default cap on an uploaded heap dump. Heap dumps
// are read from a temp file on disk (not held wholesale in memory) but the
// in-memory object graph still scales with the dump, so the cap bounds blast
// radius. Override with WithMaxUpload.
const DefaultMaxUploadBytes int64 = 8 << 30 // 8 GiB

// tempFilePattern is the os.CreateTemp pattern (and the glob used by the
// startup sweep) for spooled heap dumps. Heap dumps are sensitive, so we both
// delete them as soon as analysis finishes and sweep any that a crash left
// behind on the next boot.
const tempFilePattern = "heapbuddy-*.hprof"

// graphCachePattern is the os.CreateTemp pattern for serialized graph caches
// (Phase 2b). The boot sweep matches it with a trailing * so half-written
// .hbgraph.tmp files — and caches whose deletion Windows refused while they
// were still mapped — are also cleaned up.
const graphCachePattern = "heapbuddy-*.hbgraph"

// DefaultMaxConcurrentAnalyses bounds how many dumps are analyzed at once. Each
// analysis holds the whole object graph in memory, so a few concurrent large
// dumps can exhaust RAM; excess requests get a 429. Override with
// WithMaxConcurrentAnalyses.
const DefaultMaxConcurrentAnalyses = 2

// DefaultEnableAdvancedAnalysis controls whether the Dominator Tree and OQL views
// are exposed. They require building the full dominator tree over the heap — the
// most expensive computation — so they are off by default to keep analysis fast
// and lean (JXRay-style waste reporting). Enable with WithAdvancedAnalysis.
const DefaultEnableAdvancedAnalysis = false

// shutdownTimeout bounds how long a graceful shutdown waits for in-flight
// requests to finish before forcing the server closed.
const shutdownTimeout = 30 * time.Second

// Server holds the configuration and dependencies for the HTTP service.
type Server struct {
	maxUpload int64
	logger    *log.Logger
	store     *api.Store
	spa       *spaHandler // embedded single-page app; nil when not built in
	tempDir   string      // directory for spooled dumps; "" means the OS default
	// allowLocalSources permits the JSON {"path"}/{"url"} analyze inputs, which
	// let the server read an arbitrary file off its own disk or fetch an
	// arbitrary URL (SSRF). Off by default; only file uploads are accepted.
	allowLocalSources bool
	// analyzeSem is a counting semaphore bounding concurrent analyses; a full
	// channel means a new analyze request is shed with 429.
	analyzeSem chan struct{}
	// enableAdvanced exposes the Dominator Tree and OQL views (off by default).
	enableAdvanced bool
	// enablePprof exposes Go's net/http/pprof endpoints under /debug/pprof/
	// (off by default). For a tool whose failure mode is "ate all the RAM",
	// being able to grab a live heap profile is the difference between a bug
	// report and a fix — but the endpoints leak internals, so they are opt-in.
	enablePprof bool
	// graphCache serializes the reference graph to a temp file at analysis time
	// so interactive views mmap it instead of re-parsing the dump (on by
	// default; costs graph-sized disk per report while it is retained).
	graphCache bool
}

// Option configures a Server.
type Option func(*Server)

// WithMaxUpload sets the maximum accepted upload size in bytes. Values <= 0 are
// ignored and the default is kept.
func WithMaxUpload(n int64) Option {
	return func(s *Server) {
		if n > 0 {
			s.maxUpload = n
		}
	}
}

// WithLogger sets the logger used for request/diagnostic output.
func WithLogger(l *log.Logger) Option {
	return func(s *Server) {
		if l != nil {
			s.logger = l
		}
	}
}

// WithTempDir sets the directory used to spool uploaded/downloaded dumps. Empty
// keeps the OS default. Point it at a tmpfs/ramdisk or an encrypted volume so a
// sensitive dump never touches durable storage.
func WithTempDir(dir string) Option {
	return func(s *Server) { s.tempDir = dir }
}

// WithLocalSources enables the JSON {"path"}/{"url"} analyze inputs. These are
// powerful (arbitrary local-file read; server-side URL fetch / SSRF), so they
// are disabled by default.
func WithLocalSources(allow bool) Option {
	return func(s *Server) { s.allowLocalSources = allow }
}

// WithMaxConcurrentAnalyses caps how many analyses run at once. Values <= 0 are
// ignored and the default is kept.
func WithMaxConcurrentAnalyses(n int) Option {
	return func(s *Server) {
		if n > 0 {
			s.analyzeSem = make(chan struct{}, n)
		}
	}
}

// WithAdvancedAnalysis enables (or disables) the Dominator Tree and OQL views.
// These build the full dominator tree on demand, which is the heaviest part of an
// analysis, so they are off by default.
func WithAdvancedAnalysis(enable bool) Option {
	return func(s *Server) { s.enableAdvanced = enable }
}

// WithGraphCache enables (or disables) the graph cache: when on, the analysis
// serializes the compact reference graph to a temp file and the interactive
// views (inspector, OQL, dominator tree, leak chains) memory-map it on first
// use instead of re-parsing the whole dump. Disable to trade slower first
// interaction for zero extra disk use.
func WithGraphCache(enable bool) Option {
	return func(s *Server) { s.graphCache = enable }
}

// WithPprof enables (or disables) the /debug/pprof/ endpoints. Off by default:
// profiles expose server internals, so turn this on only when diagnosing
// memory or CPU behavior.
func WithPprof(enable bool) Option {
	return func(s *Server) { s.enablePprof = enable }
}

// New constructs a Server with the given options applied over the defaults.
func New(opts ...Option) *Server {
	s := &Server{
		maxUpload:      DefaultMaxUploadBytes,
		logger:         log.New(os.Stderr, "", log.LstdFlags),
		store:          api.NewStore(),
		spa:            newSPAHandler(),
		analyzeSem:     make(chan struct{}, DefaultMaxConcurrentAnalyses),
		enableAdvanced: DefaultEnableAdvancedAnalysis,
		graphCache:     true,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Handler returns the http.Handler exposing all routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.Handle("/api/", s.logRequests(s.apiHandler()))
	if s.enablePprof {
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	}
	return mux
}

// logRequests logs one line per API request — method, path, status, duration —
// so a self-hoster can see what the server is doing and how long the heavy
// endpoints take. The SPA assets and health probe are deliberately not logged.
func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		s.logger.Printf("api %s %s -> %d (%s)",
			r.Method, r.URL.Path, sw.status, time.Since(start).Round(time.Millisecond))
	})
}

// statusRecorder captures the response status code for request logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// handleRoot serves the embedded single-page app, which owns "/" and all
// unmatched client routes (e.g. /report/{id}); /api and /healthz are matched by
// more specific patterns first. When no SPA was built into the binary there is
// no UI to serve, so it returns a clear 503 (with a pointer to the JSON API)
// rather than a stale fallback — build with `make build`, which embeds the
// frontend.
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if s.spa == nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, "HeapBuddy UI is not built into this binary. "+
			"Run `make build` to embed the frontend, or use the JSON API under /api.\n")
		return
	}
	s.spa.ServeHTTP(w, r)
}

// ListenAndServe starts the HTTP server on addr (e.g. ":8080") and blocks until
// it stops or an interrupt/terminate signal arrives, in which case it drains
// in-flight requests (bounded by shutdownTimeout) before returning.
// ReadHeaderTimeout guards against slow-header attacks; the body and response
// timeouts are left open because large uploads and analyses can take a while.
func (s *Server) ListenAndServe(addr string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// stop() restores default signal handling so a second Ctrl-C force-quits
	// instead of waiting on the drain.
	return s.serve(ctx, addr, stop)
}

// serve runs the HTTP server until ctx is cancelled (or it fails to bind), then
// drains in-flight requests within shutdownTimeout. onShutdown is invoked once
// the shutdown begins. It is the testable core of ListenAndServe.
func (s *Server) serve(ctx context.Context, addr string, onShutdown func()) error {
	s.sweepStaleTempFiles()
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 15 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() {
		s.logger.Printf("HeapBuddy server listening on %s (max upload %s)", addr, humanizeBytes(s.maxUpload))
		serveErr <- httpSrv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		return err // failed to bind / crashed before shutdown
	case <-ctx.Done():
		onShutdown()
		s.logger.Printf("shutdown signal received; draining in-flight requests (up to %s)…", shutdownTimeout)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		return httpSrv.Shutdown(shutdownCtx)
	}
}

// sweepStaleTempFiles removes spooled dumps left behind in the temp dir by a
// previous run that died before its deferred cleanup could run (kill -9, OOM,
// power loss). It runs at startup. This assumes a single server instance per
// temp dir — fine for a local-first tool; use a dedicated --temp-dir if you run
// several side by side.
func (s *Server) sweepStaleTempFiles() {
	dir := s.tempDir
	if dir == "" {
		dir = os.TempDir()
	}
	var matches []string
	for _, pattern := range []string{tempFilePattern, graphCachePattern + "*"} {
		found, err := filepath.Glob(filepath.Join(dir, pattern))
		if err != nil {
			continue
		}
		matches = append(matches, found...)
	}
	removed := 0
	for _, m := range matches {
		if err := os.Remove(m); err == nil {
			removed++
		}
	}
	if removed > 0 {
		s.logger.Printf("swept %d stale heap-dump temp file(s) from %s", removed, dir)
	}
}

// handleHealth is a trivial liveness probe for container orchestration.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, "ok")
}

// spoolToTemp writes the uploaded stream to a temporary file in dir (empty means
// the OS default) and returns its path plus a cleanup function that removes it.
// The parser reads sequentially from disk, which keeps peak memory bounded by the
// analysis structures rather than the raw file size.
func spoolToTemp(dir string, src io.Reader) (path string, cleanup func(), err error) {
	tmp, err := os.CreateTemp(dir, tempFilePattern)
	if err != nil {
		return "", nil, fmt.Errorf("create temp file: %w", err)
	}
	cleanup = func() {
		_ = os.Remove(tmp.Name())
	}

	if _, err := io.Copy(tmp, src); err != nil {
		_ = tmp.Close()
		cleanup()
		return "", nil, fmt.Errorf("buffer upload: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("flush upload: %w", err)
	}
	return tmp.Name(), cleanup, nil
}
