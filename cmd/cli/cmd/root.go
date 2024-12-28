package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "heapbuddy",
	Short: "HeapBuddy - JVM Heap Dump Analyzer",
	Long: `HeapBuddy is a fast JVM heap dump analyzer that helps you understand memory usage,
find memory leaks, and analyze object allocation patterns in your Java applications.

For example:
  heapbuddy analyze heap.hprof`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.CompletionOptions.DisableDefaultCmd = true
}
