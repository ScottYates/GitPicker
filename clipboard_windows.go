//go:build windows

package main

// Reading the clipboard on Windows.
//
// The clipboard is read through the window manager rather than by starting
// a shell, because the picker starts fast and a process would not.
//
// The standard library reaches the window manager through a lazily loaded
// library, so no extra dependency is needed.
//
// The text is copied out of the clipboard's own memory into a buffer of
// ours rather than read through a pointer. A pointer into memory that
// belongs to another program cannot safely be turned into a Go pointer,
// and the copy costs no more than the read it replaces.

import (
	"syscall"
	"unsafe"
)

var (
	user32 = syscall.NewLazyDLL("user32.dll")

	procOpenClipboard  = user32.NewProc("OpenClipboard")
	procCloseClipboard = user32.NewProc("CloseClipboard")
	procGetClipboard   = user32.NewProc("GetClipboardData")

	procGlobalSize   = kernel32.NewProc("GlobalSize")
	procGlobalLock   = kernel32.NewProc("GlobalLock")
	procGlobalUnlock = kernel32.NewProc("GlobalUnlock")

	procGetCurrentProcess = kernel32.NewProc("GetCurrentProcess")
	procReadProcessMemory = kernel32.NewProc("ReadProcessMemory")
)

// clipUnicodeText is the clipboard format for text.
const clipUnicodeText = 13

// clipMaxBytes is the largest clipboard that is read. A repository address
// is short, and this keeps a broken clipboard from taking the memory.
const clipMaxBytes = 1 << 20

// readClipboard is the text on the clipboard, or "" when there is none.
//
// The clipboard holds other formats too, and only the text one is looked
// at, because that is the one a repository address would be copied as.
func readClipboard() string {
	if opened, _, _ := procOpenClipboard.Call(0); opened == 0 {
		return ""
	}
	defer procCloseClipboard.Call()

	handle, _, _ := procGetClipboard.Call(uintptr(clipUnicodeText))
	if handle == 0 {
		return ""
	}

	size, _, _ := procGlobalSize.Call(handle)
	if size == 0 || size > clipMaxBytes {
		return ""
	}

	src, _, _ := procGlobalLock.Call(handle)
	if src == 0 {
		return ""
	}
	// The clipboard owns this memory, so it is unlocked but never freed.
	defer procGlobalUnlock.Call(handle)

	// The text is copied into a buffer of ours. The count is the size of
	// the block the clipboard allocated, so the whole of it is read and
	// not a byte more.
	count := int(size)
	buf := make([]uint16, (count+1)/2)
	var read uint32
	current, _, _ := procGetCurrentProcess.Call()
	procReadProcessMemory.Call(
		current,
		src,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(count),
		uintptr(unsafe.Pointer(&read)),
	)
	if read == 0 {
		return ""
	}

	// The text is a null terminated string of UTF-16 code units, and the
	// part of the buffer that was not filled in is null as well, so the
	// string ends where it should.
	return syscall.UTF16ToString(buf[:int(read)/2])
}
