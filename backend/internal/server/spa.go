package server

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// webuiFS holds the built single-page app. In a fresh checkout it contains only
// placeholder.html; `make build` copies the real SPA (_shell.html + assets/)
// here before compiling, so the single binary serves the UI on the same port as
// the API. "all:" is required so the underscore-prefixed _shell.html is included.
//
//go:embed all:webui
var webuiFS embed.FS

// spaHandler serves the embedded single-page app: real files (e.g. /assets/*)
// are served directly, and any other GET path falls back to the app shell so
// client-side routes like /report/{id} work on a hard refresh.
type spaHandler struct {
	files fs.FS
	fileH http.Handler
	shell []byte
}

// newSPAHandler returns a handler for the embedded SPA, or nil when no real
// build is present (only the placeholder), in which case the server keeps its
// built-in HTML upload page.
func newSPAHandler() *spaHandler {
	sub, err := fs.Sub(webuiFS, "webui")
	if err != nil {
		return nil
	}
	// A real build ships an assets/ directory; the placeholder alone does not.
	if _, err := fs.Stat(sub, "assets"); err != nil {
		return nil
	}
	shell, err := fs.ReadFile(sub, "_shell.html")
	if err != nil {
		if shell, err = fs.ReadFile(sub, "index.html"); err != nil {
			return nil
		}
	}
	return &spaHandler{files: sub, fileH: http.FileServerFS(sub), shell: shell}
}

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if p := strings.TrimPrefix(r.URL.Path, "/"); p != "" {
		if f, err := h.files.Open(p); err == nil {
			info, statErr := f.Stat()
			_ = f.Close()
			if statErr == nil && !info.IsDir() {
				h.fileH.ServeHTTP(w, r) // real asset: correct content-type, caching, ranges
				return
			}
		}
		// A path that looks like a file (has an extension) but isn't present is a
		// genuine 404 — don't mask a missing asset with the HTML shell.
		if path.Ext(p) != "" {
			http.NotFound(w, r)
			return
		}
	}
	// SPA fallback: serve the app shell for "/" and unknown client routes.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(h.shell)
}
