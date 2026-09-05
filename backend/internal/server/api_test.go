package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/sachin-handiekar/HeapBuddy/internal/api"
)

// TestAPIAnalyzeAndFetch exercises the full Phase 1 JSON flow: upload a dump via
// POST /api/analyze, then read it back through each report endpoint.
func TestAPIAnalyzeAndFetch(t *testing.T) {
	if _, err := os.Stat(sampleDump); err != nil {
		t.Skipf("sample dump not available: %v", err)
	}

	// Enable the opt-in Dominator Tree / OQL views; this test exercises them.
	srv := New(WithAdvancedAnalysis(true))
	handler := srv.Handler()

	// 1. Analyze -> { id }
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newAPIUploadRequest(t, sampleDump))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/analyze = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var result api.AnalyzeResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode analyze result: %v", err)
	}
	if result.ID == "" {
		t.Fatal("analyze returned empty id")
	}

	// 2. GET /api/report/{id}
	var report api.HeapReport
	getJSON(t, handler, "/api/report/"+result.ID, &report)
	if report.Summary.ID != result.ID {
		t.Errorf("report summary id = %q, want %q", report.Summary.ID, result.ID)
	}
	if report.Summary.TotalObjects <= 0 {
		t.Errorf("expected positive totalObjects, got %d", report.Summary.TotalObjects)
	}
	if len(report.Histogram) == 0 {
		t.Error("expected a non-empty histogram")
	}
	if len(report.Wasted) != 4 {
		t.Errorf("expected 4 wasted categories, got %d", len(report.Wasted))
	}

	// 3. Granular endpoints
	var summary api.HeapReportSummary
	getJSON(t, handler, "/api/reports/"+result.ID+"/summary", &summary)
	if summary.ID != result.ID {
		t.Errorf("summary id = %q, want %q", summary.ID, result.ID)
	}

	var histogram []api.HistogramEntry
	getJSON(t, handler, "/api/reports/"+result.ID+"/histogram", &histogram)
	if len(histogram) == 0 {
		t.Error("histogram endpoint returned no rows")
	}

	var dominators []api.DominatorEntry
	getJSON(t, handler, "/api/reports/"+result.ID+"/dominators", &dominators)

	var wasted api.WastedDetail
	getJSON(t, handler, "/api/reports/"+result.ID+"/wasted", &wasted)

	// 4. Leak suspects (Phase 2): list + detail, and feature flags on the report.
	var leaks []api.LeakSuspect
	getJSON(t, handler, "/api/reports/"+result.ID+"/leaks", &leaks)

	var leakDetail []api.LeakSuspectDetail
	getJSON(t, handler, "/api/reports/"+result.ID+"/leaks/detail", &leakDetail)
	if len(leakDetail) != len(leaks) {
		t.Errorf("leak detail rows = %d, want %d (one per suspect)", len(leakDetail), len(leaks))
	}
	if len(leakDetail) != report.Summary.LeakSuspects {
		t.Errorf("summary.leakSuspects = %d, want %d", report.Summary.LeakSuspects, len(leakDetail))
	}
	for _, d := range leakDetail {
		if d.RootChain == nil {
			t.Errorf("leak %q rootChain should be non-nil (empty, not null) JSON", d.ID)
		}
	}
	// The reverse-reference graph should attach a concrete instance identity and a
	// retention chain to at least one suspect (regression guard for the graph
	// wiring; exact chains are dump-dependent).
	if len(leakDetail) > 0 {
		var withChain, withIdentity int
		for _, d := range leakDetail {
			if len(d.RootChain) > 0 {
				withChain++
			}
			if d.IdentityHash != "" {
				withIdentity++
			}
		}
		if withIdentity == 0 {
			t.Error("expected at least one leak suspect to have a representative instance identityHash")
		}
		if withChain == 0 {
			t.Error("expected at least one leak suspect to have a non-empty retention chain")
		}
	}
	if !report.Features.Leaks {
		t.Error("features.leaks should be true")
	}
	if !report.Features.ObjectInspector {
		t.Error("features.objectInspector should be true (reference graph is built)")
	}
	if !report.Features.DominatorTree {
		t.Error("features.dominatorTree should be true (dominator tree is built)")
	}
	if !report.Features.OQL {
		t.Error("features.oql should be true (the OQL engine is available)")
	}

	// OQL: COUNT(*) over the largest non-array class returns a single count row;
	// a bad query is a 400. (The histogram now includes array classes like
	// byte[], whose names aren't valid in an OQL FROM clause.)
	var oqlClass string
	for _, h := range report.Histogram {
		if !strings.HasSuffix(h.ClassName, "[]") && !strings.HasPrefix(h.ClassName, "[") {
			oqlClass = h.ClassName
			break
		}
	}
	if oqlClass != "" {
		var oqlRes api.OqlResult
		postJSON(t, handler, "/api/report/"+result.ID+"/oql",
			`{"query":"SELECT COUNT(*) FROM `+oqlClass+`"}`, &oqlRes)
		if oqlRes.Total != 1 || len(oqlRes.Rows) != 1 {
			t.Errorf("OQL COUNT(*) = %+v, want one count row", oqlRes.Rows)
		}
	}
	badOQL := httptest.NewRecorder()
	handler.ServeHTTP(badOQL, postReq("/api/report/"+result.ID+"/oql", `{"query":"not valid oql"}`))
	if badOQL.Code != http.StatusBadRequest {
		t.Errorf("invalid OQL = %d, want 400", badOQL.Code)
	}

	// Dominator tree: roots have true retained sizes, and a root with children
	// expands. retained >= shallow for every node.
	var domRoots []api.DomNode
	getJSON(t, handler, "/api/reports/"+result.ID+"/dominator-tree", &domRoots)
	if len(domRoots) == 0 {
		t.Error("dominator-tree returned no roots")
	}
	for _, n := range domRoots {
		if n.RetainedBytes < n.ShallowBytes {
			t.Errorf("dom node %s: retained %d < shallow %d", n.ID, n.RetainedBytes, n.ShallowBytes)
		}
	}
	for _, n := range domRoots {
		if n.ChildCount > 0 {
			var kids []api.DomNode
			getJSON(t, handler, "/api/reports/"+result.ID+"/dominator-tree/"+n.ID, &kids)
			if len(kids) == 0 {
				t.Errorf("dom node %s reports childCount %d but returned no children", n.ID, n.ChildCount)
			}
			break
		}
	}

	// 5. Object Inspector: inspect a class, then lazily expand a reference node.
	// Use the same largest non-array class (the inspector is backed by the
	// instance reference graph, which doesn't model primitive arrays as classes).
	if oqlClass != "" {
		class := oqlClass
		var inspect api.InspectorData
		getJSON(t, handler, "/api/reports/"+result.ID+"/inspect?class="+url.QueryEscape(class), &inspect)
		if inspect.IdentityHash == "" {
			t.Errorf("inspect(%q) returned no identityHash", class)
		}
		if inspect.ClassName == "" {
			t.Errorf("inspect(%q) returned no className", class)
		}
		// Expand the first available reference node's children, if any.
		var node *api.InspectorRefNode
		var dir string
		if len(inspect.Outgoing) > 0 {
			node, dir = &inspect.Outgoing[0], "outgoing"
		} else if len(inspect.Incoming) > 0 {
			node, dir = &inspect.Incoming[0], "incoming"
		}
		if node != nil {
			var kids []api.InspectorRefNode
			getJSON(t, handler, "/api/reports/"+result.ID+"/inspect/"+node.ID+"/"+dir, &kids)
		}
	}
}

