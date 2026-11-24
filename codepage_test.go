package main

// Tests for the code page the console reads the output with.

import "testing"

func TestTheCodePageIsPutBack(t *testing.T) {
	before := outputCodePage()

	restore := setOutputCodePage()
	restore()

	after := outputCodePage()
	if before != after {
		t.Fatalf("the code page: got %d, want %d", after, before)
	}
}

func TestTheCodePageIsPutBackWhenItIsNotUsed(t *testing.T) {
	// Putting the code page back twice must not change anything, because
	// the picker may be left and started again in the same console.
	restore := setOutputCodePage()
	before := outputCodePage()
	restore()
	restore()

	if after := outputCodePage(); after != before {
		t.Fatalf("the code page: got %d, want %d", after, before)
	}
}
