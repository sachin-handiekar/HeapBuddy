package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"heapbuddy/internal/parser"
	"heapbuddy/internal/report"

	"github.com/spf13/cobra"
)

var (
	topN        int
	showObjects bool
	showStrings bool
	showArrays  bool
)

func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func formatCount(count float64) string {
	if count < 1000 {
		return fmt.Sprintf("%.0f", count)
	}
	if count < 1000000 {
		return fmt.Sprintf("%.2f k", count/1000)
	}
	return fmt.Sprintf("%.2f M", count/1000000)
}

var generateHTML bool

var analyzeCmd = &cobra.Command{
	Use:   "analyze [heap-dump-file]",
	Short: "Analyze a heap dump file",
	Long: `Analyze a Java heap dump file and display detailed statistics about memory usage,
object allocation, and potential memory leaks.

Example:
  heapmagic analyze heap.hprof
  heapmagic analyze --top 20 heap.hprof
  heapmagic analyze --objects --strings heap.hprof`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		filename := args[0]
		if _, err := os.Stat(filename); os.IsNotExist(err) {
			fmt.Printf("Error: file %s does not exist\n", filename)
			os.Exit(1)
		}

		verbose, _ := cmd.Flags().GetBool("verbose")
		p, err := parser.NewParser(filename)
		if err != nil {
			fmt.Printf("Error creating parser: %v\n", err)
			os.Exit(1)
		}
		p.SetDebug(verbose)

		stats, err := p.Parse()
		if err != nil {
			fmt.Printf("Error parsing heap dump: %v\n", err)
			os.Exit(1)
		}

		// Print summary
		fmt.Printf("\nHeap Summary for %s:\n", filepath.Base(filename))
		fmt.Printf("=== General Information ===\n")
		fmt.Printf("Used Heap Size: %s\n", formatBytes(uint64(stats.TotalBytes)))
		fmt.Printf("Creation Date: %s\n", stats.CreationTime.Format("2006-01-02 15:04:05"))
		fmt.Printf("OS: %s\n", map[bool]string{true: "64 Bit", false: "32 Bit"}[stats.Is64Bit])

		fmt.Printf("\n=== Object Statistics ===\n")
		fmt.Printf("Total Objects: %s\n", formatCount(float64(stats.ObjectCount)))
		fmt.Printf("Total Classes: %s\n", formatCount(float64(len(stats.ClassStats))))
		fmt.Printf("ClassLoader Count: %d\n", stats.ClassLoaderCount)
		fmt.Printf("GC Root Count: %s\n", formatCount(float64(stats.GCRootCount)))

		if stats.HeapSummary != nil {
			fmt.Printf("\n=== Memory Usage ===\n")
			fmt.Printf("Total Live Bytes: %s\n", formatBytes(stats.HeapSummary.TotalLiveBytes))
			fmt.Printf("Total Live Instances: %d\n", stats.HeapSummary.TotalLiveInstances)
			fmt.Printf("Total Bytes Allocated: %s\n", formatBytes(stats.HeapSummary.TotalBytesAllocated))
			fmt.Printf("Total Instances Allocated: %d\n", stats.HeapSummary.TotalInstancesAllocated)
		}

		if generateHTML {
			if err := report.GenerateHTML(stats, args[0]); err != nil {
				fmt.Printf("Error generating HTML report: %v\n", err)
			} else {
				fmt.Printf("\nHTML report generated: %s.report.html\n", args[0])
			}
		}

		// Sort classes by instance count
		type classEntry struct {
			className string
			info      struct {
				ClassId           uint64
				SuperClassId      uint64
				ClassName         string
				InstanceCount     uint32
				InstanceSize      int64
				TotalShallowSize  int64
				TotalRetainedSize int64
				ArrayBytes        int64
			}
		}
		var classes []classEntry
		for classId, info := range stats.ClassStats {
			classes = append(classes, classEntry{
				className: info.ClassName,
				info: struct {
					ClassId           uint64
					SuperClassId      uint64
					ClassName         string
					InstanceCount     uint32
					InstanceSize      int64
					TotalShallowSize  int64
					TotalRetainedSize int64
					ArrayBytes        int64
				}{
					ClassId:           classId,
					SuperClassId:      info.SuperClassId,
					ClassName:         info.ClassName,
					InstanceCount:     info.InstanceCount,
					InstanceSize:      info.InstanceSize,
					TotalShallowSize:  info.TotalShallowSize,
					TotalRetainedSize: info.TotalRetainedSize,
					ArrayBytes:        info.ArrayBytes,
				},
			})
		}

		sort.Slice(classes, func(i, j int) bool {
			return classes[i].info.InstanceCount > classes[j].info.InstanceCount
		})

		// Print top classes by instance count
		fmt.Printf("\nTop %d Classes by Instance Count:\n", topN)
		for i := 0; i < topN && i < len(classes); i++ {
			info := classes[i].info
			fmt.Printf("%d. %s: %d instances, %s bytes\n", i+1, info.ClassName, info.InstanceCount, formatBytes(uint64(info.InstanceSize+info.ArrayBytes)))
		}

		// Sort classes by total size
		sort.Slice(classes, func(i, j int) bool {
			return (uint64(classes[i].info.InstanceSize + classes[i].info.ArrayBytes)) > (uint64(classes[j].info.InstanceSize + classes[j].info.ArrayBytes))
		})

		// Print top classes by size
		fmt.Printf("\nTop %d Classes by Total Size:\n", topN)
		for i := 0; i < topN && i < len(classes); i++ {
			info := classes[i].info
			fmt.Printf("%d. %s: %s bytes (%d instances)\n", i+1, info.ClassName, formatBytes(uint64(info.InstanceSize+info.ArrayBytes)), info.InstanceCount)
		}
	},
}

func init() {
	rootCmd.AddCommand(analyzeCmd)
	analyzeCmd.Flags().IntVarP(&topN, "top", "t", 10, "Show top N classes by instance count")
	analyzeCmd.Flags().BoolVarP(&showObjects, "objects", "o", false, "Show detailed object information")
	analyzeCmd.Flags().BoolVarP(&showStrings, "strings", "s", false, "Show string contents")
	analyzeCmd.Flags().BoolVarP(&showArrays, "arrays", "a", false, "Show array contents")
	analyzeCmd.Flags().BoolVarP(&generateHTML, "html", "", true, "Generate HTML report")
}
