package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sachin-handiekar/HeapBuddy/internal/analysis"
	"github.com/sachin-handiekar/HeapBuddy/internal/parser"
	"github.com/sachin-handiekar/HeapBuddy/internal/pipeline"
	"github.com/sachin-handiekar/HeapBuddy/internal/types"
)

var (
	showStringsDetail  bool
	analyzeStrings     bool
	analyzeCollections bool
	analyzeThreads     bool
	showThreads        bool
	findLeaks          bool
	maxChainLength     int
	minLoadFactor      float64
	debug              bool
	generateJSON       bool
	maxClasses         int
	maxDuplicates      int
)

// logf writes progress/diagnostic messages to stderr so that stdout carries
// only report output (important for --json piping and scripting).
func logf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format, args...)
}

func formatCount(count float64) string {
	switch {
	case count >= 1e9:
		return fmt.Sprintf("%.1fB", count/1e9)
	case count >= 1e6:
		return fmt.Sprintf("%.1fM", count/1e6)
	case count >= 1e3:
		return fmt.Sprintf("%.1fK", count/1e3)
	}
	return fmt.Sprintf("%.0f", count)
}

func formatBytes(count int64) string {
	switch {
	case count >= 1e9:
		return fmt.Sprintf("%.1fGB", float64(count)/1e9)
	case count >= 1e6:
		return fmt.Sprintf("%.1fMB", float64(count)/1e6)
	case count >= 1e3:
		return fmt.Sprintf("%.1fKB", float64(count)/1e3)
	}
	return fmt.Sprintf("%dB", count)
}