// TestAPIAdvancedAnalysisDisabledByDefault verifies the Dominator Tree and OQL
// views are off by default: the report advertises them as unavailable and their
// endpoints 404, while the always-on views (Object Inspector, leaks) still work.
func TestAPIAdvancedAnalysisDisabledByDefault(t *testing.T) {
	if _, err := os.Stat(sampleDump); err != nil {
		t.Skipf("sample dump not available: %v", err)
	}

	handler := New().Handler() // advanced analysis off by default

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newAPIUploadRequest(t, sampleDump))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/analyze = %d, want 200", rec.Code)
	}
	var result api.AnalyzeResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode analyze result: %v", err)
	}

	var report api.HeapReport
	getJSON(t, handler, "/api/report/"+result.ID, &report)
	if report.Features.DominatorTree {
		t.Error("features.dominatorTree should be false when advanced analysis is off")
	}
	if report.Features.OQL {
		t.Error("features.oql should be false when advanced analysis is off")
	}
	if !report.Features.ObjectInspector || !report.Features.Leaks {
		t.Error("Object Inspector and Leaks should remain available")
	}

	// The gated endpoints return 404.
	for _, path := range []string{
		"/api/reports/" + result.ID + "/dominator-tree",
		"/api/reports/" + result.ID + "/dominator-tree/0x1",
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404 when disabled", path, rec.Code)
		}
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, postReq("/api/report/"+result.ID+"/oql", `{"query":"SELECT * FROM java.lang.String"}`))
	if rec.Code != http.StatusNotFound {
		t.Errorf("POST oql = %d, want 404 when disabled", rec.Code)
	}

	// The Object Inspector still works without the advanced flag.
	var inspect api.InspectorData
	getJSON(t, handler, "/api/reports/"+result.ID+"/inspect?class="+url.QueryEscape("java.lang.String"), &inspect)
}

