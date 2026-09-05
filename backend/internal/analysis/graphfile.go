package analysis

import (
	"encoding/binary"
	"fmt"
	"os"
	"runtime"
	"sort"
	"unsafe"
)

// Graph-cache file (Phase 2b of docs/memory-reduction-plan.md).
//
// The compact CSR reference graph is serialized to a flat binary file at
// analysis time, while the parser's object maps are still alive. The first
// interactive request then memory-maps this file instead of re-parsing the
// dump: the numeric sections (node table, both CSR edge lists) and the decoded
// java.lang.String values are used directly out of the mapping — off the Go
// heap, invisible to the GC, paged in and out by the OS — so a multi-GB graph
// costs near-zero Go heap and only its working set stays resident.
//
// The file is a per-report cache in the server's temp dir, never portable:
// values are written in NATIVE byte order and the loader rejects a file whose
// endianness marker doesn't match. Any validation failure makes the loader
// return an error and the caller falls back to a full re-parse, so a corrupt
// or truncated cache can never produce wrong results.
//
// Layout: a fixed header of counts, then the sections below in fixed order,
// each padded to 8-byte alignment so slices can be viewed in place:
//
//	ids, classIdx, sizes, kinds, outOff, outTarget, outLabel, outWeak,
//	inOff, inSrc, inLabel, inWeak, labels(string table), classIDs,
//	classNames(string table), classRefKind, instCounts, largest,
//	rootIDs, rootLabelIdx, rootLabels(string table), strIDs, strOff, strBlob
//
// Small tables (labels, class names, root labels, per-class counts) are decoded
// onto the heap at load — they are thousands of entries. The two potentially
// huge string sections (root ids aside, the decoded String values) stay in the
// mapping and are served lazily by binary search.

const (
	graphFileMagic  = "HBGC0002"         // HeapBuddy Graph Cache, format 2 (weak-edge bitsets)
	graphFileEndian = 0x0123456789ABCDEF // written natively; mismatch = foreign file
	graphHeaderSize = 8 + 8*8            // magic + endian + 7 counts
)

// graphMapping owns the mmap backing a file-loaded graph. It is released by a
// finalizer once the graph is unreachable (i.e. after its bundle is evicted and
// no request still holds it) — never eagerly, so an in-flight request can never
// touch unmapped memory. The file itself is best-effort deleted after unmap;
// anything left behind (e.g. a crash) is swept by the server on next boot.
type graphMapping struct {
	unmap func()
	path  string
}

func (m *graphMapping) release() {
	if m.unmap != nil {
		m.unmap()
		m.unmap = nil
	}
	_ = os.Remove(m.path)
}

// WriteFile serializes the graph to path (atomically, via a rename from a
// sibling .tmp file) so it can later be loaded with LoadGraphFile.
func (g *ReferenceGraph) WriteFile(path string) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("create graph cache: %w", err)
	}
	if err := g.writeTo(f); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("write graph cache: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("flush graph cache: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("publish graph cache: %w", err)
	}
	return nil
}

