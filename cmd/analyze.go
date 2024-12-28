package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"heapbuddy/internal/parser"
	"heapbuddy/internal/report"
	"heapbuddy/internal/types"
)

var (
	topN        int
	showObjects bool
	showStrings bool
	showArrays  bool
)

func formatCount(count float64) string {
	if count >= 1e9 {
		return fmt.Sprintf("%.1fB", count/1e9)
	} else if count >= 1e6 {
		return fmt.Sprintf("%.1fM", count/1e6)
	} else if count >= 1e3 {
		return fmt.Sprintf("%.1fK", count/1e3)
	}
	return fmt.Sprintf("%.0f", count)
}

var generateHTML bool

var analyzeCmd = &cobra.Command{
	Use:   "analyze [heap-dump-file]",
	Short: "Analyze a heap dump file",
	Long: `Analyze a heap dump file and display memory usage statistics.
	
Example:
  heapbuddy analyze heap.hprof
  heapbuddy analyze heap.hprof --top 20
  heapbuddy analyze heap.hprof --show-objects`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		filename := args[0]

		// Parse heap dump
		parser, err := parser.NewParser(filename)
		if err != nil {
			fmt.Printf("Error creating parser: %v\n", err)
			os.Exit(1)
		}
		defer parser.Close()

		fmt.Printf("Analyzing heap dump: %s\n", filename)
		stats, err := parser.Parse()
		if err != nil {
			fmt.Printf("Error parsing heap dump: %v\n", err)
			os.Exit(1)
		}

		// Sort classes by instance count
		type classEntry struct {
			className string
			info      *types.ClassInfo
		}
		var classes []classEntry
		for classId, info := range stats.ClassStats {
			if className, exists := stats.ClassNameMap[classId]; exists {
				classes = append(classes, classEntry{
					className: className,
					info:      info,
				})
			}
		}

		sort.Slice(classes, func(i, j int) bool {
			return classes[i].info.InstanceCount > classes[j].info.InstanceCount
		})

		// Generate HTML report
		if generateHTML {
			fmt.Printf("Generating HTML report...\n")
			if err := report.GenerateHTML(stats, filename); err != nil {
				fmt.Printf("Error generating HTML report: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("HTML report generated: %s\n", filename+".report.html")
			return
		}

		// Print summary
		fmt.Printf("\nHeap Summary:\n")
		fmt.Printf("  Total Objects: %s\n", formatCount(float64(stats.ObjectCount)))
		fmt.Printf("  Total Bytes: %s\n", formatCount(float64(stats.TotalBytes)))
		fmt.Printf("  String Objects: %s\n", formatCount(float64(stats.StringCount)))
		fmt.Printf("  String Bytes: %s\n", formatCount(float64(stats.StringBytes)))
		fmt.Printf("  Array Objects: %s\n", formatCount(float64(stats.ArrayCount)))
		fmt.Printf("  Array Bytes: %s\n", formatCount(float64(stats.ArrayBytes)))

		// Print class histogram
		fmt.Printf("\nClass Histogram (top %d):\n", topN)
		fmt.Printf("%-50s %10s %15s %15s\n", "Class Name", "Count", "Shallow Size", "Avg Size")
		fmt.Printf("%s\n", strings.Repeat("-", 90))

		for i, class := range classes {
			if i >= topN {
				break
			}
			avgSize := float64(0)
			if class.info.InstanceCount > 0 {
				avgSize = float64(class.info.TotalShallowSize) / float64(class.info.InstanceCount)
			}
			fmt.Printf("%-50s %10s %15s %15s\n",
				class.className,
				formatCount(float64(class.info.InstanceCount)),
				formatCount(float64(class.info.TotalShallowSize)),
				formatCount(avgSize))
		}
	},
}

func init() {
	rootCmd.AddCommand(analyzeCmd)
	analyzeCmd.Flags().IntVarP(&topN, "top", "n", 20, "Number of classes to show in histogram")
	analyzeCmd.Flags().BoolVar(&generateHTML, "html", true, "Generate HTML report")
	analyzeCmd.Flags().BoolVar(&showObjects, "show-objects", false, "Show object details")
	analyzeCmd.Flags().BoolVar(&showStrings, "show-strings", false, "Show string contents")
	analyzeCmd.Flags().BoolVar(&showArrays, "show-arrays", false, "Show array contents")
}
