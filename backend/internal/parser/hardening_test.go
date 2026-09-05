package parser

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// validHeader builds a minimal valid HPROF header (8-byte ids) so tests can
// append hostile records after it.
func validHeader() []byte {
	var b []byte
	b = append(b, []byte("JAVA PROFILE ")...) // 13-byte magic
	b = append(b, []byte("1.0.2")...)         // version
	b = append(b, 0)                          // null terminator
	id := make([]byte, 4)
	binary.BigEndian.PutUint32(id, 8) // identifier size
	b = append(b, id...)
	ts := make([]byte, 8)
	b = append(b, ts...) // timestamp
	return b
}

// recordRawLen appends a record with an explicit (possibly bogus) length field
// regardless of the actual body, to exercise length guards.
func recordRawLen(tag byte, length uint32, body []byte) []byte {
	out := []byte{tag, 0, 0, 0, 0}
	l := make([]byte, 4)
	binary.BigEndian.PutUint32(l, length)
	out = append(out, l...)
	return append(out, body...)
}

func parseBytes(t *testing.T, data []byte) (*HeapStats, error) {
	t.Helper()
	f := filepath.Join(t.TempDir(), "evil.hprof")
	if err := os.WriteFile(f, data, 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := NewParser(f)
	if err != nil {
		t.Fatalf("NewParser: %v", err)
	}
	defer p.Close()
	return p.Parse() // must never panic
}

// TestParseMalformedInputs feeds hostile/truncated dumps and asserts the parser
// returns (rather than panicking or allocating unboundedly). The recover() in
// Parse converts any internal panic into an error, so reaching the assertion at
// all proves no crash.
func TestParseMalformedInputs(t *testing.T) {
	idBody := make([]byte, 8) // a single 8-byte object id

	cases := []struct {
		name string
		data []byte
	}{
		{"empty", nil},
		{"garbage", []byte("not an hprof file at all, just bytes")},
		{"header only", validHeader()},
		{"truncated mid-record", append(validHeader(), 0x01, 0, 0)},
		{
			// STRING_IN_UTF8 claiming a ~4 GiB length — must be rejected by the cap,
			// not turned into a multi-gigabyte make().
			"huge string length",
			append(validHeader(), recordRawLen(0x01, 0xFFFFFFFF, idBody)...),
		},
		{
			// STRING_IN_UTF8 whose length is smaller than the id size (underflow).
			"string length under id size",
			append(validHeader(), recordRawLen(0x01, 3, idBody)...),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// The assertion is simply that this returns. A panic would fail the
			// test (recover turns it into an error); an unbounded alloc would OOM.
			stats, err := parseBytes(t, tc.data)
			if err == nil && stats == nil {
				t.Fatal("expected either stats or an error, got neither")
			}
		})
	}
}

// TestParseValidStillWorks guards against the hardening breaking the happy path.
func TestParseValidStillWorks(t *testing.T) {
	path := sampleHPROFPath(t)
	p, err := NewParser(path)
	if err != nil {
		t.Fatalf("NewParser: %v", err)
	}
	defer p.Close()
	stats, err := p.Parse()
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if stats.ObjectCount <= 0 {
		t.Errorf("expected objects, got %d", stats.ObjectCount)
	}
}