func (g *ReferenceGraph) writeTo(f *os.File) error {
	// Flatten the map-backed tables into the arrays the file stores. The graph
	// being written was built in memory, so roots/strings are maps.
	rootIDs := make([]uint64, 0, len(g.roots))
	for id := range g.roots {
		rootIDs = append(rootIDs, id)
	}
	sort.Slice(rootIDs, func(i, j int) bool { return rootIDs[i] < rootIDs[j] })
	rootLabelIdx := make([]int32, len(rootIDs))
	var rootLabels []string
	rootLabelPos := map[string]int32{}
	for i, id := range rootIDs {
		l := g.roots[id]
		p, ok := rootLabelPos[l]
		if !ok {
			p = int32(len(rootLabels))
			rootLabels = append(rootLabels, l)
			rootLabelPos[l] = p
		}
		rootLabelIdx[i] = p
	}

	strIDs := make([]uint64, 0, len(g.strings))
	for id := range g.strings {
		strIDs = append(strIDs, id)
	}
	sort.Slice(strIDs, func(i, j int) bool { return strIDs[i] < strIDs[j] })
	strOff := make([]int64, len(strIDs)+1)
	var blobLen int64
	for i, id := range strIDs {
		strOff[i] = blobLen
		blobLen += int64(len(g.strings[id]))
	}
	strOff[len(strIDs)] = blobLen

	instCounts := make([]int64, len(g.classIDs))
	largest := make([]uint64, len(g.classIDs))
	for i, cid := range g.classIDs {
		instCounts[i] = int64(g.instCount[cid])
		largest[i] = g.largest[cid] // 0 (never a valid object id) = none
	}

	e := &graphEncoder{f: f}
	var header [graphHeaderSize]byte
	copy(header[:8], graphFileMagic)
	binary.NativeEndian.PutUint64(header[8:], graphFileEndian)
	for i, v := range []uint64{
		uint64(len(g.ids)), uint64(len(g.outTarget)), uint64(len(g.inSrc)),
		uint64(len(g.classIDs)), uint64(len(rootIDs)), uint64(len(strIDs)),
		uint64(blobLen),
	} {
		binary.NativeEndian.PutUint64(header[16+8*i:], v)
	}
	e.raw(header[:])

	writeSlice(e, g.ids)
	writeSlice(e, g.classIdx)
	writeSlice(e, g.sizes)
	writeSlice(e, g.kinds)
	writeSlice(e, g.outOff)
	writeSlice(e, g.outTarget)
	writeSlice(e, g.outLabel)
	writeSlice(e, g.outWeak)
	writeSlice(e, g.inOff)
	writeSlice(e, g.inSrc)
	writeSlice(e, g.inLabel)
	writeSlice(e, g.inWeak)
	e.stringTable(g.labels)
	writeSlice(e, g.classIDs)
	e.stringTable(g.classNames)
	writeSlice(e, g.classRefKind)
	writeSlice(e, instCounts)
	writeSlice(e, largest)
	writeSlice(e, rootIDs)
	writeSlice(e, rootLabelIdx)
	e.stringTable(rootLabels)
	writeSlice(e, strIDs)
	writeSlice(e, strOff)
	for _, id := range strIDs {
		e.raw([]byte(g.strings[id]))
	}
	e.pad()
	return e.err
}

// LoadGraphFile memory-maps a graph cache written by WriteFile and returns a
// ReferenceGraph whose big sections read directly from the mapping. On any
// validation failure it returns an error; the caller should fall back to
// re-parsing the dump.
func LoadGraphFile(path string) (*ReferenceGraph, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close() // the mapping outlives the descriptor on all platforms

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()
	if size < graphHeaderSize {
		return nil, fmt.Errorf("graph cache %s: too small (%d bytes)", path, size)
	}
	data, unmap, err := mmapFile(f, size)
	if err != nil {
		return nil, fmt.Errorf("graph cache %s: mmap: %w", path, err)
	}
	g, err := decodeGraph(data, path)
	if err != nil {
		unmap()
		return nil, fmt.Errorf("graph cache %s: %w", path, err)
	}
	m := &graphMapping{unmap: unmap, path: path}
	g.backing = m
	runtime.SetFinalizer(m, (*graphMapping).release)
	return g, nil
}

