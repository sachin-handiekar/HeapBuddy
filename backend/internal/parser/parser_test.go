package parser

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// sampleHPROFPath returns the absolute path to the sample.hprof file.
func sampleHPROFPath(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to determine test file path")
	}
	projectRoot := filepath.Join(filepath.Dir(filename), "..", "..")
	path := filepath.Join(projectRoot, "sample-hprof", "sample.hprof")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("sample HPROF file not found at %s", path)
	}
	return path
}

// TestEndToEnd_ParseAndVerify is a single test function that parses the HPROF file once
// and runs all verification subtests against the parsed result.
func TestEndToEnd_ParseAndVerify(t *testing.T) {
	path := sampleHPROFPath(t)

	p, err := NewParser(path)
	if err != nil {
		t.Fatalf("NewParser failed: %v", err)
	}
	defer p.Close()

	stats, err := p.Parse()
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	t.Run("BasicStats", func(t *testing.T) {
		if stats.ObjectCount <= 0 {
			t.Errorf("expected ObjectCount > 0, got %d", stats.ObjectCount)
		}
		if stats.TotalBytes <= 0 {
			t.Errorf("expected TotalBytes > 0, got %d", stats.TotalBytes)
		}
		if len(stats.Classes) == 0 {
			t.Error("expected Classes map to be non-empty")
		}
		if len(stats.StringMap) == 0 {
			t.Error("expected StringMap to be non-empty")
		}
		if stats.HeapSummary == nil {
			t.Error("expected HeapSummary to be non-nil")
		}
		t.Logf("Parse OK: %d objects, %d bytes, %d classes, %d strings",
			stats.ObjectCount, stats.TotalBytes, len(stats.Classes), len(stats.StringMap))
	})

	t.Run("ClassHistogram", func(t *testing.T) {
		if len(stats.Classes) == 0 {
			t.Fatal("no classes parsed")
		}
		emptyNames := 0
		for _, info := range stats.Classes {
			if info.ClassName == "" {
				emptyNames++
			}
		}
		// Some JVM internal/anonymous classes may have empty names
		if emptyNames > len(stats.Classes)/2 {
			t.Errorf("too many classes with empty names: %d/%d", emptyNames, len(stats.Classes))
		}
		if len(stats.Classes) < 10 {
			t.Errorf("expected at least 10 classes, got %d", len(stats.Classes))
		}
		t.Logf("Class histogram: %d classes parsed (%d with empty names)", len(stats.Classes), emptyNames)
	})

	t.Run("StringTracking", func(t *testing.T) {
		if stats.StringCount <= 0 {
			t.Errorf("expected StringCount > 0, got %d", stats.StringCount)
		}
		if stats.StringBytes <= 0 {
			t.Errorf("expected StringBytes > 0, got %d", stats.StringBytes)
		}
		if len(stats.StringMap) == 0 {
			t.Fatal("StringMap is empty")
		}
		emptyCount := 0
		for _, v := range stats.StringMap {
			if v == "" {
				emptyCount++
			}
		}
		if emptyCount == len(stats.StringMap) {
			t.Error("all strings in StringMap are empty")
		}
		t.Logf("Strings: %d total, %d bytes, %d in map", stats.StringCount, stats.StringBytes, len(stats.StringMap))
	})

	t.Run("StringValues", func(t *testing.T) {
		if len(stats.StringValues) == 0 {
			t.Fatal("expected resolved java.lang.String values, got none")
		}
		// Resolved values should never exceed the number of String instances.
		if int64(len(stats.StringValues)) > stats.JavaStringCount {
			t.Errorf("resolved %d string values but only %d String instances exist",
				len(stats.StringValues), stats.JavaStringCount)
		}
		// At least some values should decode to printable ASCII (class names,
		// field names, etc.), confirming the decoder is not producing garbage.
		printable := 0
		for _, v := range stats.StringValues {
			if v == "" {
				continue
			}
			ok := true
			for _, r := range v {
				if r < 0x20 || r > 0x7e {
					ok = false
					break
				}
			}
			if ok {
				printable++
			}
		}
		if printable == 0 {
			t.Error("no resolved string decoded to printable ASCII — decoder likely broken")
		}
		t.Logf("Resolved %d string values (%d printable-ASCII)", len(stats.StringValues), printable)
	})

	t.Run("References", func(t *testing.T) {
		withRefs := 0
		totalRefs := 0
		for _, obj := range stats.Objects {
			if len(obj.References) > 0 {
				withRefs++
				totalRefs += len(obj.References)
			}
		}
		if totalRefs == 0 {
			t.Error("expected instance field references to be populated, got none")
		}
		t.Logf("References: %d objects carry %d total references", withRefs, totalRefs)
	})

	t.Run("Objects", func(t *testing.T) {
		if len(stats.Objects) == 0 {
			t.Fatal("no objects parsed")
		}
		for _, obj := range stats.Objects {
			if obj.Size < 0 {
				t.Errorf("object 0x%x has negative size %d", obj.ObjectId, obj.Size)
			}
		}
		t.Logf("Objects: %d parsed", len(stats.Objects))
	})

	t.Run("HeapSummary", func(t *testing.T) {
		if stats.HeapSummary == nil {
			t.Fatal("HeapSummary is nil")
		}
		hs := stats.HeapSummary
		if hs.TotalLiveBytes == 0 {
			t.Error("expected TotalLiveBytes > 0")
		}
		if hs.TotalLiveInstances == 0 {
			t.Error("expected TotalLiveInstances > 0")
		}
		t.Logf("HeapSummary: %d live bytes, %d live instances",
			hs.TotalLiveBytes, hs.TotalLiveInstances)
	})

	t.Run("GCRoots", func(t *testing.T) {
		if stats.GCRootCount <= 0 {
			t.Errorf("expected GCRootCount > 0, got %d", stats.GCRootCount)
		}
		t.Logf("GC Roots: %d", stats.GCRootCount)
	})
}

func TestInvalidFile(t *testing.T) {
	_, err := NewParser("nonexistent_file.hprof")
	if err == nil {
		t.Error("expected error for non-existent file, got nil")
	}
}

func TestParserClose(t *testing.T) {
	path := sampleHPROFPath(t)
	p, err := NewParser(path)
	if err != nil {
		t.Fatalf("NewParser failed: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
	// Double close should not panic
	_ = p.Close()
}
