package parser

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"heapbuddy/internal/types"
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
	HProfGCTag_ROOT_JNI_GLOBAL      HProfGCTag = 0x01
	HProfGCTag_ROOT_JNI_LOCAL       HProfGCTag = 0x02
	HProfGCTag_ROOT_JAVA_FRAME      HProfGCTag = 0x03
	HProfGCTag_ROOT_NATIVE_STACK    HProfGCTag = 0x04
	HProfGCTag_ROOT_STICKY_CLASS    HProfGCTag = 0x05
	HProfGCTag_ROOT_THREAD_BLOCK    HProfGCTag = 0x06
	HProfGCTag_ROOT_MONITOR_USED    HProfGCTag = 0x07
	HProfGCTag_ROOT_THREAD_OBJ      HProfGCTag = 0x08
	HProfGCTag_ROOT_JNI_MONITOR     HProfGCTag = 0x09
	HProfGCTag_ROOT_SYSTEM_CLASS    HProfGCTag = 0x0A

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
	ClassStats    map[uint64]*types.ClassInfo
	ClassNameMap  map[uint64]string
	StringMap     map[uint64]string
	GCRootCount   int
	GCEvents      []*GCEvent
	SystemProps   map[string]string
	HeapSummary   *HeapSummary
	ObjectCount   int64
	ArrayCount    int64
	TotalBytes    int64
	StringCount   int64
	StringBytes   int64
	ArrayBytes    int64
	InstanceBytes int64
	ClassLoaderCount int
	Is64Bit      bool
	CreationTime time.Time
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
}

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
		reader:         bufio.NewReader(file),
		file:           file,
		identifierSize: 8, // Default to 8, will be updated from header
		stats: &HeapStats{
			ClassStats:   make(map[uint64]*types.ClassInfo),
			ClassNameMap: make(map[uint64]string),
			StringMap:    make(map[uint64]string),
			SystemProps:  make(map[string]string),
		},
		seenClasses: make(map[uint64]bool),
		debug:       false,
	}, nil
}