func decodeGraph(data []byte, path string) (*ReferenceGraph, error) {
	if string(data[:8]) != graphFileMagic {
		return nil, fmt.Errorf("bad magic")
	}
	if binary.NativeEndian.Uint64(data[8:]) != graphFileEndian {
		return nil, fmt.Errorf("byte order mismatch (cache from another machine?)")
	}
	var counts [7]uint64
	for i := range counts {
		counts[i] = binary.NativeEndian.Uint64(data[16+8*i:])
	}
	// Every section element is at least one byte, so any count larger than the
	// file is corrupt; rejecting here also rules out overflow in the size
	// arithmetic below.
	for i, c := range counts {
		if c > uint64(len(data)) {
			return nil, fmt.Errorf("count #%d (%d) exceeds file size", i, c)
		}
	}
	nNodes, nOut, nIn := counts[0], counts[1], counts[2]
	nClasses, nRoots, nStrings, blobLen := counts[3], counts[4], counts[5], counts[6]

	d := &graphDecoder{data: data, off: graphHeaderSize}
	g := &ReferenceGraph{}
	g.ids = viewSlice[uint64](d, nNodes)
	g.classIdx = viewSlice[int32](d, nNodes)
	g.sizes = viewSlice[int64](d, nNodes)
	g.kinds = viewSlice[uint8](d, nNodes)
	g.outOff = viewSlice[int32](d, offLen(nNodes))
	g.outTarget = viewSlice[uint64](d, nOut)
	g.outLabel = viewSlice[int32](d, nOut)
	g.outWeak = viewSlice[uint64](d, uint64(bitsetWords(int(nOut))))
	g.inOff = viewSlice[int32](d, offLen(nNodes))
	g.inSrc = viewSlice[int32](d, nIn)
	g.inLabel = viewSlice[int32](d, nIn)
	g.inWeak = viewSlice[uint64](d, uint64(bitsetWords(int(nIn))))
	g.labels = d.stringTable()
	g.classIDs = viewSlice[uint64](d, nClasses)
	g.classNames = d.stringTable()
	g.classRefKind = viewSlice[uint8](d, nClasses)
	instCounts := viewSlice[int64](d, nClasses)
	largest := viewSlice[uint64](d, nClasses)
	g.rootIDs = viewSlice[uint64](d, nRoots)
	g.rootLabelIdx = viewSlice[int32](d, nRoots)
	g.rootLabels = d.stringTable()
	g.strIDs = viewSlice[uint64](d, nStrings)
	g.strOff = viewSlice[int64](d, offLen(nStrings))
	g.strBlob = d.bytes(blobLen)
	d.padCheck()
	if d.err != nil {
		return nil, d.err
	}
	if d.off != int64(len(data)) {
		return nil, fmt.Errorf("trailing bytes: read %d of %d", d.off, len(data))
	}
	if err := g.validate(); err != nil {
		return nil, err
	}

	// Rebuild the small per-class lookups (thousands of entries) on the heap.
	g.instCount = make(map[uint64]int, nClasses)
	g.byName = make(map[string]uint64, nClasses)
	g.largest = make(map[uint64]uint64)
	for i, cid := range g.classIDs {
		g.instCount[cid] = int(instCounts[i])
		g.byName[g.classNames[i]] = cid
		if largest[i] != 0 {
			g.largest[cid] = largest[i]
		}
	}
	return g, nil
}

// offLen is the length of a CSR offsets array: n+1, but 0 stays 0 so an empty
// graph round-trips as empty sections.
func offLen(n uint64) uint64 {
	if n == 0 {
		return 1
	}
	return n + 1
}

// validate cross-checks section shapes so a corrupted cache fails the load
// instead of causing out-of-range panics on first use.
func (g *ReferenceGraph) validate() error {
	n := len(g.ids)
	if len(g.outOff) != n+1 && !(n == 0 && len(g.outOff) == 1) {
		return fmt.Errorf("outOff length %d for %d nodes", len(g.outOff), n)
	}
	check := func(name string, off []int32, edges int, labels []int32) error {
		if len(off) > 0 {
			if off[0] != 0 || int(off[len(off)-1]) != edges {
				return fmt.Errorf("%s bounds [%d,%d] for %d edges", name, off[0], off[len(off)-1], edges)
			}
			for i := 1; i < len(off); i++ {
				if off[i] < off[i-1] {
					return fmt.Errorf("%s not monotonic at %d", name, i)
				}
			}
		}
		for _, l := range labels {
			if int(l) >= len(g.labels) || l < 0 {
				return fmt.Errorf("%s label index %d out of range", name, l)
			}
		}
		return nil
	}
	if err := check("outOff", g.outOff, len(g.outTarget), g.outLabel); err != nil {
		return err
	}
	if err := check("inOff", g.inOff, len(g.inSrc), g.inLabel); err != nil {
		return err
	}
	for _, s := range g.inSrc {
		if int(s) >= n || s < 0 {
			return fmt.Errorf("inSrc index %d out of range", s)
		}
	}
	for _, ci := range g.classIdx {
		if int(ci) >= len(g.classIDs) {
			return fmt.Errorf("class index %d out of range", ci)
		}
	}
	for _, li := range g.rootLabelIdx {
		if int(li) >= len(g.rootLabels) || li < 0 {
			return fmt.Errorf("root label index %d out of range", li)
		}
	}
	for _, k := range g.classRefKind {
		if k > refKindFinal {
			return fmt.Errorf("class ref-kind %d out of range", k)
		}
	}
	if ln := len(g.strOff); ln > 0 {
		if g.strOff[0] != 0 || g.strOff[ln-1] != int64(len(g.strBlob)) {
			return fmt.Errorf("string offsets [%d,%d] for blob of %d", g.strOff[0], g.strOff[ln-1], len(g.strBlob))
		}
		for i := 1; i < ln; i++ {
			if g.strOff[i] < g.strOff[i-1] {
				return fmt.Errorf("string offsets not monotonic at %d", i)
			}
		}
	}
	return nil
}

