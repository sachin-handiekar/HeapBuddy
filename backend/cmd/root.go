package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Build information. These are overridden at build time via -ldflags, e.g.:
//
//	go build -ldflags "-X github.com/sachin-handiekar/HeapBuddy/cmd.version=v1.0.0 -X github.com/sachin-handiekar/HeapBuddy/cmd.commit=abc123 -X github.com/sachin-handiekar/HeapBuddy/cmd.date=2026-01-01"
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var rootCmd = &cobra.Command{
	Use:   "heapbuddy",
	Short: "HeapBuddy - JVM Heap Dump Analyzer",
	Long: `HeapBuddy is a fast JVM heap dump analyzer that helps you understand memory usage,
find memory leaks, and analyze object allocation patterns in your Java applications.

For example:
  heapbuddy analyze heap.hprof`,
	Version:       fmt.Sprintf("%s (commit %s, built %s)", version, commit, date),
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the root command and exits with a non-zero status on error so
// that failures are detectable in scripts and CI pipelines.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.CompletionOptions.DisableDefaultCmd = true
}
