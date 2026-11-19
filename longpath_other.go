//go:build !windows

package main

// longPath expands a Windows 8.3 short path into its real long form.
//
// There is no such thing on this platform, so the path is returned as it is.
func longPath(path string) string {
	return path
}

// shortPath is the 8.3 alias Windows gives a long folder name. There is no
// such thing on this platform.
func shortPath(path string) string {
	return ""
}