var analyzeCmd = &cobra.Command{
	Use:   "analyze [heap-dump-file]",
	Short: "Analyze a heap dump file",
	Long: `Analyze a heap dump file and display memory usage statistics.

Example:
  heapbuddy analyze heap.hprof
  heapbuddy analyze heap.hprof --max-classes 20
  heapbuddy analyze heap.hprof --json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filename := args[0]

		// Validate the input file before doing any work.
		fileInfo, err := os.Stat(filename)
		if err != nil {
			return fmt.Errorf("cannot access %q: %w", filename, err)
		}
		if fileInfo.IsDir() {
			return fmt.Errorf("%q is a directory, expected a .hprof file", filename)
		}

		// Parse heap dump.
		p, err := parser.NewParser(filename)
		if err != nil {
			return fmt.Errorf("creating parser: %w", err)
		}
		defer p.Close()

		if debug {
			p.SetDebug(true)
		}

		logf("Analyzing heap dump: %s (%s)\n", filename, formatBytes(fileInfo.Size()))
		logf("Parsing HPROF file...\n")
		stats, err := p.Parse()
		if err != nil {
			return fmt.Errorf("parsing heap dump: %w", err)
		}
		logf("Parse complete.\n")

		// Build HeapData and run the full JXRay-style analysis.
		heapData := pipeline.BuildHeapData(stats)
		logf("Running analysis...\n")
		analyzer := analysis.NewAnalyzer(heapData)
		fullReport := analyzer.RunFullAnalysis()
		logf("Analysis complete.\n")

		// JSON output mode: emit only the machine-readable report on stdout.
		if generateJSON {
			jsonData, err := json.MarshalIndent(fullReport, "", "  ")
			if err != nil {
				return fmt.Errorf("marshaling JSON: %w", err)
			}
			fmt.Println(string(jsonData))
			return nil
		}

		// Print heap summary.
		fmt.Printf("\n═══════════════════════════════════════════════════════════════════\n")
		fmt.Printf("  HEAP SUMMARY\n")
		fmt.Printf("═══════════════════════════════════════════════════════════════════\n")
		fmt.Printf("  Total Objects:    %s\n", formatCount(float64(stats.ObjectCount)))
		fmt.Printf("  Total Classes:    %s\n", formatCount(float64(stats.ClassCount)))
		fmt.Printf("  Total Heap Size:  %s\n", formatBytes(stats.TotalBytes))
		fmt.Printf("  Instance Objects: %s (%s)\n", formatCount(float64(stats.ObjectCount-stats.ArrayCount)), formatBytes(stats.InstanceBytes))
		fmt.Printf("  Array Objects:    %s (%s)\n", formatCount(float64(stats.ArrayCount)), formatBytes(stats.ArrayBytes))
		if stats.JavaStringCount > 0 {
			fmt.Printf("  String Objects:   %s (%s)\n", formatCount(float64(stats.JavaStringCount)), formatBytes(stats.JavaStringBytes))
		}
		fmt.Printf("  GC Roots:         %d\n", stats.GCRootCount)
		fmt.Printf("  ClassLoaders:     %d\n", stats.ClassLoaderCount)

		// Print ranked top issues with reclaimable-overhead headline.
		if ti := fullReport.TopIssues; ti != nil && len(ti.Issues) > 0 {
			fmt.Printf("\n═══════════════════════════════════════════════════════════════════\n")
			fmt.Printf("  TOP ISSUES — reclaimable overhead: %s (%.1f%% of heap)\n",
				formatBytes(ti.ReclaimableBytes), ti.ReclaimablePercent)
			fmt.Printf("═══════════════════════════════════════════════════════════════════\n")
			for _, is := range ti.Issues {
				note := ""
				if !is.Reclaimable {
					note = "  (structural)"
				}
				fmt.Printf("  [%-6s] %-32s %10s (%4.1f%% of heap)%s\n",
					strings.ToUpper(is.Severity), is.Title, formatBytes(is.Bytes), is.Percent, note)
			}
		}

		// Print class histogram.
		fmt.Printf("\n═══════════════════════════════════════════════════════════════════\n")
		fmt.Printf("  CLASS HISTOGRAM (top %d)\n", maxClasses)
		fmt.Printf("═══════════════════════════════════════════════════════════════════\n")
		fmt.Printf("%-50s %10s %15s %15s\n", "Class Name", "Count", "Shallow Size", "Avg Size")
		fmt.Printf("%s\n", strings.Repeat("─", 91))

		type classEntry struct {
			info  *types.ClassInfo
			total int64
		}
		entries := make([]classEntry, 0, len(stats.Classes))
		for _, info := range stats.Classes {
			total := info.InstanceSize + info.ArrayBytes
			entries = append(entries, classEntry{info, total})
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].total > entries[j].total
		})

		count := 0
		for _, entry := range entries {
			if count >= maxClasses {
				break
			}
			info := entry.info
			if info.ClassName == "" {
				continue
			}
			var avgSize float64
			if info.InstanceCount > 0 {
				avgSize = float64(entry.total) / float64(info.InstanceCount)
			}
			fmt.Printf("%-50s %10d %15s %15.1f\n",
				info.ClassName,
				info.InstanceCount,
				formatBytes(entry.total),
				avgSize)
			count++
		}

		// Print "Where Memory Goes" from full analysis.
		if fullReport.MemoryByClass != nil && len(fullReport.MemoryByClass.Entries) > 0 {
			fmt.Printf("\n═══════════════════════════════════════════════════════════════════\n")
			fmt.Printf("  WHERE MEMORY GOES (by Class, ≥0.1%% of heap)\n")
			fmt.Printf("═══════════════════════════════════════════════════════════════════\n")
			fmt.Printf("%-50s %10s %15s %8s\n", "Class Name", "Instances", "Shallow Size", "% Heap")
			fmt.Printf("%s\n", strings.Repeat("─", 84))

			maxEntries := 20
			for i, entry := range fullReport.MemoryByClass.Entries {
				if i >= maxEntries {
					fmt.Printf("  ... and %d more classes\n", len(fullReport.MemoryByClass.Entries)-maxEntries)
					break
				}
				fmt.Printf("%-50s %10d %15s %7.1f%%\n",
					entry.ClassName,
					entry.InstanceCount,
					formatBytes(entry.ShallowBytes),
					entry.Percent)
			}
		}

		// Print duplicate strings.
		if fullReport.DuplicateStrings != nil && fullReport.DuplicateStrings.TotalStrings > 0 {
			ds := fullReport.DuplicateStrings
			fmt.Printf("\n═══════════════════════════════════════════════════════════════════\n")
			fmt.Printf("  DUPLICATE STRINGS\n")
			fmt.Printf("═══════════════════════════════════════════════════════════════════\n")
			fmt.Printf("  Total String Objects:  %d\n", ds.TotalStrings)
			fmt.Printf("  Unique Strings:        %d\n", ds.UniqueStrings)
			fmt.Printf("  Duplicate Groups:      %d\n", ds.DuplicateGroups)
			fmt.Printf("  Wasted Memory:         %s (%.1f%% of heap)\n", formatBytes(ds.TotalWastedBytes), ds.WastedPercent)

			if len(ds.Groups) > 0 {
				fmt.Printf("\n  Top Duplicated Strings:\n")
				fmt.Printf("  %-60s %8s %12s\n", "Value", "Count", "Wasted")
				fmt.Printf("  %s\n", strings.Repeat("─", 82))
				for i, group := range ds.Groups {
					if i >= maxDuplicates {
						break
					}
					displayValue := group.Value
					if len(displayValue) > 58 {
						displayValue = displayValue[:55] + "..."
					}
					fmt.Printf("  %-60s %8d %12s\n",
						fmt.Sprintf("\"%s\"", displayValue),
						group.Count,
						formatBytes(group.WastedBytes))
				}
			}
		}

		// Print collection waste.
		if fullReport.CollectionWaste != nil && fullReport.CollectionWaste.TotalCollections > 0 {
			cw := fullReport.CollectionWaste
			fmt.Printf("\n═══════════════════════════════════════════════════════════════════\n")
			fmt.Printf("  COLLECTION UTILIZATION\n")
			fmt.Printf("═══════════════════════════════════════════════════════════════════\n")
			fmt.Printf("  Total Collections:     %d\n", cw.TotalCollections)
			fmt.Printf("  Empty Collections:     %d\n", cw.EmptyCollections)
			fmt.Printf("  Oversized Collections: %d\n", cw.OversizedCollections)
			fmt.Printf("  Wasted Memory:         %s (%.1f%% of heap)\n", formatBytes(cw.TotalWastedBytes), cw.WastedPercent)
		}

		// Print boxed numbers.
		if fullReport.BoxedNumbers != nil && fullReport.BoxedNumbers.TotalCount > 0 {
			bn := fullReport.BoxedNumbers
			fmt.Printf("\n═══════════════════════════════════════════════════════════════════\n")
			fmt.Printf("  BOXED PRIMITIVES\n")
			fmt.Printf("═══════════════════════════════════════════════════════════════════\n")
			fmt.Printf("  Total Boxed Objects:   %d\n", bn.TotalCount)
			fmt.Printf("  Total Memory:          %s\n", formatBytes(bn.TotalBytes))
			fmt.Printf("  Estimated Waste:       %s (%.1f%% of heap)\n", formatBytes(bn.WastedBytes), bn.WastedPercent)
			if len(bn.ByType) > 0 {
				fmt.Printf("\n  %-30s %10s %15s\n", "Type", "Count", "Bytes")
				fmt.Printf("  %s\n", strings.Repeat("─", 57))
				for _, bt := range bn.ByType {
					fmt.Printf("  %-30s %10d %15s\n", bt.ClassName, bt.Count, formatBytes(bt.Bytes))
				}
			}
		}

		// Print per-object header overhead.
		if fullReport.ObjectHeaders != nil && fullReport.ObjectHeaders.OverheadBytes > 0 {
			oh := fullReport.ObjectHeaders
			fmt.Printf("\n═══════════════════════════════════════════════════════════════════\n")
			fmt.Printf("  OBJECT HEADER OVERHEAD\n")
			fmt.Printf("═══════════════════════════════════════════════════════════════════\n")
			fmt.Printf("  Objects:               %s\n", formatCount(float64(oh.TotalObjects)))
			fmt.Printf("  Header Size:           %d B / object, %d B / array\n", oh.ObjectHeader, oh.ArrayHeader)
			fmt.Printf("  Total Overhead:        %s (%.1f%% of heap)\n", formatBytes(oh.OverheadBytes), oh.Percent)
			if len(oh.Entries) > 0 {
				fmt.Printf("\n  %-50s %10s %15s %8s\n", "Class Name", "Objects", "Overhead", "% Heap")
				fmt.Printf("  %s\n", strings.Repeat("─", 86))
				for _, e := range oh.Entries {
					fmt.Printf("  %-50s %10d %15s %7.1f%%\n", e.ClassName, e.ObjectCount, formatBytes(e.OverheadBytes), e.Percent)
				}
			}
		}

		// Print primitive-array waste.
		if aw := fullReport.ArrayWaste; aw != nil && (aw.SparseBytes > 0 || aw.HumongousBytes > 0) {
			fmt.Printf("\n═══════════════════════════════════════════════════════════════════\n")
			fmt.Printf("  ARRAY WASTE\n")
			fmt.Printf("═══════════════════════════════════════════════════════════════════\n")
			fmt.Printf("  Empty/Sparse Arrays:   %s in %s arrays (%.1f%% of heap)\n",
				formatBytes(aw.SparseBytes), formatCount(float64(aw.SparseCount)), aw.SparsePercent)
			fmt.Printf("  Humongous Arrays:      %s in %s arrays (≥%s each)\n",
				formatBytes(aw.HumongousBytes), formatCount(float64(aw.HumongousCount)), formatBytes(1<<20))
			fmt.Printf("  Largest Single Array:  %s\n", formatBytes(aw.LargestArrayBytes))
		}

		// Print recommendations.
		if len(fullReport.Recommendations) > 0 {
			fmt.Printf("\n═══════════════════════════════════════════════════════════════════\n")
			fmt.Printf("  RECOMMENDATIONS\n")
			fmt.Printf("═══════════════════════════════════════════════════════════════════\n")
			for i, rec := range fullReport.Recommendations {
				severity := strings.ToUpper(rec.Severity)
				fmt.Printf("\n  %d. [%s] %s\n", i+1, severity, rec.Title)
				fmt.Printf("     %s\n", rec.Description)
				fmt.Printf("     Estimated savings: %s (%.1f%% of heap)\n", formatBytes(rec.EstimatedSaved), rec.SavedPercent)
			}
		}

		// Legacy opt-in analysis (kept for backward compatibility).
		if analyzeStrings {
			stringAnalysis, err := analyzer.AnalyzeStrings()
			if err != nil {
				return fmt.Errorf("analyzing strings: %w", err)
			}
			totalStrings := stats.StringCount
			duplicateStrings := totalStrings - int64(stringAnalysis.UniqueStrings)

			fmt.Printf("\n  Legacy String Analysis:\n")
			fmt.Printf("  Total Strings: %s\n", formatCount(float64(totalStrings)))
			fmt.Printf("  Unique Strings: %s\n", formatCount(float64(stringAnalysis.UniqueStrings)))
			fmt.Printf("  Duplicate Strings: %s\n", formatCount(float64(duplicateStrings)))
			fmt.Printf("  Total Memory: %s\n", formatBytes(stats.StringBytes))
			fmt.Printf("  Wasted Memory: %s\n", formatBytes(stringAnalysis.TotalWastedBytes))

			if showStringsDetail {
				fmt.Printf("\n  Top Duplicated Strings:\n")
				printTopDuplicates(stringAnalysis.Duplicates, maxDuplicates)
			}
		}

		if analyzeCollections {
			collectionStats, err := analyzer.AnalyzeCollections()
			if err != nil {
				return fmt.Errorf("analyzing collections: %w", err)
			}
			printCollectionStats(collectionStats)
		}

		if analyzeThreads {
			threadAnalysis, err := analyzer.AnalyzeThreads()
			if err != nil {
				return fmt.Errorf("analyzing threads: %w", err)
			}
			fmt.Printf("\nThread Analysis:\n")
			fmt.Printf("────────────────────────────────────────\n")
			fmt.Printf("Total Threads: %d\n", threadAnalysis.TotalThreads)
			fmt.Printf("Active Threads: %d\n", threadAnalysis.ActiveThreads)
			fmt.Printf("Daemon Threads: %d\n", threadAnalysis.DaemonThreads)
			fmt.Printf("Total Stack Size: %s\n", formatCount(float64(threadAnalysis.TotalStackSize)))
			fmt.Printf("Total Retained Memory: %s\n", formatCount(float64(threadAnalysis.TotalRetained)))

			if showThreads && len(threadAnalysis.Threads) > 0 {
				fmt.Printf("\nDetailed Thread Information:\n")
				fmt.Printf("────────────────────────────────────────\n")

				type threadEntry struct {
					id   uint64
					info *types.ThreadInfo
				}
				threads := make([]threadEntry, 0, len(threadAnalysis.Threads))
				for id, info := range threadAnalysis.Threads {
					threads = append(threads, threadEntry{id, info})
				}
				sort.Slice(threads, func(i, j int) bool {
					return threads[i].info.RetainedSize > threads[j].info.RetainedSize
				})

				for _, t := range threads {
					info := t.info
					fmt.Printf("\nThread: %s (ID: 0x%x)\n", info.ThreadName, info.ThreadId)
					fmt.Printf("  State: %-12s  Group: %s\n", info.ThreadState, info.ThreadGroup)
					fmt.Printf("  Stack Size: %-10s  Retained: %s\n",
						formatCount(float64(info.StackSize)),
						formatCount(float64(info.RetainedSize)))
					fmt.Printf("  Properties: %s%s\n",
						map[bool]string{true: "Daemon ", false: ""}[info.Daemon],
						map[bool]string{true: "Active", false: "Inactive"}[info.IsAlive])
					if len(info.LocalObjects) > 0 {
						fmt.Printf("  Local Objects: %d\n", len(info.LocalObjects))
					}
				}
			}
		}

		if findLeaks {
			leakCandidates := findLeakCandidates(heapData)
			chains, err := analyzer.FindRetentionChains(leakCandidates, maxChainLength)
			if err != nil {
				return fmt.Errorf("finding retention chains: %w", err)
			}
			fmt.Printf("\nPotential Memory Leaks:\n")
			for _, chain := range chains {
				fmt.Printf("\nObject: 0x%x\n", chain.TargetObjectId)
				fmt.Printf("Retained Memory: %s\n", formatBytes(chain.TotalSize))
				fmt.Printf("Reference Chain:\n")
				for _, ref := range chain.Path {
					if classInfo, ok := stats.Classes[ref.SourceId]; ok {
						fmt.Printf("  -> %s (0x%x)\n", classInfo.ClassName, ref.SourceId)
					} else {
						fmt.Printf("  -> Unknown (0x%x)\n", ref.SourceId)
					}
				}
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(analyzeCmd)

	analyzeCmd.Flags().BoolVar(&analyzeStrings, "analyze-strings", false, "Analyze string duplicates (legacy)")
	analyzeCmd.Flags().BoolVar(&showStringsDetail, "show-strings-detail", false, "Show detailed string analysis")
	analyzeCmd.Flags().BoolVar(&analyzeCollections, "analyze-collections", false, "Analyze collection usage (legacy)")
	analyzeCmd.Flags().BoolVar(&analyzeThreads, "analyze-threads", false, "Analyze thread memory usage")
	analyzeCmd.Flags().BoolVar(&showThreads, "show-threads", false, "Show detailed thread analysis")
	analyzeCmd.Flags().BoolVar(&findLeaks, "find-leaks", false, "Find potential memory leaks")
	analyzeCmd.Flags().IntVar(&maxChainLength, "max-chain", 10, "Maximum length of reference chains")
	analyzeCmd.Flags().Float64Var(&minLoadFactor, "min-load-factor", 0.25, "Minimum collection load factor")
	analyzeCmd.Flags().BoolVar(&debug, "debug", false, "Enable debug logging")
	analyzeCmd.Flags().BoolVar(&generateJSON, "json", false, "Output analysis results as JSON")
	analyzeCmd.Flags().IntVar(&maxClasses, "max-classes", 10, "Maximum number of classes to display")
	analyzeCmd.Flags().IntVar(&maxDuplicates, "max-duplicates", 10, "Maximum number of duplicates to display")
}

func printTopDuplicates(duplicates map[string][]types.StringInstance, topN int) {
	type duplicateEntry struct {
		value     string
		instances []types.StringInstance
	}
	var entries []duplicateEntry
	for str, insts := range duplicates {
		entries = append(entries, duplicateEntry{
			value:     str,
			instances: insts,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return len(entries[i].instances) > len(entries[j].instances)
	})
	for i, entry := range entries {
		if i >= topN {
			break
		}
		fmt.Printf("%s: %d instances, total size: %s\n",
			entry.value,
			len(entry.instances),
			formatCount(float64(len(entry.instances)*int(entry.instances[0].Size))))
	}
}

func printCollectionStats(stats map[uint64]*types.CollectionStats) {
	fmt.Printf("\nCollection Analysis:\n")
	for _, stat := range stats {
		fmt.Printf("\nCollection: %s (ID: 0x%x)\n", stat.ClassName, stat.ObjectId)
		fmt.Printf("  Elements: %d/%d\n", stat.ElementCount, stat.Capacity)
		fmt.Printf("  Load Factor: %.2f\n", stat.LoadFactor)
		fmt.Printf("  Wasted Space: %s\n", formatCount(float64(stat.WastedSpace)))
		fmt.Printf("  Empty: %v\n", stat.IsEmpty)
		fmt.Printf("  Oversized: %v\n", stat.IsOversized)
	}
}

func findLeakCandidates(heapData *analysis.HeapData) []uint64 {
	candidates := make([]uint64, 0)
	threshold := int64(1024 * 1024) // 1MB

	for objId, obj := range heapData.Objects {
		if obj.Size > threshold {
			candidates = append(candidates, objId)
		}
	}
	return candidates
}
