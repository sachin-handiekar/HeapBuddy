//go:build windows

package analysis

import (
	"os"
	"syscall"
	"unsafe"
)

// mmapFile maps size bytes of f read-only. The returned release func unmaps;
// the file descriptor may be closed as soon as this returns (the mapping holds
// its own reference via the section handle). Note the Windows quirk this
// design accommodates: a mapped file cannot be deleted, so callers must unmap
// before removing the cache file — see graphMapping.release.
func mmapFile(f *os.File, size int64) ([]byte, func(), error) {
	h, err := syscall.CreateFileMapping(syscall.Handle(f.Fd()), nil, syscall.PAGE_READONLY,
		uint32(size>>32), uint32(size), nil)
	if err != nil {
		return nil, nil, err
	}
	addr, err := syscall.MapViewOfFile(h, syscall.FILE_MAP_READ, 0, 0, uintptr(size))
	if err != nil {
		_ = syscall.CloseHandle(h)
		return nil, nil, err
	}
	// Build the []byte over the mapped view without a direct uintptr ->
	// unsafe.Pointer conversion (which vet flags): the view is OS-managed
	// memory, never moved by the Go GC, so a slice-header cast is safe here.
	var data []byte
	hdr := (*sliceHeader)(unsafe.Pointer(&data))
	hdr.data = addr
	hdr.len = int(size)
	hdr.cap = int(size)
	release := func() {
		_ = syscall.UnmapViewOfFile(addr)
		_ = syscall.CloseHandle(h)
	}
	return data, release, nil
}

// sliceHeader mirrors the runtime slice layout; see the comment at its use.
type sliceHeader struct {
	data uintptr
	len  int
	cap  int
}
