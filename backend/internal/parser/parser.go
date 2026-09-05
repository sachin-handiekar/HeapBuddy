package parser

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/sachin-handiekar/HeapBuddy/internal/types"
)

// Constants for object alignment and header size
const (
	ObjectHeaderSize = 12
	ObjectAlignment  = 8
)

// Record types
type HProfRecordType uint8

const (
	HProfRecordType_STRING_IN_UTF8    HProfRecordType = 0x01
	HProfRecordType_LOAD_CLASS        HProfRecordType = 0x02
	HProfRecordType_UNLOAD_CLASS      HProfRecordType = 0x03
	HProfRecordType_STACK_FRAME       HProfRecordType = 0x04
	HProfRecordType_STACK_TRACE       HProfRecordType = 0x05
	HProfRecordType_ALLOC_SITES       HProfRecordType = 0x06
	HProfRecordType_HEAP_SUMMARY      HProfRecordType = 0x07
	HProfRecordType_THREAD_OBJECT     HProfRecordType = 0x08
	HProfRecordType_START_THREAD      HProfRecordType = 0x0A
	HProfRecordType_END_THREAD        HProfRecordType = 0x0B
	HProfRecordType_HEAP_DUMP         HProfRecordType = 0x0C
	HProfRecordType_CPU_SAMPLES       HProfRecordType = 0x0D
	HProfRecordType_CONTROL_SETTINGS  HProfRecordType = 0x0E
	HProfRecordType_HEAP_DUMP_SEGMENT HProfRecordType = 0x1C
	HProfRecordType_HEAP_DUMP_END     HProfRecordType = 0x2C
	HProfRecordType_GC_ROOT_UNKNOWN   HProfRecordType = 0xFF
)

var HProfRecordType_name = map[HProfRecordType]string{
	HProfRecordType_STRING_IN_UTF8:    "STRING_IN_UTF8",
	HProfRecordType_LOAD_CLASS:        "LOAD_CLASS",
	HProfRecordType_UNLOAD_CLASS:      "UNLOAD_CLASS",
	HProfRecordType_STACK_FRAME:       "STACK_FRAME",
	HProfRecordType_STACK_TRACE:       "STACK_TRACE",
	HProfRecordType_ALLOC_SITES:       "ALLOC_SITES",
	HProfRecordType_HEAP_SUMMARY:      "HEAP_SUMMARY",
	HProfRecordType_THREAD_OBJECT:     "THREAD_OBJECT",
	HProfRecordType_START_THREAD:      "START_THREAD",
	HProfRecordType_END_THREAD:        "END_THREAD",
	HProfRecordType_HEAP_DUMP:         "HEAP_DUMP",
	HProfRecordType_CPU_SAMPLES:       "CPU_SAMPLES",
	HProfRecordType_CONTROL_SETTINGS:  "CONTROL_SETTINGS",
	HProfRecordType_HEAP_DUMP_SEGMENT: "HEAP_DUMP_SEGMENT",
	HProfRecordType_HEAP_DUMP_END:     "HEAP_DUMP_END",
	HProfRecordType_GC_ROOT_UNKNOWN:   "GC_ROOT_UNKNOWN",
}

// Value types
type HProfValueType byte

const (
	HProfValueType_OBJECT HProfValueType = 2
	HProfValueType_BOOL   HProfValueType = 4
	HProfValueType_CHAR   HProfValueType = 5
	HProfValueType_FLOAT  HProfValueType = 6
	HProfValueType_DOUBLE HProfValueType = 7
	HProfValueType_BYTE   HProfValueType = 8
	HProfValueType_SHORT  HProfValueType = 9
	HProfValueType_INT    HProfValueType = 10
	HProfValueType_LONG   HProfValueType = 11
)

// GC tags
type HProfGCTag byte

const (
	// Root record tags (0x01-0x0F)
	HProfGCTag_ROOT_JNI_GLOBAL   HProfGCTag = 0x01
	HProfGCTag_ROOT_JNI_LOCAL    HProfGCTag = 0x02
	HProfGCTag_ROOT_JAVA_FRAME   HProfGCTag = 0x03
	HProfGCTag_ROOT_NATIVE_STACK HProfGCTag = 0x04
	HProfGCTag_ROOT_STICKY_CLASS HProfGCTag = 0x05
	HProfGCTag_ROOT_THREAD_BLOCK HProfGCTag = 0x06
	HProfGCTag_ROOT_MONITOR_USED HProfGCTag = 0x07
	HProfGCTag_ROOT_THREAD_OBJ   HProfGCTag = 0x08
	HProfGCTag_ROOT_JNI_MONITOR  HProfGCTag = 0x09
	HProfGCTag_ROOT_SYSTEM_CLASS HProfGCTag = 0x0A

	// Heap dump segment record tags (0x20-0x2F)
	HProfGCTag_CLASS_DUMP           HProfGCTag = 0x20
	HProfGCTag_INSTANCE_DUMP        HProfGCTag = 0x21
	HProfGCTag_OBJECT_ARRAY_DUMP    HProfGCTag = 0x22
	HProfGCTag_PRIMITIVE_ARRAY_DUMP HProfGCTag = 0x23

	// GC related tags (0x24-0x25)
	HProfGCTag_GC_START  HProfGCTag = 0x24
	HProfGCTag_GC_FINISH HProfGCTag = 0x25
)

type HeapStats struct {
	ObjectCount     int64
	InstanceBytes   int64
	ArrayBytes      int64
	TotalBytes      int64
	StringCount     int64 // HPROF UTF8 string table records (internal)
	StringBytes     int64 // Bytes from HPROF UTF8 string table
	JavaStringCount int64 // Actual java.lang.String instances on the heap
	JavaStringBytes int64 // Bytes from java.lang.String instances
	ArrayCount      int64
	ClassCount      int64
	// Primitive-array waste, accumulated while reading element bytes.
	SparseArrayCount  int64 // primitive arrays that are >=90% zero bytes
	SparseArrayBytes  int64 // total shallow bytes of those sparse arrays
	HumongousCount    int64 // arrays >= humongousArrayBytes
	HumongousBytes    int64 // total bytes of those humongous arrays
	LargestArrayBytes int64 // single largest primitive array
	ClassNameMap      map[uint64]string
	StringMap         map[uint64]string // HPROF UTF8 table (class/field names)
	StringValues      map[uint64]string // java.lang.String object id -> decoded value
	SystemProps       map[string]string
	Objects           map[uint64]*types.Object
	Classes           map[uint64]*types.ClassInfo
	GCEvents          []*GCEvent
	ClassLoaderCount  int
	Is64Bit           bool
	CreationTime      time.Time
	HeapSummary       *HeapSummary
	GCRootCount       int
	// GCRoots maps a GC-root object id to a human-readable root-type label
	// ("thread", "JNI global", "Java frame", …), used to anchor retention chains
	// at real roots.
	GCRoots map[uint64]string
	// StaticHolders models each class as a reference-graph node (keyed by class
	// id, which is itself an object id) whose edges are the class's static
	// object-typed fields. This lets retention chains reach objects held only
	// through static fields. Size is 0 so heap totals are unaffected.
	StaticHolders map[uint64]*types.Object
	// ObjectArrays holds object-array (Object[]) records as reference-graph nodes
	// (arrayId -> node with array->element edges). Kept separate from Objects so
	// the class histogram, object counts, and heap totals are unchanged; only the
	// reference graph consumes it, to let retention chains traverse collections.
	ObjectArrays map[uint64]*types.Object
}

type GCEvent struct {
	StartTime uint64
	EndTime   uint64
}

type HeapSummary struct {
	TotalLiveBytes          uint64
	TotalLiveInstances      uint64
	TotalBytesAllocated     uint64
	TotalInstancesAllocated uint64
}

