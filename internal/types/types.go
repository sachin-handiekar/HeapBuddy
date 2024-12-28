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
}
