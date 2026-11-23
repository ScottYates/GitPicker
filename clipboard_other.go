//go:build !windows

package main

// Reading the clipboard on the platforms that are not Windows.
//
// There is no clipboard to read here, so there is never a clone to offer
// from one.

func readClipboard() string {
	return ""
}