type HProfParser struct {
	reader                 *bufio.Reader
	file                   *os.File
	identifierSize         int
	stats                  *HeapStats
	heapDumpFrameLeftBytes uint32
	seenClasses            map[uint64]bool
	totalClasses           int
	totalInstances         int64
	totalBytes             int64
	debug                  bool

	// Parse-time scratch state used to resolve java.lang.String values and
	// build the instance reference graph. Cleared once parsing completes.
	classFields    map[uint64][]hprofField // class id -> its own (declared) instance fields, in dump order
	arrayPayloads  map[uint64]arrayPayload // byte[]/char[] object id -> raw element bytes
	stringValueRef map[uint64]uint64       // String object id -> backing array object id
	stringCoder    map[uint64]int8         // String object id -> coder (0=LATIN1, 1=UTF16, -1=char[]/unknown)

	// primArrayClassIDs maps a primitive element type to the synthetic class id
	// used to aggregate its arrays (byte[], char[], …) so they appear in the
	// per-class histogram and "where memory goes" view like any other class.
	primArrayClassIDs map[HProfValueType]uint64

	// zeroScan is a reusable buffer for counting zero bytes in array payloads.
	// Allocated once so the hot path makes no per-array allocation.
	zeroScan []byte
}

// primArrayClassIDBase is the high sentinel range for synthetic primitive-array
// class ids (e.g. byte[]). It sits at the very top of the 64-bit address space
// where no real heap object can live, so it can't collide with a real id.
const primArrayClassIDBase uint64 = 0xFFFFFFFFFFFFFF00

// hprofField describes a single declared instance field of a class.
type hprofField struct {
	Name string
	Type HProfValueType
}

// arrayPayload holds the raw bytes of a primitive array we may need to decode
// (byte[] and char[] are the possible backing stores of a java.lang.String).
type arrayPayload struct {
	elemType HProfValueType
	data     []byte
}

// maxStringBackingBytes caps the size of byte[]/char[] payloads retained for
// string-value resolution. Strings longer than this are not the target of
// duplicate-string analysis, and the cap bounds memory on large heaps.
const maxStringBackingBytes = 1 << 16 // 64 KiB

const (
	// sparseArrayMinBytes ignores tiny arrays when flagging sparse (mostly-zero)
	// buffers; only arrays at least this large contribute to array waste.
	sparseArrayMinBytes = 1024
	// sparseZeroNumer/sparseZeroDenom express the zero-byte fraction (>=90%) at
	// which an array is considered empty/sparse.
	sparseZeroNumer = 9
	sparseZeroDenom = 10
	// humongousArrayBytes flags an unusually large single array.
	humongousArrayBytes = 1 << 20 // 1 MiB
)

// maxArrayRefs caps the number of non-null element edges retained per object
// array, bounding memory on pathologically large arrays. Collection backing
// arrays are almost always well under this.
const maxArrayRefs = 1 << 16

// maxRecordBytes is a sanity ceiling on a single length-prefixed allocation
// (UTF-8 string table entry). It guards against an attacker-controlled length
// driving a multi-gigabyte make(). Class/field/method names are tiny; this is a
// generous bound well above any legitimate value.
const maxRecordBytes = 64 << 20 // 64 MiB

type HProfRecord interface{}

type HProfRecordUTF8 struct {
	StringId uint64
	Value    string
}

type HProfRecordHeapDumpSegment struct {
	Timestamp uint32
	Length    uint32
}

type HProfRecordHeapDumpEnd struct {
	Timestamp uint32
}

type HProfLoadClass struct {
	ClassSerialNum uint32
	ClassId        uint64
	StackTraceId   uint32
	ClassNameId    uint64
}

type HProfRecordGCStart struct {
	Timestamp uint64
}

type HProfRecordGCFinish struct {
	Timestamp uint64
}

type HProfRecordHeapSummary struct {
	TotalLiveBytes          uint64
	TotalLiveInstances      uint64
	TotalBytesAllocated     uint64
	TotalInstancesAllocated uint64
}

type HProfRootJNIGlobal struct {
	ObjectId uint64
}

type HProfRootJNILocal struct {
	ObjectId uint64
}

type HProfRootJavaFrame struct {
	ObjectId uint64
}

type HProfRootStickyClass struct {
	ObjectId uint64
}

type HProfRootThreadObj struct {
	ThreadObjectId uint64
}

type ParseError struct {
	Op  string
	Err error
}

func (e *ParseError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("parse error during %s", e.Op)
	}
	return fmt.Sprintf("parse error during %s: %v", e.Op, e.Err)
}

func NewParser(filename string) (*HProfParser, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}

	return &HProfParser{
		// A large read buffer keeps multi-GB sequential parses I/O-efficient
		// (far fewer syscalls than the 4 KiB default), which matters on slow or
		// network-backed storage.
		reader:         bufio.NewReaderSize(file, 1<<20),
		file:           file,
		identifierSize: 8, // Default to 8, will be updated from header
		stats: &HeapStats{
			ClassNameMap: make(map[uint64]string),
			StringMap:    make(map[uint64]string),
			SystemProps:  make(map[string]string),
			Objects:      make(map[uint64]*types.Object),
			Classes:      make(map[uint64]*types.ClassInfo),
		},
		seenClasses:       make(map[uint64]bool),
		primArrayClassIDs: make(map[HProfValueType]uint64),
		zeroScan:          make([]byte, 32*1024),
		debug:             false,
	}, nil
}

// primArrayClassName returns the Java class name for a primitive array type.
func primArrayClassName(t HProfValueType) string {
	switch t {
	case HProfValueType_BOOL:
		return "boolean[]"
	case HProfValueType_CHAR:
		return "char[]"
	case HProfValueType_FLOAT:
		return "float[]"
	case HProfValueType_DOUBLE:
		return "double[]"
	case HProfValueType_BYTE:
		return "byte[]"
	case HProfValueType_SHORT:
		return "short[]"
	case HProfValueType_INT:
		return "int[]"
	case HProfValueType_LONG:
		return "long[]"
	default:
		return "unknown[]"
	}
}

// recordPrimitiveArray attributes a primitive array's shallow bytes to a
// synthetic per-element-type class (byte[], char[], …) so primitive arrays show
// up alongside instances and object arrays in the per-class views.
func (p *HProfParser) recordPrimitiveArray(et HProfValueType, arraySize uint32) {
	cid, ok := p.primArrayClassIDs[et]
	if !ok {
		cid = primArrayClassIDBase | uint64(et)
		p.primArrayClassIDs[et] = cid
		p.stats.Classes[cid] = &types.ClassInfo{
			ClassId:   cid,
			ClassName: primArrayClassName(et),
			IsArray:   true,
		}
	}
	ci := p.stats.Classes[cid]
	ci.InstanceCount++
	ci.ArrayBytes += int64(arraySize)
}

// countZeroBytes reads n bytes from the reader and returns how many were zero.
// It lets sparse/empty primitive arrays be detected without retaining payloads.
// It reuses p.zeroScan so the hot path allocates nothing per call.
func (p *HProfParser) countZeroBytes(n int64) (int64, error) {
	buf := p.zeroScan
	var zeros int64
	for n > 0 {
		chunk := int64(len(buf))
		if chunk > n {
			chunk = n
		}
		r, err := io.ReadFull(p.reader, buf[:chunk])
		zeros += int64(bytes.Count(buf[:r], zeroByte))
		n -= int64(r)
		if err != nil {
			if err == io.ErrUnexpectedEOF {
				err = io.EOF
			}
			return zeros, err
		}
	}
	return zeros, nil
}

// zeroByte is the needle for counting zero bytes (avoids per-call allocation).
var zeroByte = []byte{0}

// recordArrayWaste classifies a primitive array by its zero-byte fraction and
// size: arrays that are mostly zero are "sparse/empty" (reclaimable capacity)
// and large single arrays are flagged humongous.
func (p *HProfParser) recordArrayWaste(size, zeroBytes int64) {
	if size >= sparseArrayMinBytes && zeroBytes*sparseZeroDenom >= size*sparseZeroNumer {
		p.stats.SparseArrayCount++
		p.stats.SparseArrayBytes += size
	}
	if size >= humongousArrayBytes {
		p.stats.HumongousCount++
		p.stats.HumongousBytes += size
	}
	if size > p.stats.LargestArrayBytes {
		p.stats.LargestArrayBytes = size
	}
}