// graphEncoder appends 8-byte-aligned sections to the file.
type graphEncoder struct {
	f   *os.File
	off int64
	err error
}

func (e *graphEncoder) raw(b []byte) {
	if e.err != nil || len(b) == 0 {
		return
	}
	if _, err := e.f.Write(b); err != nil {
		e.err = err
		return
	}
	e.off += int64(len(b))
}

var zeroPad [8]byte

func (e *graphEncoder) pad() {
	if r := e.off % 8; r != 0 {
		e.raw(zeroPad[:8-r])
	}
}

// writeSlice writes a numeric slice as raw native-order bytes, 8-byte aligned.
func writeSlice[T uint8 | int32 | int64 | uint64](e *graphEncoder, s []T) {
	e.pad()
	if len(s) == 0 {
		return
	}
	sz := int(unsafe.Sizeof(s[0]))
	e.raw(unsafe.Slice((*byte)(unsafe.Pointer(&s[0])), len(s)*sz))
}

// stringTable writes count, per-entry lengths, and the concatenated bytes.
func (e *graphEncoder) stringTable(ss []string) {
	e.pad()
	var hdr [8]byte
	binary.NativeEndian.PutUint64(hdr[:], uint64(len(ss)))
	e.raw(hdr[:])
	lens := make([]uint32, len(ss))
	for i, s := range ss {
		lens[i] = uint32(len(s))
	}
	writeSlice(e, lensAsInt32(lens))
	e.pad() // the reader aligns before every section, including the blob
	for _, s := range ss {
		e.raw([]byte(s))
	}
	e.pad()
}

// lensAsInt32 reinterprets a []uint32 as []int32 for writeSlice's constraint;
// the bit patterns are identical and lengths never exceed MaxInt32.
func lensAsInt32(u []uint32) []int32 {
	if len(u) == 0 {
		return nil
	}
	return unsafe.Slice((*int32)(unsafe.Pointer(&u[0])), len(u))
}

// graphDecoder walks the mapped bytes, handing out in-place views.
type graphDecoder struct {
	data []byte
	off  int64
	err  error
}

func (d *graphDecoder) fail(format string, args ...any) {
	if d.err == nil {
		d.err = fmt.Errorf(format, args...)
	}
}

func (d *graphDecoder) align() {
	if r := d.off % 8; r != 0 {
		d.off += 8 - r
	}
}

func (d *graphDecoder) bytes(n uint64) []byte {
	d.align()
	if d.err != nil {
		return nil
	}
	if n > uint64(int64(len(d.data))-d.off) {
		d.fail("section of %d bytes exceeds file at offset %d", n, d.off)
		return nil
	}
	b := d.data[d.off : d.off+int64(n)]
	d.off += int64(n)
	return b
}

func (d *graphDecoder) padCheck() { d.align() }

// viewSlice returns an in-place typed view over the next section. The section
// start is 8-byte aligned (as written), which satisfies alignment for every
// element type used.
func viewSlice[T uint8 | int32 | int64 | uint64](d *graphDecoder, n uint64) []T {
	var t T
	sz := uint64(unsafe.Sizeof(t))
	b := d.bytes(n * sz)
	if n == 0 || len(b) == 0 {
		return nil
	}
	return unsafe.Slice((*T)(unsafe.Pointer(&b[0])), n)
}

func (d *graphDecoder) stringTable() []string {
	d.align()
	cb := d.bytes(8)
	if cb == nil {
		return nil
	}
	count := binary.NativeEndian.Uint64(cb)
	if count > uint64(len(d.data)) { // cheap sanity bound before allocating
		d.fail("string table count %d exceeds file size", count)
		return nil
	}
	lens := viewSlice[int32](d, count)
	var total uint64
	for _, l := range lens {
		if l < 0 {
			d.fail("negative string length")
			return nil
		}
		total += uint64(l)
	}
	blob := d.bytes(total)
	if d.err != nil {
		return nil
	}
	out := make([]string, count)
	var pos int64
	for i, l := range lens {
		out[i] = string(blob[pos : pos+int64(l)])
		pos += int64(l)
	}
	d.padCheck()
	return out
}
