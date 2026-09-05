package server

import (
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sachin-handiekar/HeapBuddy/internal/analysis"
	"github.com/sachin-handiekar/HeapBuddy/internal/api"
	"github.com/sachin-handiekar/HeapBuddy/internal/parser"
	"github.com/sachin-handiekar/HeapBuddy/internal/pipeline"
	"github.com/sachin-handiekar/HeapBuddy/internal/types"
)

// urlFetchTimeout bounds how long a remote heap-dump download may take.
const urlFetchTimeout = 5 * time.Minute

// apiHandler builds the JSON API mux consumed by the React frontend
// (frontend/src/lib/api.ts). Routes are mounted under /api and wrapped with a
// permissive-for-localhost CORS layer so the Vite dev server (:5173) can call
// the Go server (:8080) during development. In the production single-binary
// build the UI is same-origin and CORS is a no-op.
func (s *Server) apiHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/analyze", s.handleAPIAnalyze)
	mux.HandleFunc("GET /api/report/{id}", s.handleAPIReport)
	mux.HandleFunc("GET /api/reports/{id}/summary", s.handleAPISummary)
	mux.HandleFunc("GET /api/reports/{id}/histogram", s.handleAPIHistogram)
	mux.HandleFunc("GET /api/reports/{id}/dominators", s.handleAPIDominators)
	mux.HandleFunc("GET /api/reports/{id}/dominator-tree", s.handleAPIDominatorTree)
	mux.HandleFunc("GET /api/reports/{id}/dominator-tree/{nodeId}", s.handleAPIDominatorChildren)
	mux.HandleFunc("GET /api/reports/{id}/leaks", s.handleAPILeaks)
	mux.HandleFunc("GET /api/reports/{id}/leaks/detail", s.handleAPILeakDetail)
	mux.HandleFunc("GET /api/reports/{id}/inspect", s.handleAPIInspect)
	mux.HandleFunc("GET /api/reports/{id}/inspect/{parentId}/{direction}", s.handleAPIInspectChildren)
	mux.HandleFunc("GET /api/reports/{id}/wasted", s.handleAPIWasted)
	mux.HandleFunc("POST /api/report/{id}/oql", s.handleAPIOQL)
	return corsMiddleware(mux)
}

// handleAPIAnalyze runs the analysis synchronously, stores the result, and
// returns its id. It accepts three input shapes the frontend offers:
//   - multipart upload (field "file"; also "heapdump" for parity with the HTML form)
//   - JSON {"path": "..."} — a heap dump already on the server's disk
//   - JSON {"url": "..."}  — a remote heap dump the server fetches
func (s *Server) handleAPIAnalyze(w http.ResponseWriter, r *http.Request) {
	// Shed load before spooling the upload: each analysis holds the whole object
	// graph in memory, so cap how many run at once.
	release, err := s.acquireAnalyzeSlot()
	if err != nil {
		writeBusyError(w, "the server is busy analyzing another dump; please retry shortly")
		return
	}
	defer release()

	path, filename, size, cleanup, ok := s.resolveAnalyzeSource(w, r)
	if !ok {
		return // resolveAnalyzeSource already wrote the error response
	}

	// Only the headline report is built here (no dominator tree). On success
	// the bundle takes ownership of the dump file so the interactive views can
	// be built on demand; the store deletes it on eviction. On any failure we
	// must release the file (and any graph cache) ourselves.
	graphPath := s.newGraphCachePath()
	var (
		stats      *parser.HeapStats
		fullReport *types.FullAnalysisReport
	)
	if graphPath != "" {
		// While the parsed maps are alive, also serialize the reference graph
		// so the interactive views mmap it later instead of re-parsing. A
		// cache-write failure only costs the fast path, never the analysis.
		var cacheErr error
		stats, fullReport, cacheErr, err = pipeline.AnalyzeFileCachingGraph(path, graphPath, false)
		if cacheErr != nil {
			s.logger.Printf("api analyze: graph cache: %v", cacheErr)
			_ = os.Remove(graphPath)
			graphPath = ""
		}
	} else {
		stats, fullReport, err = pipeline.AnalyzeFile(path, false)
	}
	if err != nil {
		cleanup()
		if graphPath != "" {
			_ = os.Remove(graphPath)
		}
		writeJSONError(w, http.StatusUnprocessableEntity, "could not analyze heap dump: "+err.Error())
		return
	}

	bundle := api.BuildBundle(filename, size, path, cleanup, false, s.enableAdvanced, stats, fullReport)
	// Lazy interactive-view builds re-parse the dump, so they must count
	// against the same concurrency cap as fresh analyses.
	bundle.SetBuildGate(s.acquireAnalyzeSlot)
	if graphPath != "" {
		bundle.SetGraphFile(graphPath)
	}
	id := s.store.Put(bundle)
	s.logger.Printf("api analyzed %q (%s) -> %s from %s", filename, humanizeBytes(size), id, r.RemoteAddr)
	writeJSON(w, http.StatusOK, api.AnalyzeResult{ID: id})
}