func (p *HProfParser) SetDebug(debug bool) {
	p.debug = debug
}

func (p *HProfParser) Close() error {
	if p.file != nil {
		return p.file.Close()
	}
	return nil
}

func (p *HProfParser) logDebug(format string, args ...interface{}) {
	if p.debug {
		fmt.Printf(format+"\n", args...)
	}
}

func (p *HProfParser) readUint32() (uint32, error) {
	var val uint32
	err := binary.Read(p.reader, binary.BigEndian, &val)
	if err != nil {
		return 0, p.newError("read uint32", err)
	}
	return val, nil
}

func (p *HProfParser) readUint64() (uint64, error) {
	var val uint64
	err := binary.Read(p.reader, binary.BigEndian, &val)
	if err != nil {
		return 0, p.newError("read uint64", err)
	}
	return val, nil
}

func (p *HProfParser) readID() (uint64, error) {
	switch p.identifierSize {
	case 4:
		val, err := p.readUint32()
		return uint64(val), err
	case 8:
		return p.readUint64()
	default:
		return 0, p.newError("read ID", fmt.Errorf("unsupported identifier size: %d", p.identifierSize))
	}
}

func (p *HProfParser) newError(op string, err error) error {
	return &ParseError{Op: op, Err: err}
}

// valueSize returns the in-dump byte width of an HPROF field/element value.
// Returns 0 for unrecognized types so callers can stop decoding safely.
func (p *HProfParser) valueSize(t HProfValueType) int {
	switch t {
	case HProfValueType_OBJECT:
		return p.identifierSize
	case HProfValueType_BOOL, HProfValueType_BYTE:
		return 1
	case HProfValueType_CHAR, HProfValueType_SHORT:
		return 2
	case HProfValueType_FLOAT, HProfValueType_INT:
		return 4
	case HProfValueType_DOUBLE, HProfValueType_LONG:
		return 8
	default:
		return 0
	}
}

// readIDFromBytes decodes a big-endian object identifier from a byte slice.
func readIDFromBytes(b []byte, idSize int) uint64 {
	switch idSize {
	case 4:
		return uint64(binary.BigEndian.Uint32(b))
	case 8:
		return binary.BigEndian.Uint64(b)
	default:
		return 0
	}
}

// decodeInstanceFields walks the class hierarchy (this class, then each
// superclass) and reads field values from the raw instance-data block.
// Object-typed fields are recorded as references on obj. For java.lang.String
// instances it captures the backing-array id and coder so the value can be
// resolved once the whole heap has been parsed. Decoding stops safely at the
// first unknown field type or buffer boundary — the outer stream is unaffected
// because the caller already consumed exactly dataSize bytes.
func (p *HProfParser) decodeInstanceFields(obj *types.Object, classID uint64, data []byte) {
	isString := false
	if ci, ok := p.stats.Classes[classID]; ok {
		isString = ci.ClassName == "java/lang/String" || ci.ClassName == "java.lang.String"
	}
	if isString {
		p.stringCoder[obj.ObjectId] = 0 // default LATIN1; overwritten if a coder field is present
	}
	if len(data) == 0 {
		return
	}

	off := 0
	visited := make(map[uint64]bool)
	for cid := classID; cid != 0 && !visited[cid]; {
		visited[cid] = true
		for _, f := range p.classFields[cid] {
			size := p.valueSize(f.Type)
			if size == 0 || off+size > len(data) {
				return // unknown type or past the buffer — stop safely
			}
			switch {
			case f.Type == HProfValueType_OBJECT:
				ref := readIDFromBytes(data[off:off+size], p.identifierSize)
				if ref != 0 {
					obj.References = append(obj.References, types.Reference{
						SourceId: obj.ObjectId,
						TargetId: ref,
						RefType:  f.Name,
					})
					if isString && f.Name == "value" {
						p.stringValueRef[obj.ObjectId] = ref
					}
				}
			case isString && f.Name == "coder" && f.Type == HProfValueType_BYTE:
				p.stringCoder[obj.ObjectId] = int8(data[off])
			}
			off += size
		}
		if ci, ok := p.stats.Classes[cid]; ok {
			cid = ci.SuperClassId
		} else {
			break
		}
	}
}

// resolveStringValues decodes the value of every java.lang.String instance
// from its backing array and populates stats.StringValues. Parse-time scratch
// buffers are released afterwards to reclaim memory before analysis.
func (p *HProfParser) resolveStringValues() {
	for objID, arrID := range p.stringValueRef {
		payload, ok := p.arrayPayloads[arrID]
		if !ok {
			continue // backing array was too large to retain, or absent
		}
		coder, ok := p.stringCoder[objID]
		if !ok {
			coder = -1
		}
		p.stats.StringValues[objID] = decodeStringBytes(payload, coder)
	}

	// Release scratch state — it is large and no longer needed.
	p.arrayPayloads = nil
	p.classFields = nil
	p.stringValueRef = nil
	p.stringCoder = nil
}

// decodeStringBytes turns a String's backing array into a Go string.
//
//   - char[] (pre-JDK 9): UTF-16, big-endian (the HPROF writer emits char
//     elements big-endian).
//   - byte[] (JDK 9+ compact strings): coder 0 = LATIN1 (one byte per char),
//     coder 1 = UTF-16. The byte order of UTF-16 byte[] is architecture
//     dependent and not recorded in the dump; little-endian (x86/ARM, the
//     overwhelming majority of dumps) is assumed.
func decodeStringBytes(payload arrayPayload, coder int8) string {
	b := payload.data
	if payload.elemType == HProfValueType_CHAR {
		return decodeUTF16(b, true)
	}
	// byte[] backing store
	if coder == 1 {
		return decodeUTF16(b, false)
	}
	// LATIN1: each byte is a code point in U+0000..U+00FF.
	runes := make([]rune, len(b))
	for i, c := range b {
		runes[i] = rune(c)
	}
	return string(runes)
}

// decodeUTF16 decodes 2-byte UTF-16 code units (bigEndian selects byte order).
func decodeUTF16(b []byte, bigEndian bool) string {
	n := len(b) / 2
	units := make([]uint16, n)
	for i := 0; i < n; i++ {
		if bigEndian {
			units[i] = uint16(b[2*i])<<8 | uint16(b[2*i+1])
		} else {
			units[i] = uint16(b[2*i+1])<<8 | uint16(b[2*i])
		}
	}
	return string(utf16.Decode(units))
}

func (p *HProfParser) parseHeader() error {
	// Read magic string "JAVA PROFILE "
	magic := make([]byte, 13)
	if _, err := io.ReadFull(p.reader, magic); err != nil {
		return p.newError("parse header", err)
	}
	if string(magic) != "JAVA PROFILE " {
		return p.newError("parse header", fmt.Errorf("invalid magic string: %s", string(magic)))
	}

	// Read version string (null-terminated)
	version := make([]byte, 0, 10)
	for {
		b, err := p.reader.ReadByte()
		if err != nil {
			return p.newError("parse header", err)
		}
		if b == 0 {
			break
		}
		version = append(version, b)
	}
	p.logDebug("Found HPROF file: JAVA PROFILE %s", string(version))

	// Read identifier size
	var identifierSize uint32
	if err := binary.Read(p.reader, binary.BigEndian, &identifierSize); err != nil {
		return p.newError("parse header", err)
	}
	p.identifierSize = int(identifierSize)
	p.logDebug("Read identifier size bytes: %d (0x%x)", p.identifierSize, p.identifierSize)

	// Set 64-bit flag based on identifier size
	p.stats.Is64Bit = p.identifierSize == 8

	// Validate identifier size
	if p.identifierSize != 4 && p.identifierSize != 8 {
		return p.newError("parse header", fmt.Errorf("invalid identifier size: %d", p.identifierSize))
	}

	// Read high resolution timestamp (microseconds since 1970)
	var timestamp uint64
	if err := binary.Read(p.reader, binary.BigEndian, &timestamp); err != nil {
		return p.newError("parse header", err)
	}

	// Convert microseconds to time.Time
	p.stats.CreationTime = time.Unix(int64(timestamp/1000000), int64(timestamp%1000000)*1000)

	return nil
}

