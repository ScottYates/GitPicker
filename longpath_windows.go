//go:build windows

package main

// Windows path helpers.

import (
	"syscall"
	"unsafe"
)

var (
	kernel32            = syscall.NewLazyDLL("kernel32.dll")
	procGetLongPathName = kernel32.NewProc("GetLongPathNameW")
)

// longPath expands a Windows 8.3 short path into its real long form.
//
// A process launched from a short path keeps that spelling in its working
// directory, so the header would show 'BTS-MA~2' instead of the folder name.
// GetLongPathNameW is the documented way to undo it. Every failure returns
// the input unchanged: a wrong header is far better than a picker that
// refuses to start.
func longPath(path string) string {
	if path == "" {
		return path
	}
	handle, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return path
	}
	// A zero-length buffer asks the API for the size it needs, counting the
	// terminating null. Zero back means the path does not resolve.
	needed, _, _ := procGetLongPathName.Call(uintptr(unsafe.Pointer(handle)), 0, 0)
	if needed == 0 {
		return path
	}
	buf := make([]uint16, needed)
	written, _, _ := procGetLongPathName.Call(
		uintptr(unsafe.Pointer(handle)),
		uintptr(unsafe.Pointer(&buf[0])),
		needed,
	)
	if written == 0 {
		return path
	}
	return syscall.UTF16ToString(buf)
}