// newGraphCachePath reserves a temp file for the serialized graph cache and
// returns its path, or "" when the cache is disabled or the file can't be
// created (the report then falls back to re-parsing for interactive views).
func (s *Server) newGraphCachePath() string {
	if !s.graphCache {
		return ""
	}
	f, err := os.CreateTemp(s.tempDir, graphCachePattern)
	if err != nil {
		s.logger.Printf("api analyze: reserving graph cache file: %v", err)
		return ""
	}
	_ = f.Close()
	return f.Name()
}

// resolveAnalyzeSource turns the request into a readable on-disk heap dump,
// returning its path, display name, size, and a cleanup func. On failure it
// writes a JSON error and returns ok=false.
func (s *Server) resolveAnalyzeSource(w http.ResponseWriter, r *http.Request) (path, filename string, size int64, cleanup func(), ok bool) {
	cleanup = func() {}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		// The path/url sources read an arbitrary local file or fetch an arbitrary
		// URL server-side (SSRF); they are disabled unless explicitly enabled.
		if !s.allowLocalSources {
			writeJSONError(w, http.StatusForbidden,
				"local path/url sources are disabled; upload the dump as a file (or start the server with --allow-local-sources)")
			return "", "", 0, cleanup, false
		}
		var body struct {
			Path string `json:"path"`
			URL  string `json:"url"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid request body")
			return "", "", 0, cleanup, false
		}
		switch {
		case strings.TrimSpace(body.Path) != "":
			return s.resolvePathSource(w, strings.TrimSpace(body.Path))
		case strings.TrimSpace(body.URL) != "":
			return s.resolveURLSource(w, strings.TrimSpace(body.URL))
		default:
			writeJSONError(w, http.StatusBadRequest, `request must include a "path" or "url"`)
			return "", "", 0, cleanup, false
		}
	}
	return s.resolveUploadSource(w, r)
}

// resolveUploadSource spools a multipart file upload to a temp file.
func (s *Server) resolveUploadSource(w http.ResponseWriter, r *http.Request) (string, string, int64, func(), bool) {
	noop := func() {}
	r.Body = http.MaxBytesReader(w, r.Body, s.maxUpload)
	file, header, err := formFile(r, "file", "heapdump")
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "upload exceeds the "+humanizeBytes(s.maxUpload)+" limit")
			return "", "", 0, noop, false
		}
		writeJSONError(w, http.StatusBadRequest, `no heap dump file in upload (expected field "file")`)
		return "", "", 0, noop, false
	}
	defer file.Close()

	path, cleanup, err := spoolToTemp(s.tempDir, file)
	if err != nil {
		s.logger.Printf("api analyze: spool: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "could not buffer upload")
		return "", "", 0, noop, false
	}
	return path, header.Filename, header.Size, cleanup, true
}

// resolvePathSource uses a heap dump already present on the server's disk. No
// copy is made; the dump is read in place.
func (s *Server) resolvePathSource(w http.ResponseWriter, p string) (string, string, int64, func(), bool) {
	noop := func() {}
	info, err := os.Stat(p)
	if err != nil || info.IsDir() {
		writeJSONError(w, http.StatusBadRequest, "no readable file at that path")
		return "", "", 0, noop, false
	}
	if info.Size() > s.maxUpload {
		writeJSONError(w, http.StatusRequestEntityTooLarge, "file exceeds the "+humanizeBytes(s.maxUpload)+" limit")
		return "", "", 0, noop, false
	}
	return p, filepath.Base(p), info.Size(), noop, true
}

// resolveURLSource fetches a remote heap dump to a temp file, bounded by the
// fetch timeout and the upload-size cap.
func (s *Server) resolveURLSource(w http.ResponseWriter, raw string) (string, string, int64, func(), bool) {
	noop := func() {}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		writeJSONError(w, http.StatusBadRequest, "url must be a valid http(s) URL")
		return "", "", 0, noop, false
	}

	client := &http.Client{Timeout: urlFetchTimeout}
	resp, err := client.Get(raw)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "could not fetch the URL: "+err.Error())
		return "", "", 0, noop, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		writeJSONError(w, http.StatusBadGateway, "fetch returned HTTP "+resp.Status)
		return "", "", 0, noop, false
	}

	capped := http.MaxBytesReader(w, io.NopCloser(resp.Body), s.maxUpload)
	path, cleanup, err := spoolToTemp(s.tempDir, capped)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "download exceeds the "+humanizeBytes(s.maxUpload)+" limit")
			return "", "", 0, noop, false
		}
		writeJSONError(w, http.StatusBadGateway, "could not download the URL")
		return "", "", 0, noop, false
	}

	name := filepath.Base(u.Path)
	if name == "" || name == "." || name == "/" {
		name = "remote.hprof"
	}
	return path, name, fileSize(path), cleanup, true
}

// fileSize returns the size of the file at path, or 0 if it can't be stat'd.
func fileSize(path string) int64 {
	if info, err := os.Stat(path); err == nil {
		return info.Size()
	}
	return 0
}

func (s *Server) handleAPIReport(w http.ResponseWriter, r *http.Request) {
	if b, ok := s.lookup(w, r); ok {
		writeJSON(w, http.StatusOK, b.Report)
	}
}

func (s *Server) handleAPISummary(w http.ResponseWriter, r *http.Request) {
	if b, ok := s.lookup(w, r); ok {
		writeJSON(w, http.StatusOK, b.Report.Summary)
	}
}

func (s *Server) handleAPIHistogram(w http.ResponseWriter, r *http.Request) {
	if b, ok := s.lookup(w, r); ok {
		writeJSON(w, http.StatusOK, b.Report.Histogram)
	}
}

func (s *Server) handleAPIDominators(w http.ResponseWriter, r *http.Request) {
	if b, ok := s.lookup(w, r); ok {
		writeJSON(w, http.StatusOK, b.Report.Dominators)
	}
}

// handleAPIDominatorTree returns the top-level dominator-tree nodes (the heap's
// largest retainers). The dominator tree is built on first request (see Bundle).
func (s *Server) handleAPIDominatorTree(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdvanced(w) {
		return
	}
	b, ok := s.lookup(w, r)
	if !ok {
		return
	}
	dt, ok := s.dominators(w, b)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, api.BuildDominatorRoots(dt))
}

// handleAPIDominatorChildren returns the dominator-tree children of an object for
// lazy expansion.
func (s *Server) handleAPIDominatorChildren(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdvanced(w) {
		return
	}
	b, ok := s.lookup(w, r)
	if !ok {
		return
	}
	dt, ok := s.dominators(w, b)
	if !ok {
		return
	}
	nodes, found := api.BuildDominatorChildren(dt, r.PathValue("nodeId"))
	if !found {
		writeJSONError(w, http.StatusNotFound, "unknown object id")
		return
	}
	writeJSON(w, http.StatusOK, nodes)
}

func (s *Server) handleAPILeaks(w http.ResponseWriter, r *http.Request) {
	if b, ok := s.lookup(w, r); ok {
		writeJSON(w, http.StatusOK, b.Report.LeakSuspects)
	}
}

func (s *Server) handleAPILeakDetail(w http.ResponseWriter, r *http.Request) {
	b, ok := s.lookup(w, r)
	if !ok {
		return
	}
	leaks, err := b.LeakDetails()
	if err != nil {
		s.writeViewBuildError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, leaks)
}

// handleAPIInspect serves the Object Inspector view for a class (?class=) or a
// specific instance (?hash=0x…).
func (s *Server) handleAPIInspect(w http.ResponseWriter, r *http.Request) {
	b, ok := s.lookup(w, r)
	if !ok {
		return
	}
	className := r.URL.Query().Get("class")
	hash := r.URL.Query().Get("hash")
	if className == "" && hash == "" {
		writeJSONError(w, http.StatusBadRequest, "inspect requires a class or hash query parameter")
		return
	}
	g, ok := s.graph(w, b)
	if !ok {
		return
	}
	data, found := api.BuildInspectorData(g, className, hash)
	if !found {
		writeJSONError(w, http.StatusNotFound, "no matching object or class in this report")
		return
	}
	writeJSON(w, http.StatusOK, data)
}

// handleAPIInspectChildren returns the incoming/outgoing neighbours of an object
// for lazy reference-tree expansion.
func (s *Server) handleAPIInspectChildren(w http.ResponseWriter, r *http.Request) {
	b, ok := s.lookup(w, r)
	if !ok {
		return
	}
	g, ok := s.graph(w, b)
	if !ok {
		return
	}
	nodes, found := api.BuildRefChildren(g, r.PathValue("parentId"), r.PathValue("direction"))
	if !found {
		writeJSONError(w, http.StatusNotFound, "unknown object id or direction")
		return
	}
	writeJSON(w, http.StatusOK, nodes)
}

// handleAPIOQL runs an OQL query against the report and returns a result table.
// A query syntax error is a 400 with the message; the body is {"query": "..."}.
func (s *Server) handleAPIOQL(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdvanced(w) {
		return
	}
	b, ok := s.lookup(w, r)
	if !ok {
		return
	}
	var body struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	g, ok := s.graph(w, b)
	if !ok {
		return
	}
	dt, ok := s.dominators(w, b)
	if !ok {
		return
	}
	result, err := api.RunOQL(g, dt, body.Query)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleAPIWasted(w http.ResponseWriter, r *http.Request) {
	if b, ok := s.lookup(w, r); ok {
		writeJSON(w, http.StatusOK, b.Wasted)
	}
}

// lookup resolves the {id} path value to a stored bundle, writing a 404 if it is
// unknown or expired.
func (s *Server) lookup(w http.ResponseWriter, r *http.Request) (*api.Bundle, bool) {
	id := r.PathValue("id")
	b, ok := s.store.Get(id)
	if !ok {
		writeJSONError(w, http.StatusNotFound, "report not found (it may have expired)")
		return nil, false
	}
	return b, true
}

// requireAdvanced gates the opt-in Dominator Tree and OQL endpoints. When the
// feature is disabled it writes a 404 (the report's Features flags already tell
// the UI to hide these views) and returns false.
func (s *Server) requireAdvanced(w http.ResponseWriter) bool {
	if !s.enableAdvanced {
		writeJSONError(w, http.StatusNotFound,
			"the Dominator Tree and OQL features are disabled on this server "+
				"(start it with --enable-advanced-analysis or HEAPBUDDY_ENABLE_ADVANCED_ANALYSIS=1)")
		return false
	}
	return true
}

// graph lazily builds (and caches) the bundle's reference graph, writing an error
// response and returning ok=false on failure. The first call re-parses the dump,
// so this can be slow for a large heap — that cost is paid only when an
// interactive view is actually opened.
func (s *Server) graph(w http.ResponseWriter, b *api.Bundle) (*analysis.ReferenceGraph, bool) {
	g, err := b.Graph()
	if err != nil {
		s.writeViewBuildError(w, err)
		return nil, false
	}
	return g, true
}

// dominators lazily builds (and caches) the bundle's dominator tree, writing an
// error response and returning ok=false on failure.
func (s *Server) dominators(w http.ResponseWriter, b *api.Bundle) (*analysis.DominatorTree, bool) {
	dt, err := b.Dominators()
	if err != nil {
		s.writeViewBuildError(w, err)
		return nil, false
	}
	return dt, true
}

// acquireAnalyzeSlot reserves a slot in the analyze semaphore without blocking,
// returning a release func, or api.ErrServerBusy if all slots are taken. It
// backs both POST /api/analyze and (via Bundle.SetBuildGate) the lazy
// interactive-view builds — without the latter, opening N interactive views
// could start N concurrent full re-parses and bypass --max-concurrent.
func (s *Server) acquireAnalyzeSlot() (func(), error) {
	select {
	case s.analyzeSem <- struct{}{}:
		return func() { <-s.analyzeSem }, nil
	default:
		return nil, api.ErrServerBusy
	}
}

// writeViewBuildError reports a failed lazy build of an interactive view. A
// saturated server maps to a retryable 429; otherwise the usual cause is the
// retained dump file having gone away (e.g. a user-supplied path that was
// moved), so it maps to a 422.
func (s *Server) writeViewBuildError(w http.ResponseWriter, err error) {
	if errors.Is(err, api.ErrServerBusy) {
		writeBusyError(w, "the server is busy analyzing another dump; please retry this view shortly")
		return
	}
	s.logger.Printf("api: building interactive view: %v", err)
	writeJSONError(w, http.StatusUnprocessableEntity, "could not build this view from the dump: "+err.Error())
}

// writeBusyError sheds a request with 429 and a Retry-After hint.
func writeBusyError(w http.ResponseWriter, msg string) {
	w.Header().Set("Retry-After", "5")
	writeJSONError(w, http.StatusTooManyRequests, msg)
}

// formFile returns the first present multipart file field from names. A size-cap
// (MaxBytesError) is returned immediately so the caller can map it to a 413.
func formFile(r *http.Request, names ...string) (multipart.File, *multipart.FileHeader, error) {
	var lastErr error
	for _, name := range names {
		f, h, err := r.FormFile(name)
		if err == nil {
			return f, h, nil
		}
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return nil, nil, err
		}
		lastErr = err
	}
	return nil, nil, lastErr
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// corsMiddleware reflects localhost origins so local dev across ports works,
// and answers CORS preflight requests. Non-localhost origins are left untouched.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); isLocalhostOrigin(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isLocalhostOrigin(origin string) bool {
	return strings.HasPrefix(origin, "http://localhost:") ||
		strings.HasPrefix(origin, "http://127.0.0.1:") ||
		origin == "http://localhost" ||
		origin == "http://127.0.0.1"
}