func (p *HProfParser) Parse() (stats *HeapStats, err error) {
	// A malformed or hostile dump can drive the parser into an out-of-range slice
	// or a bad cast; recover so a bad upload returns an error instead of crashing
	// the server process.
	defer func() {
		if r := recover(); r != nil {
			stats = nil
			err = fmt.Errorf("malformed heap dump: %v", r)
		}
	}()

	if err := p.parseHeader(); err != nil {
		return nil, fmt.Errorf("error parsing header: %v", err)
	}

	p.stats = &HeapStats{
		ClassNameMap:     make(map[uint64]string),
		StringMap:        make(map[uint64]string),
		StringValues:     make(map[uint64]string),
		SystemProps:      make(map[string]string),
		GCEvents:         make([]*GCEvent, 0),
		ClassLoaderCount: 0,
		CreationTime:     time.Now(), // Default to current time
		Objects:          make(map[uint64]*types.Object),
		Classes:          make(map[uint64]*types.ClassInfo),
		ObjectArrays:     make(map[uint64]*types.Object),
		GCRoots:          make(map[uint64]string),
		StaticHolders:    make(map[uint64]*types.Object),
	}

	// Scratch state for string-value resolution and reference-graph construction.
	p.classFields = make(map[uint64][]hprofField)
	p.arrayPayloads = make(map[uint64]arrayPayload)
	p.stringValueRef = make(map[uint64]uint64)
	p.stringCoder = make(map[uint64]int8)

	p.logDebug("Starting heap dump analysis...")

	classLoaders := make(map[uint64]bool)

	for {
		// Read record header
		tag, err := p.reader.ReadByte()
		if err != nil {
			if err == io.EOF {
				break // End of file reached normally
			}
			return nil, p.newError("parse record", err)
		}

		var timestamp uint32
		if err := binary.Read(p.reader, binary.BigEndian, &timestamp); err != nil {
			if err == io.EOF {
				break // Unexpected EOF, but we can still return what we have
			}
			return nil, p.newError("parse record", err)
		}

		var length uint32
		if err := binary.Read(p.reader, binary.BigEndian, &length); err != nil {
			if err == io.EOF {
				break // Unexpected EOF, but we can still return what we have
			}
			return nil, p.newError("parse record", err)
		}

		// Skip unknown record types
		recordType := HProfRecordType(tag)
		if _, ok := HProfRecordType_name[recordType]; !ok {
			p.logDebug("Skipping unknown record type: 0x%x", tag)
			if _, err := io.CopyN(io.Discard, p.reader, int64(length)); err != nil {
				if err == io.EOF {
					break // Unexpected EOF, but we can still return what we have
				}
				return nil, p.newError("parse record", err)
			}
			continue
		}

		// Only log important record types
		switch recordType {
		case HProfRecordType_HEAP_DUMP, HProfRecordType_HEAP_DUMP_SEGMENT:
			p.logDebug("Processing heap dump (length: %d bytes)", length)
		case HProfRecordType_HEAP_DUMP_END:
			p.logDebug("Found heap dump end marker")
		case HProfRecordType_LOAD_CLASS:
			// Don't log individual class loads as they are too frequent
		case HProfRecordType_STRING_IN_UTF8:
			// Don't log individual strings as they are too frequent
		case HProfRecordType_STACK_FRAME:
			// Don't log stack frames
		case HProfRecordType_STACK_TRACE:
			// Don't log stack traces
		default:
			p.logDebug("Processing record type: %s (length: %d bytes)", HProfRecordType_name[recordType], length)
		}

		// Process known record types
		switch recordType {
		case HProfRecordType_STRING_IN_UTF8:
			id, err := p.readID()
			if err != nil {
				if err == io.EOF {
					break // Unexpected EOF, but we can still return what we have
				}
				return nil, p.newError("parse record", err)
			}

			// Read string length (remaining bytes after ID). Guard against a
			// record length shorter than the id (which would underflow the
			// subtraction) and against an absurdly large, attacker-controlled size.
			if length < uint32(p.identifierSize) {
				return nil, p.newError("parse record", fmt.Errorf("string record length %d shorter than id size", length))
			}
			stringLen := length - uint32(p.identifierSize)
			if stringLen > maxRecordBytes {
				return nil, p.newError("parse record", fmt.Errorf("string record length %d exceeds cap", stringLen))
			}
			stringBytes := make([]byte, stringLen)
			if _, err := io.ReadFull(p.reader, stringBytes); err != nil {
				if err == io.EOF {
					break // Unexpected EOF, but we can still return what we have
				}
				return nil, p.newError("parse record", err)
			}
			p.stats.StringMap[id] = string(stringBytes)
			p.stats.StringCount++
			p.stats.StringBytes += int64(stringLen)

		case HProfRecordType_LOAD_CLASS:
			var serialNum uint32
			if err := binary.Read(p.reader, binary.BigEndian, &serialNum); err != nil {
				if err == io.EOF {
					break // Unexpected EOF, but we can still return what we have
				}
				return nil, p.newError("parse record", err)
			}

			classId, err := p.readID()
			if err != nil {
				if err == io.EOF {
					break // Unexpected EOF, but we can still return what we have
				}
				return nil, p.newError("parse record", err)
			}

			// Skip stack trace ID since we don't use it
			if _, err := io.CopyN(io.Discard, p.reader, 4); err != nil {
				if err == io.EOF {
					break // Unexpected EOF, but we can still return what we have
				}
				return nil, p.newError("parse record", err)
			}

			classNameId, err := p.readID()
			if err != nil {
				if err == io.EOF {
					break // Unexpected EOF, but we can still return what we have
				}
				return nil, p.newError("parse record", err)
			}

			if className, ok := p.stats.StringMap[classNameId]; ok {
				p.stats.ClassNameMap[classId] = className

				if !p.seenClasses[classId] {
					p.seenClasses[classId] = true
					p.totalClasses++
				}

				// Update or create class entry
				if existing, exists := p.stats.Classes[classId]; exists {
					// CLASS_DUMP may have created this without a name — fill it in
					if existing.ClassName == "" {
						existing.ClassName = className
					}
				} else {
					p.stats.Classes[classId] = &types.ClassInfo{
						ClassId:       classId,
						ClassName:     className,
						InstanceCount: 0,
						InstanceSize:  0,
						ArrayBytes:    0,
					}
				}
			}

			// Check if this is a classloader
			if info, ok := p.stats.Classes[classId]; ok {
				if strings.HasSuffix(info.ClassName, "ClassLoader") {
					classLoaders[classId] = true
					p.stats.ClassLoaderCount++
				}
			}

		case HProfRecordType_HEAP_DUMP, HProfRecordType_HEAP_DUMP_SEGMENT:
			// A classic single HEAP_DUMP (0x0C) record and a HEAP_DUMP_SEGMENT
			// (0x1C) carry the same GC sub-records; parse both the same way.
			p.heapDumpFrameLeftBytes = length
			if err := p.parseHeapDumpSegment(); err != nil {
				if err == io.EOF {
					break // Unexpected EOF, but we can still return what we have
				}
				return nil, p.newError("parse heap dump segment", err)
			}

		case HProfRecordType_HEAP_DUMP_END:
			// Calculate total bytes and instances
			var totalLiveBytes uint64
			var totalLiveInstances uint64
			for _, stats := range p.stats.Classes {
				totalLiveInstances += uint64(stats.InstanceCount)
				totalLiveBytes += uint64(stats.ArrayBytes + stats.InstanceSize)
			}

			p.stats.HeapSummary = &HeapSummary{
				TotalLiveBytes:          totalLiveBytes,
				TotalLiveInstances:      totalLiveInstances,
				TotalBytesAllocated:     totalLiveBytes,
				TotalInstancesAllocated: totalLiveInstances,
			}

		default:
			// Skip other known but unhandled record types
			p.logDebug("Skipping record type: %s", HProfRecordType_name[recordType])
			if _, err := io.CopyN(io.Discard, p.reader, int64(length)); err != nil {
				if err == io.EOF {
					break // Unexpected EOF, but we can still return what we have
				}
				return nil, p.newError("parse record", err)
			}
		}
	}

	// Resolve java.lang.String values now that all backing arrays are available.
	p.resolveStringValues()

	// Set the final object count from the Objects map
	// (includes both instances counted during parseInstanceDump and arrays)
	p.stats.ObjectCount = int64(len(p.stats.Objects))
	p.stats.ClassCount = int64(len(p.stats.Classes))

	// Count actual java.lang.String instances by looking at class info
	for _, classInfo := range p.stats.Classes {
		if classInfo.ClassName == "java/lang/String" || classInfo.ClassName == "java.lang.String" {
			p.stats.JavaStringCount = int64(classInfo.InstanceCount)
			p.stats.JavaStringBytes = classInfo.InstanceSize
			break
		}
	}

	// Ensure HeapSummary is always populated (may not have been set if HEAP_DUMP_END was absent)
	if p.stats.HeapSummary == nil {
		var totalLiveBytes uint64
		var totalLiveInstances uint64
		for _, stats := range p.stats.Classes {
			totalLiveInstances += uint64(stats.InstanceCount)
			totalLiveBytes += uint64(stats.ArrayBytes + stats.InstanceSize)
		}
		p.stats.HeapSummary = &HeapSummary{
			TotalLiveBytes:          totalLiveBytes,
			TotalLiveInstances:      totalLiveInstances,
			TotalBytesAllocated:     totalLiveBytes,
			TotalInstancesAllocated: totalLiveInstances,
		}
	}

	// If we have any data, return it even if we hit EOF
	if len(p.stats.Classes) > 0 || len(p.stats.StringMap) > 0 {
		return p.stats, nil
	}

	return nil, fmt.Errorf("no heap dump data found")
}

