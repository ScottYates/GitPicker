package main

// Path helpers. These use path/filepath, which follows the conventions of the
// platform the program was built for, so they behave the same way Python's
// os.path does on that platform.

import (
	"os"
	"path/filepath"
)

// pathExists reports whether a path exists.
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// absPath is the absolute form of a path.
func absPath(path string) string {
	full, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return full
}

// dirName is the parent directory of a path.
func dirName(path string) string {
	return filepath.Dir(path)
}
