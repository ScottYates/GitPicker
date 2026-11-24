//go:build !windows

package main

// openKeyboard returns a keyboard that reads nothing.
//
// The picker lets bubbletea read the keyboard on this build, which is what
// it has always done. Ctrl+enter arrives as ctrl+j, because a terminal
// sends the same byte for the two, and the picker treats that as the same
// key.
func openKeyboard() keyboard {
	return keyboard{}
}

// flushConsoleInput throws away the keys that are waiting to be read.
//
// There is no console of its own on this build, so there is nothing to
// flush.
func flushConsoleInput() {
}