func (p *HProfParser) GetStats() *HeapStats {
	return p.stats
}

func (p *HProfParser) parseHeapDumpSegment() error {
	var parsingErrors []error
	for p.heapDumpFrameLeftBytes > 0 {
		// Read tag byte directly to better handle EOF and invalid data
		tagByte, err := p.reader.ReadByte()
		if err != nil {
			if err == io.EOF {
				p.heapDumpFrameLeftBytes = 0
				return nil
			}
			return p.newError("read GC tag", err)
		}
		p.heapDumpFrameLeftBytes--

		tag := HProfGCTag(tagByte)
		if tagByte == 0 {
			p.logDebug("Invalid zero tag found at offset, attempting to resync. Bytes left: %d", p.heapDumpFrameLeftBytes)
			peekBytes := make([]byte, 16)
			n, _ := p.reader.Read(peekBytes)
			p.logDebug("Next 16 bytes after zero tag: % x", peekBytes[:n])
			p.heapDumpFrameLeftBytes -= uint32(n)
			// Skip only 1 byte and continue
			continue
		}

		p.logDebug("Processing GC tag: 0x%02x", tag)

		switch tag {
		case HProfGCTag_ROOT_THREAD_OBJ,
			HProfGCTag_ROOT_JNI_GLOBAL,
			HProfGCTag_ROOT_JNI_LOCAL,
			HProfGCTag_ROOT_JAVA_FRAME,
			HProfGCTag_ROOT_NATIVE_STACK,
			HProfGCTag_ROOT_STICKY_CLASS,
			HProfGCTag_ROOT_THREAD_BLOCK,
			HProfGCTag_ROOT_MONITOR_USED,
			HProfGCTag_ROOT_JNI_MONITOR,
			HProfGCTag_ROOT_SYSTEM_CLASS:
			if err := p.parseGCRoot(tag); err != nil {
				if err == io.EOF {
					p.heapDumpFrameLeftBytes = 0
					return nil
				}
				return err
			}

		case HProfGCTag_CLASS_DUMP:
			// Class dump record
			if err := p.parseClassDump(); err != nil {
				parsingErrors = append(parsingErrors, err)
			}

		case HProfGCTag_INSTANCE_DUMP:
			// Instance dump record
			if err := p.parseInstanceDump(); err != nil {
				parsingErrors = append(parsingErrors, err)
			}

		case HProfGCTag_OBJECT_ARRAY_DUMP:
			// Object array dump
			if p.debug {
				p.logDebug("parseObjectArrayDump called")
			}
			if err := p.parseObjectArrayDump(); err != nil {
				parsingErrors = append(parsingErrors, err)
			}

		case HProfGCTag_PRIMITIVE_ARRAY_DUMP:
			// Primitive array dump
			if p.debug {
				p.logDebug("parsePrimitiveArrayDump called")
			}
			if err := p.parsePrimitiveArrayDump(); err != nil {
				parsingErrors = append(parsingErrors, err)
			}

		default:
			// Unknown tag - log and skip
			p.logDebug("Skipping unknown GC tag: 0x%02x at offset, bytes left: %d", tag, p.heapDumpFrameLeftBytes)
			peekBytes := make([]byte, 16)
			n, _ := p.reader.Read(peekBytes)
			p.logDebug("Next 16 bytes after unknown tag: % x", peekBytes[:n])
			p.heapDumpFrameLeftBytes -= uint32(n)
			// Skip only 1 byte and continue
			continue
		}
	}

	if len(parsingErrors) > 0 {
		p.logDebug("parseHeapDumpSegment completed with %d non-fatal errors", len(parsingErrors))
		for _, e := range parsingErrors {
			p.logDebug("  warning: %v", e)
		}
	}

	return nil
}

// parseGCRoot reads one GC-root sub-record: the leading object id followed by the
// type-specific trailing fields (thread serials, frame numbers, etc.), recording
// the root id and its type. Consuming the correct number of trailing bytes keeps
// the heap-dump segment in sync.
func (p *HProfParser) parseGCRoot(tag HProfGCTag) error {
	objectID, err := p.readID()
	if err != nil {
		return err
	}
	p.heapDumpFrameLeftBytes -= uint32(p.identifierSize)

	if trailing := rootTrailingBytes(tag, p.identifierSize); trailing > 0 {
		if _, err := io.CopyN(io.Discard, p.reader, int64(trailing)); err != nil {
			return err
		}
		p.heapDumpFrameLeftBytes -= uint32(trailing)
	}

	p.stats.GCRootCount++
	if _, seen := p.stats.GCRoots[objectID]; !seen {
		p.stats.GCRoots[objectID] = rootTypeName(tag)
	}
	if tag == HProfGCTag_ROOT_THREAD_OBJ {
		if obj, ok := p.stats.Objects[objectID]; ok {
			obj.IsThread = true
		}
	}
	p.logDebug("Found GC root 0x%x (%s)", objectID, rootTypeName(tag))
	return nil
}

// rootTrailingBytes returns the number of bytes following the leading object id
// in a GC-root sub-record, per the HPROF format.
func rootTrailingBytes(tag HProfGCTag, idSize int) int {
	switch tag {
	case HProfGCTag_ROOT_JNI_GLOBAL:
		return idSize // JNI global ref id
	case HProfGCTag_ROOT_THREAD_OBJ:
		return 8 // thread serial (u4) + stack-trace serial (u4)
	case HProfGCTag_ROOT_JNI_LOCAL, HProfGCTag_ROOT_JAVA_FRAME, HProfGCTag_ROOT_JNI_MONITOR:
		return 8 // thread serial (u4) + frame number / depth (u4)
	case HProfGCTag_ROOT_NATIVE_STACK, HProfGCTag_ROOT_THREAD_BLOCK:
		return 4 // thread serial (u4)
	default: // STICKY_CLASS, MONITOR_USED, SYSTEM_CLASS, UNKNOWN: id only
		return 0
	}
}

