package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

// newTestSPA builds a spaHandler over an in-memory file tree so the routing
// logic can be tested without an actual embedded frontend build.
func newTestSPA() *spaHandler {
	shell := []byte("<!doctype html><title>SHELL</title>")
	files := fstest.MapFS{
		"_shell.html":   {Data: shell},
		"assets/app.js": {Data: []byte("console.log(1)")},
	}
	return &spaHandler{files: files, fileH: http.FileServerFS(files), shell: shell}
}

func TestSPAServesAssetsShellAnd404(t *testing.T) {
	h := newTestSPA()

	do := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec
	}

	// Real asset: served with its own bytes.
	if rec := do("/assets/app.js"); rec.Code != http.StatusOK || rec.Body.String() != "console.log(1)" {
		t.Errorf("asset = %d %q, want 200 and the JS body", rec.Code, rec.Body.String())
	}
	// Missing asset (has an extension): a real 404, not the shell.
	if rec := do("/assets/missing.js"); rec.Code != http.StatusNotFound {
		t.Errorf("missing asset = %d, want 404", rec.Code)
	}
	// Root and client routes: the SPA shell.
	for _, p := range []string{"/", "/report/abc123", "/dominators"} {
		rec := do(p)
		if rec.Code != http.StatusOK || rec.Body.String() != "<!doctype html><title>SHELL</title>" {
			t.Errorf("%s = %d %q, want 200 shell", p, rec.Code, rec.Body.String())
		}
	}
}

// Without an embedded build there is no UI to serve, so "/" returns a clear 503
// pointing at the JSON API rather than a stale fallback page.
func TestServerWithoutBuildReturns503(t *testing.T) {
	s := New()
	if s.spa != nil {
		t.Skip("a real frontend build is embedded in this binary; the no-UI path is not exercised")
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("GET / without build = %d, want 503", rec.Code)
	}
}
