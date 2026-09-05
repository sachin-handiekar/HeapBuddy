//go:build !windows

package analysis

import (
	"os"
	"syscall"
)

// mmapFile maps size bytes of f read-only. The returned release func unmaps;
// the file descriptor may be closed as soon as this returns.
func mmapFile(f *os.File, size int64) ([]byte, func(), error) {
	data, err := syscall.Mmap(int(f.Fd()), 0, int(size), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return nil, nil, err
	}
	return data, func() { _ = syscall.Munmap(data) }, nil
}
