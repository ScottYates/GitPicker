//go:build !windows

package main

// setOutputCodePage does nothing on this build.
//
// The console of the other builds reads what the picker writes as it is,
// so there is no code page to change.
func setOutputCodePage() func() {
	return func() {}
}

// outputCodePage is the code page the console reads the output with.
//
// It is 0 on this build, because there is no code page to ask about.
func outputCodePage() uint32 {
	return 0
}