func (p *HProfParser) SetDebug(debug bool) {
	p.debug = debug
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

func (p *HProfParser) readInt32() (int32, error) {
	var val int32
	err := binary.Read(p.reader, binary.BigEndian, &val)
	if err != nil {
		return 0, p.newError("read int32", err)
	}
	return val, nil
}

func (p *HProfParser) readInt64() (int64, error) {
	var val int64
	err := binary.Read(p.reader, binary.BigEndian, &val)
	if err != nil {
		return 0, p.newError("read int64", err)
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

func (p *HProfParser) Parse() (*HeapStats, error) {
	if err := p.parseHeader(); err != nil {
		return nil, fmt.Errorf("error parsing header: %v", err)
	}

	p.stats = &HeapStats{
		ClassStats:      make(map[uint64]*types.ClassInfo),
		ClassNameMap:    make(map[uint64]string),
		StringMap:       make(map[uint64]string),
		SystemProps:     make(map[string]string),
		GCEvents:        make([]*GCEvent, 0),
		ClassLoaderCount: 0,
		CreationTime:    time.Now(), // Default to current time
	}

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
		case HProfRecordType_HEAP_DUMP_SEGMENT:
			p.logDebug("Processing heap dump segment (length: %d bytes)", length)
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

			// Read string length (remaining bytes after ID)
			stringLen := length - uint32(p.identifierSize)
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
				if !p.seenClasses[classId] {
					p.seenClasses[classId] = true
					p.totalClasses++

					// Initialize class stats if not already present
					if _, exists := p.stats.ClassStats[classId]; !exists {
						p.stats.ClassStats[classId] = &types.ClassInfo{
							ClassId:       classId,
							ClassName:     className,
							InstanceCount: 0,
							InstanceSize:  0,
							ArrayBytes:    0,
						}
					}
				}
			}

			// Check if this is a classloader
			if info, ok := p.stats.ClassStats[classId]; ok {
				if strings.HasSuffix(info.ClassName, "ClassLoader") {
					classLoaders[classId] = true
					p.stats.ClassLoaderCount++
				}
			}

		case HProfRecordType_HEAP_DUMP_SEGMENT:
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
			for _, stats := range p.stats.ClassStats {
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

	// If we have any data, return it even if we hit EOF
	if len(p.stats.ClassStats) > 0 || len(p.stats.StringMap) > 0 {
		return p.stats, nil
	}

	return nil, fmt.Errorf("no heap dump data found")
}

func (p *HProfParser) GetStats() *HeapStats {
	return p.stats
}

func (p *HProfParser) parseRecord() (interface{}, error) {
	// Read record header
	var recordTime uint32
	if err := binary.Read(p.reader, binary.BigEndian, &recordTime); err != nil {
		if err == io.EOF {
			return nil, err
		}
		return nil, p.newError("read record time", err)
	}

	var recordLength uint32
	if err := binary.Read(p.reader, binary.BigEndian, &recordLength); err != nil {
		return nil, p.newError("read record length", err)
	}

	var recordType HProfRecordType
	if err := binary.Read(p.reader, binary.BigEndian, &recordType); err != nil {
		return nil, p.newError("read record type", err)
	}

	p.logDebug("Parsing record: type=0x%02x time=%d length=%d", recordType, recordTime, recordLength)

	switch recordType {
	case HProfRecordType_STRING_IN_UTF8:
		stringId, err := p.readID()
		if err != nil {
			return nil, p.newError("read string ID", err)
		}

		stringBytes := make([]byte, recordLength-uint32(p.identifierSize))
		if _, err := io.ReadFull(p.reader, stringBytes); err != nil {
			return nil, p.newError("read string bytes", err)
		}

		// Store string in map for later reference
		p.stats.StringMap[stringId] = string(stringBytes)
		p.stats.StringCount++
		p.stats.StringBytes += int64(len(stringBytes))
		return &HProfRecordUTF8{StringId: stringId, Value: string(stringBytes)}, nil

	case HProfRecordType_LOAD_CLASS:
		var classSerialNum uint32
		if err := binary.Read(p.reader, binary.BigEndian, &classSerialNum); err != nil {
			return nil, p.newError("read class serial number", err)
		}

		classId, err := p.readID()
		if err != nil {
			return nil, p.newError("read class ID", err)
		}

		var stackTraceId uint32
		if err := binary.Read(p.reader, binary.BigEndian, &stackTraceId); err != nil {
			return nil, p.newError("read stack trace ID", err)
		}

		classNameId, err := p.readID()
		if err != nil {
			return nil, p.newError("read class name ID", err)
		}

		// Initialize class info if not already present
		if _, exists := p.stats.ClassStats[classId]; !exists {
			if className, ok := p.stats.StringMap[classNameId]; ok {
				p.stats.ClassStats[classId] = &types.ClassInfo{
					ClassId:   classId,
					ClassName: className,
				}
			}
		}

		return &HProfLoadClass{
			ClassSerialNum: classSerialNum,
			ClassId:        classId,
			StackTraceId:   stackTraceId,
			ClassNameId:    classNameId,
		}, nil

	case HProfRecordType_HEAP_DUMP_SEGMENT:
		p.heapDumpFrameLeftBytes = recordLength
		if err := p.parseHeapDumpSegment(); err != nil {
			return nil, err
		}
		return &HProfRecordHeapDumpSegment{
			Timestamp: recordTime,
			Length:    recordLength,
		}, nil

	case HProfRecordType_HEAP_DUMP_END:
		return &HProfRecordHeapDumpEnd{Timestamp: recordTime}, nil

	default:
		p.logDebug("Skipping unknown record type: 0x%02x", recordType)
		if _, err := io.CopyN(io.Discard, p.reader, int64(recordLength)); err != nil {
			return nil, p.newError("skip unknown record", err)
		}
		return nil, nil
	}
}

func (p *HProfParser) parseHeapDumpSegment() error {
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
			p.logDebug("Invalid zero tag found, skipping remaining %d bytes", p.heapDumpFrameLeftBytes)
			if _, err := io.CopyN(io.Discard, p.reader, int64(p.heapDumpFrameLeftBytes)); err != nil && err != io.EOF {
				return p.newError("skip invalid segment", err)
			}
			p.heapDumpFrameLeftBytes = 0
			return nil
		}

		p.logDebug("Processing GC tag: 0x%02x", tag)

		switch tag {
		case HProfGCTag_ROOT_JNI_GLOBAL,
			HProfGCTag_ROOT_JNI_LOCAL,
			HProfGCTag_ROOT_JAVA_FRAME,
			HProfGCTag_ROOT_NATIVE_STACK,
			HProfGCTag_ROOT_STICKY_CLASS,
			HProfGCTag_ROOT_THREAD_BLOCK,
			HProfGCTag_ROOT_MONITOR_USED,
			HProfGCTag_ROOT_THREAD_OBJ,
			HProfGCTag_ROOT_JNI_MONITOR,
			HProfGCTag_ROOT_SYSTEM_CLASS:
			// Skip root records - we just need to track bytes read
			objectID, err := p.readID()
			if err != nil {
				if err == io.EOF {
					p.heapDumpFrameLeftBytes = 0
					return nil
				}
				return p.newError("read root object ID", err)
			}
			p.heapDumpFrameLeftBytes -= uint32(p.identifierSize)
			p.stats.GCRootCount++
			p.logDebug("Found root object: 0x%x", objectID)

		case HProfGCTag_CLASS_DUMP:
			// Class dump record
			if err := p.parseClassDump(); err != nil {
				return err
			}

		case HProfGCTag_INSTANCE_DUMP:
			// Instance dump record
			if err := p.parseInstanceDump(); err != nil {
				return err
			}

		case HProfGCTag_OBJECT_ARRAY_DUMP:
			// Object array dump
			if err := p.parseObjectArrayDump(); err != nil {
				return err
			}

		case HProfGCTag_PRIMITIVE_ARRAY_DUMP:
			// Primitive array dump
			if err := p.parsePrimitiveArrayDump(); err != nil {
				return err
			}

		default:
			// Unknown tag - log and skip
			p.logDebug("Skipping unknown GC tag: 0x%02x", tag)
			if p.heapDumpFrameLeftBytes > 0 {
				// For unknown tags, try to maintain alignment by skipping the minimum size
				skipSize := uint32(1)
				if _, err := io.CopyN(io.Discard, p.reader, int64(skipSize)); err != nil {
					if err == io.EOF {
						p.heapDumpFrameLeftBytes = 0
						return nil
					}
					return p.newError("skip unknown tag", err)
				}
				p.heapDumpFrameLeftBytes -= skipSize
			}
		}
	}

	return nil
}

func (p *HProfParser) skipBytes(n int) error {
	_, err := io.CopyN(io.Discard, p.reader, int64(n))
	if err != nil {
		return p.newError("skip bytes", err)
	}
	p.heapDumpFrameLeftBytes -= uint32(n)
	return nil
}

func (p *HProfParser) Close() error {
	if p.file != nil {
		return p.file.Close()
	}
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

	// Update class stats
	if _, exists := p.stats.ClassStats[classID]; !exists {
		p.stats.ClassStats[classID] = &types.ClassInfo{
			ClassId:      classID,
			SuperClassId: superClassID,
			InstanceSize: int64(instanceSize),
		}
		p.totalClasses++
	}

	return nil
}

func (p *HProfParser) parseInstanceDump() error {
	// Read object ID
	objectID, err := p.readID()
	if err != nil {
		if err == io.EOF {
			p.heapDumpFrameLeftBytes = 0
			return nil
		}
		return p.newError("read object ID", err)
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

	// Skip instance data
	if _, err := io.CopyN(io.Discard, p.reader, int64(instanceSize)); err != nil {
		if err == io.EOF {
			p.heapDumpFrameLeftBytes = 0
			return nil
		}
		return p.newError("skip instance data", err)
	}
	p.heapDumpFrameLeftBytes -= instanceSize

	// Update class stats
	if classInfo, ok := p.stats.ClassStats[classID]; ok {
		classInfo.InstanceCount++
		classInfo.TotalShallowSize += int64(instanceSize)
		p.stats.ObjectCount++
		p.stats.InstanceBytes += int64(instanceSize)
		p.stats.TotalBytes += int64(instanceSize)

		className := classInfo.ClassName
		if className == "" {
			if cn, ok := p.stats.StringMap[classID]; ok {
				className = cn
				classInfo.ClassName = cn
			}
		}
		p.logDebug("Parsed instance dump: object=0x%x class=%s (%x) size=%d", objectID, className, classID, instanceSize)
	} else {
		p.logDebug("Parsed instance dump: object=0x%x class=0x%x size=%d (class not found)", objectID, classID, instanceSize)
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

	// Skip array elements
	arraySize := arrayLength * uint32(p.identifierSize)
	if _, err := io.CopyN(io.Discard, p.reader, int64(arraySize)); err != nil {
		if err == io.EOF {
			p.heapDumpFrameLeftBytes = 0
			return nil
		}
		return p.newError("skip array elements", err)
	}
	p.heapDumpFrameLeftBytes -= arraySize

	// Update class stats
	if classInfo, ok := p.stats.ClassStats[arrayClassID]; ok {
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
	elementSize := uint32(1)
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

	// Skip array elements
	arraySize := arrayLength * elementSize
	if _, err := io.CopyN(io.Discard, p.reader, int64(arraySize)); err != nil {
		if err == io.EOF {
			p.heapDumpFrameLeftBytes = 0
			return nil
		}
		return p.newError("skip array elements", err)
	}
	p.heapDumpFrameLeftBytes -= arraySize

	p.stats.ArrayCount++
	p.stats.ArrayBytes += int64(arraySize)
	p.stats.TotalBytes += int64(arraySize)

	p.logDebug("Parsed primitive array dump: array=0x%x type=%d length=%d size=%d", arrayID, elementType, arrayLength, arraySize)
	return nil
}
