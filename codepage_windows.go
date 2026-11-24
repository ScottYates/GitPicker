//go:build windows

package main

// The code page the console reads the output with.
//
// A console is normally on a DOS code page, where 437 is the usual one. The
// bytes of a character outside ASCII are then read as two or three
// characters of that page, so a character that is drawn as one thing on
// screen turns into rubbish.
//
// The picker draws with the line drawing characters of the console, so the
// console is put on UTF-8 while the picker runs and put back when the
// picker is done.

import "golang.org/x/sys/windows"

// utf8CodePage is the code page that reads the output as UTF-8.
const utf8CodePage = 65001

// setOutputCodePage puts the console on the UTF-8 code page.
//
// It returns the function that puts the code page back. A console that
// cannot be changed is not a problem: the picker then draws with what the
// console already understands, which is what the other builds do anyway.
func setOutputCodePage() func() {
	previous, err := windows.GetConsoleOutputCP()
	if err != nil || previous == utf8CodePage {
		return func() {}
	}
	if err := windows.SetConsoleOutputCP(utf8CodePage); err != nil {
		return func() {}
	}
	return func() {
		_ = windows.SetConsoleOutputCP(previous)
	}
}

// outputCodePage is the code page the console reads the output with.
//
// It is 0 when there is no console to ask.
func outputCodePage() uint32 {
	cp, err := windows.GetConsoleOutputCP()
	if err != nil {
		return 0
	}
	return cp
}
