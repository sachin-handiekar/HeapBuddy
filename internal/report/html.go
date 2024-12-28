package report

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"time"

	"heapbuddy/internal/parser"

	"github.com/dustin/go-humanize"
)

type HTMLReport struct {
	Title              string
	Active             string
	Stats              *parser.HeapStats
	GenTime            string
	Filename           string
	UsedHeap           string
	FreeMemory         string
	TotalObjects       string
	UnreachableObjects string
	Objects            []ObjectInfo
	ClassStats         []ClassStatInfo
	Threads            []ThreadInfo
	Properties         []PropertyInfo
}

type PropertyInfo struct {
	Name  string
	Value string
}

type ObjectInfo struct {
	ID           string
	ClassName    string
	Size         string
	RetainedSize string
	GCRoot       string
}

type ThreadInfo struct {
	Name       string
	State      string
	StateColor string
	StackTrace string
}

type ClassStatInfo struct {
	ClassName     string
	InstanceCount string
	TotalSize     string
	AvgSize       string
}

func formatBytes(bytes interface{}) string {
	switch v := bytes.(type) {
	case uint64:
		return humanize.Bytes(v)
	case int64:
		if v < 0 {
			return "0 B"
		}
		return humanize.Bytes(uint64(v))
	default:
		return "0 B"
	}
}

func formatCount(count interface{}) string {
	switch v := count.(type) {
	case int64:
		return humanize.Comma(v)
	case uint32:
		return humanize.Comma(int64(v))
	case int:
		return humanize.Comma(int64(v))
	default:
		return "0"
	}
}

func toInt64(v interface{}) int64 {
	switch x := v.(type) {
	case int:
		return int64(x)
	case int32:
		return int64(x)
	case int64:
		return x
	case uint:
		return int64(x)
	case uint32:
		return int64(x)
	case uint64:
		return int64(x)
	default:
		return 0
	}
}

func GenerateHTML(stats *parser.HeapStats, filename string) error {
	report := HTMLReport{
		Title:              "Heap Analysis Report",
		Active:             "overview",
		Stats:              stats,
		GenTime:            time.Now().Format("2006-01-02 15:04:05"),
		Filename:           filepath.Base(filename),
		UsedHeap:           formatBytes(stats.TotalBytes),
		FreeMemory:         formatBytes(uint64(stats.HeapSummary.TotalBytesAllocated) - uint64(stats.TotalBytes)),
		TotalObjects:       formatCount(stats.ObjectCount),
		UnreachableObjects: formatCount(stats.ObjectCount - int64(stats.GCRootCount)),
	}

	// Add largest objects
	for id, class := range stats.ClassStats {
		if class.TotalRetainedSize > 0 {
			report.Objects = append(report.Objects, ObjectInfo{
				ID:           fmt.Sprintf("0x%x", id),
				ClassName:    class.ClassName,
				Size:         formatBytes(class.TotalShallowSize),
				RetainedSize: formatBytes(class.TotalRetainedSize),
				GCRoot:       fmt.Sprintf("%d", stats.GCRootCount),
			})
		}
	}

	// Add class histogram
	for _, class := range stats.ClassStats {
		avgSize := int64(0)
		if class.InstanceCount > 0 {
			avgSize = class.TotalShallowSize / int64(uint64(class.InstanceCount))
		}
		report.ClassStats = append(report.ClassStats, ClassStatInfo{
			ClassName:     class.ClassName,
			InstanceCount: formatCount(class.InstanceCount),
			TotalSize:     formatBytes(class.TotalShallowSize),
			AvgSize:       formatBytes(avgSize),
		})
	}

	// Add system properties
	for key, value := range stats.SystemProps {
		report.Properties = append(report.Properties, PropertyInfo{
			Name:  key,
			Value: value,
		})
	}

	funcMap := template.FuncMap{
		"formatBytes": formatBytes,
		"formatCount": formatCount,
		"int64":      toInt64,
	}

	// Parse template file
	tmpl, err := template.New("template.html").Funcs(funcMap).ParseFiles(filepath.Join("internal", "report", "template.html"))
	if err != nil {
		return fmt.Errorf("parsing template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "template.html", report); err != nil {
		return fmt.Errorf("executing template: %w", err)
	}

	outputFile := filename + ".report.html"
	return os.WriteFile(outputFile, buf.Bytes(), 0644)
}