// TestParseClassicHeapDumpRecord verifies the body of a classic single
// HEAP_DUMP (0x0C) record is parsed, not skipped. Many dumps (jmap, older
// OpenJDK 8, Hadoop/YARN containers) emit one HEAP_DUMP record instead of the
// segmented HEAP_DUMP_SEGMENT (0x1C) form; before the fix the parser fell
// through to the default branch and discarded the entire heap (0 objects/roots).
func TestParseClassicHeapDumpRecord(t *testing.T) {
	data := validHeader()

	// A STRING_IN_UTF8 record so Parse() has top-level data and won't bail with
	// "no heap dump data found" before we can inspect the heap-dump result.
	strBody := make([]byte, 8)
	binary.BigEndian.PutUint64(strBody, 0x1)
	strBody = append(strBody, 'x')
	data = append(data, recordRawLen(byte(HProfRecordType_STRING_IN_UTF8), uint32(len(strBody)), strBody)...)

	// A classic HEAP_DUMP (0x0C) record whose body is one GC-root sub-record:
	// ROOT_STICKY_CLASS (0x05) + an 8-byte object id.
	id := make([]byte, 8)
	binary.BigEndian.PutUint64(id, 0xCAFEBABE)
	heapBody := append([]byte{byte(HProfGCTag_ROOT_STICKY_CLASS)}, id...)
	data = append(data, recordRawLen(byte(HProfRecordType_HEAP_DUMP), uint32(len(heapBody)), heapBody)...)

	stats, err := parseBytes(t, data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if stats.GCRootCount != 1 {
		t.Fatalf("GCRootCount = %d, want 1 — HEAP_DUMP (0x0C) body was skipped", stats.GCRootCount)
	}
}

// TestPrimitiveArraysAttributedToClass verifies primitive arrays (byte[], …) are
// attributed to a synthetic per-element-type class so they show up in the
// histogram and "where memory goes" — previously they were only counted in the
// global array totals and were invisible in the per-class views.
func TestPrimitiveArraysAttributedToClass(t *testing.T) {
	data := validHeader()

	// STRING so Parse() returns rather than "no heap dump data found".
	s := make([]byte, 8)
	binary.BigEndian.PutUint64(s, 0x1)
	s = append(s, 'x')
	data = append(data, recordRawLen(byte(HProfRecordType_STRING_IN_UTF8), uint32(len(s)), s)...)

	// HEAP_DUMP with one PRIMITIVE_ARRAY_DUMP: a byte[] of length 4.
	sub := []byte{byte(HProfGCTag_PRIMITIVE_ARRAY_DUMP)}
	aid := make([]byte, 8)
	binary.BigEndian.PutUint64(aid, 0xA1)
	sub = append(sub, aid...)                    // array object id
	sub = append(sub, 0, 0, 0, 0)                // stack-trace serial
	sub = append(sub, 0, 0, 0, 4)                // length = 4
	sub = append(sub, byte(HProfValueType_BYTE)) // element type
	sub = append(sub, 1, 2, 3, 4)                // 4 bytes of payload
	data = append(data, recordRawLen(byte(HProfRecordType_HEAP_DUMP), uint32(len(sub)), sub)...)

	stats, err := parseBytes(t, data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var found bool
	for _, ci := range stats.Classes {
		if ci.ClassName == "byte[]" {
			found = true
			if ci.ArrayBytes != 4 {
				t.Errorf("byte[] ArrayBytes = %d, want 4", ci.ArrayBytes)
			}
			if ci.InstanceCount != 1 {
				t.Errorf("byte[] InstanceCount = %d, want 1", ci.InstanceCount)
			}
		}
	}
	if !found {
		t.Fatal("byte[] was not attributed to a class (primitive array invisible in per-class views)")
	}
}

// TestSparseArrayDetection verifies a large all-zero primitive array is flagged
// as sparse/empty (over-allocated buffer) with its bytes counted as waste.
func TestSparseArrayDetection(t *testing.T) {
	data := validHeader()
	s := make([]byte, 8)
	binary.BigEndian.PutUint64(s, 0x1)
	s = append(s, 'x')
	data = append(data, recordRawLen(byte(HProfRecordType_STRING_IN_UTF8), uint32(len(s)), s)...)

	const n = 2048 // >= sparseArrayMinBytes; all zero -> 100% sparse
	sub := []byte{byte(HProfGCTag_PRIMITIVE_ARRAY_DUMP)}
	aid := make([]byte, 8)
	binary.BigEndian.PutUint64(aid, 0xB2)
	sub = append(sub, aid...)
	sub = append(sub, 0, 0, 0, 0)       // stack-trace serial
	sub = append(sub, 0, 0, 0x08, 0x00) // length = 2048
	sub = append(sub, byte(HProfValueType_BYTE))
	sub = append(sub, make([]byte, n)...) // n zero bytes
	data = append(data, recordRawLen(byte(HProfRecordType_HEAP_DUMP), uint32(len(sub)), sub)...)

	stats, err := parseBytes(t, data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if stats.SparseArrayCount != 1 {
		t.Errorf("SparseArrayCount = %d, want 1", stats.SparseArrayCount)
	}
	if stats.SparseArrayBytes != n {
		t.Errorf("SparseArrayBytes = %d, want %d", stats.SparseArrayBytes, n)
	}
}