// rootTypeName returns a short human label for a GC-root tag.
func rootTypeName(tag HProfGCTag) string {
	switch tag {
	case HProfGCTag_ROOT_JNI_GLOBAL:
		return "JNI global"
	case HProfGCTag_ROOT_JNI_LOCAL:
		return "JNI local"
	case HProfGCTag_ROOT_JAVA_FRAME:
		return "Java frame"
	case HProfGCTag_ROOT_NATIVE_STACK:
		return "native stack"
	case HProfGCTag_ROOT_STICKY_CLASS:
		return "sticky class"
	case HProfGCTag_ROOT_THREAD_BLOCK:
		return "thread block"
	case HProfGCTag_ROOT_MONITOR_USED:
		return "monitor"
	case HProfGCTag_ROOT_THREAD_OBJ:
		return "thread"
	case HProfGCTag_ROOT_JNI_MONITOR:
		return "JNI monitor"
	case HProfGCTag_ROOT_SYSTEM_CLASS:
		return "system class"
	default:
		return "GC root"
	}
}

func (p *HProfParser) parseInstanceDump() error {
	// Read object ID
	objectId, err := p.readID()
	if err != nil {
		return p.newError("read object ID", err)
	}
	p.heapDumpFrameLeftBytes -= uint32(p.identifierSize)

	// Skip stack trace serial number
	if _, err := io.CopyN(io.Discard, p.reader, 4); err != nil {
		return p.newError("skip stack trace serial", err)
	}
	p.heapDumpFrameLeftBytes -= 4

	// Read class ID
	classId, err := p.readID()
	if err != nil {
		return p.newError("read class ID", err)
	}
	p.heapDumpFrameLeftBytes -= uint32(p.identifierSize)

	// Read instance field data size (number of bytes of field values).
	var dataSize uint32
	if err := binary.Read(p.reader, binary.BigEndian, &dataSize); err != nil {
		return p.newError("read data size", err)
	}
	p.heapDumpFrameLeftBytes -= 4

	// Read the entire field-value block into a buffer. Consuming exactly
	// dataSize bytes keeps the outer stream in sync no matter how much of the
	// class hierarchy we can decode, so partial/missing field metadata can
	// never desync the parser.
	// The field block must fit within the bytes left in this heap-dump segment;
	// a larger claimed size means the record is corrupt.
	if dataSize > p.heapDumpFrameLeftBytes {
		return p.newError("read instance field data", fmt.Errorf("field size %d exceeds remaining segment %d", dataSize, p.heapDumpFrameLeftBytes))
	}
	var fieldData []byte
	if dataSize > 0 {
		fieldData = make([]byte, dataSize)
		if _, err := io.ReadFull(p.reader, fieldData); err != nil {
			return p.newError("read instance field data", err)
		}
		p.heapDumpFrameLeftBytes -= dataSize
	}

	// Calculate instance size including object header
	instanceSize := int64(dataSize) + int64(ObjectHeaderSize)

	// Create and store the object entry
	obj := &types.Object{
		ObjectId:   objectId,
		ClassId:    classId,
		Size:       instanceSize,
		References: make([]types.Reference, 0),
	}

	// Decode the field block: capture object references for the reference graph
	// and, for java.lang.String instances, the backing array id and coder.
	p.decodeInstanceFields(obj, classId, fieldData)

	// Check if this is a thread object
	if classInfo, ok := p.stats.Classes[classId]; ok {
		if classInfo.ClassName == "java/lang/Thread" || classInfo.ClassName == "java.lang.Thread" {
			obj.IsThread = true
			p.logDebug("Found Thread instance: 0x%x", objectId)
		}
	}

	// Store object
	p.stats.Objects[objectId] = obj

	// Update class statistics
	if classInfo, ok := p.stats.Classes[classId]; ok {
		classInfo.InstanceCount++
		classInfo.InstanceSize += instanceSize
	}

	// Update global statistics
	p.stats.ObjectCount++
	p.stats.InstanceBytes += instanceSize
	p.stats.TotalBytes += instanceSize
	p.totalInstances++
	p.totalBytes += instanceSize

	p.logDebug("Parsed instance dump: obj=0x%x class=0x%x size=%d", objectId, classId, instanceSize)

	return nil
}

