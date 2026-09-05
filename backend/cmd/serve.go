package cmd

import (
	"fmt"
	"net"
	rtdebug "runtime/debug"

	"github.com/spf13/cobra"

	"github.com/sachin-handiekar/HeapBuddy/internal/server"
)

var (
	serveAddr              string
	serveMaxUpload         int64
	serveTempDir           string
	serveAllowLocalSources bool
	serveMaxConcurrent     int
	serveMemLimit          int64
	serveEnableAdvanced    bool
	serveEnablePprof       bool
	serveGraphCache        bool
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the HeapBuddy web server",
	Long: `Start a self-hosted web server for uploading and analyzing heap dumps
in a browser. Open the printed URL, drop in a .hprof file, and view the report.

The server ships without authentication, so it binds to localhost by default.
Pass --addr 0.0.0.0:8080 to expose it on your network (only do this inside a
trusted network — heap dumps can contain secrets). The --max-upload flag bounds
the accepted file size.

Each flag has an environment-variable default (handy for containers); an
explicit flag always wins:
  --addr                 HEAPBUDDY_ADDR
  --max-upload           HEAPBUDDY_MAX_UPLOAD
  --temp-dir             HEAPBUDDY_TEMP_DIR
  --allow-local-sources  HEAPBUDDY_ALLOW_LOCAL_SOURCES
  --max-concurrent       HEAPBUDDY_MAX_CONCURRENT
  --mem-limit            HEAPBUDDY_MEM_LIMIT
  --enable-advanced-analysis  HEAPBUDDY_ENABLE_ADVANCED_ANALYSIS
  --enable-pprof         HEAPBUDDY_ENABLE_PPROF
  --graph-cache          HEAPBUDDY_GRAPH_CACHE

Example:
  heapbuddy serve
  heapbuddy serve --addr 0.0.0.0:9000 --max-upload 2147483648`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// A soft memory limit makes the Go runtime collect more aggressively as it
		// approaches the cap instead of letting the heap grow until the host swaps
		// and hangs. Combined with --max-concurrent (which sheds excess analyses
		// with 429), it keeps a large dump from taking the whole machine down.
		// Go also honors the GOMEMLIMIT env var; an explicit --mem-limit wins.
		if serveMemLimit > 0 {
			rtdebug.SetMemoryLimit(serveMemLimit)
			logf("Soft memory limit set to %d bytes\n", serveMemLimit)
		}

		srv := server.New(
			server.WithMaxUpload(serveMaxUpload),
			server.WithTempDir(serveTempDir),
			server.WithLocalSources(serveAllowLocalSources),
			server.WithMaxConcurrentAnalyses(serveMaxConcurrent),
			server.WithAdvancedAnalysis(serveEnableAdvanced),
			server.WithPprof(serveEnablePprof),
			server.WithGraphCache(serveGraphCache),
		)

		if host, port, err := net.SplitHostPort(serveAddr); err == nil {
			display := host
			if host == "" || host == "0.0.0.0" || host == "::" {
				display = "localhost"
			}
			logf("Open http://%s:%s in your browser\n", display, port)
			if !isLoopbackHost(host) {
				logf("WARNING: listening on %s — reachable from other hosts and unauthenticated. "+
					"Heap dumps can contain secrets; expose only inside a trusted network.\n", serveAddr)
			}
		}

		if err := srv.ListenAndServe(serveAddr); err != nil {
			return fmt.Errorf("server stopped: %w", err)
		}
		return nil
	},
}

// isLoopbackHost reports whether the listen host is confined to the local
// machine. An empty host or the unspecified address (0.0.0.0 / ::) binds every
// interface and is therefore not loopback.
func isLoopbackHost(host string) bool {
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	case "", "0.0.0.0", "::":
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

func init() {
	rootCmd.AddCommand(serveCmd)

	// Defaults come from environment variables (container-friendly); an explicit
	// flag overrides the env default because cobra uses the set value.
	serveCmd.Flags().StringVar(&serveAddr, "addr", envStr("HEAPBUDDY_ADDR", "127.0.0.1:8080"), "Address to listen on (host:port); use 0.0.0.0:8080 to expose externally")
	serveCmd.Flags().Int64Var(&serveMaxUpload, "max-upload", envInt64("HEAPBUDDY_MAX_UPLOAD", server.DefaultMaxUploadBytes), "Maximum upload size in bytes")
	serveCmd.Flags().StringVar(&serveTempDir, "temp-dir", envStr("HEAPBUDDY_TEMP_DIR", ""), "Directory for spooling uploaded dumps (default: OS temp dir)")
	serveCmd.Flags().BoolVar(&serveAllowLocalSources, "allow-local-sources", envBool("HEAPBUDDY_ALLOW_LOCAL_SOURCES", false), "Allow analyzing a server-side file path or remote URL (off by default; SSRF/local-file risk)")
	serveCmd.Flags().IntVar(&serveMaxConcurrent, "max-concurrent", envInt("HEAPBUDDY_MAX_CONCURRENT", server.DefaultMaxConcurrentAnalyses), "Maximum number of dumps analyzed concurrently")
	serveCmd.Flags().Int64Var(&serveMemLimit, "mem-limit", envInt64("HEAPBUDDY_MEM_LIMIT", 0), "Soft memory limit in bytes (0 = unset; the Go runtime GCs harder as usage nears it). Also honors GOMEMLIMIT.")
	serveCmd.Flags().BoolVar(&serveEnableAdvanced, "enable-advanced-analysis", envBool("HEAPBUDDY_ENABLE_ADVANCED_ANALYSIS", server.DefaultEnableAdvancedAnalysis), "Enable the Dominator Tree and OQL views (off by default; they build the full dominator tree, the heaviest step)")
	serveCmd.Flags().BoolVar(&serveEnablePprof, "enable-pprof", envBool("HEAPBUDDY_ENABLE_PPROF", false), "Expose Go pprof profiling endpoints under /debug/pprof/ (off by default; for diagnosing memory/CPU use)")
	serveCmd.Flags().BoolVar(&serveGraphCache, "graph-cache", envBool("HEAPBUDDY_GRAPH_CACHE", true), "Serialize the reference graph to a temp file at analysis time so interactive views memory-map it instead of re-parsing the dump (costs graph-sized disk per retained report)")
}