// TestAPIAnalyzePathSource analyzes a dump already on disk via JSON {"path": …}.
func TestAPIAnalyzePathSource(t *testing.T) {
	if _, err := os.Stat(sampleDump); err != nil {
		t.Skipf("sample dump not available: %v", err)
	}
	abs, err := filepath.Abs(sampleDump)
	if err != nil {
		t.Fatal(err)
	}

	handler := New(WithLocalSources(true)).Handler()
	var result api.AnalyzeResult
	postJSON(t, handler, "/api/analyze", `{"path":`+strconv.Quote(abs)+`}`, &result)
	if result.ID == "" {
		t.Fatal("path-source analyze returned empty id")
	}
	var summary api.HeapReportSummary
	getJSON(t, handler, "/api/reports/"+result.ID+"/summary", &summary)
	if summary.TotalObjects <= 0 {
		t.Errorf("expected positive totalObjects from path source, got %d", summary.TotalObjects)
	}
}

func TestAPIAnalyzeBadSources(t *testing.T) {
	handler := New(WithLocalSources(true)).Handler()
	for _, tc := range []struct {
		body string
		want int
	}{
		{`{"path":"/definitely/not/here.hprof"}`, http.StatusBadRequest},
		{`{"url":"ftp://example/x"}`, http.StatusBadRequest},
		{`{}`, http.StatusBadRequest},
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, postReq("/api/analyze", tc.body))
		if rec.Code != tc.want {
			t.Errorf("analyze %s = %d, want %d", tc.body, rec.Code, tc.want)
		}
	}
}

// Local path/url sources are disabled by default: such a request is rejected
// with 403 before any file is read or URL fetched.
func TestAPIAnalyzeLocalSourcesDisabledByDefault(t *testing.T) {
	handler := New().Handler()
	for _, body := range []string{`{"path":"/etc/passwd"}`, `{"url":"http://169.254.169.254/"}`} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, postReq("/api/analyze", body))
		if rec.Code != http.StatusForbidden {
			t.Errorf("analyze %s with sources disabled = %d, want 403", body, rec.Code)
		}
	}
}

// When the analyze semaphore is saturated, a new analyze request is shed with
// 429 before any upload is spooled or analysis run.
func TestAnalyzeConcurrencyLimitReturns429(t *testing.T) {
	s := New(WithMaxConcurrentAnalyses(1))
	s.analyzeSem <- struct{}{} // occupy the only slot

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/analyze", nil))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("saturated analyze = %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("429 response should set a Retry-After header")
	}
}