func (p *HProfParser) parseClassDump() error {
	// Read class ID
	classID, err := p.readID()
	if err != nil {
		if err == io.EOF {
			p.heapDumpFrameLeftBytes = 0
			return nil
		}
		return p.newError("read class ID", err)
	}
	p.heapDumpFrameLeftBytes -= uint32(p.identifierSize)

	// Skip stack trace serial number
	if _, err := io.CopyN(io.Discard, p.reader, 4); err != nil {
		if err == io.EOF {
			p.heapDumpFrameLeftBytes = 0
			return nil
		}
		return p.newError("skip stack trace serial", err)
	}
	p.heapDumpFrameLeftBytes -= 4

	// Read superclass ID
	superClassID, err := p.readID()
	if err != nil {
		if err == io.EOF {
			p.heapDumpFrameLeftBytes = 0
			return nil
		}
		return p.newError("read superclass ID", err)
	}
	p.heapDumpFrameLeftBytes -= uint32(p.identifierSize)

	// Skip class loader ID and signers ID
	for i := 0; i < 2; i++ {
		if _, err := io.CopyN(io.Discard, p.reader, int64(p.identifierSize)); err != nil {
			if err == io.EOF {
				p.heapDumpFrameLeftBytes = 0
				return nil
			}
			return p.newError("skip class loader/signers ID", err)
		}
		p.heapDumpFrameLeftBytes -= uint32(p.identifierSize)
	}

	// Skip protection domain ID
	if _, err := io.CopyN(io.Discard, p.reader, int64(p.identifierSize)); err != nil {
		if err == io.EOF {
			p.heapDumpFrameLeftBytes = 0
			return nil
		}
		return p.newError("skip protection domain ID", err)
	}
	p.heapDumpFrameLeftBytes -= uint32(p.identifierSize)

	// Skip reserved fields
	if _, err := io.CopyN(io.Discard, p.reader, int64(2*p.identifierSize)); err != nil {
		if err == io.EOF {
			p.heapDumpFrameLeftBytes = 0
			return nil
		}
		return p.newError("skip reserved fields", err)
	}
	p.heapDumpFrameLeftBytes -= uint32(2 * p.identifierSize)

	// Read instance size
	var instanceSize uint32
	if err := binary.Read(p.reader, binary.BigEndian, &instanceSize); err != nil {
		if err == io.EOF {
			p.heapDumpFrameLeftBytes = 0
			return nil
		}
		return p.newError("read instance size", err)
	}
	p.heapDumpFrameLeftBytes -= 4

	// Skip constant pool
	var constantPoolSize uint16
	if err := binary.Read(p.reader, binary.BigEndian, &constantPoolSize); err != nil {
		if err == io.EOF {
			p.heapDumpFrameLeftBytes = 0
			return nil
		}
		return p.newError("read constant pool size", err)
	}
	p.heapDumpFrameLeftBytes -= 2

	for i := uint16(0); i < constantPoolSize; i++ {
		// Skip index
		if _, err := io.CopyN(io.Discard, p.reader, 2); err != nil {
			if err == io.EOF {
				p.heapDumpFrameLeftBytes = 0
				return nil
			}
			return p.newError("skip constant pool index", err)
		}
		p.heapDumpFrameLeftBytes -= 2

		// Skip type
		var valueType HProfValueType
		if err := binary.Read(p.reader, binary.BigEndian, &valueType); err != nil {
			if err == io.EOF {
				p.heapDumpFrameLeftBytes = 0
				return nil
			}
			return p.newError("read constant pool type", err)
		}
		p.heapDumpFrameLeftBytes--

		// Skip value based on type
		var valueSize int64
		switch valueType {
		case HProfValueType_OBJECT:
			valueSize = int64(p.identifierSize)
		case HProfValueType_BOOL, HProfValueType_BYTE:
			valueSize = 1
		case HProfValueType_CHAR, HProfValueType_SHORT:
			valueSize = 2
		case HProfValueType_FLOAT, HProfValueType_INT:
			valueSize = 4
		case HProfValueType_DOUBLE, HProfValueType_LONG:
			valueSize = 8
		}

		if valueSize > 0 {
			if _, err := io.CopyN(io.Discard, p.reader, valueSize); err != nil {
				if err == io.EOF {
					p.heapDumpFrameLeftBytes = 0
					return nil
				}
				return p.newError("skip constant pool value", err)
			}
			p.heapDumpFrameLeftBytes -= uint32(valueSize)
		}
	}

	// Skip static fields
	var staticFieldCount uint16
	if err := binary.Read(p.reader, binary.BigEndian, &staticFieldCount); err != nil {
		if err == io.EOF {
			p.heapDumpFrameLeftBytes = 0
			return nil
		}
		return p.newError("read static field count", err)
	}
	p.heapDumpFrameLeftBytes -= 2

	var staticRefs []types.Reference
	for i := uint16(0); i < staticFieldCount; i++ {
		// Read field name ID (needed to label object-typed static references).
		fieldNameID, err := p.readID()
		if err != nil {
			if err == io.EOF {
				p.heapDumpFrameLeftBytes = 0
				return nil
			}
			return p.newError("read static field name ID", err)
		}
		p.heapDumpFrameLeftBytes -= uint32(p.identifierSize)

		// Read field type
		var fieldType HProfValueType
		if err := binary.Read(p.reader, binary.BigEndian, &fieldType); err != nil {
			if err == io.EOF {
				p.heapDumpFrameLeftBytes = 0
				return nil
			}
			return p.newError("read static field type", err)
		}
		p.heapDumpFrameLeftBytes--

		if fieldType == HProfValueType_OBJECT {
			// Read the referenced object id and record a static-field edge.
			target, err := p.readID()
			if err != nil {
				if err == io.EOF {
					p.heapDumpFrameLeftBytes = 0
					return nil
				}
				return p.newError("read static field value", err)
			}
			p.heapDumpFrameLeftBytes -= uint32(p.identifierSize)
			if target != 0 {
				staticRefs = append(staticRefs, types.Reference{
					SourceId: classID,
					TargetId: target,
					RefType:  p.stats.StringMap[fieldNameID],
				})
			}
			continue
		}

		// Skip non-object field values.
		var fieldSize int64
		switch fieldType {
		case HProfValueType_BOOL, HProfValueType_BYTE:
			fieldSize = 1
		case HProfValueType_CHAR, HProfValueType_SHORT:
			fieldSize = 2
		case HProfValueType_FLOAT, HProfValueType_INT:
			fieldSize = 4
		case HProfValueType_DOUBLE, HProfValueType_LONG:
			fieldSize = 8
		}
		if fieldSize > 0 {
			if _, err := io.CopyN(io.Discard, p.reader, fieldSize); err != nil {
				if err == io.EOF {
					p.heapDumpFrameLeftBytes = 0
					return nil
				}
				return p.newError("skip static field value", err)
			}
			p.heapDumpFrameLeftBytes -= uint32(fieldSize)
		}
	}
	if len(staticRefs) > 0 {
		p.stats.StaticHolders[classID] = &types.Object{
			ObjectId:   classID,
			ClassId:    classID,
			Size:       0, // synthetic node; the class's own memory isn't counted here
			References: staticRefs,
		}
	}

	// Read instance fields (just field name + type, no values here)
	var instanceFieldCount uint16
	if err := binary.Read(p.reader, binary.BigEndian, &instanceFieldCount); err != nil {
		if err == io.EOF {
			p.heapDumpFrameLeftBytes = 0
			return nil
		}
		return p.newError("read instance field count", err)
	}
	p.heapDumpFrameLeftBytes -= 2

	fields := make([]hprofField, 0, instanceFieldCount)
	for i := uint16(0); i < instanceFieldCount; i++ {
		// Read field name ID
		fieldNameID, err := p.readID()
		if err != nil {
			if err == io.EOF {
				p.heapDumpFrameLeftBytes = 0
				return nil
			}
			return p.newError("read instance field name ID", err)
		}
		p.heapDumpFrameLeftBytes -= uint32(p.identifierSize)

		// Read field type
		var fieldType HProfValueType
		if err := binary.Read(p.reader, binary.BigEndian, &fieldType); err != nil {
			if err == io.EOF {
				p.heapDumpFrameLeftBytes = 0
				return nil
			}
			return p.newError("read instance field type", err)
		}
		p.heapDumpFrameLeftBytes--

		fields = append(fields, hprofField{
			Name: p.stats.StringMap[fieldNameID],
			Type: fieldType,
		})
	}
	// Field-value blocks in INSTANCE_DUMP records are laid out in this same
	// declared order (this class first, then each superclass), so retain it.
	p.classFields[classID] = fields

	// Update class stats — merge with any existing entry from LOAD_CLASS
	if existing, exists := p.stats.Classes[classID]; exists {
		// LOAD_CLASS already created this entry with a class name — merge in CLASS_DUMP data
		existing.SuperClassId = superClassID
		if existing.InstanceSize == 0 {
			existing.InstanceSize = int64(instanceSize)
		}
	} else {
		// CLASS_DUMP arrived before LOAD_CLASS — look up name from ClassNameMap
		className := ""
		if p.stats.ClassNameMap != nil {
			if cn, ok := p.stats.ClassNameMap[classID]; ok {
				className = cn
			}
		}
		p.stats.Classes[classID] = &types.ClassInfo{
			ClassId:      classID,
			ClassName:    className,
			SuperClassId: superClassID,
			InstanceSize: int64(instanceSize),
		}
		p.totalClasses++
	}

	return nil
}

func (p *HProfParser) parseObjectArrayDump() error {
	// Read array ID
	arrayID, err := p.readID()
	if err != nil {
		if err == io.EOF {
			p.heapDumpFrameLeftBytes = 0
			return nil
		}
		return p.newError("read array ID", err)
	}
	p.heapDumpFrameLeftBytes -= uint32(p.identifierSize)

	// Skip stack trace serial number
	if _, err := io.CopyN(io.Discard, p.reader, 4); err != nil {
		if err == io.EOF {
			p.heapDumpFrameLeftBytes = 0
			return nil
		}
		return p.newError("skip stack trace serial", err)
	}
	p.heapDumpFrameLeftBytes -= 4

	// Read array length
	var arrayLength uint32
	if err := binary.Read(p.reader, binary.BigEndian, &arrayLength); err != nil {
		if err == io.EOF {
			p.heapDumpFrameLeftBytes = 0
			return nil
		}
		return p.newError("read array length", err)
	}
	p.heapDumpFrameLeftBytes -= 4

	// Read array class ID
	arrayClassID, err := p.readID()
	if err != nil {
		if err == io.EOF {
			p.heapDumpFrameLeftBytes = 0
			return nil
		}
		return p.newError("read array class ID", err)
	}
	p.heapDumpFrameLeftBytes -= uint32(p.identifierSize)

	// Read array elements and record array->element edges for the reference
	// graph. Each element is an object identifier; zero ids are null slots. The
	// element block must fit within the remaining segment (a 64-bit multiply
	// avoids an overflow making a corrupt size look small).
	if uint64(arrayLength)*uint64(p.identifierSize) > uint64(p.heapDumpFrameLeftBytes) {
		return p.newError("read array elements", fmt.Errorf("array element block exceeds remaining segment"))
	}
	arraySize := arrayLength * uint32(p.identifierSize)
	elemBytes := make([]byte, arraySize)
	if _, err := io.ReadFull(p.reader, elemBytes); err != nil {
		if err == io.EOF {
			p.heapDumpFrameLeftBytes = 0
			return nil
		}
		return p.newError("read array elements", err)
	}
	p.heapDumpFrameLeftBytes -= arraySize

	idSize := uint32(p.identifierSize)
	refs := make([]types.Reference, 0)
	for i := uint32(0); i < arrayLength && len(refs) < maxArrayRefs; i++ {
		elemID := readIDFromBytes(elemBytes[i*idSize:(i+1)*idSize], p.identifierSize)
		if elemID == 0 {
			continue // null slot
		}
		refs = append(refs, types.Reference{
			SourceId: arrayID,
			TargetId: elemID,
			RefType:  fmt.Sprintf("[%d]", i),
		})
	}
	p.stats.ObjectArrays[arrayID] = &types.Object{
		ObjectId:   arrayID,
		ClassId:    arrayClassID,
		Size:       int64(arraySize) + int64(ObjectHeaderSize),
		References: refs,
	}

	// Update class stats
	if classInfo, ok := p.stats.Classes[arrayClassID]; ok {
		classInfo.InstanceCount++
		classInfo.ArrayBytes += int64(arraySize)
		p.stats.ArrayCount++
		p.stats.ArrayBytes += int64(arraySize)
		p.stats.TotalBytes += int64(arraySize)

		className := classInfo.ClassName
		if className == "" {
			if cn, ok := p.stats.StringMap[arrayClassID]; ok {
				className = cn
				classInfo.ClassName = cn
			}
		}
		p.logDebug("Parsed object array dump: array=0x%x class=%s (%x) length=%d", arrayID, className, arrayClassID, arrayLength)
	} else {
		p.logDebug("Parsed object array dump: array=0x%x class=0x%x length=%d (class not found)", arrayID, arrayClassID, arrayLength)
	}

	return nil
}

