//go:build windows

package main

// Tests for reading the clipboard.
//
// The clipboard is shared with whatever else is running, so the test puts
// the clipboard back the way it found it.

import (
	"testing"
	"unicode/utf16"
	"unsafe"
)

var (
	procGlobalAlloc        = kernel32.NewProc("GlobalAlloc")
	procWriteProcessMemory = kernel32.NewProc("WriteProcessMemory")
	procEmptyClipboard     = user32.NewProc("EmptyClipboard")
	procSetClipboardData   = user32.NewProc("SetClipboardData")
)

// gmemMoveable is the flag for memory that can be moved.
const gmemMoveable = 0x0002

// setClipboard puts text on the clipboard.
//
// It reports whether the clipboard could be written, because it cannot be
// on a machine with no desktop session.
func setClipboard(text string) bool {
	if opened, _, _ := procOpenClipboard.Call(0); opened == 0 {
		return false
	}
	defer procCloseClipboard.Call()

	units := utf16.Encode([]rune(text + "\x00"))
	size := uintptr(len(units) * 2)

	block, _, _ := procGlobalAlloc.Call(gmemMoveable, size)
	if block == 0 {
		return false
	}
	ptr, _, _ := procGlobalLock.Call(block)
	if ptr == 0 {
		return false
	}

	// The text is written into the block the clipboard will hold, the
	// same way round as reading it back.
	var written uint32
	current, _, _ := procGetCurrentProcess.Call()
	procWriteProcessMemory.Call(
		current,
		ptr,
		uintptr(unsafe.Pointer(&units[0])),
		size,
		uintptr(unsafe.Pointer(&written)),
	)
	procGlobalUnlock.Call(block)

	procEmptyClipboard.Call()
	procSetClipboardData.Call(uintptr(clipUnicodeText), block)
	return true
}

func TestTheClipboardIsRead(t *testing.T) {
	saved := readClipboard()
	t.Cleanup(func() {
		setClipboard(saved)
	})

	const text = "https://github.com/owner/repo.git"
	if !setClipboard(text) {
		t.Skip("the clipboard cannot be written on this machine")
	}

	got := readClipboard()
	if got != text {
		t.Fatalf("the clipboard: got %q, want %q", got, text)
	}
}

func TestAnEmptyClipboardReadsAsNothing(t *testing.T) {
	saved := readClipboard()
	t.Cleanup(func() {
		setClipboard(saved)
	})

	if !setClipboard("") {
		t.Skip("the clipboard cannot be written on this machine")
	}

	if got := readClipboard(); got != "" {
		t.Fatalf("an empty clipboard should read as nothing, got %q", got)
	}
}