// A lazy interactive-view build that must re-parse the dump (graph cache off)
// respects the analyze concurrency cap: while the semaphore is saturated the
// request is shed with 429 instead of starting another full parse. Once the
// graph is cached in memory the view no longer needs a slot and is served even
// under saturation.
func TestLazyViewBuildRespectsConcurrencyLimit(t *testing.T) {
	if _, err := os.Stat(sampleDump); err != nil {
		t.Skipf("sample dump not available: %v", err)
	}
	s := New(WithMaxConcurrentAnalyses(1), WithGraphCache(false))
	handler := s.Handler()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newAPIUploadRequest(t, sampleDump))
	if rec.Code != http.StatusOK {
		t.Fatalf("analyze = %d; body: %s", rec.Code, rec.Body.String())
	}
	var result api.AnalyzeResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode analyze result: %v", err)
	}
	inspectURL := "/api/reports/" + result.ID + "/inspect?class=" + url.QueryEscape("java.lang.String")

	// Saturated: the first interactive request needs a slot to re-parse.
	s.analyzeSem <- struct{}{}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, inspectURL, nil))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("saturated lazy build = %d, want 429; body: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("429 response should set a Retry-After header")
	}

	// Slot freed: the build proceeds and the graph is cached on the bundle.
	<-s.analyzeSem
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, inspectURL, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("lazy build with free slot = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	// Cached: saturation no longer blocks the view.
	s.analyzeSem <- struct{}{}
	defer func() { <-s.analyzeSem }()
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, inspectURL, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("cached view under saturation = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
}

// With the graph cache (default on), interactive views are served from the
// serialized graph written at analysis time: they need neither the dump file
// nor an analyze-semaphore slot. This is the Phase 2b contract — no re-parse.
func TestGraphCacheServesViewsWithoutReparse(t *testing.T) {
	if _, err := os.Stat(sampleDump); err != nil {
		t.Skipf("sample dump not available: %v", err)
	}
	// Not t.TempDir(): the loaded graph keeps the cache file mapped for the
	// bundle's lifetime, and on Windows a mapped file cannot be deleted —
	// production defers that to the mapping finalizer or the boot sweep, so
	// this test's cleanup must be best-effort too.
	tempDir, err := os.MkdirTemp("", "hb-graphcache-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)
	s := New(WithMaxConcurrentAnalyses(1), WithTempDir(tempDir))
	handler := s.Handler()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newAPIUploadRequest(t, sampleDump))
	if rec.Code != http.StatusOK {
		t.Fatalf("analyze = %d; body: %s", rec.Code, rec.Body.String())
	}
	var result api.AnalyzeResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode analyze result: %v", err)
	}

	// The cache file exists and the spooled dump can be removed — the views
	// must not need it anymore.
	caches, _ := filepath.Glob(filepath.Join(tempDir, "heapbuddy-*.hbgraph"))
	if len(caches) != 1 {
		t.Fatalf("graph cache files in temp dir = %d, want 1", len(caches))
	}
	dumps, _ := filepath.Glob(filepath.Join(tempDir, "heapbuddy-*.hprof"))
	for _, d := range dumps {
		if err := os.Remove(d); err != nil {
			t.Fatalf("removing spooled dump: %v", err)
		}
	}

	// Saturate the analyze semaphore: the cache load needs no slot.
	s.analyzeSem <- struct{}{}
	defer func() { <-s.analyzeSem }()

	rec = httptest.NewRecorder()
	inspectURL := "/api/reports/" + result.ID + "/inspect?class=" + url.QueryEscape("java.lang.String")
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, inspectURL, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("inspect from graph cache = %d, want 200 (no dump, saturated semaphore); body: %s",
			rec.Code, rec.Body.String())
	}
}

// The pprof endpoints are opt-in: absent by default (the route falls through to
// the SPA/503 root handler), served when enabled with WithPprof.
func TestPprofOptIn(t *testing.T) {
	rec := httptest.NewRecorder()
	New().Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil))
	if strings.Contains(rec.Body.String(), "Types of profiles available") {
		t.Error("pprof index served with pprof disabled")
	}

	rec = httptest.NewRecorder()
	New(WithPprof(true)).Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("pprof enabled = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Types of profiles available") {
		t.Error("enabled /debug/pprof/ did not serve the pprof index")
	}
}

func TestAPIReportNotFound(t *testing.T) {
	rec := httptest.NewRecorder()
	New().Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/report/rpt_missing", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown report = %d, want 404", rec.Code)
	}
}

func TestAPIAnalyzeMissingFile(t *testing.T) {
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/analyze", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	rec := httptest.NewRecorder()
	New().Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing file = %d, want 400", rec.Code)
	}
}

func TestAPICORSPreflight(t *testing.T) {
	req := httptest.NewRequest(http.MethodOptions, "/api/analyze", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	rec := httptest.NewRecorder()
	New().Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight = %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Errorf("ACAO = %q, want reflected localhost origin", got)
	}
}

func postReq(path, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func postJSON(t *testing.T, h http.Handler, path, body string, dst any) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, postReq(path, body))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST %s = %d, want 200; body: %s", path, rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}

func getJSON(t *testing.T, h http.Handler, path string, dst any) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200; body: %s", path, rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}

// newAPIUploadRequest builds a multipart POST /api/analyze request carrying path
// as the "file" field (the name the frontend uses).
func newAPIUploadRequest(t *testing.T, path string) *http.Request {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open sample: %v", err)
	}
	defer f.Close()

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	part, err := mw.CreateFormFile("file", filepath.Base(path))
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := io.Copy(part, f); err != nil {
		t.Fatalf("copy sample: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/analyze", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}