func (p *HProfParser) parsePrimitiveArrayDump() error {
	// Read array ID
	arrayID, err := p.readID()
	if err != nil {
		if err == io.EOF {
			p.heapDumpFrameLeftBytes = 0
			return nil
		}
		return p.newError("read array ID", err)
	}
	p.heapDumpFrameLeftBytes -= uint32(p.identifierSize)

	// Skip stack trace serial number
	if _, err := io.CopyN(io.Discard, p.reader, 4); err != nil {
		if err == io.EOF {
			p.heapDumpFrameLeftBytes = 0
			return nil
		}
		return p.newError("skip stack trace serial", err)
	}
	p.heapDumpFrameLeftBytes -= 4

	// Read array length
	var arrayLength uint32
	if err := binary.Read(p.reader, binary.BigEndian, &arrayLength); err != nil {
		if err == io.EOF {
			p.heapDumpFrameLeftBytes = 0
			return nil
		}
		return p.newError("read array length", err)
	}
	p.heapDumpFrameLeftBytes -= 4

	// Read element type
	var elementType byte
	if err := binary.Read(p.reader, binary.BigEndian, &elementType); err != nil {
		if err == io.EOF {
			p.heapDumpFrameLeftBytes = 0
			return nil
		}
		return p.newError("read element type", err)
	}
	p.heapDumpFrameLeftBytes--

	// Calculate element size
	var elementSize uint32
	switch HProfValueType(elementType) {
	case HProfValueType_BOOL, HProfValueType_BYTE:
		elementSize = 1
	case HProfValueType_CHAR, HProfValueType_SHORT:
		elementSize = 2
	case HProfValueType_FLOAT, HProfValueType_INT:
		elementSize = 4
	case HProfValueType_DOUBLE, HProfValueType_LONG:
		elementSize = 8
	default:
		return p.newError("parse primitive array", fmt.Errorf("unknown element type: %d", elementType))
	}

	// Read or skip the array elements. byte[] and char[] are the possible
	// backing stores of a java.lang.String, so retain those (up to a size cap)
	// for later value resolution; everything else is skipped.
	arraySize := arrayLength * elementSize
	et := HProfValueType(elementType)
	var zeroBytes int64
	// Only arrays large enough to be flagged sparse need a zero scan; smaller ones
	// (the vast majority) skip past their payload with no extra work, as before.
	scanZeros := arraySize >= sparseArrayMinBytes
	retain := (et == HProfValueType_BYTE || et == HProfValueType_CHAR) && arraySize <= maxStringBackingBytes
	switch {
	case retain:
		data := make([]byte, arraySize)
		if _, err := io.ReadFull(p.reader, data); err != nil {
			if err == io.EOF {
				p.heapDumpFrameLeftBytes = 0
				return nil
			}
			return p.newError("read array elements", err)
		}
		if scanZeros {
			zeroBytes = int64(bytes.Count(data, zeroByte))
		}
		p.arrayPayloads[arrayID] = arrayPayload{elemType: et, data: data}
	case scanZeros:
		// Read (not seek) the payload to count zero bytes, without retaining it.
		z, err := p.countZeroBytes(int64(arraySize))
		if err != nil {
			if err == io.EOF {
				p.heapDumpFrameLeftBytes = 0
				return nil
			}
			return p.newError("skip array elements", err)
		}
		zeroBytes = z
	default:
		if _, err := io.CopyN(io.Discard, p.reader, int64(arraySize)); err != nil {
			if err == io.EOF {
				p.heapDumpFrameLeftBytes = 0
				return nil
			}
			return p.newError("skip array elements", err)
		}
	}
	p.heapDumpFrameLeftBytes -= arraySize

	p.stats.ArrayCount++
	p.stats.ArrayBytes += int64(arraySize)
	p.stats.TotalBytes += int64(arraySize)
	p.recordPrimitiveArray(et, arraySize)
	p.recordArrayWaste(int64(arraySize), zeroBytes)

	p.logDebug("Parsed primitive array dump: array=0x%x type=%d length=%d size=%d", arrayID, elementType, arrayLength, arraySize)
	return nil
}

func ParseThreadObject(obj *types.Object, stats *HeapStats) *types.ThreadInfo {
	info := &types.ThreadInfo{
		ThreadId:     obj.ObjectId,
		LocalObjects: make([]uint64, 0),
	}

	// Extract thread name and state
	for _, ref := range obj.References {
		if classInfo, ok := stats.Classes[ref.SourceId]; ok {
			switch {
			case strings.HasSuffix(classInfo.ClassName, "java.lang.String") && ref.RefType == "name":
				if name, ok := stats.StringMap[ref.SourceId]; ok {
					info.ThreadName = name
				}
			case strings.HasSuffix(classInfo.ClassName, "java.lang.Thread$State") && ref.RefType == "threadStatus":
				switch ref.SourceId {
				case 0:
					info.ThreadState = "NEW"
				case 1:
					info.ThreadState = "RUNNABLE"
				case 2:
					info.ThreadState = "BLOCKED"
				case 3:
					info.ThreadState = "WAITING"
				case 4:
					info.ThreadState = "TIMED_WAITING"
				case 5:
					info.ThreadState = "TERMINATED"
				default:
					info.ThreadState = "UNKNOWN"
				}
			case strings.HasSuffix(classInfo.ClassName, "java.lang.ThreadGroup") && ref.RefType == "group":
				if group, ok := stats.StringMap[ref.SourceId]; ok {
					info.ThreadGroup = group
				}
			}
		}
	}

	// Set default values if not found
	if info.ThreadName == "" {
		info.ThreadName = fmt.Sprintf("Thread-%d", info.ThreadId)
	}
	if info.ThreadGroup == "" {
		info.ThreadGroup = "main"
	}
	if info.ThreadState == "" {
		info.ThreadState = "UNKNOWN"
	}

	// Calculate stack size and check if daemon
	for _, ref := range obj.References {
		if ref.RefType == "stackSize" {
			info.StackSize = int64(ref.SourceId)
		} else if ref.RefType == "daemon" {
			info.Daemon = ref.SourceId != 0
		}
	}

	info.IsAlive = info.ThreadState != "TERMINATED" && info.ThreadState != "NEW"

	return info
}
