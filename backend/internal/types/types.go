package types

// ClassInfo holds information about a Java class in the heap dump
type ClassInfo struct {
	ClassId           uint64
	SuperClassId      uint64
	ClassName         string
	InstanceCount     uint32
	InstanceSize      int64
	TotalShallowSize  int64
	TotalRetainedSize int64
	ArrayBytes        int64
	ArrayLength       int
	IsArray           bool
	LoaderClass       uint64
}

// Reference represents a reference between objects in the heap
type Reference struct {
	SourceId uint64
	TargetId uint64
	RefType  string
}

// Object is a heap object node: its identity, class, shallow size, and outgoing
// references. It is built by the parser and read by the analysis layer; both
// share this single definition so the (large) object graph is held once and
// referenced by pointer, never copied between layers.
type Object struct {
	ObjectId   uint64
	ClassId    uint64
	Size       int64
	References []Reference
	IsThread   bool
}

// StringInstance represents a String object in the heap
type StringInstance struct {
	ObjectId     uint64
	Value        string
	Size         int64
	RetainedSize int64
	References   []Reference
}

// StringAnalysis contains information about string usage and duplicates
type StringAnalysis struct {
	UniqueStrings    int
	DuplicateStrings int
	TotalWastedBytes int64
	Duplicates       map[string][]StringInstance
}

// CollectionStats contains statistics about a Java collection
type CollectionStats struct {
	ObjectId     uint64
	ClassId      uint64
	ClassName    string
	Size         int64
	ElementCount int
	Capacity     int
	LoadFactor   float64
	WastedSpace  int64
	IsEmpty      bool
	IsOversized  bool
}

// ThreadInfo contains information about a thread's memory usage
type ThreadInfo struct {
	ThreadId     uint64
	ThreadName   string
	ThreadState  string
	StackSize    int64
	LocalObjects []uint64
	RetainedSize int64
	ThreadGroup  string
	Priority     int
	Daemon       bool
	IsAlive      bool
}

// ThreadAnalysis contains the results of thread memory analysis
type ThreadAnalysis struct {
	Threads        map[uint64]*ThreadInfo
	TotalThreads   int
	ActiveThreads  int
	DaemonThreads  int
	TotalStackSize int64
	TotalRetained  int64
}

// RetentionChain represents a chain of object references
type RetentionChain struct {
	TargetObjectId uint64
	Path           []Reference
	TotalSize      int64
	Description    string
}
